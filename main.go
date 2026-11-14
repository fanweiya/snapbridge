package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image/jpeg"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/transfer"
)

// Shot 暴露给前端调用，负责交出当前截图、提交结果与取消。
type Shot struct{}

// SnapshotData 是遮罩窗口一开始需要的全部数据。
type SnapshotData struct {
	ImageURL string `json:"imageUrl"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	OffsetX  int    `json:"offsetX"`
	OffsetY  int    `json:"offsetY"`
	SaveDir  string `json:"saveDir"`
	ClipMode string `json:"clipMode"`
	Preset   []int  `json:"preset"`
	Mode     string `json:"mode"`
}

// CommitRequest 是前端合成好的成品。
type CommitRequest struct {
	PNG      string `json:"png"`
	Save     bool   `json:"save"`
	Copy     bool   `json:"copy"`
	ClipMode string `json:"clipMode"`
}

// CommitResult 告诉前端这次保存与复制的落点。
type CommitResult struct {
	Path    string `json:"path"`
	Saved   bool   `json:"saved"`
	Copied  bool   `json:"copied"`
	Message string `json:"message"`
}

// 限制单边像素与总像素量，防止异常超大图片消耗过多资源引发崩溃。
const maxImageDimension = 16384
const maxImagePixels = 64 * 1024 * 1024

// validatePNG 完整校验前端传回的图片数据流，保证图片未损坏截断，并返回宽高尺寸。
func validatePNG(data []byte) (int, int, error) {
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0, fmt.Errorf("解析图片头部失败: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return 0, 0, fmt.Errorf("图片尺寸为空")
	}
	if cfg.Width > maxImageDimension || cfg.Height > maxImageDimension || int64(cfg.Width)*int64(cfg.Height) > maxImagePixels {
		return 0, 0, fmt.Errorf("图片尺寸超出系统允许的最大限制")
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return 0, 0, fmt.Errorf("解码图片数据流失败: %w", err)
	}
	b := img.Bounds()
	return b.Dx(), b.Dy(), nil
}

var (
	stateMu   sync.Mutex
	captureMu sync.Mutex
	pending   *captureSession
	overlay   *mygo.Window
	overlayMu sync.Mutex
	clipMode  = "both"
	saveDir   string
)

type captureSession struct {
	jpeg    []byte
	width   int
	height  int
	offsetX int
	offsetY int
	preset  []int
	mode    string
}

// Snapshot 交出当前待处理的截图。
func (Shot) Snapshot() (SnapshotData, error) {
	stateMu.Lock()
	s := pending
	stateMu.Unlock()
	if s == nil {
		return SnapshotData{}, fmt.Errorf("没有待处理的截图")
	}
	return SnapshotData{
		ImageURL: "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(s.jpeg),
		Width:    s.width,
		Height:   s.height,
		OffsetX:  s.offsetX,
		OffsetY:  s.offsetY,
		SaveDir:  saveDir,
		ClipMode: currentClipMode(),
		Preset:   s.preset,
		Mode:     s.mode,
	}, nil
}

// checkCallerOverlay 验证当前调用是否来自有效的活跃遮罩窗口。
// 若调用来自已被覆盖或关闭的历史窗口，返回 false 以免误操作新会话。
func checkCallerOverlay(ctx context.Context) bool {
	caller := mygo.CallerWindow(ctx)
	if caller == nil {
		return true
	}
	overlayMu.Lock()
	cur := overlay
	overlayMu.Unlock()
	if cur != nil && caller != cur {
		return false
	}
	return true
}

// Commit 保存成品、写入剪贴板，然后关闭遮罩窗口。
func (s Shot) Commit(ctx context.Context, req CommitRequest) (CommitResult, error) {
	// 在锁外先进行 CPU 密集的解码与尺寸校验，避免持锁阻塞快捷键响应。
	raw := strings.TrimPrefix(req.PNG, "data:image/png;base64,")
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return CommitResult{}, fmt.Errorf("解析图片失败: %w", err)
	}
	width, height, err := validatePNG(data)
	if err != nil {
		return CommitResult{}, err
	}
	pngBytes := data

	// 整个提交阶段加锁保护，防止写盘与复制期间与 openCapture 交错。
	captureMu.Lock()
	defer captureMu.Unlock()

	// 确认当前调用来自最新遮罩，防止历史页面的延迟提交误关闭新会话。
	if !checkCallerOverlay(ctx) {
		return CommitResult{}, fmt.Errorf("提交属于已过期的截图会话")
	}

	res := CommitResult{}
	path := ""
	if req.Save || req.Copy {
		path, err = writeShot(pngBytes)
		if err != nil {
			return CommitResult{}, err
		}
		res.Path = path
		res.Saved = req.Save
	}
	if req.Copy {
		if err := copyShot(path, pngBytes, req.ClipMode); err != nil {
			return CommitResult{}, fmt.Errorf("写入剪贴板失败: %w", err)
		}
		res.Copied = true
	}
	res.Message = fmt.Sprintf("%d × %d", width, height)

	// 只有在写盘与复制均成功完成之后，才关闭遮罩并清理会话。
	// 若前面发生错误，遮罩与会话保持完好，前端可捕获错误并提示重试。
	closeOverlay()
	stateMu.Lock()
	pending = nil
	stateMu.Unlock()

	return res, nil
}

// Ready 由前端在首帧绘制完成后调用，此时才让遮罩窗口露面，
// 避免窗口已经可见而选区事件尚未绑定的空档。
func (Shot) Ready(ctx context.Context) error {
	overlayMu.Lock()
	w := overlay
	overlayMu.Unlock()
	if w == nil || w.IsDestroyed() {
		return fmt.Errorf("遮罩窗口不存在")
	}
	if !checkCallerOverlay(ctx) {
		return fmt.Errorf("会话已失效")
	}
	w.Show()
	w.Focus()
	return nil
}

// Cancel 丢弃本次截图并关闭遮罩窗口。
func (Shot) Cancel(ctx context.Context) error {
	captureMu.Lock()
	defer captureMu.Unlock()

	if !checkCallerOverlay(ctx) {
		return nil
	}

	closeOverlay()
	stateMu.Lock()
	pending = nil
	stateMu.Unlock()
	return nil
}

// SetClipMode 切换复制方式：both 图片与路径、image 只要图片、path 只要路径。
func (Shot) SetClipMode(mode string) error {
	switch mode {
	case "both", "image", "path":
		stateMu.Lock()
		clipMode = mode
		stateMu.Unlock()
		refreshTrayMenu()
		return nil
	default:
		return fmt.Errorf("未知的复制方式: %s", mode)
	}
}

// GetClipMode 返回当前复制方式。
func (Shot) GetClipMode() string {
	return currentClipMode()
}

// OpenSaveDir 在资源管理器里打开截图目录。
func (Shot) OpenSaveDir() error {
	if err := os.MkdirAll(saveDir, 0o755); err != nil {
		return err
	}
	return mygo.Shell.OpenPath(saveDir)
}

func currentClipMode() string {
	stateMu.Lock()
	defer stateMu.Unlock()
	return clipMode
}

func writeShot(pngBytes []byte) (string, error) {
	now := time.Now()
	dir := filepath.Join(saveDir, now.Format("2006-01"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("创建目录失败: %w", err)
	}
	name := fmt.Sprintf("shot_%s.png", now.Format("20060102_150405.000"))
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, pngBytes, 0o644); err != nil {
		return "", fmt.Errorf("写入文件失败: %w", err)
	}
	return path, nil
}

func fileURL(path string) ([]byte, error) {
	fd, err := transfer.FileData(path)
	if err != nil {
		return nil, err
	}
	return fd.Read(transfer.FileList)
}

func copyShot(path string, pngBytes []byte, mode string) error {
	if mode == "" {
		mode = currentClipMode()
	}
	var item transfer.Item
	switch mode {
	case "image":
		item = transfer.NewItem(transfer.Bytes(transfer.PNG, pngBytes))
	case "path":
		url, err := fileURL(path)
		if err != nil {
			return err
		}
		item = transfer.NewItem(
			transfer.Bytes(transfer.FileList, url),
			transfer.Bytes(transfer.Text, []byte(path)),
		)
	default:
		url, err := fileURL(path)
		if err != nil {
			return err
		}
		item = transfer.NewItem(
			transfer.Bytes(transfer.FileList, url),
			transfer.Bytes(transfer.Text, []byte(path)),
			transfer.Bytes(transfer.PNG, pngBytes),
		)
	}
	return mygo.Clipboard.Write(transfer.New(item))
}

func openCapture(mode string) {
	// 一次只放一个截图流程进来：键盘钩子的消费协程与托盘菜单点击都可能触发，
	// 两者交错时先建的窗口会被后建的一方覆盖掉，成为一个永不显示的全屏置顶孤儿。
	if !captureMu.TryLock() {
		return
	}
	defer captureMu.Unlock()

	closeOverlay()

	cap, err := CaptureScreen()
	if err != nil {
		notify("截图失败", err.Error())
		return
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, cap.Image, &jpeg.Options{Quality: 92}); err != nil {
		notify("截图失败", err.Error())
		return
	}

	bounds := cap.Image.Bounds()
	preset := []int{0, 0, bounds.Dx(), bounds.Dy()}
	switch mode {
	case "region":
		preset = nil
	case "window":
		if r, err := ForegroundWindowRect(); err == nil {
			x := clamp(r.Min.X-cap.X, 0, bounds.Dx()-1)
			y := clamp(r.Min.Y-cap.Y, 0, bounds.Dy()-1)
			w := clamp(r.Dx(), 1, bounds.Dx()-x)
			h := clamp(r.Dy(), 1, bounds.Dy()-y)
			preset = []int{x, y, w, h}
		}
	}

	stateMu.Lock()
	pending = &captureSession{
		jpeg:    buf.Bytes(),
		width:   bounds.Dx(),
		height:  bounds.Dy(),
		offsetX: cap.X,
		offsetY: cap.Y,
		preset:  preset,
		mode:    mode,
	}
	stateMu.Unlock()

	showOverlay()
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func showOverlay() {
	d := mygo.Screen.PrimaryDisplay()
	overlayMu.Lock()
	defer overlayMu.Unlock()
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:             "SnapBridge",
		URL:               "/",
		X:                 d.Bounds.X,
		Y:                 d.Bounds.Y,
		Width:             d.Bounds.Width,
		Height:            d.Bounds.Height,
		UseContentSize:    true,
		Frameless:         true,
		AlwaysOnTop:       true,
		SkipTaskbar:       true,
		Hidden:            true,
		DisableResize:     true,
		DisableMove:       true,
		DisableMinimize:   true,
		DisableMaximize:   true,
		DisableShadow:     true,
		DisableFullScreen: true,
		BackgroundColor:   "#000000",
	})
	overlay = w
	w.OnClosed(func() {
		overlayMu.Lock()
		if overlay == w {
			overlay = nil
		}
		overlayMu.Unlock()
	})
}

func closeOverlay() {
	overlayMu.Lock()
	w := overlay
	overlay = nil
	overlayMu.Unlock()
	if w == nil || w.IsDestroyed() {
		return
	}
	w.Close()
	// Close 会阻塞到主线程真正销毁窗口，但屏幕上的像素要等下方窗口重绘、
	// 合成器出一帧之后才恢复；不等就抓屏，会把上一次的遮罩拍进新截图。
	deadline := time.Now().Add(time.Second)
	for !w.IsDestroyed() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !w.IsDestroyed() {
		log.Printf("旧遮罩窗口未在 1 秒内完成销毁，本次截图可能包含旧画面")
	}
	dwmFlush()
}

func notify(title, body string) {
	n := mygo.NewNotification(mygo.NotificationOptions{Title: title, Body: body})
	if err := n.Show(); err != nil {
		log.Printf("%s: %s", title, body)
	}
}

var tray *mygo.Tray

func buildTrayMenu() *mygo.Menu {
	radio := func(label, mode string) *mygo.MenuItem {
		return &mygo.MenuItem{
			Label:   label,
			Type:    mygo.MenuItemRadio,
			Checked: currentClipMode() == mode,
			Click: func(*mygo.MenuItem, *mygo.Window) {
				_ = Shot{}.SetClipMode(mode)
			},
		}
	}
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: "区域截图", Accelerator: "PrintScreen", Click: func(*mygo.MenuItem, *mygo.Window) { openCapture("region") }},
		{Label: "全屏截图", Accelerator: "Ctrl+PrintScreen", Click: func(*mygo.MenuItem, *mygo.Window) { openCapture("fullscreen") }},
		{Label: "当前窗口", Accelerator: "Alt+PrintScreen", Click: func(*mygo.MenuItem, *mygo.Window) { openCapture("window") }},
		mygo.Separator(),
		{Label: "复制内容", Submenu: []*mygo.MenuItem{
			radio("图片 + 路径", "both"),
			radio("只要图片", "image"),
			radio("只要路径", "path"),
		}},
		{Label: "打开截图目录", Click: func(*mygo.MenuItem, *mygo.Window) { _ = Shot{}.OpenSaveDir() }},
		updater.MenuItem(),
		{Label: "接管 Print Screen 键", Type: mygo.MenuItemCheckbox, Checked: snippingKeyOwned(), Click: func(it *mygo.MenuItem, _ *mygo.Window) {
			if err := setSnippingKeyOwned(it.Checked); err != nil {
				notify("设置失败", err.Error())
			}
			refreshTrayMenu()
		}},
		mygo.Separator(),
		{Label: "退出 SnapBridge", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
	})
}

func refreshTrayMenu() {
	if tray == nil {
		return
	}
	tray.SetMenu(buildTrayMenu())
}

func setupTray(icon []byte) {
	t, err := mygo.NewTray(mygo.TrayOptions{
		ID:      "main",
		Icon:    icon,
		ToolTip: "SnapBridge · PrintScreen 截图",
		Menu:    buildTrayMenu(),
	})
	if err != nil {
		log.Printf("托盘创建失败: %v", err)
		return
	}
	tray = t
}

func registerHotkeys() {
	// PrintScreen 系列由 hotkey_windows.go 的低级钩子接管。
	// Windows 的「使用 Print Screen 键打开屏幕截图」开着时，截图工具经 Raw Input 取走按键，
	// 低级键盘钩子拦不住，必须在注册表里把这个键交还应用程序。
	if !snippingKeyOwned() {
		if err := setSnippingKeyOwned(true); err != nil {
			log.Printf("收回 Print Screen 键失败: %v", err)
			notify("Print Screen 键被系统截图工具占用",
				"系统设置里的「使用 Print Screen 键打开屏幕截图」处于开启状态，SnapBridge 拿不到这个键。可在托盘菜单里手动切换归属。")
		}
	}
	if err := startSnapshotHook(); err != nil {
		// 钩子不可用时退回系统热键注册。
		_ = mygo.GlobalShortcut.Register("PrintScreen", func() { openCapture("region") })
		_ = mygo.GlobalShortcut.Register("Ctrl+PrintScreen", func() { openCapture("fullscreen") })
		_ = mygo.GlobalShortcut.Register("Alt+PrintScreen", func() { openCapture("window") })
	}

	failed := ""
	bind := func(acc, mode string) {
		if err := mygo.GlobalShortcut.Register(acc, func() { openCapture(mode) }); err != nil {
			failed += fmt.Sprintf("%s：%v\n", acc, err)
			log.Printf("%s 注册失败: %v", acc, err)
		}
	}
	// 备用键位，不与系统截图开关冲突。
	bind("Ctrl+Shift+A", "region")
	bind("F1", "region")
	if failed != "" {
		notify("快捷键提示", failed)
	}
}

func defaultSaveDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "SnapBridge")
	}
	return filepath.Join(home, "Pictures", "SnapBridge")
}

func main() {
	saveDir = defaultSaveDir()

	// 声明界面语言，更新窗口才会跟着系统语言在中文与英文之间选。
	mygo.App.SetLanguages("zh-Hans", "en")

	// 更新检查与下载走镜像回退，必须在插件第一次检查之前装好。
	useMirroredUpdates()
	mygo.Use(updater.New(updater.Options{Icon: appIconPNG()}))

	mygo.Bind(Shot{})

	mygo.App.WhenReady(func() {
		setupTray(trayIconPNG())
		registerHotkeys()
	})

	// 关闭遮罩窗口后进程继续留在托盘。
	mygo.App.OnWindowAllClosed(func() {})

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

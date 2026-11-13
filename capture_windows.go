//go:build windows

package main

import (
	"fmt"
	"image"
	"syscall"
	"unsafe"
)

var (
	user32 = syscall.NewLazyDLL("user32.dll")
	gdi32  = syscall.NewLazyDLL("gdi32.dll")

	procSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	procGetDC                         = user32.NewProc("GetDC")
	procReleaseDC                     = user32.NewProc("ReleaseDC")
	procGetSystemMetrics              = user32.NewProc("GetSystemMetrics")
	procGetForegroundWindow           = user32.NewProc("GetForegroundWindow")
	procGetWindowRect                 = user32.NewProc("GetWindowRect")
	procGetCursorPos                  = user32.NewProc("GetCursorPos")
	procGetWindowLong                 = user32.NewProc("GetWindowLongW")
	procCreateCompatibleDC            = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBitmap        = gdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject                  = gdi32.NewProc("SelectObject")
	procBitBlt                        = gdi32.NewProc("BitBlt")
	procDeleteObject                  = gdi32.NewProc("DeleteObject")
	procDeleteDC                      = gdi32.NewProc("DeleteDC")
	procGetDIBits                     = gdi32.NewProc("GetDIBits")
)

const (
	smXVirtualScreen  = 76
	smYVirtualScreen  = 77
	smCXVirtualScreen = 78
	smCYVirtualScreen = 79
	srcCopy           = 0x00CC0020
	biRGB             = 0
	dibRGBColors      = 0
)

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type bitmapInfo struct {
	Header bitmapInfoHeader
	Colors [1]uint32
}

type point struct {
	X int32
	Y int32
}

type rect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

// ScreenCapture 是一次虚拟桌面截图，带上虚拟桌面原点的物理像素坐标。
type ScreenCapture struct {
	Image *image.RGBA
	X     int
	Y     int
}

func init() {
	// 让位图与系统度量都用物理像素，否则高分屏截图会被系统拉伸。
	ctx := ^uintptr(3) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2
	procSetProcessDpiAwarenessContext.Call(ctx)
}

// CaptureScreen 抓取整个虚拟桌面（所有显示器）。
func CaptureScreen() (*ScreenCapture, error) {
	x, _, _ := procGetSystemMetrics.Call(smXVirtualScreen)
	y, _, _ := procGetSystemMetrics.Call(smYVirtualScreen)
	w, _, _ := procGetSystemMetrics.Call(smCXVirtualScreen)
	h, _, _ := procGetSystemMetrics.Call(smCYVirtualScreen)
	if int(w) <= 0 || int(h) <= 0 {
		return nil, fmt.Errorf("无法读取虚拟桌面尺寸")
	}

	hdcScreen, _, _ := procGetDC.Call(0)
	if hdcScreen == 0 {
		return nil, fmt.Errorf("GetDC 失败")
	}
	defer procReleaseDC.Call(0, hdcScreen)

	hdcMem, _, _ := procCreateCompatibleDC.Call(hdcScreen)
	if hdcMem == 0 {
		return nil, fmt.Errorf("CreateCompatibleDC 失败")
	}
	defer procDeleteDC.Call(hdcMem)

	hBitmap, _, _ := procCreateCompatibleBitmap.Call(hdcScreen, uintptr(w), uintptr(h))
	if hBitmap == 0 {
		return nil, fmt.Errorf("CreateCompatibleBitmap 失败")
	}
	defer procDeleteObject.Call(hBitmap)

	hOld, _, _ := procSelectObject.Call(hdcMem, hBitmap)
	defer procSelectObject.Call(hdcMem, hOld)

	ret, _, _ := procBitBlt.Call(hdcMem, 0, 0, uintptr(w), uintptr(h), hdcScreen, x, y, srcCopy)
	if ret == 0 {
		return nil, fmt.Errorf("BitBlt 失败")
	}

	img := image.NewRGBA(image.Rect(0, 0, int(w), int(h)))
	var info bitmapInfo
	info.Header.Size = uint32(unsafe.Sizeof(info.Header))
	info.Header.Width = int32(w)
	info.Header.Height = -int32(h)
	info.Header.Planes = 1
	info.Header.BitCount = 32
	info.Header.Compression = biRGB

	lines, _, _ := procGetDIBits.Call(
		hdcMem,
		hBitmap,
		0,
		uintptr(h),
		uintptr(unsafe.Pointer(&img.Pix[0])),
		uintptr(unsafe.Pointer(&info)),
		uintptr(dibRGBColors),
	)
	if lines == 0 {
		return nil, fmt.Errorf("GetDIBits 失败")
	}

	// GDI 给的是 BGRA，交换红蓝通道并补齐不透明度。
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+2] = img.Pix[i+2], img.Pix[i]
		img.Pix[i+3] = 255
	}

	return &ScreenCapture{Image: img, X: int(x), Y: int(y)}, nil
}

// ForegroundWindowRect 返回前台窗口的物理像素矩形。
func ForegroundWindowRect() (image.Rectangle, error) {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return image.Rectangle{}, fmt.Errorf("没有前台窗口")
	}
	var r rect
	ret, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	if ret == 0 {
		return image.Rectangle{}, fmt.Errorf("GetWindowRect 失败")
	}
	return image.Rect(int(r.Left), int(r.Top), int(r.Right), int(r.Bottom)), nil
}

// CursorPosition 返回光标在虚拟桌面上的物理像素位置。
func CursorPosition() (int, int, error) {
	var p point
	ret, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	if ret == 0 {
		return 0, 0, fmt.Errorf("GetCursorPos 失败")
	}
	return int(p.X), int(p.Y), nil
}

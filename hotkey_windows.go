//go:build windows

package main

import (
	"log"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// PrintScreen 在 Windows 11 上会被「按 Print Screen 键打开屏幕截图」这一
// 系统设置以低级钩子先行吞掉，RegisterHotKey 注册成功却收不到按键。
// 这里自己挂一个低级键盘钩子，按键到达系统截图服务之前就由本程序接管。
const (
	whKeyboardLL  = 13
	wmKeyDown     = 0x0100
	wmKeyUp       = 0x0101
	wmSysKeyDown  = 0x0104
	wmSysKeyUp    = 0x0105
	vkSnapshot    = 0x2C
	vkControl     = 0x11
	vkMenu        = 0x12
	snapshotGuard = 280 * time.Millisecond
)

type kbdllHookStruct struct {
	vkCode    uint32
	scanCode  uint32
	flags     uint32
	time      uint32
	extraInfo uintptr
}

type winMsg struct {
	hwnd     uintptr
	message  uint32
	wParam   uintptr
	lParam   uintptr
	time     uint32
	pt       struct{ x, y int32 }
	lPrivate uint32
}

var (
	user32DLL   = syscall.NewLazyDLL("user32.dll")
	kernel32DLL = syscall.NewLazyDLL("kernel32.dll")

	procSetWindowsHookExW = user32DLL.NewProc("SetWindowsHookExW")
	procCallNextHookEx    = user32DLL.NewProc("CallNextHookEx")
	procUnhookWindowsHook = user32DLL.NewProc("UnhookWindowsHookEx")
	procGetMessageW       = user32DLL.NewProc("GetMessageW")
	procPeekMessageW      = user32DLL.NewProc("PeekMessageW")
	procGetAsyncKeyState  = user32DLL.NewProc("GetAsyncKeyState")
	procGetModuleHandleW  = kernel32DLL.NewProc("GetModuleHandleW")
)

var (
	snapshotMu   sync.Mutex
	lastSnapshot time.Time
	snapshotSink = make(chan string, 4)
)

// keyHeld 判断某个虚拟键此刻是否处于按下状态。
func keyHeld(vk uintptr) bool {
	r, _, _ := procGetAsyncKeyState.Call(vk)
	return r&0x8000 != 0
}

// snapshotMode 根据修饰键决定本次是区域、全屏还是活动窗口。
func snapshotMode() string {
	switch {
	case keyHeld(vkControl):
		return "fullscreen"
	case keyHeld(vkMenu):
		return "window"
	default:
		return "region"
	}
}

// fireSnapshot 去抖后把一次截图请求交给消费协程。
// 时间戳只在请求真的进入队列后才记：通道满时请求被丢，
// 要是那时也记了时间，之后 280 毫秒内的按键会被去抖挡掉，一起丢掉。
func fireSnapshot(mode string) {
	snapshotMu.Lock()
	defer snapshotMu.Unlock()
	if time.Since(lastSnapshot) < snapshotGuard {
		return
	}
	select {
	case snapshotSink <- mode:
		lastSnapshot = time.Now()
	default:
	}
}

var snapshotCallback = syscall.NewCallback(func(nCode, wParam, lParam uintptr) uintptr {
	if int32(uint32(nCode)) == 0 {
		// lParam 是系统在本次回调期间交给钩子的 KBDLLHOOKSTRUCT 指针，
		// 转换出的指针只在这个栈帧里使用、不逃逸，是 syscall 回调的惯用写法；
		// go vet 的 unsafeptr 检查在这里是已知误报，检查时带 -unsafeptr=false。
		kb := (*kbdllHookStruct)(unsafe.Pointer(lParam))
		if kb.vkCode == vkSnapshot {
			switch wParam {
			case wmKeyDown, wmSysKeyDown:
				fireSnapshot(snapshotMode())
			}
			// 吞掉按下与抬起，系统截图服务与剪贴板行为都不再触发。
			return 1
		}
	}
	r, _, _ := procCallNextHookEx.Call(0, nCode, wParam, lParam)
	return r
})

// startSnapshotHook 在独占的操作系统线程上安装钩子并跑消息循环，
// 低级钩子的回调依赖安装线程自己的消息泵。
func startSnapshotHook() error {
	ready := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		// 先建消息队列，否则 SetWindowsHookEx 装的钩子收不到任何事件。
		var m winMsg
		procPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, 0)

		hInst, _, _ := procGetModuleHandleW.Call(0)
		h, _, err := procSetWindowsHookExW.Call(whKeyboardLL, snapshotCallback, hInst, 0)
		if h == 0 {
			ready <- err
			return
		}
		ready <- nil

		for {
			r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if int32(uint32(r)) <= 0 {
				procUnhookWindowsHook.Call(h)
				return
			}
		}
	}()

	// 消费协程独立于钩子线程，避免在钩子回调里做耗时的抓屏。
	go func() {
		for mode := range snapshotSink {
			openCapture(mode)
		}
	}()

	err := <-ready
	if err != nil {
		log.Printf("PrintScreen 钩子安装失败: %v", err)
	}
	return err
}

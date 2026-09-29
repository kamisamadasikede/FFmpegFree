//go:build windows

package live

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"
)

var (
	user32                   = syscall.NewLazyDLL("user32.dll")
	dwmapi                   = syscall.NewLazyDLL("dwmapi.dll")
	procEnumWindows          = user32.NewProc("EnumWindows")
	procIsWindowVisible      = user32.NewProc("IsWindowVisible")
	procIsIconic             = user32.NewProc("IsIconic")
	procGetWindowTextLengthW = user32.NewProc("GetWindowTextLengthW")
	procGetWindowTextW       = user32.NewProc("GetWindowTextW")
	procGetClassNameW        = user32.NewProc("GetClassNameW")
	procGetWindowThreadPID   = user32.NewProc("GetWindowThreadProcessId")
	procGetWindowLongW       = user32.NewProc("GetWindowLongW")
	procGetWindow            = user32.NewProc("GetWindow")
	procGetClientRect        = user32.NewProc("GetClientRect")
	procDwmGetWindowAttr     = dwmapi.NewProc("DwmGetWindowAttribute")

	enumMu      sync.Mutex // 回调只创建一次（syscall.NewCallback 有数量上限，不能每次调用都建），用互斥保护结果
	enumResult  []RawWindow
	enumWinCB   = syscall.NewCallback(enumWindowsProc)
	monitorsCB  = syscall.NewCallback(enumMonitorsProc)
	monitorsRes []ScreenInfo
	monitorsGI  = user32.NewProc("GetMonitorInfoW")
	monitorsEnu = user32.NewProc("EnumDisplayMonitors")
)

const (
	gwlExStyle       = ^uintptr(19) // -20
	gwOwner          = 4
	dwmwaCloaked     = 14
	maxWindowTitleCh = 1024
)

func enumWindowsProc(hwnd, lparam uintptr) uintptr {
	if w, ok := readWindow(hwnd); ok {
		enumResult = append(enumResult, w)
	}
	return 1 // 继续枚举
}

func readWindow(hwnd uintptr) (RawWindow, bool) {
	w := RawWindow{HWND: uint64(hwnd)}
	if r, _, _ := procIsWindowVisible.Call(hwnd); r != 0 {
		w.Visible = true
	}
	if !w.Visible {
		return w, false // 不可见的直接丢，省得为每个隐藏窗口取标题
	}
	if r, _, _ := procIsIconic.Call(hwnd); r != 0 {
		w.Minimized = true
	}
	n, _, _ := procGetWindowTextLengthW.Call(hwnd)
	if n == 0 {
		return w, false
	}
	if n > maxWindowTitleCh {
		n = maxWindowTitleCh
	}
	buf := make([]uint16, n+1)
	if r, _, _ := procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf))); r == 0 {
		return w, false
	}
	w.Title = syscall.UTF16ToString(buf)
	cls := make([]uint16, 256)
	if r, _, _ := procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&cls[0])), uintptr(len(cls))); r != 0 {
		w.Class = syscall.UTF16ToString(cls)
	}
	var pid uint32
	procGetWindowThreadPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	w.PID = pid
	ex, _, _ := procGetWindowLongW.Call(hwnd, gwlExStyle)
	w.ExStyle = uint32(ex)
	owner, _, _ := procGetWindow.Call(hwnd, gwOwner)
	w.Owner = uint64(owner)
	var cloaked uint32
	if procDwmGetWindowAttr.Find() == nil {
		if hr, _, _ := procDwmGetWindowAttr.Call(hwnd, dwmwaCloaked, uintptr(unsafe.Pointer(&cloaked)), unsafe.Sizeof(cloaked)); hr == 0 && cloaked != 0 {
			w.Cloaked = true
		}
	}
	// gdigrab 采的是客户区（GetClientRect），大小与它保持一致。最小化窗口的客户区是 0，本来也不会被列出。
	var rc winRect
	if r, _, _ := procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc))); r != 0 {
		w.Width, w.Height = int(rc.Right-rc.Left), int(rc.Bottom-rc.Top)
	}
	return w, true
}

// enumTopLevelWindows 用 EnumWindows 列出所有可见、有标题的顶层窗口（未做采集过滤，过滤见 filterCaptureWindows）。不用 cgo。
func enumTopLevelWindows() ([]RawWindow, error) {
	enumMu.Lock()
	defer enumMu.Unlock()
	enumResult = nil
	if r, _, err := procEnumWindows.Call(enumWinCB, 0); r == 0 {
		enumResult = nil
		return nil, fmt.Errorf("EnumWindows 失败: %v", err)
	}
	res := enumResult
	enumResult = nil
	return res, nil
}

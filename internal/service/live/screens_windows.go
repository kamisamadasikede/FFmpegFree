//go:build windows

package live

import (
	"fmt"
	"syscall"
	"unsafe"
)

type winRect struct{ Left, Top, Right, Bottom int32 }

type winMonitorInfo struct {
	Size    uint32
	Monitor winRect
	Work    winRect
	Flags   uint32
}

// listWindowsMonitors 用 EnumDisplayMonitors 列出显示器（不用 cgo）。坐标是系统报告的值：
// 进程不是 DPI 感知时可能是缩放后的逻辑像素（未在 Windows 真机验证，见契约 6.10「未验证」）。
func listWindowsMonitors() ([]ScreenInfo, error) {
	user32 := syscall.NewLazyDLL("user32.dll")
	enum := user32.NewProc("EnumDisplayMonitors")
	getInfo := user32.NewProc("GetMonitorInfoW")
	var res []ScreenInfo
	cb := syscall.NewCallback(func(hMon, hdc, rc, lparam uintptr) uintptr {
		mi := winMonitorInfo{Size: uint32(unsafe.Sizeof(winMonitorInfo{}))}
		if r, _, _ := getInfo.Call(hMon, uintptr(unsafe.Pointer(&mi))); r != 0 {
			n := len(res)
			si := ScreenInfo{
				ID: fmt.Sprintf("monitor:%d", n), Primary: mi.Flags&1 != 0, Scale: 1,
				X: int(mi.Monitor.Left), Y: int(mi.Monitor.Top),
				Width: int(mi.Monitor.Right - mi.Monitor.Left), Height: int(mi.Monitor.Bottom - mi.Monitor.Top),
			}
			si.Name = fmt.Sprintf("显示器 %d", n+1)
			if si.Primary {
				si.Name += "（主）"
			}
			res = append(res, si)
		}
		return 1
	})
	if r, _, err := enum.Call(0, 0, cb, 0); r == 0 {
		return nil, fmt.Errorf("EnumDisplayMonitors 失败: %v", err)
	}
	if len(res) == 0 {
		return nil, fmt.Errorf("没有找到显示器")
	}
	return res, nil
}

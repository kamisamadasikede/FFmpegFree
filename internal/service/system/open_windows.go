//go:build windows

package system

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procShellExecuteW = windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteW")

// ShellExecuteW 失败时的返回值（<= 32）里表示“没有关联程序”的两个（契约 6.14.3）。
const (
	seErrAssocIncomplete = 27
	seErrNoAssoc         = 31
)

// openDefault 用 ShellExecuteW 的 open 动词打开 path：路径作为文件参数传入，不经过命令行拼接。
func openDefault(path string) error {
	verb, err := windows.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	file, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	r, _, callErr := procShellExecuteW.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(file)), 0, 0, uintptr(windows.SW_SHOWNORMAL))
	if r > 32 {
		return nil
	}
	if r == seErrNoAssoc || r == seErrAssocIncomplete {
		return ErrNoApp
	}
	return fmt.Errorf("ShellExecuteW 返回 %d: %v", r, callErr)
}

//go:build windows

package doc

import (
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"FFmpegFree/internal/apperr"
)

var (
	modKernel32      = windows.NewLazySystemDLL("kernel32.dll")
	procReplaceFileW = modKernel32.NewProc("ReplaceFileW")
)

const (
	replaceFileIgnoreMergeErrors = 0x00000002
)

// replaceFile：优先 ReplaceFileW，失败再 MoveFileExW(REPLACE_EXISTING)（契约 6.12.41）。
func replaceFile(tmp, target string) error {
	from, err := windows.UTF16PtrFromString(tmp)
	if err != nil {
		return mapWriteErr(err)
	}
	to, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return mapWriteErr(err)
	}
	// 目标不存在：直接改名
	if _, err := os.Lstat(target); os.IsNotExist(err) {
		if err := windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING); err != nil {
			return mapWinReplaceErr(err)
		}
		return nil
	}
	r, _, callErr := procReplaceFileW.Call(
		uintptr(unsafe.Pointer(to)),
		uintptr(unsafe.Pointer(from)),
		0,
		uintptr(replaceFileIgnoreMergeErrors),
		0, 0,
	)
	if r != 0 {
		return nil
	}
	if errno, ok := callErr.(syscall.Errno); ok && errno != 0 {
		// ERROR_UNABLE_TO_REMOVE_REPLACED = 1175；共享冲突等
		if errno == 32 || errno == 33 || errno == 1175 {
			return apperr.New(apperr.IOError, "文件正被其他程序占用，请关闭后再保存。").WithDetail("reason=in_use")
		}
		if errno == 5 { // ACCESS_DENIED
			return apperr.New(apperr.IOError, "没有权限保存到这里，请另存到其他位置。").WithDetail("reason=permission")
		}
		// 退回 MoveFileEx
		if err := windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING); err != nil {
			return mapWinReplaceErr(err)
		}
		return nil
	}
	if err := windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING); err != nil {
		return mapWinReplaceErr(err)
	}
	return nil
}

func mapWinReplaceErr(err error) error {
	if err == nil {
		return nil
	}
	if errno, ok := err.(syscall.Errno); ok {
		switch errno {
		case 32, 33, 1175:
			return apperr.New(apperr.IOError, "文件正被其他程序占用，请关闭后再保存。").WithDetail("reason=in_use")
		case 5:
			return apperr.New(apperr.IOError, "没有权限保存到这里，请另存到其他位置。").WithDetail("reason=permission")
		case 112, 39:
			return apperr.Wrap(apperr.ConvertDiskFull, "磁盘空间不足，没有保存。", err)
		}
	}
	return mapWriteErr(err)
}

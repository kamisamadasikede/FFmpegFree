//go:build windows

package langasr

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

func diskFree(dir string) (int64, error) {
	dir = filepath.Clean(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	p, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		return 0, err
	}
	var free, total, totalFree uint64
	mod := syscall.NewLazyDLL("kernel32.dll")
	proc := mod.NewProc("GetDiskFreeSpaceExW")
	r, _, e := proc.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&free)), uintptr(unsafe.Pointer(&total)), uintptr(unsafe.Pointer(&totalFree)))
	if r == 0 {
		return 0, e
	}
	return int64(free), nil
}

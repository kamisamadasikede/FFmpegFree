//go:build windows

package i18n

import (
	"syscall"
	"unsafe"
)

var (
	modKernel32               = syscall.NewLazyDLL("kernel32.dll")
	procGetUserDefaultLocaleN = modKernel32.NewProc("GetUserDefaultLocaleName")
)

// DetectOSLocaleRaw 调用 GetUserDefaultLocaleName（契约 6.20.3）。
func DetectOSLocaleRaw() string {
	buf := make([]uint16, 85) // LOCALE_NAME_MAX_LENGTH
	r, _, _ := procGetUserDefaultLocaleN.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

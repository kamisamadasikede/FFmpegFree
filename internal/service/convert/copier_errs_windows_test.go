//go:build windows

package convert

import "syscall"

var (
	errBusy    = syscall.Errno(32)  // ERROR_SHARING_VIOLATION
	errNoSpace = syscall.Errno(112) // ERROR_DISK_FULL
)

//go:build windows

package task

import (
	"errors"
	"syscall"
)

const (
	errorHandleDiskFull syscall.Errno = 39
	errorDiskFull       syscall.Errno = 112
)

func isPlatformDiskFull(err error) bool {
	return errors.Is(err, errorDiskFull) || errors.Is(err, errorHandleDiskFull)
}

const (
	errorSharingViolation syscall.Errno = 32
	errorLockViolation    syscall.Errno = 33
)

// isPlatformInUse：Windows 的 ERROR_SHARING_VIOLATION（32）/ ERROR_LOCK_VIOLATION（33）（契约 6.14.4 的 in_use）。
func isPlatformInUse(err error) bool {
	return errors.Is(err, errorSharingViolation) || errors.Is(err, errorLockViolation)
}

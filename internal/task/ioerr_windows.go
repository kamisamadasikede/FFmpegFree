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

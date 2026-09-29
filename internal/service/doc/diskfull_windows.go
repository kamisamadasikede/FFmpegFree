//go:build windows

package doc

import (
	"errors"
	"syscall"
)

// ERROR_DISK_FULL(112) / ERROR_HANDLE_DISK_FULL(39)，与 internal/task 的判定一致。
func isDiskFull(err error) bool {
	var en syscall.Errno
	return errors.As(err, &en) && (en == 112 || en == 39)
}

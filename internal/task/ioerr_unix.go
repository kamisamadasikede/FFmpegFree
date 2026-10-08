//go:build !windows

package task

import (
	"errors"
	"syscall"
)

func isPlatformDiskFull(err error) bool { return errors.Is(err, syscall.EDQUOT) }

// isPlatformInUse：Unix 的 EBUSY / ETXTBSY（契约 6.14.4 的 in_use）。
func isPlatformInUse(err error) bool {
	return errors.Is(err, syscall.EBUSY) || errors.Is(err, syscall.ETXTBSY)
}

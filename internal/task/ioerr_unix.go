//go:build !windows

package task

import (
	"errors"
	"syscall"
)

func isPlatformDiskFull(err error) bool { return errors.Is(err, syscall.EDQUOT) }

//go:build !windows

package doc

import (
	"errors"
	"syscall"
)

func isDiskFull(err error) bool {
	return errors.Is(err, syscall.ENOSPC) || errors.Is(err, syscall.EDQUOT)
}

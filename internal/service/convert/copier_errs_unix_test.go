//go:build !windows

package convert

import "syscall"

var (
	errBusy    = syscall.EBUSY
	errNoSpace = syscall.ENOSPC
)

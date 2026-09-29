//go:build !windows

package media

import "syscall"

func syscallMkfifo(p string) error { return syscall.Mkfifo(p, 0o644) }

//go:build !windows

package task

import "syscall"

// inUseErrno 是本平台“文件正在被使用”的 errno（Unix：EBUSY）。
var inUseErrno error = syscall.EBUSY

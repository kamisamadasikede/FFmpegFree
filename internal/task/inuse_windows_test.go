//go:build windows

package task

import "syscall"

// inUseErrno 是本平台“文件正在被使用”的 errno（Windows：ERROR_SHARING_VIOLATION = 32）。
var inUseErrno error = syscall.Errno(32)

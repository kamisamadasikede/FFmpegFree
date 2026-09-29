package task

import (
	"errors"
	"syscall"

	"FFmpegFree/internal/apperr"
)

// isDiskFull 判断 err 是不是"磁盘空间不足"：Unix 的 ENOSPC（含 EDQUOT 配额用尽），
// Windows 的 ERROR_DISK_FULL(112) / ERROR_HANDLE_DISK_FULL(39)（见 ioerr_windows.go）。
func isDiskFull(err error) bool {
	return errors.Is(err, syscall.ENOSPC) || isPlatformDiskFull(err)
}

// outputIOError 把创建输出目录、提交输出文件时的系统错误包成契约错误码：
// 磁盘满 → CONVERT_DISK_FULL，其余 → IO_ERROR。
func outputIOError(msg string, err error) error {
	if isDiskFull(err) {
		return apperr.Wrap(apperr.ConvertDiskFull, "磁盘空间不足，无法写入输出文件", err)
	}
	return apperr.Wrap(apperr.IOError, msg, err)
}

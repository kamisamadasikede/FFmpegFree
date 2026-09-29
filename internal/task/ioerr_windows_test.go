//go:build windows

package task

import (
	"os"
	"testing"
)

func TestIsDiskFullWindowsErrno(t *testing.T) {
	if !isDiskFull(&os.PathError{Op: "write", Err: errorDiskFull}) || !isDiskFull(errorHandleDiskFull) {
		t.Fatal("ERROR_DISK_FULL(112) / ERROR_HANDLE_DISK_FULL(39) 应识别为磁盘满")
	}
}

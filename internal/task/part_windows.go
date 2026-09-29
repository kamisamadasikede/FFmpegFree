//go:build windows

package task

import (
	"io/fs"
	"os"

	"golang.org/x/sys/windows"
)

// renameNoReplace 用不带 MOVEFILE_REPLACE_EXISTING 的 MoveFileEx 改名：目标已存在时失败，而不是像 os.Rename 那样覆盖
// （契约 6.11.3「提交阶段 os.Link 不可用时的回退」）。目标存在时返回带 fs.ErrExist 的 *os.LinkError。未在 Windows 真机验证。
func renameNoReplace(oldpath, newpath string) error {
	from, err := windows.UTF16PtrFromString(oldpath)
	if err != nil {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: err}
	}
	to, err := windows.UTF16PtrFromString(newpath)
	if err != nil {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: err}
	}
	if err := windows.MoveFileEx(from, to, 0); err != nil {
		if err == windows.ERROR_ALREADY_EXISTS || err == windows.ERROR_FILE_EXISTS {
			err = fs.ErrExist
		}
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: err}
	}
	return nil
}

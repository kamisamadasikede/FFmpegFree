//go:build !windows

package task

import "os"

// renameNoReplace 在非 Windows 平台就是 os.Rename（调用方已检查目标不存在；POSIX rename 会覆盖，残余竞态见契约 6.11.3）。
func renameNoReplace(oldpath, newpath string) error { return os.Rename(oldpath, newpath) }

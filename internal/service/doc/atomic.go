package doc

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/task"
)

// writeAtomic 在同目录写临时文件再替换目标（契约 6.12.41）：失败时原文件不变。
func writeAtomic(target string, data []byte) error {
	dir := filepath.Dir(target)
	base := filepath.Base(target)
	tmp, err := tempBeside(dir, base)
	if err != nil {
		return mapWriteErr(err)
	}
	defer os.Remove(tmp) // 成功替换后临时文件已不在；失败时清掉

	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return mapWriteErr(err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return mapWriteErr(err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return mapWriteErr(err)
	}
	if err := f.Close(); err != nil {
		return mapWriteErr(err)
	}
	if fi, err := os.Lstat(target); err == nil && !fi.IsDir() {
		_ = os.Chmod(tmp, fi.Mode().Perm())
	}
	if err := replaceFile(tmp, target); err != nil {
		return err
	}
	return nil
}

// copyAtomic 把 src 整文件复制到 target（同目录临时文件再替换）。
func copyAtomic(src, target string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return mapReadWriteErr(err)
	}
	return writeAtomic(target, data)
}

func tempBeside(dir, base string) (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	name := fmt.Sprintf(".%s.ffmpegfree-%s.tmp", base, hex.EncodeToString(b[:]))
	return filepath.Join(dir, name), nil
}

func mapWriteErr(err error) *apperr.AppError {
	if err == nil {
		return nil
	}
	if task.IsDiskFull(err) {
		return apperr.Wrap(apperr.ConvertDiskFull, "磁盘空间不足，没有保存。", err)
	}
	if isInUseErr(err) {
		return apperr.New(apperr.IOError, "文件正被其他程序占用，请关闭后再保存。").WithDetail("reason=in_use")
	}
	if isPermErr(err) {
		return apperr.New(apperr.IOError, "没有权限保存到这里，请另存到其他位置。").WithDetail("reason=permission")
	}
	return apperr.Wrap(apperr.IOError, "保存失败，请重试或另存为。", err).WithDetail("reason=io\n" + err.Error())
}

func mapReadWriteErr(err error) *apperr.AppError {
	if err == nil {
		return nil
	}
	if os.IsNotExist(err) {
		return apperr.New(apperr.NotFound, "原文件已经不在了，改完只能另存为。").WithDetail("reason=file")
	}
	if isInUseErr(err) {
		return apperr.New(apperr.IOError, "文件正被其他程序占用，请关闭后再保存。").WithDetail("reason=in_use")
	}
	return mapWriteErr(err)
}

func isInUseErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.EBUSY) || errors.Is(err, syscall.ETXTBSY) {
		return true
	}
	// Windows 错误在 atomic_windows 里用 replaceFile 直接判；这里看字符串兜底
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "sharing violation") || strings.Contains(msg, "being used by another") ||
		strings.Contains(msg, "lock violation")
}

func isPermErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrPermission) || errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "access is denied") || strings.Contains(msg, "permission denied")
}

// pathKey 比较用：Clean + 平台大小写。
func pathKey(p string) string {
	p = filepath.Clean(p)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.ToLower(p)
	}
	return p
}

func realPath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}

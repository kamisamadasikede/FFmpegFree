//go:build !windows

package convert

import "syscall"

// diskFree 返回 dir 所在磁盘此刻可用的空间（statfs 的 Bavail × Bsize，契约 6.15.4）。
func diskFree(dir string) (int64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, err
	}
	return int64(st.Bavail) * int64(st.Bsize), nil //nolint:unconvert // 各平台字段类型不同
}

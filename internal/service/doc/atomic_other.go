//go:build !windows

package doc

import "os"

// replaceFile：Unix 用 rename 覆盖（契约 6.12.41）。
func replaceFile(tmp, target string) error {
	if err := os.Rename(tmp, target); err != nil {
		return mapWriteErr(err)
	}
	return nil
}

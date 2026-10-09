package doc

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"FFmpegFree/internal/apperr"
)

const maxDocxBackups = 3

// backupDocxOverwrite 覆盖前备份（6.12.51）。失败一律 IO_ERROR reason=backup。
func backupDocxOverwrite(src, revision string) (backupPath string, err error) {
	sum, _, err := fileSHA256(src)
	if err != nil {
		return "", apperr.New(apperr.IOError, "没法在这个文件夹留备份，文件没有保存。请另存为。").WithDetail("reason=backup\ncause=io")
	}
	if sum != revision {
		return "", apperr.New(apperr.TaskConflict, "文件在别处被改过了，请重新打开，或另存为。").WithDetail("reason=file_changed")
	}
	dir := filepath.Dir(src)
	base := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
	stamp := time.Now().Format("20060102-150405")
	name := fmt.Sprintf("%s.bak-%s.docx", base, stamp)
	dst := filepath.Join(dir, name)
	for n := 2; ; n++ {
		if _, err := os.Lstat(dst); os.IsNotExist(err) {
			break
		}
		name = fmt.Sprintf("%s.bak-%s-%d.docx", base, stamp, n)
		dst = filepath.Join(dir, name)
	}
	tmp, err := tempBeside(dir, filepath.Base(dst))
	if err != nil {
		return "", backupFail(err)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		os.Remove(tmp)
		return "", backupFail(err)
	}
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		os.Remove(tmp)
		return "", backupFail(err)
	}
	if fi, err := os.Stat(src); err == nil {
		_ = os.Chtimes(tmp, fi.ModTime(), fi.ModTime())
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return "", backupFail(err)
	}
	return dst, nil
}

func backupFail(err error) *apperr.AppError {
	cause := "io"
	if isPermErr(err) {
		cause = "permission"
	} else if isInUseErr(err) {
		cause = "in_use"
	}
	return apperr.New(apperr.IOError, "没法在这个文件夹留备份，文件没有保存。请另存为。").
		WithDetail("reason=backup\ncause=" + cause)
}

// pruneDocxBackups 替换成功后只保留最近 3 份本应用格式备份。
func pruneDocxBackups(originalPath string) {
	dir := filepath.Dir(originalPath)
	base := strings.TrimSuffix(filepath.Base(originalPath), filepath.Ext(originalPath))
	re := regexp.MustCompile(`(?i)^` + regexp.QuoteMeta(base) + `\.bak-[0-9]{8}-[0-9]{6}(-[0-9]+)?\.docx$`)
	ents, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type item struct{ name, path string }
	var list []item
	for _, e := range ents {
		if e.IsDir() || !re.MatchString(e.Name()) {
			continue
		}
		p := filepath.Join(dir, e.Name())
		fi, err := os.Lstat(p)
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		list = append(list, item{name: e.Name(), path: p})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].name > list[j].name })
	for i := maxDocxBackups; i < len(list); i++ {
		_ = os.Remove(list[i].path)
	}
}

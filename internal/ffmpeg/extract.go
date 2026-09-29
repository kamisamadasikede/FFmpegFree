package ffmpeg

import (
	"archive/tar"
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/ulikunitz/xz"
)

// maxExtractSize 是单个提取文件的大小上限，防止损坏或恶意压缩包撑爆磁盘。
// 静态构建的 ffmpeg 约 100~250 MB。
const maxExtractSize = 1 << 30

// extractItems 从压缩包里只取出 items 列出的文件，写到 destDir/<Name>。
// 不会按压缩包内路径展开，因此不存在路径穿越问题。
func extractItems(archivePath, typ string, items []ExtractItem, destDir string) error {
	switch typ {
	case "zip":
		return extractZip(archivePath, items, destDir)
	case "tar.xz":
		return extractTarXz(archivePath, items, destDir)
	}
	return fmt.Errorf("不支持的压缩包类型: %s", typ)
}

func cleanEntry(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	return strings.TrimPrefix(path.Clean(p), "./")
}

func extractZip(archivePath string, items []ExtractItem, destDir string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("打开 zip 失败: %w", err)
	}
	defer zr.Close()
	byPath := map[string]*zip.File{}
	for _, f := range zr.File {
		byPath[cleanEntry(f.Name)] = f
	}
	for _, it := range items {
		zf, ok := byPath[cleanEntry(it.Path)]
		if !ok || zf.FileInfo().IsDir() {
			return fmt.Errorf("压缩包中没有 %s", it.Path)
		}
		rc, err := zf.Open()
		if err != nil {
			return fmt.Errorf("读取 %s 失败: %w", it.Path, err)
		}
		err = writeExtracted(filepath.Join(destDir, it.Name), rc)
		rc.Close()
		if err != nil {
			return fmt.Errorf("提取 %s 失败: %w", it.Path, err)
		}
	}
	return nil
}

func extractTarXz(archivePath string, items []ExtractItem, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	xr, err := xz.NewReader(f)
	if err != nil {
		return fmt.Errorf("打开 xz 失败: %w", err)
	}
	want := map[string]string{}
	for _, it := range items {
		want[cleanEntry(it.Path)] = it.Name
	}
	tr := tar.NewReader(xr)
	for len(want) > 0 {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("读取 tar 失败: %w", err)
		}
		name, ok := want[cleanEntry(h.Name)]
		if !ok || h.Typeflag != tar.TypeReg {
			continue
		}
		if err := writeExtracted(filepath.Join(destDir, name), tr); err != nil {
			return fmt.Errorf("提取 %s 失败: %w", h.Name, err)
		}
		delete(want, cleanEntry(h.Name))
	}
	if len(want) > 0 {
		var miss []string
		for p := range want {
			miss = append(miss, p)
		}
		return fmt.Errorf("压缩包中没有: %s", strings.Join(miss, ", "))
	}
	return nil
}

func writeExtracted(dst string, r io.Reader) error {
	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, io.LimitReader(r, maxExtractSize+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if n > maxExtractSize {
		return fmt.Errorf("文件超过 %d 字节上限", int64(maxExtractSize))
	}
	return nil
}

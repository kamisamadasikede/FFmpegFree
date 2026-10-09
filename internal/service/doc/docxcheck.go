package doc

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"strings"

	"FFmpegFree/internal/apperr"
)

const (
	maxDocxUncompressed = 200 << 20 // 200 MiB
	maxDocxEntries      = 10000
)

// validateDocxForSave Commit 时的完整性检查（6.12.50）。失败：malformed 或 format（宏）。
func validateDocxForSave(path string) error {
	if err := checkZipEntries(path); err != nil {
		return apperr.New(apperr.InvalidArgument, "文件没能保存，原文件没有改动。请另存为再试。").WithDetail("reason=malformed")
	}
	// 中央目录条目与声明解压大小
	zr, err := zip.OpenReader(path)
	if err != nil {
		return apperr.New(apperr.InvalidArgument, "文件没能保存，原文件没有改动。请另存为再试。").WithDetail("reason=malformed")
	}
	defer zr.Close()
	if len(zr.File) > maxDocxEntries {
		return apperr.New(apperr.InvalidArgument, "文件没能保存，原文件没有改动。请另存为再试。").WithDetail("reason=malformed")
	}
	var declared uint64
	for _, f := range zr.File {
		declared += f.UncompressedSize64
		if declared > maxDocxUncompressed {
			return apperr.New(apperr.InvalidArgument, "文件没能保存，原文件没有改动。请另存为再试。").WithDetail("reason=malformed")
		}
	}
	var actual uint64
	need := map[string]bool{
		"[content_types].xml": false,
		"_rels/.rels":         false,
		"word/document.xml":   false,
	}
	var docXML *zip.File
	var ctXML *zip.File
	for _, f := range zr.File {
		name := strings.ToLower(strings.ReplaceAll(f.Name, "\\", "/"))
		if strings.HasSuffix(name, "vbaproject.bin") {
			return apperr.New(apperr.InvalidArgument, "文件没能保存，原文件没有改动。请另存为再试。").WithDetail("reason=format")
		}
		switch name {
		case "[content_types].xml":
			need[name] = true
			ctXML = f
		case "_rels/.rels":
			need[name] = true
		case "word/document.xml":
			need[name] = true
			docXML = f
		}
		rc, err := f.Open()
		if err != nil {
			return apperr.New(apperr.InvalidArgument, "文件没能保存，原文件没有改动。请另存为再试。").WithDetail("reason=malformed")
		}
		n, err := io.Copy(io.Discard, io.LimitReader(rc, maxDocxUncompressed+1))
		rc.Close()
		if err != nil {
			return apperr.New(apperr.InvalidArgument, "文件没能保存，原文件没有改动。请另存为再试。").WithDetail("reason=malformed")
		}
		actual += uint64(n)
		if actual > maxDocxUncompressed {
			return apperr.New(apperr.InvalidArgument, "文件没能保存，原文件没有改动。请另存为再试。").WithDetail("reason=malformed")
		}
	}
	for _, ok := range need {
		if !ok {
			return apperr.New(apperr.InvalidArgument, "文件没能保存，原文件没有改动。请另存为再试。").WithDetail("reason=malformed")
		}
	}
	if docXML != nil {
		if err := wellFormedXML(docXML); err != nil {
			return apperr.New(apperr.InvalidArgument, "文件没能保存，原文件没有改动。请另存为再试。").WithDetail("reason=malformed")
		}
	}
	if ctXML != nil {
		rc, err := ctXML.Open()
		if err == nil {
			b, _ := io.ReadAll(io.LimitReader(rc, 4<<20))
			rc.Close()
			if bytes.Contains(bytes.ToLower(b), []byte("macroenabled")) {
				return apperr.New(apperr.InvalidArgument, "文件没能保存，原文件没有改动。请另存为再试。").WithDetail("reason=format")
			}
		}
	}
	return nil
}

func wellFormedXML(f *zip.File) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	dec := xml.NewDecoder(io.LimitReader(rc, maxDocxUncompressed))
	dec.Strict = true
	for {
		if _, err := dec.Token(); err == io.EOF {
			return nil
		} else if err != nil {
			return err
		}
	}
}

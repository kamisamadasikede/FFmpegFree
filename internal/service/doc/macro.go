package doc

import (
	"archive/zip"
	"os"
	"strings"

	"github.com/richardlehane/mscfb"
)

// hasMacro 判断文件是否含宏（契约 6.12.26 第 2 条 / 6.12.48）。
func hasMacro(path, ext string) bool {
	switch ext {
	case "docm", "dotm", "xlsm", "xlsb", "pptm", "potm":
		return true
	}
	switch ext {
	case "docx", "xlsx", "pptx", "docm", "xlsm", "pptm":
		zr, err := zip.OpenReader(path)
		if err != nil {
			return false
		}
		defer zr.Close()
		for _, f := range zr.File {
			n := strings.ToLower(f.Name)
			if strings.Contains(n, "vbaproject.bin") {
				return true
			}
		}
	case "odt", "ods", "odp":
		zr, err := zip.OpenReader(path)
		if err != nil {
			return false
		}
		defer zr.Close()
		for _, f := range zr.File {
			if strings.HasPrefix(f.Name, "Basic/") || strings.HasPrefix(f.Name, "basic/") {
				return true
			}
		}
	case "doc", "xls", "ppt":
		f, err := os.Open(path)
		if err != nil {
			return false
		}
		defer f.Close()
		doc, err := mscfb.New(f)
		if err != nil {
			return false
		}
		for e, err := doc.Next(); err == nil; e, err = doc.Next() {
			n := strings.ToLower(e.Name)
			if n == "macros" || n == "_vba_project_cur" || n == "vba" || strings.Contains(n, "vba") {
				return true
			}
		}
	}
	return false
}

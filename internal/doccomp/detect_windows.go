//go:build windows

package doccomp

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

// windowsCandidates：注册表 HKLM\SOFTWARE\LibreOffice\UNO\InstallPath（再看 WOW6432Node），再看 %ProgramFiles%。
func windowsCandidates() []string {
	var out []string
	for _, k := range []string{`SOFTWARE\LibreOffice\UNO\InstallPath`, `SOFTWARE\WOW6432Node\LibreOffice\UNO\InstallPath`} {
		key, err := registry.OpenKey(registry.LOCAL_MACHINE, k, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		v, _, err := key.GetStringValue("")
		key.Close()
		if err == nil && v != "" {
			out = append(out, filepath.Join(v, "soffice.com")) // InstallPath 指向 program 目录
		}
	}
	for _, env := range []string{"ProgramFiles", "ProgramW6432", "ProgramFiles(x86)"} {
		if pf := os.Getenv(env); pf != "" {
			out = append(out, filepath.Join(pf, "LibreOffice", "program", "soffice.com"))
		}
	}
	return out
}

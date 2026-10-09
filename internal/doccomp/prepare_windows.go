//go:build windows

package doccomp

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf16"

	"FFmpegFree/internal/proc"
)

// preparePackage：msiexec /a 管理员解包到 staging（不需要管理员权限、不弹 UAC、不登记已安装程序，契约 6.12.12）。
func preparePackage(ctx context.Context, pkg, staging, tmp string) error {
	logPath := filepath.Join(tmp, "doc-msi.log")
	os.Remove(logPath)
	sysdir := os.Getenv("SystemRoot")
	exe := "msiexec.exe"
	if sysdir != "" {
		exe = filepath.Join(sysdir, "System32", "msiexec.exe")
	}
	cmd := exec.Command(exe)
	proc.Configure(cmd)
	// msiexec 要求 PROPERTY="value" 这种写法，Go 默认的参数转义会把整个 TARGETDIR=... 包进引号，所以手写命令行。
	cmd.SysProcAttr.CmdLine = `"` + exe + `" /a "` + pkg + `" /qn TARGETDIR="` + staging + `" /L*v "` + logPath + `"`
	code, err := runTree(ctx, cmd)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil && code != 1641 && code != 3010 {
		return installFailed(codeDetail("msiexec", code, logTail(logPath, staging, pkg)))
	}
	return nil
}

// logTail 读 msiexec 日志（通常是 UTF-16LE）最后 15 行，去掉路径。
func logTail(p string, hide ...string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	s := string(b)
	if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE {
		u := make([]uint16, 0, len(b)/2)
		for i := 2; i+1 < len(b); i += 2 {
			u = append(u, uint16(b[i])|uint16(b[i+1])<<8)
		}
		s = string(utf16.Decode(u))
	}
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	if len(lines) > 15 {
		lines = lines[len(lines)-15:]
	}
	return sanitize(strings.Join(lines, "\n"), Job{Input: hide[0], WorkDir: hide[1]})
}

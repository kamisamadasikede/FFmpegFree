package doccomp

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/proc"
)

const (
	versionTimeout = 60 * time.Second  // 第一次运行慢
	smokeTimeout   = 120 * time.Second // 1 行 txt → pdf
)

// exeName 是各平台用的可执行文件名（Windows 用控制台版 soffice.com，等转换结束并有退出码）。
func exeName() string {
	if runtime.GOOS == "windows" {
		return "soffice.com"
	}
	return "soffice"
}

// findInTree 在 root 下最多 depth 层里找 program/<exe>（Windows 管理员解包后的层级没在真机验证，契约 6.12.12）。
func findInTree(root string, depth int) string {
	var walk func(dir string, d int) string
	walk = func(dir string, d int) string {
		cand := filepath.Join(dir, "program", exeName())
		if regular(cand) {
			return cand
		}
		if runtime.GOOS == "darwin" {
			if c := filepath.Join(dir, "LibreOffice.app", "Contents", "MacOS", "soffice"); regular(c) {
				return c
			}
		}
		if d >= depth {
			return ""
		}
		ents, err := os.ReadDir(dir)
		if err != nil {
			return ""
		}
		for _, e := range ents {
			if e.IsDir() {
				if p := walk(filepath.Join(dir, e.Name()), d+1); p != "" {
					return p
				}
			}
		}
		return ""
	}
	return walk(root, 0)
}

func regular(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}

// linuxCandidates 是 Linux 上的系统安装位置（snap / flatpak 不支持）。
func linuxCandidates(lookPath func(string) (string, error)) []string {
	var out []string
	for _, n := range []string{"soffice", "libreoffice"} {
		if p, err := lookPath(n); err == nil {
			out = append(out, p)
		}
	}
	out = append(out, "/usr/lib/libreoffice/program/soffice", "/usr/lib64/libreoffice/program/soffice")
	if m, _ := filepath.Glob("/opt/libreoffice*/program/soffice"); len(m) > 0 {
		out = append(out, m...)
	}
	return out
}

func darwinCandidates() []string {
	out := []string{"/Applications/LibreOffice.app/Contents/MacOS/soffice"}
	if home, err := os.UserHomeDir(); err == nil {
		out = append(out, filepath.Join(home, "Applications", "LibreOffice.app", "Contents", "MacOS", "soffice"))
	}
	return out
}

// systemCandidates 按契约 6.12.12 的顺序列出系统安装的位置（去重，不检查是否存在）。
func systemCandidates() []string {
	var list []string
	switch runtime.GOOS {
	case "windows":
		list = windowsCandidates()
	case "darwin":
		list = darwinCandidates()
	default:
		list = linuxCandidates(exec.LookPath)
	}
	seen := map[string]bool{}
	var out []string
	for _, p := range list {
		k := p
		if r, err := filepath.EvalSymlinks(p); err == nil {
			k = r
		}
		if p == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, p)
	}
	return out
}

// checkResult 是一次校验的结果。
type checkResult struct {
	version  string
	outdated bool   // 版本读出来了但 < 7.2
	reason   string // 失败原因（给 detail 的 check=）：version | outdated | smoke
}

// Validate 校验一个组件：--version（60 秒）≥ 7.2，再做一次 1 行 txt → pdf 冒烟转换（120 秒），产出非空 PDF 才通过。
// tmp 是放临时配置目录的地方，结束时删除。
func Validate(ctx context.Context, exe, tmp string) (version string, err error) {
	r := validate(ctx, exe, tmp)
	if r.reason == "" {
		return r.version, nil
	}
	return r.version, apperr.New(apperr.DocComponentInstallFailed, "文档组件准备失败，请重试。").WithDetail("check=" + r.reason)
}

func validate(ctx context.Context, exe, tmp string) checkResult {
	if !regular(exe) {
		return checkResult{reason: "missing"}
	}
	dir, err := os.MkdirTemp(tmp, "check-")
	if err != nil {
		return checkResult{reason: "tmp"}
	}
	defer os.RemoveAll(dir)
	out, err := runVersion(ctx, exe, filepath.Join(dir, "vprofile"))
	full, major, minor, ok := ParseVersion(out)
	if !ok {
		_ = err
		return checkResult{reason: "version"}
	}
	if !VersionOK(major, minor) {
		return checkResult{version: full, outdated: true, reason: "outdated"}
	}
	in := filepath.Join(dir, "check.txt")
	if err := os.WriteFile(in, []byte("FFmpegFree 文档组件检测\n"), 0o644); err != nil {
		return checkResult{version: full, reason: "tmp"}
	}
	if _, err := Convert(ctx, Job{Exe: exe, Input: in, ConvertTo: "pdf:writer_pdf_Export", InFilter: "Text (encoded):UTF8",
		WorkDir: filepath.Join(dir, "smoke"), OutExt: "pdf", Timeout: smokeTimeout}); err != nil {
		return checkResult{version: full, reason: "smoke"}
	}
	return checkResult{version: full}
}

func runVersion(ctx context.Context, exe, profile string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, versionTimeout)
	defer cancel()
	if err := SeedProfile(profile); err != nil {
		return "", err
	}
	cmd := exec.Command(exe, "--headless", "-env:UserInstallation="+FileURL(profile), "--version")
	var out tailBuffer
	cmd.Stdout, cmd.Stderr = &out, &out
	proc.Configure(cmd)
	if err := proc.Start(cmd); err != nil {
		return "", err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return strings.TrimSpace(out.String()), err
	case <-ctx.Done():
		_ = proc.Kill(cmd)
		<-done
		return out.String(), ctx.Err()
	}
}

package about

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"FFmpegFree/internal/apperr"
)

func TestLicenseTextOFL(t *testing.T) {
	s, err := LicenseText("OFL")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, "SIL OPEN FONT LICENSE Version 1.1") {
		t.Fatal("内容不含 OFL 1.1 标题")
	}
	if n := len(s); n < 3000 || n > 20000 {
		t.Fatalf("长度不合理: %d", n)
	}
	if !strings.HasPrefix(s, "Copyright ") {
		t.Fatalf("应保留版权行: %.40q", s)
	}
}

func TestLicenseTextRejectsUnknown(t *testing.T) {
	long := strings.Repeat("长x", 500)
	for _, name := range []string{"", "OFL-nunito", "ofl-nunito", "OFL-Nunito.txt", "OFL-Nunito\n", "../OFL-Nunito", "Nunito", "ofl", "Ofl", "OFL.txt", "../OFL", "./OFL", `..\OFL`, "OFL\n", " OFL", "MIT", long} {
		s, err := LicenseText(name)
		if !apperr.Is(err, apperr.InvalidArgument) || s != "" {
			t.Fatalf("%q 应为 INVALID_ARGUMENT: %q %v", name, s, err)
		}
		var ae *apperr.AppError
		if !errors.As(err, &ae) {
			t.Fatal("应是 AppError")
		}
		if strings.Contains(ae.Detail, "\n") || len([]rune(ae.Detail)) > 100 {
			t.Fatalf("detail 应为单行且不超长: %q", ae.Detail)
		}
	}
}

func TestAppVersionDefault(t *testing.T) {
	old := Version
	defer func() { Version = old }()
	Version = ""
	if got := AppVersion(); got != "开发版" {
		t.Fatalf("%q", got)
	}
	Version = "1.2.3"
	if got := AppVersion(); got != "1.2.3" {
		t.Fatalf("%q", got)
	}
}

// 真正走一遍 go build -ldflags -X，确认变量路径写对了、注入值生效。
func TestVersionInjectedByLdflags(t *testing.T) {
	if testing.Short() {
		t.Skip("short 模式跳过编译")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("找不到 go")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	prog := `package main

import (
	"fmt"

	"FFmpegFree/internal/about"
)

func main() { fmt.Print(about.AppVersion()) }
`
	// 程序必须位于本模块内才能 import internal 包。真文件写在 t.TempDir()，用 go build -overlay
	// 把它映射到模块根下一个不存在的目录：仓库里不落任何文件（以前在仓库根建 ldflags_probe_*，
	// 并发跑全量测试时会让 apperr 扫描源码的测试遍历到一半目录消失而失败）。
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	virt := filepath.Join(root, "ldflags_probe_overlay", "main.go")
	ov, err := json.Marshal(map[string]map[string]string{"Replace": {virt: src}})
	if err != nil {
		t.Fatal(err)
	}
	overlay := filepath.Join(dir, "overlay.json")
	if err := os.WriteFile(overlay, ov, 0o644); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "probe")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	run := func(ldflags string) string {
		args := []string{"build", "-overlay", overlay, "-o", exe}
		if ldflags != "" {
			args = append(args, "-ldflags", ldflags)
		}
		args = append(args, virt)
		cmd := exec.Command(goBin, args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("go build 失败: %v\n%s", err, out)
		}
		out, err := exec.Command(exe).Output()
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}
	if got := run(""); got != "开发版" {
		t.Fatalf("未注入应为 开发版: %q", got)
	}
	if got := run("-X FFmpegFree/internal/about.Version=v9.8.7"); got != "v9.8.7" {
		t.Fatalf("注入值未生效: %q", got)
	}
}

func TestLicenseTextNunito(t *testing.T) {
	n, err := LicenseText("OFL-Nunito")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(n, "SIL OPEN FONT LICENSE Version 1.1") {
		t.Fatal("内容不含 OFL 1.1 标题")
	}
	if !strings.HasPrefix(n, "Copyright 2016 The Nunito Project Authors") {
		t.Fatalf("应保留 Nunito 版权行: %.60q", n)
	}
	if l := len(n); l < 3000 || l > 20000 {
		t.Fatalf("长度不合理: %d", l)
	}
	noto, err := LicenseText("OFL")
	if err != nil {
		t.Fatal(err)
	}
	if n == noto {
		t.Fatal("OFL 与 OFL-Nunito 应是两份不同的文本")
	}
	if !strings.Contains(noto, "Adobe") || strings.Contains(noto, "Nunito") {
		t.Fatalf("OFL 应仍是 Noto Sans SC 那份: %.60q", noto)
	}
	if strings.Contains(n, "Adobe") {
		t.Fatal("OFL-Nunito 不应含 Noto 的版权行")
	}
}

// 前端那份 Nunito 许可变了，这里会失败提示把 internal/about/OFL-Nunito.txt 同步过来。
func TestNunitoLicenseMatchesFrontend(t *testing.T) {
	want, err := os.ReadFile(filepath.Join("..", "..", "frontend", "src", "assets", "fonts", "OFL.txt"))
	if err != nil {
		t.Skipf("找不到前端的 OFL.txt（打包环境？）: %v", err)
	}
	got, err := LicenseText("OFL-Nunito")
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256([]byte(got)) != sha256.Sum256(want) {
		t.Fatalf("internal/about/OFL-Nunito.txt 与 frontend/src/assets/fonts/OFL.txt 内容不一致，请同步（原样拷贝，不改一个字节）")
	}
}

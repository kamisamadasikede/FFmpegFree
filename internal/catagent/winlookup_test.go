package catagent

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func fakeEnv(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestExpandWinEnvAndRegistryMerge(t *testing.T) {
	env := fakeEnv(map[string]string{"SystemRoot": `C:\Windows`, "USERPROFILE": `C:\Users\张 三`})
	if got := expandWinEnv(`%SystemRoot%\system32;%NOPE%\x`, env); got != `C:\Windows\system32;%NOPE%\x` {
		t.Fatalf("expand: %q", got)
	}
	dirs := registryPathDirs(winLookupEnv{
		RegistryPath: func() (string, string) {
			return `%SystemRoot%\system32;;C:\Shared`, `"%USERPROFILE%\bin";c:\shared\;C:\UserOnly`
		},
		Getenv: env,
	})
	want := []string{`C:\Windows\system32`, `C:\Shared`, `C:\Users\张 三\bin`, `C:\UserOnly`}
	if strings.Join(dirs, "|") != strings.Join(want, "|") {
		t.Fatalf("机器在前、用户在后、去重：%q", dirs)
	}
}

func TestWinFallbackFindOrder(t *testing.T) {
	root := t.TempDir()
	mk := func(rel string) string {
		p := filepath.Join(root, rel)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte("x"), 0o644)
		return p
	}
	regDir := filepath.Join(root, "reg")
	profile := filepath.Join(root, "home")
	appdata := filepath.Join(root, "appdata")
	local := filepath.Join(root, "local")
	envm := map[string]string{"USERPROFILE": profile, "APPDATA": appdata, "LOCALAPPDATA": local}
	env := winLookupEnv{RegistryPath: func() (string, string) { return "", regDir }, Getenv: fakeEnv(envm)}

	cases := []struct {
		name  string
		files []string
		kind  string
		base  string
		want  string
	}{
		{"都没有", nil, KindClaudeCLI, "claude", ""},
		{"已知目录 npm .cmd", []string{"appdata/npm/claude.cmd", "appdata/npm/claude.ps1"}, KindClaudeCLI, "claude", "appdata/npm/claude.cmd"},
		{"同目录 .exe 优先", []string{"appdata/npm/claude.cmd", "appdata/npm/claude.exe"}, KindClaudeCLI, "claude", "appdata/npm/claude.exe"},
		{"表顺序：.local\\bin 在 npm 前", []string{"appdata/npm/claude.exe", "home/.local/bin/claude.exe"}, KindClaudeCLI, "claude", "home/.local/bin/claude.exe"},
		{"注册表 PATH 先于已知目录", []string{"home/.local/bin/claude.exe", "reg/claude.cmd"}, KindClaudeCLI, "claude", "reg/claude.cmd"},
		{"WinGet Links", []string{"local/Microsoft/WinGet/Links/claude.exe"}, KindClaudeCLI, "claude", "local/Microsoft/WinGet/Links/claude.exe"},
		{"Codex 安装脚本目录", []string{"local/Programs/OpenAI/Codex/bin/codex.exe", "appdata/npm/codex.cmd"}, KindCodexCLI, "codex", "local/Programs/OpenAI/Codex/bin/codex.exe"},
		{"Grok GUESS 目录", []string{"local/Programs/grok/grok.ps1"}, KindCatBuild, "grok", "local/Programs/grok/grok.ps1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_ = os.RemoveAll(root)
			for _, f := range c.files {
				mk(f)
			}
			got := winFallbackFind(c.kind, c.base, env)
			want := ""
			if c.want != "" {
				want = filepath.Join(root, c.want)
			}
			if got != want {
				t.Fatalf("got %q want %q", got, want)
			}
		})
	}
	// CODEX_INSTALL_DIR 设了就查它（不再查默认目录）
	_ = os.RemoveAll(root)
	mk("local/Programs/OpenAI/Codex/bin/codex.exe")
	custom := mk("custom/codex.exe")
	envm["CODEX_INSTALL_DIR"] = filepath.Dir(custom)
	if got := winFallbackFind(KindCodexCLI, "codex", env); got != custom {
		t.Fatalf("CODEX_INSTALL_DIR: %q", got)
	}
}

func TestCmdShimCommandLine(t *testing.T) {
	line := cmdShimCommandLine(`C:\Windows\System32\cmd.exe`, `C:\Users\A B&C\AppData\Roaming\npm\claude.cmd`,
		[]string{"-p", "--cwd", `C:\my dir\50% & "x"`, "a|b<c>d^e"})
	if !strings.HasPrefix(line, `"C:\Windows\System32\cmd.exe" /d /s /c ""C:\Users\A B&C\AppData\Roaming\npm\claude.cmd" `) || !strings.HasSuffix(line, `"`) {
		t.Fatalf("外层结构: %s", line)
	}
	args := strings.TrimSuffix(strings.SplitN(line, `claude.cmd" `, 2)[1], `"`)
	// 参数里的元字符一律被 ^ 转义（转两遍），不会出现裸 & | < > % "
	for _, ch := range []string{"&", "|", "<", ">", "%", `"`} {
		for i := 0; i < len(args); i++ {
			if string(args[i]) == ch && (i == 0 || args[i-1] != '^') {
				t.Fatalf("未转义的 %s: %s", ch, args)
			}
		}
	}
	if !strings.Contains(args, "^^^%") || !strings.Contains(args, "^^^&") {
		t.Fatalf("应双重转义: %s", args)
	}
	if quoteWinArg(`a\b"c\`) != `"a\b\"c\\"` {
		t.Fatalf("msvcrt quote: %s", quoteWinArg(`a\b"c\`))
	}
	if got := ps1ShimArgs(`C:\x\grok.ps1`, []string{"-v"}); strings.Join(got, " ") != `-NoProfile -ExecutionPolicy Bypass -File C:\x\grok.ps1 -v` {
		t.Fatalf("ps1: %v", got)
	}
}

func TestShimStdinArgsNeverPutsPromptOnCommandLine(t *testing.T) {
	a := NewClaudeAdapter(CLIAgentConfig{DataTemp: t.TempDir()})
	prompt := `hi & del /q "%USERPROFILE%" | x`
	args := a.buildClaudeArgs(TurnOptions{Rules: "规则 & %PATH%"}, prompt, "", false)
	out, stdin, clean, err := shimStdinArgs(cliKindClaude, args, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer clean()
	if stdin != prompt || out[0] != "-p" {
		t.Fatalf("claude stdin: %q %v", stdin, out)
	}
	for i, x := range out {
		if strings.Contains(x, "hi &") || strings.Contains(x, "规则") {
			t.Fatalf("用户输入留在命令行: %v", out)
		}
		if x == "--append-system-prompt-file" {
			b, _ := os.ReadFile(out[i+1])
			if string(b) != "规则 & %PATH%" {
				t.Fatalf("rules 文件: %q", b)
			}
		}
	}
	c := NewCodexAdapter(CLIAgentConfig{})
	for _, resume := range []bool{false, true} {
		cargs := c.buildCodexArgs(TurnOptions{Rules: "R"}, prompt, "/w", "sid", resume)
		out, stdin, _, _ := shimStdinArgs(cliKindCodex, cargs, "")
		if out[len(out)-1] != "-" || stdin != "R\n\n"+prompt || strings.Contains(strings.Join(out, " "), "hi &") {
			t.Fatalf("codex resume=%v: %v %q", resume, out, stdin)
		}
	}
	if !isShimExt(`C:\a\claude.CMD`) || !isShimExt("x.ps1") || isShimExt("claude.exe") {
		t.Fatal("isShimExt")
	}
}

func TestAdapterUsesWinFallbackAndOnChange(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "claude.cmd")
	_ = os.WriteFile(exe, []byte("x"), 0o644)
	oldE, oldF := winFallbackEnabled, winFallback
	t.Cleanup(func() { winFallbackEnabled, winFallback = oldE, oldF })
	winFallbackEnabled = func() bool { return true }
	var asked atomic.Value
	winFallback = func(kind, base string) string { asked.Store(kind + "/" + base); return exe }

	var changes atomic.Int32
	a := NewClaudeAdapter(CLIAgentConfig{
		DataTemp: dir,
		LookPath: func(string) (string, error) { return "", os.ErrNotExist },
		OnChange: func(k string) {
			if k == KindClaudeCLI {
				changes.Add(1)
			}
		},
		Now: func() time.Time { return time.UnixMilli(1234) },
	})
	if a.CheckedAt() != 0 {
		t.Fatal("未检测前 CheckedAt 应为 0")
	}
	if got := a.resolveExe(); got != exe || asked.Load() != KindClaudeCLI+"/claude" {
		t.Fatalf("fallback: %q %v", got, asked.Load())
	}
	winFallback = func(string, string) string { return "" }
	st := a.Recheck()
	if st.State != StateMissing || a.CheckedAt() != 1234 || changes.Load() < 2 {
		t.Fatalf("state=%s at=%d changes=%d", st.State, a.CheckedAt(), changes.Load())
	}
}

func TestDetectTimeoutFlagsThenLateResultApplies(t *testing.T) {
	old := DetectTimeout
	DetectTimeout = 50 * time.Millisecond
	t.Cleanup(func() { DetectTimeout = old })
	release := make(chan struct{})
	var changes atomic.Int32
	a := NewCodexAdapter(CLIAgentConfig{
		LookPath: func(string) (string, error) { <-release; return "", os.ErrNotExist },
		OnChange: func(string) { changes.Add(1) },
	})
	start := time.Now()
	st := a.Recheck()
	if time.Since(start) > 2*time.Second || st.State != StateChecking || !a.DetectTimedOut() || a.CheckedAt() != 0 {
		t.Fatalf("超时：state=%s timedOut=%v", st.State, a.DetectTimedOut())
	}
	n := changes.Load()
	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for a.Status().State == StateChecking && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if a.Status().State != StateMissing || a.DetectTimedOut() || a.CheckedAt() == 0 || changes.Load() <= n {
		t.Fatalf("迟到结果应生效: %s timedOut=%v", a.Status().State, a.DetectTimedOut())
	}
}

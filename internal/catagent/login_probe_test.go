package catagent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func readLoginSample(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "login", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// 真机未登录样本（codex 0.162.1，2026-10-14 本机实跑 exit 1）。
func TestParseCodexLoginStatusSample(t *testing.T) {
	out := readLoginSample(t, "codex-status-loggedout.stdout") + readLoginSample(t, "codex-status-loggedout.stderr")
	if v := parseCodexLoginStatus(1, out); v == nil || *v {
		t.Fatalf("未登录样本应为 false: %v", v)
	}
	// 官方：「exits with 0 when credentials are present」
	if v := parseCodexLoginStatus(0, "Logged in using ChatGPT\n"); v == nil || !*v {
		t.Fatalf("exit 0 应为 true: %v", v)
	}
	for _, c := range []struct {
		code int
		out  string
	}{{1, "error: something else"}, {2, "Not logged in"}, {-1, ""}} {
		if v := parseCodexLoginStatus(c.code, c.out); v != nil {
			t.Fatalf("%+v 应判断不了: %v", c, *v)
		}
	}
}

// 真机未登录样本（claude 2.1.296 `auth status --json`，exit 1，stdout JSON）。
func TestParseClaudeAuthStatusSample(t *testing.T) {
	if v := parseClaudeAuthStatus([]byte(readLoginSample(t, "claude-auth-status-loggedout.json"))); v == nil || *v {
		t.Fatalf("未登录样本应为 false: %v", v)
	}
	if v := parseClaudeAuthStatus([]byte(`{"loggedIn": true, "authMethod": "claude.ai"}`)); v == nil || !*v {
		t.Fatalf("应为 true: %v", v)
	}
	for _, s := range []string{"", "Not logged in.", `{"authMethod":"none"}`, `{"loggedIn":"yes"}`} {
		if v := parseClaudeAuthStatus([]byte(s)); v != nil {
			t.Fatalf("%q 应判断不了", s)
		}
	}
}

// 登录查询的子命令 / 开关必须在真机 help 快照里。
func TestLoginProbeArgsWhitelisted(t *testing.T) {
	codexLogin, _ := os.ReadFile(filepath.Join("testdata", "help", "codex-login.txt"))
	if !strings.Contains(string(codexLogin), "\n  status ") {
		t.Fatal("codex login --help 快照里没有 status 子命令")
	}
	codexTop, _ := os.ReadFile(filepath.Join("testdata", "help", "codex.txt"))
	if !strings.Contains(string(codexTop), "\n  login ") {
		t.Fatal("codex --help 快照里没有 login 子命令")
	}
	claudeAuth, _ := os.ReadFile(filepath.Join("testdata", "help", "claude-auth.txt"))
	if !strings.Contains(string(claudeAuth), "status [options]") {
		t.Fatal("claude auth --help 快照里没有 status 子命令")
	}
	claudeTop, _ := os.ReadFile(filepath.Join("testdata", "help", "claude.txt"))
	if !strings.Contains(string(claudeTop), "\n  auth") {
		t.Fatal("claude --help 快照里没有 auth 子命令")
	}
	assertWhitelisted(t, "claude", "claude-auth-status.txt", []string{"auth", "status", "--json"})
	assertWhitelisted(t, "codex", "codex-login-status.txt", []string{"login", "status"})
}

func fakeLoginAdapter(t *testing.T, kind cliKind, script string) *CLIAgentAdapter {
	t.Helper()
	dir := t.TempDir()
	fake := filepath.Join(dir, "cli-fake")
	_ = os.WriteFile(fake, []byte("#!/bin/sh\n"+script), 0o755)
	name := KindCodexCLI
	if kind == cliKindClaude {
		name = KindClaudeCLI
	}
	cfg := CLIAgentConfig{
		DataTemp: t.TempDir(),
		LookPath: func(string) (string, error) { return fake, nil },
		Exec: func(ctx context.Context, n string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, n, args...)
		},
	}
	return newCLIAgent(cfg, kind, name)
}

func TestProbeLoginEndToEnd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix script fake")
	}
	t.Setenv("CODEX_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	codexOut := `case "$1 $2" in "login status") echo 'Not logged in' >&2; exit 1;; esac; echo 'codex-cli 0.162.1'`
	a := fakeLoginAdapter(t, cliKindCodex, codexOut+"\n")
	a.detect()
	if v := a.LiveLoggedIn(); v == nil || *v {
		t.Fatalf("codex 未登录应为 false: %v", v)
	}
	// 环境变量 Key：login status 不反映 → nil
	t.Setenv("CODEX_API_KEY", "sk-x")
	a.detect()
	if v := a.LiveLoggedIn(); v != nil {
		t.Fatalf("环境 Key 时应判断不了: %v", *v)
	}

	c := fakeLoginAdapter(t, cliKindClaude, `if [ "$1 $2 $3" = "auth status --json" ]; then echo '{"loggedIn": true}'; exit 0; fi; echo '2.1.296 (Claude Code)'`+"\n")
	c.detect()
	if v := c.LiveLoggedIn(); v == nil || !*v {
		t.Fatalf("claude 已登录应为 true: %v", v)
	}
	// 自定义服务商：不判断
	c.SetProviderOverlay(ProviderOverlay{Enabled: true, AgentKind: KindClaudeCLI})
	c.detect()
	if v := c.LiveLoggedIn(); v != nil {
		t.Fatalf("自定义服务商时应判断不了: %v", *v)
	}
	// 命令失败（非 0/1 退出）→ nil
	f := fakeLoginAdapter(t, cliKindClaude, `[ "$1" = "auth" ] && exit 3; echo 2.1.296`+"\n")
	f.detect()
	if v := f.LiveLoggedIn(); v != nil {
		t.Fatalf("失败应为 nil: %v", *v)
	}
}

package catagent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"FFmpegFree/internal/apperr"
)

// v0.47.3（6.19.21）：ready = 已安装 + 版本可读；检测不调用任何登录 / 凭据探测。
func TestCLIDetect_ReadyWithoutLoginProbe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix script fake")
	}
	for _, mk := range []struct {
		name string
		new  func(CLIAgentConfig) *CLIAgentAdapter
	}{{"codex", NewCodexAdapter}, {"claude", NewClaudeAdapter}} {
		t.Run(mk.name, func(t *testing.T) {
			dir := t.TempDir()
			fake := filepath.Join(dir, mk.name)
			argsLog := filepath.Join(dir, "args")
			// 登出样本：login / auth 子命令会报未登录并 exit 1——检测不得调用它们，也不得受其影响。
			script := `#!/bin/sh
echo "$*" >> ` + argsLog + `
case "$1" in
  --version) echo 9.9.9; exit 0;;
  login|auth) echo "Not logged in" >&2; echo '{"loggedIn": false}'; exit 1;;
esac
exit 0
`
			if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			a := mk.new(CLIAgentConfig{
				DataTemp: t.TempDir(), DataRoot: t.TempDir(),
				LookPath: func(string) (string, error) { return fake, nil },
				Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
					return exec.CommandContext(ctx, fake, args...)
				},
			})
			a.detect()
			if st := a.Status(); st.State != StateReady || st.Error != nil {
				t.Fatalf("status %+v", st)
			}
			b, _ := os.ReadFile(argsLog)
			for _, line := range strings.Split(string(b), "\n") {
				f := strings.Fields(line)
				if len(f) > 0 && (f[0] == "login" || f[0] == "auth") {
					t.Fatalf("login probe invoked: %q", line)
				}
			}
		})
	}
}

// 真实回复因未登录失败 → 普通 CAT_REPLY_FAILED；适配器仍 ready，下一句照常发出。
func TestCodexRunTurn_NotLoggedInIsOrdinaryFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix script fake")
	}
	dir := t.TempDir()
	fake := filepath.Join(dir, "codex")
	count := filepath.Join(dir, "count")
	script := `#!/bin/sh
[ "$1" = "--version" ] && { echo codex-cli 0.162.1; exit 0; }
[ "$1" = "exec" ] || exit 0
echo x >> ` + count + `
printf '%s\n' '{"type":"error","message":"unexpected status 401 Unauthorized: Not logged in"}'
exit 1
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewCodexAdapter(CLIAgentConfig{
		DataTemp: t.TempDir(), DataRoot: t.TempDir(),
		LookPath: func(string) (string, error) { return fake, nil },
		Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, fake, args...)
		},
	})
	a.detect()
	for i := 0; i < 2; i++ {
		_, err := a.RunTurn(TurnOptions{
			Ctx: context.Background(), ConversationID: "c1",
			Messages: []WireMessage{{Role: "user", Content: "hi"}},
		})
		if err == nil {
			t.Fatalf("turn %d: want error", i)
		}
		if !apperr.Is(err, apperr.CatReplyFailed) {
			t.Fatalf("turn %d: want CAT_REPLY_FAILED, got %v", i, err)
		}
		if ae := apperr.From(err); ae.Code == apperr.CatNotReady || strings.Contains(ae.Detail, "reason=auth") {
			t.Fatalf("turn %d: login gated: %v (%s)", i, err, ae.Detail)
		}
		if st := a.Status(); st.State != StateReady {
			t.Fatalf("turn %d: status %+v", i, st)
		}
	}
	b, _ := os.ReadFile(count)
	if strings.Count(string(b), "x") != 2 {
		t.Fatalf("send blocked: runs=%q", b)
	}
}

// CAT_NOT_READY 只在未安装 / 版本不可读时出现；登录问题（Claude 未登录）绝不产生 CAT_NOT_READY。
func TestClaudeRunTurn_LoginErrorNeverCatNotReady(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix script fake")
	}
	fake := filepath.Join(t.TempDir(), "claude")
	script := `#!/bin/sh
[ "$1" = "--version" ] && { echo "2.1.296 (Claude Code)"; exit 0; }
echo "Invalid API key · Please run /login" >&2
printf '%s\n' '{"type":"result","subtype":"error","is_error":true,"result":"Not logged in · Please run /login"}'
exit 1
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewClaudeAdapter(CLIAgentConfig{
		DataTemp: t.TempDir(), DataRoot: t.TempDir(),
		LookPath: func(string) (string, error) { return fake, nil },
		Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, fake, args...)
		},
	})
	a.detect()
	if st := a.Status(); st.State != StateReady || st.Error != nil {
		t.Fatalf("status %+v", st)
	}
	_, err := a.RunTurn(TurnOptions{
		Ctx: context.Background(), ConversationID: "c1",
		Messages: []WireMessage{{Role: "user", Content: "hi"}},
	})
	if apperr.Is(err, apperr.CatNotReady) || !apperr.Is(err, apperr.CatReplyFailed) {
		t.Fatalf("want CAT_REPLY_FAILED, got %v", err)
	}
	if st := a.Status(); st.State != StateReady || st.Error != nil {
		t.Fatalf("status after login failure %+v", st)
	}
}

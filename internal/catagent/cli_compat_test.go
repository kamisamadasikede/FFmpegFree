package catagent

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"FFmpegFree/internal/apperr"
)

func TestCLIVersionBelow(t *testing.T) {
	cases := []struct {
		ver, min string
		want     bool
	}{
		{"2.1.199", "2.1.200", true},
		{"2.1.200", "2.1.200", false},
		{"2.1.296 (Claude Code)", "2.1.200", false},
		{"1.0.108", "1.0.109", true},
		{"codex-cli 0.162.1", "0.1.0", false},
		{"path", "2.1.200", false},
		{"", "2.1.200", false},
	}
	for _, c := range cases {
		if got := cliVersionBelow(c.ver, c.min); got != c.want {
			t.Errorf("cliVersionBelow(%q,%q)=%v want %v", c.ver, c.min, got, c.want)
		}
	}
}

func TestClaudePermissionArgs_VersionBranch(t *testing.T) {
	cases := []struct{ ver, want string }{
		{"2.1.199", "default"},
		{"1.0.120", "default"},
		{"2.1.200", "manual"},
		{"2.1.296", "manual"},
		{"path", "manual"},
		{"", "manual"},
	}
	for _, c := range cases {
		got := claudePermissionArgs("normal", c.ver)
		if len(got) != 2 || got[1] != c.want {
			t.Errorf("ver %q: got %q want %s", c.ver, got, c.want)
		}
		// 其它模式不受版本影响
		if p := claudePermissionArgs("plan", c.ver); p[1] != "plan" {
			t.Errorf("plan changed: %q", p)
		}
		if p := claudePermissionArgs("", c.ver); p[1] != "bypassPermissions" {
			t.Errorf("approve-all changed: %q", p)
		}
	}
}

func TestCachedCLIVersion_PerPathMtime(t *testing.T) {
	f := filepath.Join(t.TempDir(), "bin")
	_ = os.WriteFile(f, []byte("a"), 0o755)
	n := 0
	probe := func(string) string { n++; return "1.2.3" }
	cachedCLIVersion(f, probe)
	cachedCLIVersion(f, probe)
	if n != 1 {
		t.Fatalf("probe ran %d times, want 1", n)
	}
	_ = os.WriteFile(f, []byte("bb"), 0o755) // 换了二进制（大小变了）
	cachedCLIVersion(f, probe)
	if n != 2 {
		t.Fatalf("probe should rerun after binary changed, ran %d", n)
	}
}

func TestCLIArgRejected(t *testing.T) {
	for _, s := range []string{
		"error: unexpected argument '--ask-for-approval' found",
		"error: unknown option '--include-partial-messages'",
		"error: invalid value 'manual' for '--permission-mode <MODE>'",
		"error: option '--permission-mode <mode>' argument 'manual' is invalid. Allowed choices are default, plan.",
	} {
		if !cliArgRejected(s) {
			t.Errorf("not detected: %s", s)
		}
	}
	if cliArgRejected("Error: network timeout") {
		t.Error("false positive")
	}
}

func assertOutdated(t *testing.T, err error, kind, label, ver, rawSub string) {
	t.Helper()
	var ae *apperr.AppError
	if !errors.As(err, &ae) {
		t.Fatalf("want AppError, got %v", err)
	}
	if ae.Code != apperr.CatReplyFailed {
		t.Fatalf("code %s", ae.Code)
	}
	if want := label + " CLI 版本过旧，请升级后重试。"; ae.Message != want {
		t.Fatalf("message %q want %q", ae.Message, want)
	}
	lines := strings.Split(ae.Detail, "\n")
	if len(lines) != 4 || lines[0] != "reason=cli_outdated" || lines[1] != "agentKind="+kind || lines[2] != "cliVersion="+ver || !strings.HasPrefix(lines[3], "raw=") {
		t.Fatalf("detail shape:\n%s", ae.Detail)
	}
	if !strings.Contains(lines[3], rawSub) {
		t.Fatalf("raw missing %q: %s", rawSub, lines[3])
	}
}

func fakeRejectingCLI(t *testing.T, version, stderr string) string {
	fake := filepath.Join(t.TempDir(), "cli-fake")
	script := "#!/bin/sh\nif [ \"$1\" = \"--version\" ] || [ \"$1\" = \"-v\" ]; then echo '" + version + "'; exit 0; fi\nif [ \"$1\" = \"debug\" ] || [ \"$1\" = \"models\" ]; then exit 1; fi\necho \"" + stderr + "\" >&2\nexit 2\n"
	_ = os.WriteFile(fake, []byte(script), 0o755)
	return fake
}

func TestOutdatedMapping_ClaudeCodexGrok(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix script fake")
	}
	execFn := func(fake string) func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, fake, args...)
		}
	}
	msgs := []WireMessage{{Role: "user", Content: "你好"}}

	// Codex：参数被拒
	fx := fakeRejectingCLI(t, "codex-cli 0.40.0", "error: unexpected argument '--ask-for-approval' found")
	cx := NewCodexAdapter(CLIAgentConfig{DataRoot: t.TempDir(), LookPath: func(string) (string, error) { return fx, nil }, Exec: execFn(fx)})
	cx.detect()
	_, err := cx.RunTurn(TurnOptions{Ctx: context.Background(), ConversationID: "c1", Messages: msgs})
	assertOutdated(t, err, KindCodexCLI, "Codex", "0.40.0", "--ask-for-approval")

	// Claude：低于最低版本，不起进程直接报
	fc := fakeRejectingCLI(t, "1.0.50 (Claude Code)", "should not run")
	cl := NewClaudeAdapter(CLIAgentConfig{DataRoot: t.TempDir(), LookPath: func(string) (string, error) { return fc, nil }, Exec: execFn(fc)})
	cl.detect()
	_, err = cl.RunTurn(TurnOptions{Ctx: context.Background(), ConversationID: "c2", Messages: msgs})
	assertOutdated(t, err, KindClaudeCLI, "Claude", "1.0.50", "")

	// Claude：版本够但仍拒收参数
	fc2 := fakeRejectingCLI(t, "2.1.250 (Claude Code)", "error: option '--permission-mode <mode>' argument 'manual' is invalid.")
	cl2 := NewClaudeAdapter(CLIAgentConfig{DataRoot: t.TempDir(), LookPath: func(string) (string, error) { return fc2, nil }, Exec: execFn(fc2)})
	cl2.detect()
	_, err = cl2.RunTurn(TurnOptions{Ctx: context.Background(), ConversationID: "c3", Messages: msgs})
	assertOutdated(t, err, KindClaudeCLI, "Claude", "2.1.250", "manual")

	// Grok
	fg := fakeRejectingCLI(t, "grok 1.0.10 (abc) [stable]", "error: unexpected argument '--prompt-json' found")
	g := NewBuildAdapter(BuildConfig{DataTemp: t.TempDir(), LookPath: func(string) (string, error) { return fg, nil }, Exec: execFn(fg)})
	g.detect()
	_, err = g.RunTurn(TurnOptions{Ctx: context.Background(), ConversationID: "c4", ProjectPath: t.TempDir(), Messages: msgs})
	assertOutdated(t, err, KindCatBuild, "Grok", "1.0.10", "--prompt-json")
}

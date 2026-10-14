package catagent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// 真机 help 白名单：适配器生成的每个开关都必须出现在对应 CLI 子命令的真机 --help 快照里
// （testdata/help/*.txt，2026-10-14 采集，首行注明版本）。CLI 升级后重新采集快照。

var (
	helpLongDef  = regexp.MustCompile(`(?m)^\s*(?:-[A-Za-z0-9],\s*)?(--[A-Za-z0-9][A-Za-z0-9-]*)`)
	helpShortDef = regexp.MustCompile(`(?m)^\s*(-[A-Za-z0-9])[,\s]`)
)

func loadHelpFlags(t *testing.T, name string) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "help", name))
	if err != nil {
		t.Fatalf("read help snapshot %s: %v", name, err)
	}
	set := map[string]bool{}
	for _, m := range helpLongDef.FindAllStringSubmatch(string(raw), -1) {
		set[m[1]] = true
	}
	for _, m := range helpShortDef.FindAllStringSubmatch(string(raw), -1) {
		set[m[1]] = true
	}
	return set
}

func emittedFlags(args []string) []string {
	var out []string
	for _, a := range args {
		if !strings.HasPrefix(a, "-") || a == "-" {
			continue
		}
		if i := strings.IndexByte(a, '='); i > 0 {
			a = a[:i]
		}
		out = append(out, a)
	}
	return out
}

func assertWhitelisted(t *testing.T, cli, snapshot string, args []string) {
	t.Helper()
	allowed := loadHelpFlags(t, snapshot)
	for _, f := range emittedFlags(args) {
		key := cli + "\x00" + f
		if !allowed[f] && !reported[key] {
			reported[key] = true
			t.Errorf("%s: 生成了真机 help（%s）里没有的参数 %q；完整参数：%q", cli, snapshot, f, args)
		}
	}
}

var reported = map[string]bool{}

var allModes = []string{"", "approve", "always-approve", "full", "plan", "auto", "auto-review", "normal", "ask", "default"}

func TestHelpWhitelist_Codex(t *testing.T) {
	a := NewCodexAdapter(CLIAgentConfig{DataTemp: t.TempDir()})
	for _, mode := range allModes {
		for _, model := range []string{"", "default", "gpt-6-astra"} {
			for _, rules := range []string{"", "be brief"} {
				o := TurnOptions{PermissionMode: mode, ModelID: model, Rules: rules}
				assertWhitelisted(t, "codex exec", "codex-exec.txt", a.buildCodexArgs(o, "你好", "/proj", "", false))
				assertWhitelisted(t, "codex exec", "codex-exec.txt", a.buildCodexArgs(o, "你好", "", "sess", false))
				r := a.buildCodexArgs(o, "你好", "/proj", "sess", true)
				if len(r) < 2 || r[1] != "resume" {
					t.Fatalf("resume args shape: %q", r)
				}
				assertWhitelisted(t, "codex exec resume", "codex-exec-resume.txt", r)
			}
		}
	}
	// 固定调用：codex --version；codex debug models [--bundled]
	assertWhitelisted(t, "codex", "codex.txt", []string{"--version"})
	assertWhitelisted(t, "codex debug models", "codex-debug-models.txt", []string{"--bundled"})
}

func TestHelpWhitelist_Claude(t *testing.T) {
	a := NewClaudeAdapter(CLIAgentConfig{DataTemp: t.TempDir()})
	for _, mode := range allModes {
		for _, model := range []string{"", "default", "sonnet"} {
			for _, rules := range []string{"", "be brief"} {
				for _, resume := range []bool{false, true} {
					o := TurnOptions{PermissionMode: mode, ModelID: model, Rules: rules}
					assertWhitelisted(t, "claude -p", "claude.txt", a.buildClaudeArgs(o, "你好", "sess", resume))
				}
			}
		}
	}
	assertWhitelisted(t, "claude", "claude.txt", []string{"--version"})
}

func TestHelpWhitelist_Grok(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix true")
	}
	dir := t.TempDir()
	img := filepath.Join(dir, "a.png")
	_ = os.WriteFile(img, []byte("\x89PNG\r\n\x1a\n"), 0o644)
	var mu sync.Mutex
	var captured [][]string
	a := NewBuildAdapter(BuildConfig{
		DataTemp: t.TempDir(),
		LookPath: func(string) (string, error) { return "/bin/true", nil },
		Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			mu.Lock()
			captured = append(captured, append([]string(nil), args...))
			mu.Unlock()
			return exec.CommandContext(ctx, "true")
		},
	})
	a.detect()
	for _, mode := range allModes {
		for _, model := range []string{"", "grok-build"} {
			for _, think := range []string{"", "high"} {
				for _, resume := range []bool{false, true} {
					for _, files := range [][]PromptFile{nil, {{Name: "a.png", Path: img}}} {
						_, _ = a.RunTurn(TurnOptions{
							Ctx: context.Background(), ConversationID: "conv-1", ProjectPath: dir,
							PermissionMode: mode, ModelID: model, ThinkLevelID: think, Resume: resume,
							Rules: "be brief", Files: files,
							Messages: []WireMessage{{Role: "user", Content: "hello"}},
						})
					}
				}
			}
		}
	}
	mu.Lock()
	defer mu.Unlock()
	var turns int
	for _, args := range captured {
		switch {
		case len(args) > 0 && args[0] == "inspect":
			assertWhitelisted(t, "grok inspect", "grok-inspect.txt", args[1:])
		case len(args) > 0 && (args[0] == "models" || args[0] == "export"):
			assertWhitelisted(t, "grok "+args[0], "grok.txt", args[1:])
		default:
			turns++
			assertWhitelisted(t, "grok", "grok.txt", args)
		}
	}
	if turns == 0 {
		t.Fatal("no grok turn args captured")
	}
	assertWhitelisted(t, "grok", "grok.txt", []string{"-v"})
	assertWhitelisted(t, "grok inspect", "grok-inspect.txt", []string{"--json"})
}

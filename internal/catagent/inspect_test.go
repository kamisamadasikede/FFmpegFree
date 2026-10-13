package catagent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRulesCLIArgs(t *testing.T) {
	if got := rulesCLIArgs("  "); got != nil {
		t.Fatalf("blank rules must not add args: %v", got)
	}
	got := rulesCLIArgs(" -用中文回答 ")
	if len(got) != 1 || got[0] != "--rules=-用中文回答" {
		t.Fatalf("got %v", got)
	}
}

func TestXHighDisplayName(t *testing.T) {
	for _, l := range defaultThinks {
		if l.ID == "xhigh" && l.DisplayName != "极高" {
			t.Fatalf("xhigh display = %q", l.DisplayName)
		}
	}
}

const inspectFixture = `{"grokVersion":"1.0.40","cwd":"/x","hooks":[{"event":"PreToolUse","command":"rm -rf /","source":"/home/u/.grok/hooks.json"}],
"skills":[{"name":"pdf","description":"PDF","source":{"type":"bundled","path":"/home/u/.grok/bundled/skills/pdf/SKILL.md"},"userInvocable":true},
{"name":"off","source":{"type":"user","path":"/p"},"enabled":false}],
"agents":[{"name":"explore","description":"read-only","source":{"type":"builtin"}}],
"plugins":[],"mcpServers":{"gh":{"command":"npx","env":{"TOKEN":"secret"},"disabled":true}}}`

func TestParseInspectDropsPathsAndSecrets(t *testing.T) {
	res, ok := ParseInspect([]byte(inspectFixture))
	if !ok {
		t.Fatal("parse failed")
	}
	if res.Version != "1.0.40" || len(res.Skills) != 2 || res.Skills[0].Source != "bundled" || !res.Skills[0].Enabled || res.Skills[1].Enabled {
		t.Fatalf("skills %+v", res)
	}
	if len(res.Hooks) != 1 || res.Hooks[0].Name != "PreToolUse" || res.Hooks[0].Source != "" {
		t.Fatalf("hooks %+v", res.Hooks)
	}
	if len(res.MCPServers) != 1 || res.MCPServers[0].Name != "gh" || res.MCPServers[0].Enabled {
		t.Fatalf("mcp %+v", res.MCPServers)
	}
	if res.Plugins == nil || len(res.Agents) != 1 {
		t.Fatalf("%+v", res)
	}
	for _, it := range append(append(res.Skills, res.Hooks...), res.MCPServers...) {
		s := it.Name + it.Description + it.Source
		if strings.Contains(s, "/") || strings.Contains(s, "secret") || strings.Contains(s, "npx") {
			t.Fatalf("leaked %+v", it)
		}
	}
	if _, ok := ParseInspect([]byte("not json")); ok {
		t.Fatal("garbage must fail")
	}
	if _, ok := ParseInspect([]byte(`{"foo":1}`)); ok {
		t.Fatal("unknown shape must fail")
	}
}

func fakeGrokAdapter(t *testing.T, body string) *BuildAdapter {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("unix script fake")
	}
	dir := t.TempDir()
	fake := filepath.Join(dir, "grok")
	script := "#!/bin/sh\ncase \"$1\" in\n  -v) echo 'grok 1.0.40'; exit 0 ;;\n  models) exit 0 ;;\nesac\n" + body
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewBuildAdapter(BuildConfig{
		ComponentDir: filepath.Join(dir, "c"), DataRoot: dir,
		LookPath: func(string) (string, error) { return fake, nil },
		Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, name, args...)
		},
	})
	a.Start()
	deadline := time.Now().Add(3 * time.Second)
	for a.Status().State != StateReady {
		if time.Now().After(deadline) {
			t.Fatalf("not ready %+v", a.Status())
		}
		time.Sleep(10 * time.Millisecond)
	}
	return a
}

func TestInspectRunsCLIAndReasons(t *testing.T) {
	a := fakeGrokAdapter(t, "if [ \"$1\" = inspect ] && [ \"$2\" = --json ]; then printf '%s' '"+strings.ReplaceAll(inspectFixture, "\n", "")+"'; fi\n")
	res := a.Inspect(context.Background(), t.TempDir())
	if res.Reason != "" || len(res.Skills) != 2 {
		t.Fatalf("%+v", res)
	}

	b := fakeGrokAdapter(t, "echo nope\n")
	if r := b.Inspect(context.Background(), ""); r.Reason != InspectParse || r.Skills == nil || len(r.Skills) != 0 {
		t.Fatalf("%+v", r)
	}

	old := InspectTimeout
	InspectTimeout = 200 * time.Millisecond
	defer func() { InspectTimeout = old }()
	c := fakeGrokAdapter(t, "sleep 5\n")
	start := time.Now()
	if r := c.Inspect(context.Background(), ""); r.Reason != InspectTimedOut {
		t.Fatalf("%+v", r)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("timeout not enforced")
	}

	d := NewBuildAdapter(BuildConfig{LookPath: func(string) (string, error) { return "", exec.ErrNotFound }})
	if r := d.Inspect(context.Background(), ""); r.Reason != InspectNotReady {
		t.Fatalf("%+v", r)
	}
}

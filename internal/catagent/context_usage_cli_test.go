package catagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func openFixture(t *testing.T, p string) *os.File {
	t.Helper()
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func TestCodexRolloutContext_LastTokenCount(t *testing.T) {
	used, win := parseCodexRolloutContext(openFixture(t, "testdata/codex/rollout-token-count-doc-derived.jsonl"))
	if used != 28500 || win != 272000 {
		t.Fatalf("got %d/%d", used, win)
	}
}

func TestCodexRolloutContext_RealUnauthHasWindowOnly(t *testing.T) {
	// 真机样本（未登录）：只有 task_started.model_context_window=258400，没有 token_count → 已用 0，不画圆环。
	used, win := parseCodexRolloutContext(openFixture(t, "testdata/codex/rollout-2026-10-14T10-37-32-01a13845-acb6-7382-a6de-f154547ff010.jsonl"))
	if used != 0 || win != 258400 {
		t.Fatalf("got %d/%d", used, win)
	}
}

func TestClaudeTranscriptContext(t *testing.T) {
	used, model := parseClaudeTranscriptContext(openFixture(t, "testdata/claude/context-usage-doc-derived.jsonl"))
	if used != 3+500+21000+120 || model != "claude-sonnet-4-6" {
		t.Fatalf("got %d %q", used, model)
	}
}

func TestClaudeTranscriptContext_RealUnauthSynthetic(t *testing.T) {
	used, _ := parseClaudeTranscriptContext(openFixture(t, "testdata/claude/061a686f-48a9-43ac-91f6-cc737232921d.jsonl"))
	if used != 0 {
		t.Fatalf("synthetic error row must not count, got %d", used)
	}
}

func TestClaudeResultContextWindow(t *testing.T) {
	b, _ := os.ReadFile("testdata/claude/result-modelusage-doc-derived.json")
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	if w := claudeResultContextWindow(raw, "claude-sonnet-4-6"); w != 200000 {
		t.Fatalf("named: %d", w)
	}
	if w := claudeResultContextWindow(raw, ""); w != 200000 {
		t.Fatalf("max: %d", w)
	}
	// 真机未登录：modelUsage 为 {} → 0。
	var empty map[string]json.RawMessage
	_ = json.Unmarshal([]byte(`{"type":"result","modelUsage":{}}`), &empty)
	if w := claudeResultContextWindow(empty, ""); w != 0 {
		t.Fatalf("empty: %d", w)
	}
}

func TestEmitClaudeResultLine_EmitsCtxWin(t *testing.T) {
	b, _ := os.ReadFile("testdata/claude/result-modelusage-doc-derived.json")
	var got []streamPiece
	emitClaudeResultLine([]byte(strings.TrimSpace(string(b))), func(p streamPiece) { got = append(got, p) }, nil, nil)
	found := false
	for _, p := range got {
		if p.Kind == "ctxwin" && p.ID == "200000" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no ctxwin piece: %+v", got)
	}
}

func TestTurnContextUsage_FindsFiles(t *testing.T) {
	// Codex：$CODEX_HOME/sessions/YYYY/MM/DD/rollout-*-<thread>.jsonl
	home := t.TempDir()
	tid := "01a13845-acb6-7382-a6de-f154547ff011"
	dir := filepath.Join(home, "sessions", "2026", "10", "14")
	_ = os.MkdirAll(dir, 0o755)
	b, _ := os.ReadFile("testdata/codex/rollout-token-count-doc-derived.jsonl")
	_ = os.WriteFile(filepath.Join(dir, "rollout-2026-10-14T10-37-32-"+tid+".jsonl"), b, 0o644)
	t.Setenv("CODEX_HOME", home)
	cx := &CLIAgentAdapter{kind: cliKindCodex}
	if u, w := cx.turnContextUsage(tid, "", 0); u != 28500 || w != 272000 {
		t.Fatalf("codex %d/%d", u, w)
	}
	if u, w := cx.turnContextUsage("", "", 0); u != 0 || w != 0 {
		t.Fatalf("no session must be 0")
	}
}

func TestTurnContextUsage_Claude(t *testing.T) {
	cfg := t.TempDir()
	sid := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	cwd := "/tmp/proj"
	dir := filepath.Join(cfg, "projects", claudeProjectDirName(cwd))
	_ = os.MkdirAll(dir, 0o755)
	b, _ := os.ReadFile("testdata/claude/context-usage-doc-derived.jsonl")
	_ = os.WriteFile(filepath.Join(dir, sid+".jsonl"), b, 0o644)
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	cl := &CLIAgentAdapter{kind: cliKindClaude}
	if u, w := cl.turnContextUsage(sid, cwd, 200000); u != 21623 || w != 200000 {
		t.Fatalf("claude %d/%d", u, w)
	}
	// 没拿到 result.modelUsage.contextWindow → 不画（不猜容量）。
	if u, w := cl.turnContextUsage(sid, cwd, 0); u != 0 || w != 0 {
		t.Fatalf("no window must be 0, got %d/%d", u, w)
	}
}

package catagent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const realClaudeSID = "061a686f-48a9-43ac-91f6-cc737232921d"

func seedClaudeHome(t *testing.T, projectDir, file string) string {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, "projects", projectDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join("testdata", "claude", file))
	if err != nil {
		t.Fatal(err)
	}
	name := file
	if strings.HasPrefix(file, "doc-derived") {
		name = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee.jsonl"
	}
	if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestParseClaudeTranscript_RealAuthFail(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "claude", realClaudeSID+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got := dumpWires(ParseClaudeTranscript(f))
	// 真机 2.1.296：一句 user "hi" + 一条 isApiErrorMessage 的合成助手失败 → status
	if !strings.Contains(got, "user: hi\n") {
		t.Fatalf("missing user:\n%s", got)
	}
	if !strings.Contains(got, "status: Not logged in") {
		t.Fatalf("missing auth status:\n%s", got)
	}
	if strings.Contains(got, "assistant:") {
		t.Fatalf("api error must not be assistant bubble:\n%s", got)
	}
}

func TestParseClaudeTranscript_DocDerivedMergeAndSkip(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "claude", "doc-derived-reply.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got := dumpWires(ParseClaudeTranscript(f))
	want := "user: 帮我看一下这段代码\nthought: 先读文件\nassistant: 我看看这段。\ntool: Read {\"path\":\"/tmp/x.go\"}\nassistant: 这里有个小问题。\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if strings.Contains(got, "sidechain") || strings.Contains(got, "ignore") {
		t.Fatal("sidechain / meta must be skipped")
	}
}

func TestClaudeHistoryPersistsAfterReload(t *testing.T) {
	data := t.TempDir()
	// 真机 cwd=/tmp/ccprobe → projects/-tmp-ccprobe/<sid>.jsonl
	home := seedClaudeHome(t, "-tmp-ccprobe", realClaudeSID+".jsonl")
	a1 := NewClaudeAdapter(CLIAgentConfig{DataRoot: data, ClaudeConfigDir: home})
	a1.saveSessionID("conv-1", realClaudeSID)

	a2 := NewClaudeAdapter(CLIAgentConfig{DataRoot: data, ClaudeConfigDir: home})
	msgs, err := a2.LoadTranscript(context.Background(), "conv-1", "/tmp/ccprobe")
	if err != nil {
		t.Fatal(err)
	}
	if dumpWires(msgs) == "" {
		t.Fatal("刷新后应读回 Claude 记录")
	}
	if !a2.SessionResumable("conv-1", "/tmp/ccprobe") {
		t.Fatal("有 session_id 时应可续聊")
	}
	if label := a2.SessionLabel("conv-1", "/tmp/ccprobe"); label != "hi" {
		t.Fatalf("sidebar label = %q", label)
	}
}

func TestClaudeTranscript_MissingFileEmpty(t *testing.T) {
	a := NewClaudeAdapter(CLIAgentConfig{DataRoot: t.TempDir(), ClaudeConfigDir: t.TempDir()})
	a.saveSessionID("c", "00000000-0000-0000-0000-000000000000")
	msgs, err := a.LoadTranscript(context.Background(), "c", "/tmp/x")
	if !errors.Is(err, ErrHistoryMissing) {
		t.Fatalf("want ErrHistoryMissing, got %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("missing file → empty, got %v", msgs)
	}
}

func TestClaudeTranscript_ScanFallback(t *testing.T) {
	// 目录名被 CLAUDE_CODE_PROJECT_DIR_NAME 改过：拼规则找不到，应扫一层。
	home := seedClaudeHome(t, "renamed-project", realClaudeSID+".jsonl")
	a := NewClaudeAdapter(CLIAgentConfig{DataRoot: t.TempDir(), ClaudeConfigDir: home})
	a.saveSessionID("c", realClaudeSID)
	msgs, err := a.LoadTranscript(context.Background(), "c", "/tmp/ccprobe")
	if err != nil || dumpWires(msgs) == "" {
		t.Fatalf("scan fallback failed: %v %q", err, dumpWires(msgs))
	}
}

func TestBuildClaudeArgs_ResumeWhenSessionID(t *testing.T) {
	a := NewClaudeAdapter(CLIAgentConfig{DataTemp: t.TempDir()})
	args := a.buildClaudeArgs(TurnOptions{}, "你好", "sess-uuid", true)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--resume sess-uuid") {
		t.Fatalf("want --resume: %q", args)
	}
	args2 := a.buildClaudeArgs(TurnOptions{}, "你好", "", false)
	if strings.Contains(strings.Join(args2, " "), "--resume") {
		t.Fatalf("no session → no resume: %q", args2)
	}
}

func TestClaudeSessionMissing_ClearsID(t *testing.T) {
	if !claudeSessionMissing("No conversation found with session ID: x") {
		t.Fatal("detector")
	}
	data := t.TempDir()
	a := NewClaudeAdapter(CLIAgentConfig{DataRoot: data})
	a.saveSessionID("c", "sid")
	a.clearSessionID("c")
	if a.loadSessionID("c") != "" {
		t.Fatal("cleared")
	}
}

func TestCodexSessionLabel_FirstUser(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "sessions", "2026", "10", "14")
	_ = os.MkdirAll(dir, 0o755)
	src := filepath.Join("testdata", "codex", "rollout-2026-10-14T10-37-32-01a13845-acb6-7382-a6de-f154547ff010.jsonl")
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, filepath.Base(src)), b, 0o644)
	a := NewCodexAdapter(CLIAgentConfig{DataRoot: t.TempDir(), CodexHome: home})
	a.saveSessionID("c", "01a13845-acb6-7382-a6de-f154547ff010")
	if label := a.SessionLabel("c", "/tmp"); label != "say hi" {
		t.Fatalf("label=%q", label)
	}
}

func TestClaudeTranscript_ReadFailed(t *testing.T) {
	home := t.TempDir()
	const sid = "11111111-2222-3333-4444-555555555555"
	dir := filepath.Join(home, "projects", "-tmp-x")
	_ = os.MkdirAll(dir, 0o755)
	p := filepath.Join(dir, sid+".jsonl")
	_ = os.WriteFile(p, []byte("{}\n"), 0o000) // 文件在但打不开
	if f, err := os.Open(p); err == nil {
		f.Close()
		t.Skip("running as root: chmod 000 still readable")
	}
	a := NewClaudeAdapter(CLIAgentConfig{DataRoot: t.TempDir(), ClaudeConfigDir: home})
	a.saveSessionID("c", sid)
	msgs, err := a.LoadTranscript(context.Background(), "c", "/tmp/x")
	if !errors.Is(err, ErrHistoryReadFailed) || len(msgs) != 0 {
		t.Fatalf("msgs=%v err=%v", msgs, err)
	}
}

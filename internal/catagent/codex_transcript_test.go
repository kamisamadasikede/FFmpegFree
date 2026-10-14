package catagent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const realCodexThread = "01a13845-acb6-7382-a6de-f154547ff010"

type transcriptLoader interface {
	LoadTranscript(context.Context, string, string) ([]WireMessage, error)
}

// 把真机 rollout 放进假 CODEX_HOME 的 sessions/YYYY/MM/DD/ 下。
func seedCodexHome(t *testing.T, files ...string) string {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, "sessions", "2026", "10", "14")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		b, err := os.ReadFile(filepath.Join("testdata", "codex", f))
		if err != nil {
			t.Fatal(err)
		}
		name := f
		if strings.HasPrefix(f, "rollout-doc-derived") {
			name = "rollout-2026-10-14T10-40-00-01a13845-acb6-7382-a6de-f154547ff011.jsonl"
		}
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

// 复现 P0：thread_id 来自真机 `codex exec --json`，存下后“刷新”（新建适配器实例）应能读回记录。
func TestCodexHistoryPersistsAfterReload(t *testing.T) {
	data := t.TempDir()
	home := seedCodexHome(t, "rollout-2026-10-14T10-37-32-01a13845-acb6-7382-a6de-f154547ff010.jsonl")

	// 第一个实例：解析真机 stdout 拿 thread_id 并存下（与 RunTurn 同一路径）。
	a1 := NewCodexAdapter(CLIAgentConfig{DataRoot: data, CodexHome: home})
	raw, err := os.ReadFile(filepath.Join("testdata", "codex", "exec-json-unauth-0.162.1.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var sid string
	_ = a1.scanCLIStream(strings.NewReader(string(raw)), func(streamPiece) {}, func(string) {}, func(s string) { sid = s })
	if sid != realCodexThread {
		t.Fatalf("thread id = %q", sid)
	}
	a1.saveSessionID("conv-1", sid)

	// 刷新：新实例，只靠磁盘。
	a2 := NewCodexAdapter(CLIAgentConfig{DataRoot: data, CodexHome: home})
	var any interface{} = a2
	loader, ok := any.(transcriptLoader)
	if !ok {
		t.Fatal("codex 适配器没有 LoadTranscript：刷新后会话详情永远是空列表")
	}
	msgs, err := loader.LoadTranscript(context.Background(), "conv-1", "/tmp/cxw")
	if err != nil {
		t.Fatal(err)
	}
	got := dumpWires(msgs)
	want := "user: say hi\nstatus: unexpected status 401 Unauthorized: Missing bearer or basic authentication in header, url: https://api.openai.com/v1/responses, cf-ray: a4a334401be23913-IAD, request id: req_492c475adb444a4e80c93792425e869a\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if !a2.SessionResumable("conv-1") {
		t.Fatal("存了 thread_id 后下一句应走 exec resume")
	}
}

func TestCodexTranscriptDocDerivedReply(t *testing.T) {
	data := t.TempDir()
	home := seedCodexHome(t, "rollout-doc-derived-reply.jsonl")
	a := NewCodexAdapter(CLIAgentConfig{DataRoot: data, CodexHome: home})
	a.saveSessionID("conv-2", "01a13845-acb6-7382-a6de-f154547ff011")
	msgs, err := a.LoadTranscript(context.Background(), "conv-2", "")
	if err != nil {
		t.Fatal(err)
	}
	want := "user: 列出当前目录\nthought: 先用 ls 看看目录\ntool: exec_command {\"cmd\":\"ls\"}\nassistant: 目录里有 a.mp4 和 b.mp4。\n"
	if got := dumpWires(msgs); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestCodexTranscriptMissingIsEmptyNotError(t *testing.T) {
	a := NewCodexAdapter(CLIAgentConfig{DataRoot: t.TempDir(), CodexHome: t.TempDir()})
	// 没有 thread_id。
	msgs, err := a.LoadTranscript(context.Background(), "conv-x", "")
	if err != nil || msgs == nil || len(msgs) != 0 {
		t.Fatalf("msgs=%v err=%v", msgs, err)
	}
	// 有 thread_id 但 rollout 已被删。
	a.saveSessionID("conv-x", "deadbeef-0000")
	msgs, err = a.LoadTranscript(context.Background(), "conv-x", "")
	if err != nil || msgs == nil || len(msgs) != 0 {
		t.Fatalf("msgs=%v err=%v", msgs, err)
	}
}

func dumpWires(ws []WireMessage) string {
	var b strings.Builder
	for _, w := range ws {
		b.WriteString(w.Role + ": " + w.Content + "\n")
	}
	return b.String()
}

// 第二句必须带上第一句存下的 thread_id 走 `codex exec resume`（service 传的 Resume 对 Codex 恒为 false）。
func TestCodexSecondTurnResumesSavedThread(t *testing.T) {
	if os.Getenv("GOOS") == "windows" || filepath.Separator == '\\' {
		t.Skip("unix script fake")
	}
	fake := filepath.Join(t.TempDir(), "codex-fake")
	script := "#!/bin/sh\nprintf '%s\\n' '{\"type\":\"thread.started\",\"thread_id\":\"" + realCodexThread + "\"}'\n" +
		"printf '%s\\n' '{\"type\":\"item.completed\",\"item\":{\"id\":\"i1\",\"type\":\"agent_message\",\"text\":\"ok\"}}'\nexit 0\n"
	_ = os.WriteFile(fake, []byte(script), 0o755)
	var calls [][]string
	a := NewCodexAdapter(CLIAgentConfig{
		DataRoot: t.TempDir(),
		LookPath: func(string) (string, error) { return fake, nil },
		Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			calls = append(calls, args)
			return exec.CommandContext(ctx, fake)
		},
	})
	a.detect()
	for i := 0; i < 2; i++ {
		if _, err := a.RunTurn(TurnOptions{Ctx: context.Background(), ConversationID: "c-1",
			Messages: []WireMessage{{Role: "user", Content: "hi"}}}); err != nil {
			t.Fatal(err)
		}
	}
	last := strings.Join(calls[len(calls)-1], " ")
	first := strings.Join(calls[len(calls)-2], " ")
	if strings.Contains(first, "resume") {
		t.Fatalf("第一句不该 resume: %s", first)
	}
	if !strings.Contains(last, "exec resume") || !strings.Contains(last, realCodexThread) {
		t.Fatalf("第二句应 resume 已存 thread: %s", last)
	}
}

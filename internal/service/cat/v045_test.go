package cat

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/store"
)

// 打开会话必须走 SessionHistory（agentKind + 存下的 sessionId → CLI 文件），不拼本地消息缓存。
func TestGetCatConversation_ClaudeViaSessionHistory(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dataRoot := filepath.Join(dir, "data")
	claudeHome := filepath.Join(dir, "claude")
	dbPath := filepath.Join(dir, "app.db")
	const sid = "061a686f-48a9-43ac-91f6-cc737232921d"
	sample, err := os.ReadFile(filepath.Join("..", "..", "catagent", "testdata", "claude", sid+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(claudeHome, "projects", "-tmp-ccprobe")
	_ = os.MkdirAll(proj, 0o755)
	if err := os.WriteFile(filepath.Join(proj, sid+".jsonl"), sample, 0o644); err != nil {
		t.Fatal(err)
	}

	boot := func() (*Service, *store.Store) {
		st, err := store.Open(ctx, dbPath)
		if err != nil {
			t.Fatal(err)
		}
		reg := catagent.NewRegistry()
		reg.Register(catagent.NewClaudeAdapter(catagent.CLIAgentConfig{
			DataRoot: dataRoot, ClaudeConfigDir: claudeHome,
		}))
		return New(Config{Store: st, Registry: reg, DataRoot: dataRoot}), st
	}

	svc1, st1 := boot()
	c, err := svc1.CreateCatConversation(ctx, CreateConversationRequest{AgentKind: catagent.KindClaudeCLI})
	if err != nil {
		t.Fatal(err)
	}
	sp := filepath.Join(dataRoot, "cat", "cli-sessions", catagent.KindClaudeCLI, c.ID)
	_ = os.MkdirAll(filepath.Dir(sp), 0o755)
	if err := os.WriteFile(sp, []byte(sid), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = st1.Close()

	svc2, st2 := boot()
	defer st2.Close()
	d, err := svc2.GetCatConversation(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Messages) == 0 || d.Messages[0].Role != "user" || d.Messages[0].Content != "hi" {
		t.Fatalf("messages=%+v (want CLI jsonl, not empty/local cache)", d.Messages)
	}
	if d.Title != "hi" {
		t.Fatalf("sidebar title=%q", d.Title)
	}

	// 文件不在 → 空列表，不报错、不捏造
	_ = os.Remove(filepath.Join(proj, sid+".jsonl"))
	d2, err := svc2.GetCatConversation(ctx, c.ID)
	if err != nil || len(d2.Messages) != 0 {
		t.Fatalf("missing file → empty: err=%v msgs=%v", err, d2.Messages)
	}
}

func TestFinish_TimingOnCancelAndFail(t *testing.T) {
	s := New(Config{DataRoot: t.TempDir(), Logf: func(string, ...any) {}})
	now := time.Now().UnixMilli()
	var events []catagent.TurnEvent
	s.cfg.Emit = func(ev string, p any) {
		if ev == catagent.EventTurn {
			events = append(events, p.(catagent.TurnEvent))
		}
	}

	tCancel := &turnState{convID: "c", turnID: "t1", msgID: "m1", startedAt: now - 500, cancel: func() {}, done: make(chan struct{})}
	s.setTurn(tCancel)
	s.lastMu.Lock()
	s.lastTurns["c"] = &lastTurn{turnID: "t1", status: catagent.TurnRunning}
	s.lastMu.Unlock()
	s.finish(tCancel, catagent.TurnCancelled)
	if len(events) == 0 {
		t.Fatal("no cancel event")
	}
	ev := events[len(events)-1]
	if ev.TotalMs == nil || *ev.TotalMs < 400 {
		t.Fatalf("cancel totalMs=%v", ev.TotalMs)
	}
	if ev.FirstTokenMs != nil {
		t.Fatalf("no token → omit firstTokenMs, got %v", *ev.FirstTokenMs)
	}

	tFail := &turnState{convID: "c2", turnID: "t2", msgID: "m2", startedAt: now - 800, cancel: func() {}, done: make(chan struct{})}
	tFail.text.WriteString("hi")
	tFail.seq = 1
	tFail.firstTokenAt = now - 600
	s.setTurn(tFail)
	events = nil
	s.finish(tFail, catagent.TurnFailed)
	ev = events[len(events)-1]
	if ev.FirstTokenMs == nil || *ev.FirstTokenMs < 100 {
		t.Fatalf("firstTokenMs=%v", ev.FirstTokenMs)
	}
	if ev.TotalMs == nil || *ev.TotalMs < 700 {
		t.Fatalf("fail totalMs=%v", ev.TotalMs)
	}
}

type histErrAdapter struct {
	*catagent.CLIAgentAdapter
	err error
}

func (h *histErrAdapter) LoadTranscript(context.Context, string, string) ([]catagent.WireMessage, error) {
	return []catagent.WireMessage{}, h.err
}

func TestGetCatConversation_HistoryStatus(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(ctx, filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	dataRoot := filepath.Join(dir, "data")
	claudeHome := filepath.Join(dir, "claude")
	reg := catagent.NewRegistry()
	reg.Register(catagent.NewClaudeAdapter(catagent.CLIAgentConfig{DataRoot: dataRoot, ClaudeConfigDir: claudeHome}))
	svc := New(Config{Store: st, Registry: reg, DataRoot: dataRoot})
	c, err := svc.CreateCatConversation(ctx, CreateConversationRequest{AgentKind: catagent.KindClaudeCLI})
	if err != nil {
		t.Fatal(err)
	}
	// 从没存过 sessionId（新对话）→ 省略
	d, err := svc.GetCatConversation(ctx, c.ID)
	if err != nil || d.HistoryStatus != nil || len(d.Messages) != 0 {
		t.Fatalf("no sid: err=%v st=%v msgs=%v", err, d.HistoryStatus, d.Messages)
	}
	// 存下 sessionId
	const sid = "22222222-3333-4444-5555-666666666666"
	sp := filepath.Join(dataRoot, "cat", "cli-sessions", catagent.KindClaudeCLI, c.ID)
	_ = os.MkdirAll(filepath.Dir(sp), 0o755)
	_ = os.WriteFile(sp, []byte(sid), 0o600)
	// 存了 sessionId 但文件不在 → missing
	d, err = svc.GetCatConversation(ctx, c.ID)
	if err != nil || d.HistoryStatus == nil || *d.HistoryStatus != HistoryMissing || len(d.Messages) != 0 {
		t.Fatalf("sid but no file: err=%v st=%v", err, d.HistoryStatus)
	}
	proj := filepath.Join(claudeHome, "projects", "x")
	_ = os.MkdirAll(proj, 0o755)
	_ = os.WriteFile(filepath.Join(proj, sid+".jsonl"), []byte(`{"type":"queue-operation"}`+"\n"), 0o644)
	// 文件可读、对话本来就空 → 省略
	d, err = svc.GetCatConversation(ctx, c.ID)
	if err != nil || d.HistoryStatus != nil {
		t.Fatalf("readable empty: err=%v st=%v", err, d.HistoryStatus)
	}
	// 读失败 → read_failed（适配器报 ErrHistoryReadFailed）
	reg2 := catagent.NewRegistry()
	reg2.Register(&histErrAdapter{CLIAgentAdapter: catagent.NewClaudeAdapter(catagent.CLIAgentConfig{DataRoot: dataRoot}), err: catagent.ErrHistoryReadFailed})
	svc2 := New(Config{Store: st, Registry: reg2, DataRoot: dataRoot})
	d, err = svc2.GetCatConversation(ctx, c.ID)
	if err != nil || d.HistoryStatus == nil || *d.HistoryStatus != HistoryReadFailed || len(d.Messages) != 0 {
		t.Fatalf("read_failed: err=%v st=%v msgs=%v", err, d.HistoryStatus, d.Messages)
	}
}

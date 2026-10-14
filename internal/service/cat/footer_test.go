package cat_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/service/cat"
	"FFmpegFree/internal/store"
)

type transcriptFake struct {
	fakeAdapter
	wires []catagent.WireMessage
}

func (f *transcriptFake) LoadTranscript(context.Context, string, string) ([]catagent.WireMessage, error) {
	return f.wires, nil
}
func (f *transcriptFake) SessionLabel(string, string) string   { return "" }
func (f *transcriptFake) SessionResumable(string, string) bool { return len(f.wires) > 0 }

func setupFooter(t *testing.T, run func(catagent.TurnOptions) (catagent.TurnResponse, error), wires []catagent.WireMessage) (*cat.Service, *recorder, string, string) {
	t.Helper()
	root := t.TempDir()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	reg := catagent.NewRegistry()
	fa := &transcriptFake{fakeAdapter: fakeAdapter{run: run}, wires: wires}
	reg.Register(fa)
	rec := &recorder{turnCh: make(chan catagent.TurnEvent, 16)}
	svc := cat.New(cat.Config{Store: st, Registry: reg, Emit: rec.emit, DataRoot: root})
	c, err := svc.CreateCatConversation(context.Background(), cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild})
	if err != nil {
		t.Fatal(err)
	}
	return svc, rec, c.ID, root
}

func waitTerminal(t *testing.T, rec *recorder) catagent.TurnEvent {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case ev := <-rec.turnCh:
			if ev.Status != catagent.TurnRunning {
				return ev
			}
		case <-deadline:
			t.Fatal("timeout waiting terminal turn")
		}
	}
}

func TestFooterCompletedWithUsage(t *testing.T) {
	svc, rec, conv, _ := setupFooter(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		time.Sleep(30 * time.Millisecond)
		o.OnTextDelta("你好")
		return catagent.TurnResponse{
			Message:      catagent.WireMessage{Role: "assistant", Content: "你好"},
			TokenTotal:   120,
			OutputTokens: 40,
		}, nil
	}, nil)
	if _, err := svc.SendCatMessage(context.Background(), cat.SendMessageRequest{ConversationID: conv, Content: "hi"}); err != nil {
		t.Fatal(err)
	}
	svc.Wait()
	ev := waitTerminal(t, rec)
	if ev.Status != catagent.TurnCompleted {
		t.Fatalf("%+v", ev)
	}
	if ev.FinishedAt <= 0 {
		t.Fatalf("missing finishedAt: %+v", ev)
	}
	if ev.TokenTotal == nil || *ev.TokenTotal != 120 {
		t.Fatalf("tokenTotal %+v", ev.TokenTotal)
	}
	if ev.OutputTokens == nil || *ev.OutputTokens != 40 {
		t.Fatalf("outputTokens %+v", ev.OutputTokens)
	}
	// v0.47：40 / ((totalMs − firstTokenMs)/1000)，按事件里的计时复算。
	if ev.TotalMs == nil || ev.FirstTokenMs == nil || ev.TokensPerSec == nil {
		t.Fatalf("timing/tps missing: %+v", ev)
	}
	gen := *ev.TotalMs - *ev.FirstTokenMs
	if gen <= 0 {
		gen = *ev.TotalMs
	}
	if want := 40 / (float64(gen) / 1000); *ev.TokensPerSec != want {
		t.Fatalf("tokensPerSec %v want %v", *ev.TokensPerSec, want)
	}
	if ev.MessageID == "" {
		t.Fatalf("messageId empty: %+v", ev)
	}
	// running 不得带尾字段
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.turns) < 1 || rec.turns[0].Status != catagent.TurnRunning {
		t.Fatalf("turns %+v", rec.turns)
	}
	run := rec.turns[0]
	if run.FinishedAt != 0 || run.TokenTotal != nil || run.TokensPerSec != nil || run.OutputTokens != nil {
		t.Fatalf("running must omit footer: %+v", run)
	}
}

func TestFooterNoUsageOnlyFinishedAt(t *testing.T) {
	svc, rec, conv, _ := setupFooter(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		o.OnTextDelta("ok")
		return catagent.TurnResponse{Message: catagent.WireMessage{Content: "ok"}}, nil
	}, nil)
	if _, err := svc.SendCatMessage(context.Background(), cat.SendMessageRequest{ConversationID: conv, Content: "hi"}); err != nil {
		t.Fatal(err)
	}
	svc.Wait()
	ev := waitTerminal(t, rec)
	if ev.FinishedAt <= 0 || ev.TokenTotal != nil || ev.TokensPerSec != nil || ev.OutputTokens != nil {
		t.Fatalf("want only finishedAt: %+v", ev)
	}
}

// v0.47：只有合计、没有输出 token → outputTokens 与 tokensPerSec 都省略，不得用合计顶替。
func TestFooterTotalOnlyOmitsSpeed(t *testing.T) {
	svc, rec, conv, _ := setupFooter(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		time.Sleep(50 * time.Millisecond)
		o.OnTextDelta("x")
		return catagent.TurnResponse{Message: catagent.WireMessage{Content: "x"}, TokenTotal: 10}, nil
	}, nil)
	if _, err := svc.SendCatMessage(context.Background(), cat.SendMessageRequest{ConversationID: conv, Content: "hi"}); err != nil {
		t.Fatal(err)
	}
	svc.Wait()
	ev := waitTerminal(t, rec)
	if ev.TokenTotal == nil || *ev.TokenTotal != 10 {
		t.Fatalf("%+v", ev)
	}
	if ev.TokensPerSec != nil || ev.OutputTokens != nil {
		t.Fatalf("total-only must omit outputTokens/tokensPerSec: %+v", ev)
	}
}

func TestFooterFailedPartialUsage(t *testing.T) {
	svc, rec, conv, _ := setupFooter(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		o.OnTextDelta("半句")
		time.Sleep(20 * time.Millisecond)
		return catagent.TurnResponse{TokenTotal: 7, OutputTokens: 3}, apperr.New(apperr.CatReplyFailed, "boom")
	}, nil)
	if _, err := svc.SendCatMessage(context.Background(), cat.SendMessageRequest{ConversationID: conv, Content: "hi"}); err != nil {
		t.Fatal(err)
	}
	svc.Wait()
	ev := waitTerminal(t, rec)
	if ev.Status != catagent.TurnFailed {
		t.Fatalf("%+v", ev)
	}
	if ev.FinishedAt <= 0 || ev.TokenTotal == nil || *ev.TokenTotal != 7 {
		t.Fatalf("%+v", ev)
	}
	if ev.OutputTokens == nil || *ev.OutputTokens != 3 || ev.TokensPerSec == nil || *ev.TokensPerSec <= 0 {
		t.Fatalf("partial out/tps %+v %+v", ev.OutputTokens, ev.TokensPerSec)
	}
}

func TestFooterPersistedIntoGetConversation(t *testing.T) {
	wires := []catagent.WireMessage{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "你好"},
	}
	svc, _, conv, _ := setupFooter(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		o.OnTextDelta("你好")
		time.Sleep(20 * time.Millisecond)
		return catagent.TurnResponse{
			Message: catagent.WireMessage{Content: "你好"}, TokenTotal: 50, OutputTokens: 20,
		}, nil
	}, wires)
	if _, err := svc.SendCatMessage(context.Background(), cat.SendMessageRequest{ConversationID: conv, Content: "hi"}); err != nil {
		t.Fatal(err)
	}
	svc.Wait()
	d, err := svc.GetCatConversation(context.Background(), conv)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Messages) != 2 {
		t.Fatalf("%+v", d.Messages)
	}
	asst := d.Messages[1]
	if asst.Role != "assistant" || asst.FinishedAt <= 0 {
		t.Fatalf("%+v", asst)
	}
	if asst.TokenTotal == nil || *asst.TokenTotal != 50 {
		t.Fatalf("token %+v", asst.TokenTotal)
	}
	if asst.OutputTokens == nil || *asst.OutputTokens != 20 {
		t.Fatalf("out %+v", asst.OutputTokens)
	}
	if asst.TokensPerSec == nil || *asst.TokensPerSec <= 0 {
		t.Fatalf("tps %+v", asst.TokensPerSec)
	}
	// user 无尾字段
	if d.Messages[0].FinishedAt != 0 || d.Messages[0].TokenTotal != nil {
		t.Fatalf("user must omit footer: %+v", d.Messages[0])
	}
}

func TestFooterOldMessagesNoFakeData(t *testing.T) {
	wires := []catagent.WireMessage{
		{Role: "user", Content: "old"},
		{Role: "assistant", Content: "legacy"},
	}
	svc, _, conv, _ := setupFooter(t, func(catagent.TurnOptions) (catagent.TurnResponse, error) {
		return catagent.TurnResponse{Message: catagent.WireMessage{Content: "x"}}, nil
	}, wires)
	// 不发消息 → 无 footer 文件
	d, err := svc.GetCatConversation(context.Background(), conv)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Messages) != 2 {
		t.Fatalf("%+v", d.Messages)
	}
	if d.Messages[1].FinishedAt != 0 || d.Messages[1].TokenTotal != nil || d.Messages[1].TokensPerSec != nil {
		t.Fatalf("must not invent footer: %+v", d.Messages[1])
	}
}

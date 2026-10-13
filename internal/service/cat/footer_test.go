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
			Message:    catagent.WireMessage{Role: "assistant", Content: "你好"},
			TokenTotal: 120,
			DurationMs: 2000,
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
	if ev.TokensPerSec == nil || *ev.TokensPerSec != 60 {
		t.Fatalf("tokensPerSec %+v want 60", ev.TokensPerSec)
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
	if run.FinishedAt != 0 || run.TokenTotal != nil || run.TokensPerSec != nil {
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
	if ev.FinishedAt <= 0 || ev.TokenTotal != nil || ev.TokensPerSec != nil {
		t.Fatalf("want only finishedAt: %+v", ev)
	}
}

func TestFooterWallClockWhenNoCLIDuration(t *testing.T) {
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
	if ev.TokensPerSec == nil || *ev.TokensPerSec <= 0 {
		t.Fatalf("wall-clock tokensPerSec missing: %+v", ev)
	}
}

func TestFooterFailedPartialUsage(t *testing.T) {
	svc, rec, conv, _ := setupFooter(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		o.OnTextDelta("半句")
		return catagent.TurnResponse{TokenTotal: 7, DurationMs: 1000}, apperr.New(apperr.CatReplyFailed, "boom")
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
	if ev.TokensPerSec == nil || *ev.TokensPerSec != 7 {
		t.Fatalf("tps %+v", ev.TokensPerSec)
	}
}

func TestFooterPersistedIntoGetConversation(t *testing.T) {
	wires := []catagent.WireMessage{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "你好"},
	}
	svc, _, conv, _ := setupFooter(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		o.OnTextDelta("你好")
		return catagent.TurnResponse{
			Message: catagent.WireMessage{Content: "你好"}, TokenTotal: 50, DurationMs: 1000,
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
	if asst.TokensPerSec == nil || *asst.TokensPerSec != 50 {
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

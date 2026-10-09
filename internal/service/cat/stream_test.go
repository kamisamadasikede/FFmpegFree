package cat_test

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/service/cat"
	"FFmpegFree/internal/store"
)

// fakeAdapter 是可控的就绪适配器。
type fakeAdapter struct {
	run func(opts catagent.TurnOptions) (catagent.TurnResponse, error)
}

func (f *fakeAdapter) Kind() string           { return catagent.KindCatBuild }
func (f *fakeAdapter) ExecutablePath() string { return "" }
func (f *fakeAdapter) ProtocolVersion() int   { return 1 }
func (f *fakeAdapter) Status() catagent.Status {
	return catagent.Status{State: catagent.StateReady}
}
func (f *fakeAdapter) Recheck() catagent.Status                        { return f.Status() }
func (f *fakeAdapter) ListModels() ([]catagent.Model, error)           { return nil, nil }
func (f *fakeAdapter) ListThinkLevels() ([]catagent.ThinkLevel, error) { return nil, nil }
func (f *fakeAdapter) RunTurn(o catagent.TurnOptions) (catagent.TurnResponse, error) {
	return f.run(o)
}

type recorder struct {
	mu     sync.Mutex
	msgs   []catagent.MessageEvent
	turns  []catagent.TurnEvent
	order  []string
	turnCh chan catagent.TurnEvent
}

func (r *recorder) emit(event string, payload any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch event {
	case catagent.EventMessage:
		ev := payload.(catagent.MessageEvent)
		r.msgs = append(r.msgs, ev)
		r.order = append(r.order, "msg:"+ev.Op)
	case catagent.EventTurn:
		ev := payload.(catagent.TurnEvent)
		r.turns = append(r.turns, ev)
		r.order = append(r.order, "turn:"+ev.Status)
		if r.turnCh != nil {
			select {
			case r.turnCh <- ev:
			default:
			}
		}
	}
}

func setupFake(t *testing.T, run func(catagent.TurnOptions) (catagent.TurnResponse, error)) (*cat.Service, *recorder, string) {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	reg := catagent.NewRegistry()
	reg.Register(&fakeAdapter{run: run})
	rec := &recorder{turnCh: make(chan catagent.TurnEvent, 16)}
	svc := cat.New(cat.Config{Store: st, Registry: reg, Emit: rec.emit})
	c, err := svc.CreateCatConversation(context.Background(), cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild})
	if err != nil {
		t.Fatal(err)
	}
	return svc, rec, c.ID
}

func checkSeq(t *testing.T, evs []catagent.MessageEvent) {
	t.Helper()
	last := map[string]int{}
	done := map[string]bool{}
	for _, e := range evs {
		if done[e.MessageID] {
			t.Fatalf("event after done: %+v", e)
		}
		if e.Seq <= last[e.MessageID] {
			t.Fatalf("seq not increasing: %+v", evs)
		}
		last[e.MessageID] = e.Seq
		if e.Op == catagent.OpDone {
			done[e.MessageID] = true
		}
	}
}

func TestStreamCompletedChunksWholeReply(t *testing.T) {
	long := strings.Repeat("猫", 100)
	svc, rec, conv := setupFake(t, func(catagent.TurnOptions) (catagent.TurnResponse, error) {
		return catagent.TurnResponse{Message: catagent.WireMessage{Role: "assistant", Content: long}}, nil
	})
	res, err := svc.SendCatMessage(context.Background(), cat.SendMessageRequest{ConversationID: conv, Content: "hi"})
	if err != nil || res.TurnID == "" {
		t.Fatalf("%+v %v", res, err)
	}
	svc.Wait()
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.turns) != 2 || rec.turns[0].Status != catagent.TurnRunning || rec.turns[1].Status != catagent.TurnCompleted {
		t.Fatalf("turns %+v", rec.turns)
	}
	for _, te := range rec.turns {
		if te.ConvID != conv || te.TurnID != res.TurnID {
			t.Fatalf("turn ids %+v", te)
		}
	}
	checkSeq(t, rec.msgs)
	var sb strings.Builder
	appends := 0
	for _, e := range rec.msgs {
		if e.TurnID != res.TurnID || e.ConvID != conv {
			t.Fatalf("msg ids %+v", e)
		}
		if e.Op == catagent.OpAppend {
			appends++
			sb.WriteString(e.TextDelta)
		}
	}
	if appends < 2 || sb.String() != long || rec.msgs[0].Seq != 1 || rec.msgs[len(rec.msgs)-1].Op != catagent.OpDone {
		t.Fatalf("appends=%d msgs=%+v", appends, rec.msgs)
	}
	if rec.order[len(rec.order)-1] != "turn:completed" {
		t.Fatalf("order %v", rec.order)
	}
	d, _ := svc.GetCatConversation(context.Background(), conv)
	if len(d.Messages) != 2 || d.Messages[1].ID != rec.msgs[0].MessageID || d.Messages[1].Content != long {
		t.Fatalf("%+v", d.Messages)
	}
}

func TestStreamRealDeltas(t *testing.T) {
	svc, rec, conv := setupFake(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		o.OnTextDelta("你")
		o.OnTextDelta("好")
		return catagent.TurnResponse{Message: catagent.WireMessage{Content: "ignored"}}, nil
	})
	if _, err := svc.SendCatMessage(context.Background(), cat.SendMessageRequest{ConversationID: conv, Content: "hi"}); err != nil {
		t.Fatal(err)
	}
	svc.Wait()
	d, _ := svc.GetCatConversation(context.Background(), conv)
	if len(d.Messages) != 2 || d.Messages[1].Content != "你好" {
		t.Fatalf("%+v", d.Messages)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	checkSeq(t, rec.msgs)
	if len(rec.msgs) != 3 {
		t.Fatalf("%+v", rec.msgs)
	}
}

func TestStreamCancelKeepsPartialAndIsIdempotent(t *testing.T) {
	started := make(chan struct{})
	svc, rec, conv := setupFake(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		o.OnTextDelta("已出")
		close(started)
		<-o.Ctx.Done()
		o.OnTextDelta("迟到") // 取消后的迟到增量要被丢弃
		return catagent.TurnResponse{}, apperr.New(apperr.Canceled, "已取消")
	})
	res, err := svc.SendCatMessage(context.Background(), cat.SendMessageRequest{ConversationID: conv, Content: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	// turnId 不匹配：no-op
	if err := svc.CancelCatTurn(context.Background(), cat.CancelTurnRequest{ConvID: conv, TurnID: "other"}); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-rec.turnCh: // running
		if ev.Status != catagent.TurnRunning {
			t.Fatalf("%+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("no running")
	}
	if err := svc.CancelCatTurn(context.Background(), cat.CancelTurnRequest{ConvID: conv, TurnID: res.TurnID}); err != nil {
		t.Fatal(err)
	}
	// 立即：Cancel 返回时已发 cancelled
	select {
	case ev := <-rec.turnCh:
		if ev.Status != catagent.TurnCancelled || ev.TurnID != res.TurnID {
			t.Fatalf("%+v", ev)
		}
	default:
		t.Fatal("cancelled not emitted synchronously")
	}
	if err := svc.CancelCatTurn(context.Background(), cat.CancelTurnRequest{ConvID: conv, TurnID: res.TurnID}); err != nil {
		t.Fatal(err)
	}
	svc.Wait()
	rec.mu.Lock()
	checkSeq(t, rec.msgs)
	want := "msg:append,msg:done,turn:cancelled"
	got := strings.Join(rec.order[1:], ",")
	rec.mu.Unlock()
	if got != want || len(rec.turns) != 2 {
		t.Fatalf("got %s turns %+v", got, rec.turns)
	}
	d, _ := svc.GetCatConversation(context.Background(), conv)
	if len(d.Messages) != 2 || d.Messages[1].Content != "已出" {
		t.Fatalf("%+v", d.Messages)
	}
}

func TestStreamFailed(t *testing.T) {
	svc, rec, conv := setupFake(t, func(catagent.TurnOptions) (catagent.TurnResponse, error) {
		return catagent.TurnResponse{}, apperr.New(apperr.CatReplyFailed, catagent.MsgReplyFailed)
	})
	if _, err := svc.SendCatMessage(context.Background(), cat.SendMessageRequest{ConversationID: conv, Content: "hi"}); err != nil {
		t.Fatal(err)
	}
	svc.Wait()
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if strings.Join(rec.order, ",") != "turn:running,turn:failed" {
		t.Fatalf("%v", rec.order)
	}
	d, _ := svc.GetCatConversation(context.Background(), conv)
	if len(d.Messages) != 1 {
		t.Fatalf("%+v", d.Messages)
	}
}

func TestStreamEmptyReplyFails(t *testing.T) {
	svc, rec, conv := setupFake(t, func(catagent.TurnOptions) (catagent.TurnResponse, error) {
		return catagent.TurnResponse{}, nil
	})
	if _, err := svc.SendCatMessage(context.Background(), cat.SendMessageRequest{ConversationID: conv, Content: "hi"}); err != nil {
		t.Fatal(err)
	}
	svc.Wait()
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.turns[len(rec.turns)-1].Status != catagent.TurnFailed {
		t.Fatalf("%+v", rec.turns)
	}
}

func TestCancelValidation(t *testing.T) {
	svc, _, _ := setupFake(t, func(catagent.TurnOptions) (catagent.TurnResponse, error) {
		return catagent.TurnResponse{}, nil
	})
	if err := svc.CancelCatTurn(context.Background(), cat.CancelTurnRequest{}); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("%v", err)
	}
	if err := svc.CancelCatTurn(context.Background(), cat.CancelTurnRequest{ConvID: "x", TurnID: "y"}); err != nil {
		t.Fatalf("%v", err)
	}
}

package cat_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/service/cat"
)

func TestOrdinaryTurnNotifyCompletedFailedNotCancelled(t *testing.T) {
	ctx := context.Background()
	n := &notes{}
	var fg atomic.Bool
	fg.Store(false)
	svc := newNotifySvc(t, n, fg.Load, &fakeAdapter{run: func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		return catagent.TurnResponse{Message: catagent.WireMessage{Role: "assistant", Content: "ok"}}, nil
	}})

	c, err := svc.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild, Title: "问候"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SendCatMessage(ctx, cat.SendMessageRequest{ConversationID: c.ID, Content: "hi"}); err != nil {
		t.Fatal(err)
	}
	waitNotify(t, n, 1)
	got := n.all()
	if got[0].Title != "Cat 已回复" || got[0].ConvID != c.ID {
		t.Fatalf("completed: %+v", got)
	}

	n2 := &notes{}
	svc2 := newNotifySvc(t, n2, fg.Load, &fakeAdapter{run: func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		return catagent.TurnResponse{}, apperr.New(apperr.CatReplyFailed, "boom")
	}})
	c2, _ := svc2.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild})
	_, _ = svc2.SendCatMessage(ctx, cat.SendMessageRequest{ConversationID: c2.ID, Content: "x"})
	waitNotify(t, n2, 1)
	if n2.all()[0].Title != "Cat 回复失败" {
		t.Fatalf("failed: %+v", n2.all())
	}

	n3 := &notes{}
	gate := newGate(true)
	svc3 := newNotifySvc(t, n3, fg.Load, gate.adapter())
	c3, _ := svc3.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild})
	res, err := svc3.SendCatMessage(ctx, cat.SendMessageRequest{ConversationID: c3.ID, Content: "x"})
	if err != nil {
		t.Fatal(err)
	}
	_ = svc3.CancelCatTurn(ctx, cat.CancelTurnRequest{ConvID: c3.ID, TurnID: res.TurnID})
	close(gate.release)
	svc3.Wait()
	time.Sleep(100 * time.Millisecond)
	if len(n3.all()) != 0 {
		t.Fatalf("cancelled must not notify: %+v", n3.all())
	}
}

func TestOrdinaryTurnNotifyRespectsSettingAndForeground(t *testing.T) {
	ctx := context.Background()
	n := &notes{}
	var fg atomic.Bool
	fg.Store(true)
	svc := newNotifySvc(t, n, fg.Load, &fakeAdapter{run: func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		return catagent.TurnResponse{Message: catagent.WireMessage{Role: "assistant", Content: "ok"}}, nil
	}})
	c, _ := svc.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild})
	_, _ = svc.SendCatMessage(ctx, cat.SendMessageRequest{ConversationID: c.ID, Content: "hi"})
	svc.Wait()
	time.Sleep(80 * time.Millisecond)
	if len(n.all()) != 0 {
		t.Fatalf("foreground: %+v", n.all())
	}

	n2 := &notes{}
	fg.Store(false)
	off := false
	svc2 := newNotifySvc(t, n2, fg.Load, &fakeAdapter{run: func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		return catagent.TurnResponse{Message: catagent.WireMessage{Role: "assistant", Content: "ok"}}, nil
	}})
	if _, err := svc2.SaveCatSettings(ctx, cat.Settings{NotifyOnFinish: &off}); err != nil {
		t.Fatal(err)
	}
	c2, _ := svc2.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild})
	_, _ = svc2.SendCatMessage(ctx, cat.SendMessageRequest{ConversationID: c2.ID, Content: "hi"})
	svc2.Wait()
	time.Sleep(80 * time.Millisecond)
	if len(n2.all()) != 0 {
		t.Fatalf("notify off: %+v", n2.all())
	}
}

func TestOpenFromNotificationTurnEmitsCatNotify(t *testing.T) {
	var got []any
	svc, _ := newProjSvc(t, openStore(t), nil, cat.Config{
		Emit: func(event string, payload any) {
			if event == cat.EventNotify {
				got = append(got, payload)
			}
		},
	})
	svc.OpenFromNotification("", "conv-1")
	if len(got) != 1 {
		t.Fatalf("%+v", got)
	}
	if got[0].(cat.NotifyEvent).OpenConvID != "conv-1" {
		t.Fatalf("%+v", got[0])
	}
	// 定时仍走 cat:schedule
	var sched int
	svc2, _ := newProjSvc(t, openStore(t), nil, cat.Config{
		Emit: func(event string, payload any) {
			if event == cat.EventSchedule {
				sched++
			}
			if event == cat.EventNotify {
				t.Fatalf("schedule click should not emit cat:notify")
			}
		},
	})
	svc2.OpenFromNotification("sched-1", "conv-2")
	if sched != 1 {
		t.Fatalf("sched=%d", sched)
	}
}

func newNotifySvc(t *testing.T, n *notes, fg func() bool, ad catagent.Adapter) *cat.Service {
	t.Helper()
	st := openStore(t)
	cfg := cat.Config{Notify: n.add, Foreground: fg, DataRoot: t.TempDir()}
	svc, _ := newProjSvc(t, st, ad, cfg)
	t.Cleanup(func() { svc.Wait() })
	return svc
}

func waitNotify(t *testing.T, n *notes, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(n.all()) >= want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("want %d notifies, got %+v", want, n.all())
}

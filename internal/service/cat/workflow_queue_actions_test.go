package cat_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/service/cat"
)

func TestWorkflowQueueEditHoldReorderAndSend(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)
	reg := catagent.NewRegistry()
	started := make(chan catagent.TurnOptions, 4)
	release := make(chan struct{}, 4)
	reg.Register(&fakeAdapter{run: func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		started <- o
		select {
		case <-release:
			return catagent.TurnResponse{Message: catagent.WireMessage{Role: "assistant", Content: "ok"}}, nil
		case <-o.Ctx.Done():
			return catagent.TurnResponse{}, o.Ctx.Err()
		}
	}})
	cfg := cat.Config{Store: st, Registry: reg, DataRoot: t.TempDir(), CLIHome: t.TempDir()}
	s := cat.New(cfg)
	c, e := s.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild})
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.SendCatMessage(ctx, cat.SendMessageRequest{ConversationID: c.ID, Content: "first"})
	if e != nil {
		t.Fatal(e)
	}
	<-started
	q, e := s.QueueCatMessage(ctx, cat.SendMessageRequest{ConversationID: c.ID, Content: "second"})
	if e != nil {
		t.Fatal(e)
	}
	first := q.Items[0].ID
	q, e = s.QueueCatMessage(ctx, cat.SendMessageRequest{ConversationID: c.ID, Content: "third"})
	if e != nil {
		t.Fatal(e)
	}
	third := q.Items[1].ID
	q, e = s.UpdateCatMessageQueue(ctx, cat.QueueAction{ConvID: c.ID, ID: third, BeforeID: first, Action: "move"})
	if e != nil || q.Items[0].ID != third {
		t.Fatalf("%+v %v", q, e)
	}
	q, e = s.UpdateCatMessageQueue(ctx, cat.QueueAction{ConvID: c.ID, ID: third, Action: "edit-start"})
	if e != nil || !q.Items[0].Held {
		t.Fatalf("%+v %v", q, e)
	}
	release <- struct{}{}
	s.Wait()
	select {
	case <-started:
		t.Fatal("edited draft was dispatched before save")
	default:
	}
	restored, e := cat.New(cfg).GetCatMessageQueue(ctx, c.ID)
	if e != nil || !restored.Items[0].Held || !restored.Paused {
		t.Fatalf("%+v %v", restored, e)
	}
	q, e = s.UpdateCatMessageQueue(ctx, cat.QueueAction{ConvID: c.ID, Action: "queue-off"})
	if e != nil || !q.QueueingDisabled {
		t.Fatalf("%+v %v", q, e)
	}
	q, e = s.UpdateCatMessageQueue(ctx, cat.QueueAction{ConvID: c.ID, ID: third, Action: "edit-send", Content: "edited third"})
	if e != nil {
		t.Fatal(e)
	}
	select {
	case o := <-started:
		if o.Messages[0].Content != "edited third" {
			t.Fatal(o.Messages)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("quick submit did not start")
	}
	if q.QueueingDisabled {
		t.Fatal("explicit send must resume queue")
	}
	_, _ = s.UpdateCatMessageQueue(ctx, cat.QueueAction{ConvID: c.ID, Action: "queue-off"})
	release <- struct{}{}
	s.Wait()
	q, e = s.GetCatMessageQueue(ctx, c.ID)
	if e != nil || !q.Paused || !q.QueueingDisabled || len(q.Items) != 1 || q.Items[0].ID != first {
		t.Fatalf("%+v %v", q, e)
	}
}

func TestWorkflowQueueNativeSteerKeepsProcessAndRejectedDraft(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)
	reg := catagent.NewRegistry()
	release := make(chan struct{})
	started := make(chan catagent.TurnOptions, 2)
	var mu sync.Mutex
	var messages []string
	reg.Register(&fakeAdapter{run: func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		o.OnSteerReady(func(_ context.Context, text string, _ []catagent.PromptFile) error {
			if text == "reject" {
				return errors.New("stale")
			}
			mu.Lock()
			messages = append(messages, text)
			mu.Unlock()
			return nil
		})
		defer o.OnSteerReady(nil)
		started <- o
		<-release
		return catagent.TurnResponse{Message: catagent.WireMessage{Role: "assistant", Content: "ok"}}, nil
	}})
	s := cat.New(cat.Config{Store: st, Registry: reg, DataRoot: t.TempDir(), CLIHome: t.TempDir()})
	c, e := s.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild})
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.SendCatMessage(ctx, cat.SendMessageRequest{ConversationID: c.ID, Content: "initial"})
	if e != nil {
		t.Fatal(e)
	}
	running := <-started
	q, e := s.QueueCatMessage(ctx, cat.SendMessageRequest{ConversationID: c.ID, Content: "reject"})
	if e != nil {
		t.Fatal(e)
	}
	id := q.Items[0].ID
	if !q.CanSteer {
		t.Fatal("native capability not exposed")
	}
	_, e = s.UpdateCatMessageQueue(ctx, cat.QueueAction{ConvID: c.ID, ID: id, Action: "steer"})
	if e == nil {
		t.Fatal("rejection ignored")
	}
	q, e = s.GetCatMessageQueue(ctx, c.ID)
	if e != nil || len(q.Items) != 1 {
		t.Fatalf("rejected draft lost: %+v %v", q, e)
	}
	q, e = s.UpdateCatMessageQueue(ctx, cat.QueueAction{ConvID: c.ID, ID: id, Action: "edit-steer", Content: "补充"})
	if e != nil || len(q.Items) != 0 {
		t.Fatalf("%+v %v", q, e)
	}
	_, e = s.UpdateCatMessageQueue(ctx, cat.QueueAction{ConvID: c.ID, Action: "steer-new", Request: &cat.SendMessageRequest{ConversationID: c.ID, Content: "继续"}})
	if e != nil {
		t.Fatal(e)
	}
	mu.Lock()
	got := append([]string{}, messages...)
	mu.Unlock()
	if len(got) != 2 || got[0] != "补充" || got[1] != "继续" {
		t.Fatal(got)
	}
	if running.Ctx.Err() != nil {
		t.Fatal("steer interrupted running task")
	}
	select {
	case <-started:
		t.Fatal("steer started another process")
	default:
	}
	close(release)
	s.Wait()
}

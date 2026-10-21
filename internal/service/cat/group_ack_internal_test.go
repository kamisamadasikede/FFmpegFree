package cat

import (
	"context"
	"strings"
	"sync"
	"testing"

	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/store"
)

func TestGroupAcknowledgementPrecedesExecutionAndPersists(t *testing.T) {
	ctx := context.Background()
	started := make(chan catagent.TurnOptions, 1)
	emitted := make(chan store.CatGroupMessage, 8)
	release := make(chan struct{})
	var once sync.Once
	s, g, ms, _ := groupWorkTestService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		started <- o
		select {
		case <-release:
			return catagent.TurnResponse{Message: catagent.WireMessage{Content: "检查完成，这是实际结果。"}}, nil
		case <-o.Ctx.Done():
			return catagent.TurnResponse{}, o.Ctx.Err()
		}
	})
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	s.cfg.Emit = func(event string, payload any) {
		if event == "cat:group-message" {
			if msg, ok := payload.(store.CatGroupMessage); ok && msg.SystemKind == "ack" {
				emitted <- msg
			}
		}
	}
	sent, err := s.SendCatGroupMessage(ctx, SendGroupMessageRequest{GroupID: g.ID, Text: "检查当前界面", Mentions: []string{ms[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	call := nextGroupCall(t, started)
	var ack store.CatGroupMessage
	select {
	case ack = <-emitted:
	default:
		t.Fatal("native execution began before acknowledgement was emitted")
	}
	if ack.Kind != "system" || ack.MemberID != ms[0].ID || ack.ChainID != sent.ChainID || ack.TurnID != call.TurnID || ack.Report != nil || ack.Route != nil {
		t.Fatalf("acknowledgement must identify the running member without claiming delivery: %+v", ack)
	}
	saved, err := s.cfg.Store.GetCatGroupMessage(ctx, g.ID, ack.Seq)
	if err != nil || saved.SystemKind != "ack" || saved.Text != ack.Text || saved.MemberID != ack.MemberID || saved.TurnID != ack.TurnID {
		t.Fatalf("receipt did not survive reload: %+v %v", saved, err)
	}
	messages, err := s.cfg.Store.ListCatGroupMessages(ctx, g.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, msg := range messages {
		if msg.Kind == "member" {
			t.Fatalf("receipt became premature delivery: %+v", msg)
		}
	}
	prompt, readThrough := s.buildMemberPromptSnapshot(ctx, g, ms[1], ms, sent.UserMessage.Seq)
	if strings.Contains(prompt, ack.Text) || !strings.Contains(prompt, "检查当前界面") || readThrough < ack.Seq {
		t.Fatalf("receipt leaked into model history or user request was lost: %s", prompt)
	}
	once.Do(func() { close(release) })
	s.Wait()
	messages, err = s.cfg.Store.ListCatGroupMessages(ctx, g.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	deliveries := 0
	for _, msg := range messages {
		if msg.Kind == "member" {
			deliveries++
			if msg.Seq <= ack.Seq || msg.TurnID != ack.TurnID || msg.Text != "检查完成，这是实际结果。" {
				t.Fatalf("final result must follow receipt: %+v", msg)
			}
		}
	}
	if deliveries != 1 {
		t.Fatalf("got %d final deliveries", deliveries)
	}
}

func TestGroupAcknowledgementReportsQueueAndStopPreventsLateStart(t *testing.T) {
	ctx := context.Background()
	started := make(chan catagent.TurnOptions, 2)
	s, g, ms, _ := groupWorkTestService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		started <- o
		<-o.Ctx.Done()
		return catagent.TurnResponse{}, o.Ctx.Err()
	})
	if _, err := s.SendCatGroupMessage(ctx, SendGroupMessageRequest{GroupID: g.ID, Text: "第一项", Mentions: []string{ms[0].ID}}); err != nil {
		t.Fatal(err)
	}
	nextGroupCall(t, started)
	second, err := s.SendCatGroupMessage(ctx, SendGroupMessageRequest{GroupID: g.ID, Text: "第二项", Mentions: []string{ms[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	waitGroupWork(t, func() bool {
		msgs, _ := s.cfg.Store.ListCatGroupMessages(ctx, g.ID, 0, 100)
		for _, m := range msgs {
			if m.ChainID == second.ChainID && m.SystemKind == "ack" {
				return true
			}
		}
		return false
	})
	messages, err := s.cfg.Store.ListCatGroupMessages(ctx, g.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	queued := 0
	for _, msg := range messages {
		if msg.ChainID == second.ChainID && msg.SystemKind == "ack" {
			queued++
			if msg.MemberID != ms[0].ID || msg.TurnID != "" || !strings.Contains(msg.Text, "排队") {
				t.Fatalf("queue acknowledgement falsely claims execution: %+v", msg)
			}
		}
	}
	if queued != 1 {
		t.Fatalf("got %d queue acknowledgements", queued)
	}
	if err := s.StopCatGroup(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	s.Wait()
	select {
	case <-started:
		t.Fatal("stopped queue started another turn")
	default:
	}
	messages, err = s.cfg.Store.ListCatGroupMessages(ctx, g.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, msg := range messages {
		if msg.ChainID == second.ChainID && msg.SystemKind == "ack" && msg.TurnID != "" {
			t.Fatalf("stopped queued request received a late start acknowledgement: %+v", msg)
		}
	}
}

func TestGroupAcknowledgementDoesNotCompleteWorkflow(t *testing.T) {
	ctx := context.Background()
	started := make(chan catagent.TurnOptions, 2)
	s, g, ms, _ := groupWorkTestService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		started <- o
		<-o.Ctx.Done()
		return catagent.TurnResponse{}, o.Ctx.Err()
	})
	msg, err := s.cfg.Store.InsertCatGroupMessage(ctx, store.CatGroupMessage{GroupID: g.ID, Kind: "user", Text: "截取当前界面并验收", ChainID: "ack-workflow"})
	if err != nil {
		t.Fatal(err)
	}
	tasks := []store.CatGroupWorkTask{
		{ID: "capture", Title: "截取当前界面", MemberID: ms[0].ID, Instruction: "截图", Kind: "develop", Status: "pending"},
		{ID: "verify", Title: "验收截图", MemberID: ms[1].ID, Instruction: "验收", Kind: "verify", Status: "pending", DependsOn: []string{"capture"}},
	}
	if err := s.startGroupWorkflow(ctx, g, msg, tasks); err != nil {
		t.Fatal(err)
	}
	call := nextGroupCall(t, started)
	work, err := s.cfg.Store.GetCatGroupWorkflow(ctx, msg.ChainID)
	if err != nil {
		t.Fatal(err)
	}
	if work.Status != "running" || work.Tasks[0].Status != "running" || work.Tasks[1].Status != "pending" || work.Tasks[0].Result != "" {
		t.Fatalf("receipt advanced workflow: %+v", work)
	}
	messages, err := s.cfg.Store.ListCatGroupMessages(ctx, g.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, item := range messages {
		if item.SystemKind == "ack" {
			count++
			if item.TurnID != call.TurnID || !strings.Contains(item.Text, "截取当前界面") {
				t.Fatalf("receipt missing task context: %+v", item)
			}
		}
		if item.Kind == "member" || item.SystemKind == "workflow_summary" {
			t.Fatalf("premature result: %+v", item)
		}
	}
	if count != 1 {
		t.Fatalf("got %d receipts before first task finished", count)
	}
}

func TestGroupAcknowledgementRequiresRunnableTurn(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		name := "unavailable"
		if cancelled {
			name = "cancelled"
		}
		t.Run(name, func(t *testing.T) {
			s, g, ms, _ := groupWorkTestService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
				t.Error("invalid turn reached native execution")
				return catagent.TurnResponse{}, nil
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if cancelled {
				cancel()
			} else {
				s.cfg.Registry.Register(&routeUnavailableAdapter{&groupWorkAdapter{kind: ms[0].AgentKind}})
			}
			s.wakeMember(ctx, g, ms[0].ID, "invalid-ack", 0, 0, false, "mention")
			s.Wait()
			messages, err := s.cfg.Store.ListCatGroupMessages(context.Background(), g.ID, 0, 100)
			if err != nil {
				t.Fatal(err)
			}
			for _, msg := range messages {
				if msg.SystemKind == "ack" {
					t.Fatalf("unrunnable task promised execution: %+v", msg)
				}
			}
		})
	}
}

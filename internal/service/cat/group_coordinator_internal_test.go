package cat

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/store"
)

// Unlike the executor fixtures, these tests explicitly drive real coordination
// for @ messages too. JSONSchema identifies routing, not tool permissions.
func coordinatorTestService(t *testing.T, run func(catagent.TurnOptions) (catagent.TurnResponse, error)) (*Service, store.CatGroup, []store.CatGroupMember) {
	t.Helper()
	s, g, ms, _ := groupWorkTestService(t, run)
	for _, kind := range catagent.CreatableKinds() {
		s.cfg.Registry.Register(&groupWorkAdapter{kind: kind, run: run})
	}
	return s, g, ms
}
func coordinatorAnswer(member, cap string) catagent.TurnResponse {
	b, _ := json.Marshal(map[string]any{"memberIds": []string{member}, "everyone": false, "reason": "按职责安排", "tasks": []any{}, "capability": cap})
	return catagent.TurnResponse{Message: catagent.WireMessage{Content: string(b)}}
}
func TestCoordinatorMentionAnalyzedAsynchronouslyAndDispatchedOnce(t *testing.T) {
	ctx := context.Background()
	routeStarted := make(chan catagent.TurnOptions, 1)
	release := make(chan struct{})
	finish := sync.OnceFunc(func() { close(release) })
	defer finish()
	calls := make(chan catagent.TurnOptions, 2)
	var target string
	s, g, ms := coordinatorTestService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		if o.JSONSchema == catagent.RouteSchema {
			routeStarted <- o
			select {
			case <-release:
				return coordinatorAnswer(target, "read_only"), nil
			case <-o.Ctx.Done():
				return catagent.TurnResponse{}, o.Ctx.Err()
			}
		}
		calls <- o
		return catagent.TurnResponse{Message: catagent.WireMessage{Content: "我来检查数据库故障"}}, nil
	})
	target = ms[1].ID
	sent, err := s.SendCatGroupMessage(ctx, SendGroupMessageRequest{GroupID: g.ID, Text: "查数据库故障", Mentions: []string{ms[0].ID}, ClientMessageID: "client-once"})
	if err != nil {
		t.Fatal(err)
	}
	route := nextGroupCall(t, routeStarted)
	if !strings.Contains(route.Messages[0].Content, ms[0].ID) || !strings.Contains(route.Messages[0].Content, "查数据库故障") {
		t.Fatal("mention intent missing")
	}
	r, err := s.cfg.Store.GetCatGroupRequest(ctx, sent.ChainID)
	if err != nil || r.Status != "routing" || r.FirstReplyAt != 0 {
		t.Fatalf("bad receipt %+v %v", r, err)
	}
	duplicate, err := s.SendCatGroupMessage(ctx, SendGroupMessageRequest{GroupID: g.ID, Text: "查数据库故障", Mentions: []string{ms[0].ID}, ClientMessageID: "client-once"})
	if err != nil || duplicate.ChainID != sent.ChainID {
		t.Fatal("duplicate receipt", err)
	}
	finish()
	s.Wait()
	if len(calls) != 1 {
		t.Fatalf("dispatch count %d", len(calls))
	}
	call := <-calls
	if call.ConversationID != target || call.Capability != "read_only" {
		t.Fatalf("bad assignee %+v", call)
	}
	r, _ = s.cfg.Store.GetCatGroupRequest(ctx, sent.ChainID)
	if r.Status != "answered" || r.FirstReplyAt == 0 {
		t.Fatalf("no effective answer %+v", r)
	}
}
func TestCoordinatorPinsExplicitRecipientAndReadOnlyCeiling(t *testing.T) {
	ctx := context.Background()
	var other string
	calls := make(chan catagent.TurnOptions, 2)
	s, g, ms := coordinatorTestService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		if o.JSONSchema == catagent.RouteSchema {
			return coordinatorAnswer(other, "write"), nil
		}
		calls <- o
		return catagent.TurnResponse{Message: catagent.WireMessage{Content: "只分析结果"}}, nil
	})
	other = ms[1].ID
	sent, err := s.SendCatGroupMessage(ctx, SendGroupMessageRequest{GroupID: g.ID, Text: "只让你分析，不要修改，不要转交", Mentions: []string{ms[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	s.Wait()
	if len(calls) != 1 {
		t.Fatalf("calls=%d", len(calls))
	}
	call := <-calls
	if call.ConversationID != ms[0].ID || call.Capability != "read_only" {
		t.Fatalf("restriction bypassed %+v", call)
	}
	a, err := s.cfg.Store.GetCatGroupAssignment(ctx, groupAssignmentID(sent.ChainID, ms[0].ID, ""))
	if err != nil || a.Capability != "read_only" || a.Status != "done" {
		t.Fatalf("bad persisted capability %+v %v", a, err)
	}
}
func TestCoordinatorFixedInputSurvivesLaterMessagesAndUnreadCursor(t *testing.T) {
	ctx := context.Background()
	var owner string
	firstRoute := make(chan struct{})
	release := make(chan struct{})
	finish := sync.OnceFunc(func() { close(release) })
	defer finish()
	calls := make(chan catagent.TurnOptions, 4)
	var routes atomic.Int32
	s, g, ms := coordinatorTestService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		if o.JSONSchema == catagent.RouteSchema {
			if routes.Add(1) == 1 {
				close(firstRoute)
				select {
				case <-release:
				case <-o.Ctx.Done():
					return catagent.TurnResponse{}, o.Ctx.Err()
				}
			}
			return coordinatorAnswer(owner, "read_only"), nil
		}
		calls <- o
		return catagent.TurnResponse{Message: catagent.WireMessage{Content: "已按独立请求回复"}}, nil
	})
	owner = ms[0].ID
	first, err := s.SendCatGroupMessage(ctx, SendGroupMessageRequest{GroupID: g.ID, Text: "FIRST_REQUEST_UNIQUE"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-firstRoute:
	case <-time.After(5 * time.Second):
		t.Fatal("route not started")
	}
	s.cfg.Store.UpdateCatGroupMemberLastSeen(ctx, owner, 999999)
	second, err := s.SendCatGroupMessage(ctx, SendGroupMessageRequest{GroupID: g.ID, Text: "SECOND_REQUEST_UNIQUE"})
	if err != nil {
		t.Fatal(err)
	}
	// The later request may complete first. The earlier one must not consume it.
	call2 := nextGroupCall(t, calls)
	if !strings.Contains(call2.Messages[0].Content, second.ChainID) {
		t.Fatal("wrong second assignment")
	}
	finish()
	s.Wait()
	call1 := nextGroupCall(t, calls)
	if !strings.Contains(call1.Messages[0].Content, first.ChainID) || !strings.Contains(call1.Messages[0].Content, "FIRST_REQUEST_UNIQUE") || strings.Contains(call1.Messages[0].Content, "SECOND_REQUEST_UNIQUE") {
		t.Fatal("input mixed or lost", call1.Messages[0].Content)
	}
}
func TestCoordinatorStopPendingDecisionCannotRestartOldRequest(t *testing.T) {
	ctx := context.Background()
	var owner string
	started := make(chan struct{})
	release := make(chan struct{})
	finish := sync.OnceFunc(func() { close(release) })
	defer finish()
	calls := make(chan catagent.TurnOptions, 3)
	var routes atomic.Int32
	s, g, ms := coordinatorTestService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		if o.JSONSchema == catagent.RouteSchema {
			if routes.Add(1) == 1 {
				close(started)
				<-release
			} // Simulate an adapter returning after cancellation.
			return coordinatorAnswer(owner, "read_only"), nil
		}
		calls <- o
		return catagent.TurnResponse{Message: catagent.WireMessage{Content: "新请求已回复"}}, nil
	})
	owner = ms[0].ID
	first, err := s.SendCatGroupMessage(ctx, SendGroupMessageRequest{GroupID: g.ID, Text: "旧请求"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("route not started")
	}
	if err = s.StopCatGroup(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	next, err := s.SendCatGroupMessage(ctx, SendGroupMessageRequest{GroupID: g.ID, Text: "新的问题"})
	if err != nil {
		t.Fatal(err)
	}
	finish()
	s.Wait()
	if len(calls) != 1 {
		t.Fatalf("stale dispatch calls=%d", len(calls))
	}
	if !strings.Contains((<-calls).Messages[0].Content, next.ChainID) {
		t.Fatal("wrong request")
	}
	old, _ := s.cfg.Store.GetCatGroupRequest(ctx, first.ChainID)
	as, _ := s.cfg.Store.ListCatGroupAssignments(ctx, first.ChainID)
	if old.Status != "stopped" || len(as) != 0 {
		t.Fatalf("old revived %+v %+v", old, as)
	}
}

func TestCoordinatorReassignQueuedRequestPreservesUnrelatedExecution(t *testing.T) {
	ctx := context.Background()
	started := make(chan catagent.TurnOptions, 4)
	release := make(chan struct{})
	finish := sync.OnceFunc(func() { close(release) })
	defer finish()
	var owner string
	s, g, ms := coordinatorTestService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		if o.JSONSchema == catagent.RouteSchema {
			return coordinatorAnswer(owner, "read_only"), nil
		}
		started <- o
		if strings.Contains(o.Messages[0].Content, "原始用户请求：\n先处理A") {
			select {
			case <-release:
			case <-o.Ctx.Done():
				return catagent.TurnResponse{}, o.Ctx.Err()
			}
		}
		return catagent.TurnResponse{Message: catagent.WireMessage{Content: "完成本条答复"}}, nil
	})
	owner = ms[0].ID
	first, err := s.SendCatGroupMessage(ctx, SendGroupMessageRequest{GroupID: g.ID, Text: "先处理A"})
	if err != nil {
		t.Fatal(err)
	}
	a := nextGroupCall(t, started)
	second, err := s.SendCatGroupMessage(ctx, SendGroupMessageRequest{GroupID: g.ID, Text: "然后处理B"})
	if err != nil {
		t.Fatal(err)
	}
	waitGroupWork(t, func() bool {
		a, _ := s.cfg.Store.GetCatGroupAssignment(ctx, groupAssignmentID(second.ChainID, owner, ""))
		return a.Status == "enqueued"
	})
	if _, err = s.ReassignCatGroupRoute(ctx, ReassignGroupRouteRequest{GroupID: g.ID, ChainID: second.ChainID, ToMemberID: ms[1].ID}); err != nil {
		t.Fatal(err)
	}
	if a.Ctx.Err() != nil {
		t.Fatal("改派B取消了A")
	}
	finish()
	s.Wait()
	b := nextGroupCall(t, started)
	if b.ConversationID != ms[1].ID || !strings.Contains(b.Messages[0].Content, second.ChainID) {
		t.Fatalf("wrong reassignment: %+v", b)
	}
	if len(started) != 0 {
		t.Fatal("重复执行了排队任务")
	}
	r, _ := s.cfg.Store.GetCatGroupRequest(ctx, first.ChainID)
	if r.Status != "answered" {
		t.Fatalf("unrelated request lost: %+v", r)
	}
	old, _ := s.cfg.Store.GetCatGroupAssignment(ctx, groupAssignmentID(second.ChainID, owner, ""))
	if old.Status != "superseded" {
		t.Fatalf("old assignment not fenced: %+v", old)
	}
}

func TestCoordinatorReassignWaitsForOldProcessAndFencesLateAnswer(t *testing.T) {
	ctx := context.Background()
	started := make(chan catagent.TurnOptions, 3)
	release := make(chan struct{})
	finish := sync.OnceFunc(func() { close(release) })
	defer finish()
	var owner string
	s, g, ms := coordinatorTestService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		if o.JSONSchema == catagent.RouteSchema {
			return coordinatorAnswer(owner, "read_only"), nil
		}
		started <- o
		if o.ConversationID == owner {
			<-release
			return catagent.TurnResponse{Message: catagent.WireMessage{Content: "迟到的旧结果"}}, nil
		}
		return catagent.TurnResponse{Message: catagent.WireMessage{Content: "接手成员答复"}}, nil
	})
	owner = ms[0].ID
	sent, err := s.SendCatGroupMessage(ctx, SendGroupMessageRequest{GroupID: g.ID, Text: "处理这项请求"})
	if err != nil {
		t.Fatal(err)
	}
	old := nextGroupCall(t, started)
	if _, err = s.ReassignCatGroupRoute(ctx, ReassignGroupRouteRequest{GroupID: g.ID, ChainID: sent.ChainID, ToMemberID: ms[1].ID}); err != nil {
		t.Fatal(err)
	}
	if old.Ctx.Err() == nil {
		t.Fatal("old process not cancelled")
	}
	next, _ := s.cfg.Store.GetCatGroupAssignment(ctx, groupAssignmentID(sent.ChainID, ms[1].ID, ""))
	if next.Status != "held" {
		t.Fatalf("replacement released before old exit: %+v", next)
	}
	if err = s.RetryCatGroupMemberTurn(ctx, RetryGroupMemberTurnRequest{GroupID: g.ID, ChainID: sent.ChainID, MemberID: owner}); err == nil {
		t.Fatal("retry reopened old assignment during handoff")
	}
	if _, err = s.ReassignCatGroupRoute(ctx, ReassignGroupRouteRequest{GroupID: g.ID, ChainID: sent.ChainID, ToMemberID: ms[2].ID}); err == nil {
		t.Fatal("second handoff bypassed old process exit")
	}
	select {
	case <-started:
		t.Fatal("new execution started before old exit")
	default:
	}
	finish()
	s.Wait()
	replacement := nextGroupCall(t, started)
	if replacement.ConversationID != ms[1].ID {
		t.Fatal("wrong replacement")
	}
	messages, _ := s.cfg.Store.ListCatGroupMessages(ctx, g.ID, 0, 100)
	replies := 0
	for _, m := range messages {
		if m.Kind == "member" {
			replies++
			if m.Text != "接手成员答复" {
				t.Fatalf("stale reply leaked: %+v", m)
			}
		}
	}
	if replies != 1 {
		t.Fatalf("reply count %d", replies)
	}
}

func TestCoordinatorWatchIncludesRoutingAndQueueWithoutDuplicateDispatch(t *testing.T) {
	ctx := context.Background()
	started := make(chan catagent.TurnOptions, 2)
	release := make(chan struct{})
	finish := sync.OnceFunc(func() { close(release) })
	defer finish()
	var owner string
	s, g, ms := coordinatorTestService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		started <- o
		if o.JSONSchema == catagent.RouteSchema {
			select {
			case <-release:
			case <-o.Ctx.Done():
				return catagent.TurnResponse{}, o.Ctx.Err()
			}
			return coordinatorAnswer(owner, "read_only"), nil
		}
		return catagent.TurnResponse{Message: catagent.WireMessage{Content: "正常回复"}}, nil
	})
	owner = ms[0].ID
	s.cfg.GroupReplyWait = 10 * time.Millisecond
	sent, err := s.SendCatGroupMessage(ctx, SendGroupMessageRequest{GroupID: g.ID, Text: "需要答复"})
	if err != nil {
		t.Fatal(err)
	}
	_ = nextGroupCall(t, started)
	waitGroupWork(t, func() bool {
		msgs, _ := s.cfg.Store.ListCatGroupMessages(ctx, g.ID, 0, 100)
		for _, m := range msgs {
			if m.ChainID == sent.ChainID && m.SystemKind == "reply_wait" {
				return true
			}
		}
		return false
	})
	if len(started) != 0 {
		t.Fatal("watchdog started duplicate work")
	}
	r, _ := s.cfg.Store.GetCatGroupRequest(ctx, sent.ChainID)
	if r.FirstReplyAt != 0 {
		t.Fatal("receipt or wait reminder counted as answer")
	}
	finish()
	s.Wait()
	if len(started) != 1 {
		t.Fatal("execution not dispatched exactly once")
	}
}

func TestCoordinatorManagedWorkflowRetriesAndReworkKeepOriginalInput(t *testing.T) {
	ctx := context.Background()
	var tasks []store.CatGroupWorkTask
	var mu sync.Mutex
	attempts := map[string]int{}
	var capabilities []string
	s, g, ms := coordinatorTestService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		if o.JSONSchema == catagent.RouteSchema {
			raw, _ := json.Marshal(map[string]any{"memberIds": []string{tasks[0].MemberID}, "everyone": false, "reason": "实现并验收", "tasks": tasks, "capability": "write"})
			return catagent.TurnResponse{Message: catagent.WireMessage{Content: string(raw)}}, nil
		}
		tid := "a"
		if strings.Contains(o.Messages[0].Content, "你本轮只负责任务 v：") {
			tid = "v"
		}
		mu.Lock()
		attempts[tid]++
		attempt := attempts[tid]
		capabilities = append(capabilities, o.Capability)
		mu.Unlock()
		if !strings.Contains(o.Messages[0].Content, "原始用户请求：\n修复登录并验收") {
			return catagent.TurnResponse{}, errors.New("missing original request")
		}
		if tid == "a" && attempt == 1 {
			return catagent.TurnResponse{}, errors.New("temporary failure")
		}
		if tid == "v" && attempt == 1 {
			return catagent.TurnResponse{Message: catagent.WireMessage{Content: `需要返工` + groupTaskResultTag + `{"status":"failed","summary":"需要修复","evidence":"test failed","rework":["a"]}</cat_task_result>`}}, nil
		}
		if tid == "v" && o.OnActivity != nil {
			o.OnActivity("tool", "verify", "exec_command\ngo test ./...", true)
		}
		return catagent.TurnResponse{Message: catagent.WireMessage{Content: `已完成` + groupTaskResultTag + `{"status":"done","summary":"已完成","evidence":"go test PASS"}</cat_task_result>`}}, nil
	})
	tasks = []store.CatGroupWorkTask{{ID: "a", Title: "修复", MemberID: ms[0].ID, Instruction: "修复登录", Kind: "develop", DependsOn: []string{}}, {ID: "v", Title: "验收", MemberID: ms[1].ID, Instruction: "运行测试", Kind: "verify", DependsOn: []string{"a"}}}
	sent, err := s.SendCatGroupMessage(ctx, SendGroupMessageRequest{GroupID: g.ID, Text: "修复登录并验收"})
	if err != nil {
		t.Fatal(err)
	}
	s.Wait()
	w, err := s.cfg.Store.GetCatGroupWorkflow(ctx, sent.ChainID)
	if err != nil || w.Status != "completed" {
		t.Fatalf("workflow not completed: %+v %v", w, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if attempts["a"] != 3 || attempts["v"] != 2 {
		t.Fatalf("retry/rework counts %+v", attempts)
	}
	for _, cap := range capabilities {
		if cap != "write" {
			t.Fatal("capability lost during retry/rework", cap)
		}
	}
	assignments, _ := s.cfg.Store.ListCatGroupAssignments(ctx, sent.ChainID)
	if len(assignments) != 2 {
		t.Fatalf("outbox duplicate: %+v", assignments)
	}
	for _, a := range assignments {
		if a.Status != "done" {
			t.Fatalf("unfinished outbox %+v", a)
		}
	}
}

func TestCoordinatorManagedWorkflowReassignCommitsHandoffAndWaitsForOldExit(t *testing.T) {
	ctx := context.Background()
	started := make(chan catagent.TurnOptions, 4)
	release := make(chan struct{})
	finish := sync.OnceFunc(func() { close(release) })
	defer finish()
	var tasks []store.CatGroupWorkTask
	var owner string
	s, g, ms := coordinatorTestService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		if o.JSONSchema == catagent.RouteSchema {
			raw, _ := json.Marshal(map[string]any{"memberIds": []string{owner}, "everyone": false, "reason": "实现并验收", "tasks": tasks, "capability": "write"})
			return catagent.TurnResponse{Message: catagent.WireMessage{Content: string(raw)}}, nil
		}
		started <- o
		if o.ConversationID == owner {
			<-release // Deliberately emulate a process that exits after cancellation.
			return catagent.TurnResponse{Message: catagent.WireMessage{Content: "迟到的旧协作结果"}}, nil
		}
		if o.OnActivity != nil {
			o.OnActivity("tool", "verify", "exec_command\ngo test ./...", true)
		}
		return catagent.TurnResponse{Message: catagent.WireMessage{Content: `已完成` + groupTaskResultTag + `{"status":"done","summary":"已完成","evidence":"go test PASS"}</cat_task_result>`}}, nil
	})
	owner = ms[0].ID
	tasks = []store.CatGroupWorkTask{{ID: "a", Title: "实现", MemberID: owner, Instruction: "修复登录", Kind: "develop", DependsOn: []string{}}, {ID: "v", Title: "验收", MemberID: ms[2].ID, Instruction: "运行测试", Kind: "verify", DependsOn: []string{"a"}}}
	sent, err := s.SendCatGroupMessage(ctx, SendGroupMessageRequest{GroupID: g.ID, Text: "修复登录并验收"})
	if err != nil {
		t.Fatal(err)
	}
	old := nextGroupCall(t, started)
	if _, err = s.ReassignCatGroupRoute(ctx, ReassignGroupRouteRequest{GroupID: g.ID, ChainID: sent.ChainID, TaskID: "a", ToMemberID: ms[1].ID}); err != nil {
		t.Fatal(err)
	}
	if old.Ctx.Err() == nil {
		t.Fatal("old process not cancelled")
	}
	oldAssignment, _ := s.cfg.Store.GetCatGroupAssignment(ctx, groupAssignmentID(sent.ChainID, owner, "a"))
	next, _ := s.cfg.Store.GetCatGroupAssignment(ctx, groupAssignmentID(sent.ChainID, ms[1].ID, "a"))
	w, err := s.cfg.Store.GetCatGroupWorkflow(ctx, sent.ChainID)
	if err != nil || oldAssignment.Status != "superseded" || next.Status != "held" || w.Tasks[0].MemberID != ms[1].ID || w.Tasks[0].Status != "queued" {
		t.Fatalf("handoff not durable: old=%+v next=%+v workflow=%+v err=%v", oldAssignment, next, w, err)
	}
	if next.Capability != "write" || next.Instruction != oldAssignment.Instruction {
		t.Fatal("handoff changed immutable input/capability")
	}
	if _, err = s.ReassignCatGroupRoute(ctx, ReassignGroupRouteRequest{GroupID: g.ID, ChainID: sent.ChainID, TaskID: "a", ToMemberID: ms[2].ID}); err == nil {
		t.Fatal("second handoff bypassed old exit")
	}
	if err = s.RetryCatGroupMemberTurn(ctx, RetryGroupMemberTurnRequest{GroupID: g.ID, ChainID: sent.ChainID, MemberID: owner, TaskID: "a"}); err == nil {
		t.Fatal("retry bypassed handoff")
	}
	select {
	case <-started:
		t.Fatal("replacement started before old exit")
	default:
	}
	finish()
	s.Wait()
	w, err = s.cfg.Store.GetCatGroupWorkflow(ctx, sent.ChainID)
	if err != nil || w.Status != "completed" || w.Tasks[0].MemberID != ms[1].ID || w.Tasks[0].Status != "done" {
		t.Fatalf("handoff did not finish: %+v %v", w, err)
	}
	assignments, _ := s.cfg.Store.ListCatGroupAssignments(ctx, sent.ChainID)
	if len(assignments) != 3 {
		t.Fatalf("unexpected outbox: %+v", assignments)
	}
	for _, a := range assignments {
		if a.MemberID == owner && a.TaskID == "a" {
			if a.Status != "superseded" {
				t.Fatal("old assignment revived", a)
			}
		} else if a.Status != "done" {
			t.Fatal("unfinished assignment", a)
		}
	}
	messages, _ := s.cfg.Store.ListCatGroupMessages(ctx, g.ID, 0, 100)
	for _, m := range messages {
		if m.Kind == "member" && strings.Contains(m.Text, "迟到的旧协作结果") {
			t.Fatal("old workflow reply leaked")
		}
	}
}

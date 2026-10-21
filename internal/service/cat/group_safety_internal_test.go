package cat

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"FFmpegFree/internal/agentexec"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/store"
)

func nextGroupCall(t *testing.T, ch <-chan catagent.TurnOptions) catagent.TurnOptions {
	t.Helper()
	select {
	case o := <-ch:
		return o
	case <-time.After(10 * time.Second):
		t.Fatal("agent did not start")
		return catagent.TurnOptions{}
	}
}

func TestGroupStopCancelsRoutingAndRejectsLateDispatch(t *testing.T) {
	for _, plan := range []bool{false, true} {
		t.Run(fmt.Sprint("workflow=", plan), func(t *testing.T) {
			routing := make(chan catagent.TurnOptions, 1)
			started := make(chan catagent.TurnOptions, 4)
			release := make(chan struct{})
			var once sync.Once
			var members []store.CatGroupMember
			s, g, ms, _ := groupWorkTestService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
				if o.NoTools {
					routing <- o
					<-release // Deliberately ignore cancellation and return a valid late result.
					response := map[string]any{"memberIds": []string{members[0].ID}, "everyone": false, "reason": "开发"}
					if plan {
						response["tasks"] = groupPlanTasks(members)
					}
					b, _ := json.Marshal(response)
					return catagent.TurnResponse{Message: catagent.WireMessage{Content: string(b)}}, nil
				}
				started <- o
				return catagent.TurnResponse{Message: catagent.WireMessage{Content: "已完成"}}, nil
			})
			members = ms
			t.Cleanup(func() { once.Do(func() { close(release) }) })
			done := make(chan error, 1)
			go func() {
				_, err := s.SendCatGroupMessage(context.Background(), SendGroupMessageRequest{GroupID: g.ID, Text: "开始开发"})
				done <- err
			}()
			route := nextGroupCall(t, routing)
			if err := s.StopCatGroup(context.Background(), g.ID); err != nil {
				t.Fatal(err)
			}
			select {
			case <-route.Ctx.Done():
			case <-time.After(time.Second):
				t.Fatal("router was not cancelled")
			}
			once.Do(func() { close(release) })
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("send stuck")
			}
			s.Wait()
			select {
			case <-started:
				t.Fatal("late result started a member after stop")
			default:
			}
			works, err := s.cfg.Store.ListCatGroupWorkflows(context.Background(), g.ID)
			if err != nil || len(works) != 0 {
				t.Fatalf("late plan persisted: %+v %v", works, err)
			}
			// Stopping an old request must not poison an explicit new request.
			if _, err := s.SendCatGroupMessage(context.Background(), SendGroupMessageRequest{GroupID: g.ID, Text: "新任务", Mentions: []string{ms[0].ID}}); err != nil {
				t.Fatal(err)
			}
			nextGroupCall(t, started)
			s.Wait()
		})
	}
}

func TestGroupSharedWorkspaceSerializesAndCapturesOwnChanges(t *testing.T) {
	started := make(chan catagent.TurnOptions, 4)
	release := make(chan struct{})
	var once sync.Once
	s, g, ms, root := groupWorkTestService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		name := o.ConversationID + ".txt"
		if err := os.WriteFile(filepath.Join(o.ProjectPath, name), []byte(o.ConversationID), 0600); err != nil {
			return catagent.TurnResponse{}, err
		}
		o.OnActivity("tool", "write", "Write {\"file_path\":\""+name+"\"}", true)
		started <- o
		select {
		case <-release:
		case <-o.Ctx.Done():
			return catagent.TurnResponse{}, o.Ctx.Err()
		}
		return catagent.TurnResponse{Message: catagent.WireMessage{Content: "已完成"}}, nil
	})
	initSnapshotTestRepo(t, root)
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	if _, err := s.SendCatGroupMessage(context.Background(), SendGroupMessageRequest{GroupID: g.ID, Text: "一起检查", Mentions: []string{ms[0].ID, ms[1].ID}}); err != nil {
		t.Fatal(err)
	}
	first := nextGroupCall(t, started)
	select {
	case <-started:
		t.Fatal("two native writers entered one worktree")
	case <-time.After(150 * time.Millisecond):
	}
	once.Do(func() { close(release) })
	second := nextGroupCall(t, started)
	s.Wait()
	if first.ConversationID == second.ConversationID {
		t.Fatal("same member ran twice")
	}
	for _, o := range []catagent.TurnOptions{first, second} {
		changes, err := s.GetCatTurnChangeDetail(context.Background(), o.ConversationID, o.TurnID)
		if err != nil || len(changes.Files) != 1 || changes.Files[0].Path != o.ConversationID+".txt" {
			t.Fatalf("mixed attribution: %+v %v", changes, err)
		}
		meta, err := s.cfg.Store.GetCatGroupTurnMeta(context.Background(), o.TurnID)
		if err != nil || meta.SnapshotBefore == "" || meta.EndHashes[o.ConversationID+".txt"] == "" {
			t.Fatalf("missing rollback state: %+v %v", meta, err)
		}
	}
}

func TestGroupStopCancelsQueuedNativeTurns(t *testing.T) {
	started := make(chan catagent.TurnOptions, 4)
	s, g, ms, _ := groupWorkTestService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		started <- o
		<-o.Ctx.Done()
		return catagent.TurnResponse{}, o.Ctx.Err()
	})
	if _, err := s.SendCatGroupMessage(context.Background(), SendGroupMessageRequest{GroupID: g.ID, Text: "一起开始", Mentions: []string{ms[0].ID, ms[1].ID}}); err != nil {
		t.Fatal(err)
	}
	nextGroupCall(t, started)
	if _, err := s.SendCatGroupMessage(context.Background(), SendGroupMessageRequest{GroupID: g.ID, Text: "排队", Mentions: []string{ms[0].ID}}); err != nil {
		t.Fatal(err)
	}
	if err := s.StopCatGroup(context.Background(), g.ID); err != nil {
		t.Fatal(err)
	}
	s.Wait()
	select {
	case <-started:
		t.Fatal("queued native turn executed after stop")
	default:
	}
}

func TestGroupPermissionsInheritSettings(t *testing.T) {
	calls := make(chan catagent.TurnOptions, 4)
	s, g, ms, _ := groupWorkTestService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		calls <- o
		return catagent.TurnResponse{Message: catagent.WireMessage{Content: "已完成"}}, nil
	})
	settings := Settings{DefaultModeByAgent: map[string]string{
		catagent.KindCatBuild:  catagent.ModeAuto,
		catagent.KindClaudeCLI: catagent.ModeAcceptEdits,
		catagent.KindCodexCLI:  catagent.ModeCodexWorkspace,
	}}
	if err := s.cfg.Store.SetSetting(context.Background(), SettingsKey, settings); err != nil {
		t.Fatal(err)
	}
	for _, m := range ms {
		if _, err := s.SendCatGroupMessage(context.Background(), SendGroupMessageRequest{GroupID: g.ID, Text: "执行", Mentions: []string{m.ID}}); err != nil {
			t.Fatal(err)
		}
		o := nextGroupCall(t, calls)
		if o.PermissionMode != settings.DefaultModeFor(m.AgentKind) {
			t.Fatalf("mode=%q, want %q", o.PermissionMode, settings.DefaultModeFor(m.AgentKind))
		}
		s.Wait()
	}
}

func TestGroupRevertWaitsForWorkspaceWriter(t *testing.T) {
	s, g, ms, root := groupWorkTestService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) { return catagent.TurnResponse{}, nil })
	initSnapshotTestRepo(t, root)
	before := createGitSnapshot(context.Background(), root)
	writeSnapshotFile(t, root, "tracked.txt", "agent overwrite")
	meta := store.CatGroupTurnMeta{TurnID: "revert-test", GroupID: g.ID, MemberID: ms[0].ID, SnapshotBefore: before, ClaimedFiles: []string{"tracked.txt"}, EndHashes: map[string]string{"tracked.txt": revertFileState(filepath.Join(root, "tracked.txt"))}}
	if err := s.cfg.Store.SaveCatGroupTurnMeta(context.Background(), meta); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	writer := &workspaceExecutionAdapter{run: func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		close(started)
		select {
		case <-release:
		case <-o.Ctx.Done():
		}
		return catagent.TurnResponse{}, nil
	}}
	doneWriter := make(chan struct{})
	go func() {
		defer close(doneWriter)
		_, _ = s.cfg.Execution.Run(writer, agentexec.Scope{OwnerID: "writer", Workspace: root, NativeConfigKey: projectConfigKey(root)}, catagent.TurnOptions{ConversationID: "writer"})
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("writer did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if _, err := s.RevertCatOverreach(ctx, RevertOverreachRequest{MemberID: ms[0].ID, TurnID: meta.TurnID, Files: []string{"tracked.txt"}}); err == nil {
		t.Fatal("revert bypassed the workspace lock")
	}
	assertSnapshotFile(t, root, "tracked.txt", "agent overwrite")
	once.Do(func() { close(release) })
	<-doneWriter
	if _, err := s.RevertCatOverreach(context.Background(), RevertOverreachRequest{MemberID: ms[0].ID, TurnID: meta.TurnID, Files: []string{"tracked.txt"}}); err != nil {
		t.Fatal(err)
	}
	assertSnapshotFile(t, root, "tracked.txt", "base\n")
}

func TestGroupWorkflowRetryAfterStopUsesFreshLifetime(t *testing.T) {
	ctx := context.Background()
	started := make(chan catagent.TurnOptions, 4)
	var calls int
	s, g, ms, _ := groupWorkTestService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		calls++ // Native calls are serialized by the shared workspace lease.
		started <- o
		if calls == 1 {
			<-o.Ctx.Done()
			return catagent.TurnResponse{}, o.Ctx.Err()
		}
		return catagent.TurnResponse{Message: catagent.WireMessage{Content: "finished\n" + groupTaskResultTag + `{"status":"done","summary":"done","evidence":"checked"}</cat_task_result>`}}, nil
	})
	tasks := []store.CatGroupWorkTask{
		{ID: "a", Title: "develop", MemberID: ms[0].ID, Kind: "develop", Instruction: "implement", Status: "pending"},
		{ID: "b", Title: "integrate", MemberID: ms[1].ID, Kind: "integrate", Instruction: "integrate", Status: "pending", DependsOn: []string{"a"}},
	}
	if err := s.startGroupWorkflow(ctx, g, store.CatGroupMessage{ChainID: "stop-retry", Text: "request"}, tasks); err != nil {
		t.Fatal(err)
	}
	first := nextGroupCall(t, started)
	if err := s.StopCatGroup(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	s.Wait()
	w, err := s.cfg.Store.GetCatGroupWorkflow(ctx, "stop-retry")
	if err != nil || w.Status != "stopped" {
		t.Fatalf("workflow did not stop: %+v %v", w, err)
	}
	select {
	case <-started:
		t.Fatal("dependency started after stop")
	default:
	}
	if err := s.RetryCatGroupMemberTurn(ctx, RetryGroupMemberTurnRequest{GroupID: g.ID, MemberID: ms[0].ID, ChainID: "stop-retry", TaskID: "a"}); err != nil {
		t.Fatal(err)
	}
	second := nextGroupCall(t, started)
	third := nextGroupCall(t, started)
	s.Wait()
	if second.TurnID == first.TurnID || second.ConversationID != ms[0].ID || third.ConversationID != ms[1].ID {
		t.Fatalf("wrong retry/dependency: %s %s", second.ConversationID, third.ConversationID)
	}
	w, err = s.cfg.Store.GetCatGroupWorkflow(ctx, "stop-retry")
	if err != nil || w.Status != "completed" || w.Tasks[0].Attempts != 2 || w.Tasks[1].Attempts != 1 {
		t.Fatalf("retry failed to complete graph: %+v %v", w, err)
	}
}

func TestGroupQueuedRetryRetainsStoppedLifetime(t *testing.T) {
	ctx := context.Background()
	started := make(chan catagent.TurnOptions, 2)
	s, g, ms, _ := groupWorkTestService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		started <- o
		<-o.Ctx.Done()
		return catagent.TurnResponse{}, o.Ctx.Err()
	})
	if _, err := s.SendCatGroupMessage(ctx, SendGroupMessageRequest{GroupID: g.ID, Text: "work", Mentions: []string{ms[0].ID}}); err != nil {
		t.Fatal(err)
	}
	nextGroupCall(t, started)
	if err := s.RetryCatGroupMemberTurn(ctx, RetryGroupMemberTurnRequest{GroupID: g.ID, MemberID: ms[0].ID, ChainID: "retry-queued"}); err != nil {
		t.Fatal(err)
	}
	groupQueueMu.Lock()
	items := append([]groupQueueItem(nil), groupQueues[ms[0].ID].items...)
	groupQueueMu.Unlock()
	if len(items) != 1 {
		t.Fatalf("retry did not queue: %d", len(items))
	}
	if err := s.StopCatGroup(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	s.Wait()
	if !groupContextStopped(items[0].ctx) {
		t.Fatal("queued retry lost the group lifetime")
	}
	select {
	case <-started:
		t.Fatal("queued retry ran after stop")
	default:
	}
}

func TestGroupWorkflowStartupFailureDoesNotDeadlockDispatch(t *testing.T) {
	s, g, ms, _ := groupWorkTestService(t, func(catagent.TurnOptions) (catagent.TurnResponse, error) {
		t.Fatal("unavailable adapter should not start")
		return catagent.TurnResponse{}, nil
	})
	s.cfg.Registry = catagent.NewRegistry()
	ctx := s.groupContext(context.Background(), g.ID)
	tasks := []store.CatGroupWorkTask{{ID: "a", Title: "unavailable", MemberID: ms[0].ID, Kind: "develop", Instruction: "work", Status: "pending"}}
	done := make(chan error, 1)
	go func() {
		s.groupDispatchMu.Lock()
		defer s.groupDispatchMu.Unlock()
		done <- s.startGroupWorkflow(ctx, g, store.CatGroupMessage{ChainID: "startup-failure", Text: "work"}, tasks)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("startup failure deadlocked group dispatch")
	}
	w, err := s.cfg.Store.GetCatGroupWorkflow(context.Background(), "startup-failure")
	if err != nil || w.Status != "blocked" || w.Tasks[0].Status != "failed" {
		t.Fatalf("startup failure was not recorded: %+v %v", w, err)
	}
}

func TestGroupStopWaitsForTurnRegistration(t *testing.T) {
	started := make(chan catagent.TurnOptions, 1)
	s, g, ms, _ := groupWorkTestService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		started <- o
		return catagent.TurnResponse{}, nil
	})
	ctx := s.groupContext(context.Background(), g.ID)
	life := ctx.Value(groupLifetimeKey{}).(*groupLifetime)
	registering, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	s.cfg.Emit = func(event string, payload any) {
		if event == catagent.EventTurn {
			if e, ok := payload.(catagent.TurnEvent); ok && e.Status == catagent.TurnRunning {
				close(registering)
				<-release
			}
		}
	}
	woken := make(chan struct{})
	go func() {
		defer close(woken)
		s.wakeMember(ctx, g, ms[0].ID, "registration", 0, 0, false, "handoff")
	}()
	select {
	case <-registering:
	case <-time.After(5 * time.Second):
		t.Fatal("turn registration did not start")
	}
	stopped := make(chan error, 1)
	go func() { stopped <- s.StopCatGroup(context.Background(), g.ID) }()
	select {
	case <-life.ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("stop did not cancel group lifetime")
	}
	early := false
	select {
	case err := <-stopped:
		early = true
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(150 * time.Millisecond):
	}
	once.Do(func() { close(release) })
	if !early {
		select {
		case err := <-stopped:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("stop remained blocked after registration")
		}
	}
	<-woken
	s.Wait()
	if early {
		t.Fatal("stop returned before initial running state was registered")
	}
	select {
	case <-started:
		t.Fatal("cancelled registration started a native turn")
	default:
	}
}

func TestGroupCancelledSnapshotDoesNotStartNativeTurn(t *testing.T) {
	called := false
	s, _, ms, root := groupWorkTestService(t, func(catagent.TurnOptions) (catagent.TurnResponse, error) {
		called = true
		return catagent.TurnResponse{}, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, err := s.adapter(ms[0].AgentKind)
	if err != nil {
		t.Fatal(err)
	}
	state := &turnState{convID: ms[0].ID, turnID: "cancel-before-native", cancel: cancel, usageCwd: root, agentKind: ms[0].AgentKind}
	state.beforeExecution = func(context.Context) { cancel() }
	s.runTurn(ctx, state, a, catagent.TurnOptions{Ctx: ctx, ConversationID: ms[0].ID, TurnID: state.turnID, ProjectPath: root})
	if called {
		t.Fatal("native adapter started after the before-snapshot was cancelled")
	}
	if s.cfg.Execution.WorkspaceBusy(root) {
		t.Fatal("cancelled snapshot retained the workspace lease")
	}
}

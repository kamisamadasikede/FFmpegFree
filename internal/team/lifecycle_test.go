package team

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"FFmpegFree/internal/catagent"
)

func employeeMembers(ms []Member) []Member {
	out := []Member{}
	for _, m := range ms {
		if m.Kind == "agent" {
			out = append(out, m)
		}
	}
	return out
}
func inputTask(o catagent.TurnOptions) (Task, []Member) {
	var data struct {
		Task    Task `json:"task"`
		Members []struct {
			ID   string `json:"memberId"`
			Kind string `json:"kind"`
		} `json:"members"`
	}
	raw := o.Messages[0].Content
	_ = json.Unmarshal([]byte(raw[strings.Index(raw, "{"):]), &data)
	ms := []Member{}
	for _, m := range data.Members {
		ms = append(ms, Member{ID: m.ID, Kind: m.Kind})
	}
	return data.Task, ms
}
func TestHumanReplyBeforeNativeExitResumesExactlyOnce(t *testing.T) {
	var calls atomic.Int32
	asked := make(chan CommandResult, 1)
	exit := make(chan struct{})
	s, _ := testService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		if calls.Add(1) == 1 {
			_, ms := inputTask(o)
			human := ""
			for _, m := range ms {
				if m.Kind == "human" {
					human = m.ID
				}
			}
			raw, e := toolCall(o, "team_ask", map[string]any{"requestId": "question", "toMemberId": human, "content": "请确认配色"})
			if e != nil {
				return catagent.TurnResponse{}, e
			}
			var result CommandResult
			_ = json.Unmarshal(raw, &result)
			asked <- result
			select {
			case <-exit:
			case <-o.Ctx.Done():
			}
			return catagent.TurnResponse{}, nil
		}
		if !strings.Contains(o.Messages[0].Content, "蓝色") {
			return catagent.TurnResponse{}, errors.New("answer missing")
		}
		_, e := toolCall(o, "team_finish", map[string]any{"requestId": "done", "result": "已按蓝色交付", "validation": "not_run"})
		return catagent.TurnResponse{}, e
	})
	r, ms := testRoom(t, s, 1)
	agent := employeeMembers(ms)[0]
	_, e := s.Command(context.Background(), Command{RequestID: "start", RoomID: r.ID, Intent: "new_task", Content: "设计配色", TargetMemberIDs: []string{agent.ID}})
	if e != nil {
		t.Fatal(e)
	}
	var q CommandResult
	select {
	case q = <-asked:
	case <-time.After(5 * time.Second):
		v, _ := s.Snapshot(context.Background(), r.ID, 0)
		t.Fatalf("no question: %+v", v.Tasks)
	}
	_, e = s.Command(context.Background(), Command{RequestID: "answer", RoomID: r.ID, Intent: "reply", ReplyToID: q.Message.ID, Content: "用蓝色"})
	if e != nil {
		t.Fatal(e)
	}
	close(exit)
	eventually(t, func() bool {
		v, _ := s.Snapshot(context.Background(), r.ID, 0)
		return len(v.Tasks) == 1 && v.Tasks[0].Status == "completed"
	})
	if calls.Load() != 2 {
		t.Fatal(calls.Load())
	}
	v, _ := s.Snapshot(context.Background(), r.ID, 0)
	answered := false
	for _, in := range v.Inbox {
		if in.MessageID == q.Message.ID {
			answered = in.AnsweredAt > 0
		}
	}
	if !answered {
		t.Fatal("answer not linked")
	}
	runs, e := s.Runs(context.Background(), v.Tasks[0].ID)
	if e != nil || len(runs) != 2 || runs[0].FinishedAt == 0 {
		t.Fatalf("runs=%+v %v", runs, e)
	}
}

func TestFailedRequiredChildCannotLeaveParentWaiting(t *testing.T) {
	childID := ""
	s, _ := testService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		task, _ := inputTask(o)
		if task.MemberID == childID {
			return catagent.TurnResponse{}, errors.New("child failed")
		}
		_, e := toolCall(o, "team_finish", map[string]any{"requestId": "done", "result": "提交结果", "validation": "not_run"})
		return catagent.TurnResponse{}, e
	})
	r, ms := testRoom(t, s, 2)
	agents := employeeMembers(ms)
	childID = agents[1].ID
	var parent, child Task
	e := s.write(context.Background(), func(tx *Tx) error {
		msg := Message{ID: "seed", RoomID: r.ID, FromMemberID: ms[0].ID, Kind: "new_task", Content: "任务", Status: "sent"}
		if e := tx.SaveMessage(&msg); e != nil {
			return e
		}
		var e error
		parent, e = s.newTask(tx, r, agents[0], ms[0].ID, "", msg, Command{Content: "整合"})
		if e != nil {
			return e
		}
		child, e = s.newTask(tx, r, agents[1], agents[0].ID, parent.ID, msg, Command{Content: "执行", Required: true})
		if e != nil {
			return e
		}
		child.Status = "failed"
		child.Error = "失败"
		return tx.SaveTask(&child)
	})
	if e != nil {
		t.Fatal(e)
	}
	eventually(t, func() bool {
		v, _ := s.Snapshot(context.Background(), r.ID, 0)
		for _, task := range v.Tasks {
			if task.ID == parent.ID {
				return task.Status == "needs_review"
			}
		}
		return false
	})
}

func TestCancelledParentCancelsExclusiveDescendants(t *testing.T) {
	s, _ := testService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		<-o.Ctx.Done()
		return catagent.TurnResponse{}, o.Ctx.Err()
	})
	r, ms := testRoom(t, s, 2)
	r.Paused = true
	if _, e := s.UpdateRoom(context.Background(), r); e != nil {
		t.Fatal(e)
	}
	agents := employeeMembers(ms)
	var parent, child Task
	if e := s.write(context.Background(), func(tx *Tx) error {
		msg := Message{ID: "seed", RoomID: r.ID, FromMemberID: ms[0].ID, Kind: "new_task", Content: "任务", Status: "sent"}
		if e := tx.SaveMessage(&msg); e != nil {
			return e
		}
		var e error
		parent, e = s.newTask(tx, r, agents[0], ms[0].ID, "", msg, Command{Content: "父任务"})
		if e != nil {
			return e
		}
		child, e = s.newTask(tx, r, agents[1], agents[0].ID, parent.ID, msg, Command{Content: "子任务"})
		return e
	}); e != nil {
		t.Fatal(e)
	}
	if e := s.Stop(context.Background(), parent.ID); e != nil {
		t.Fatal(e)
	}
	v, _ := s.Snapshot(context.Background(), r.ID, 0)
	for _, task := range v.Tasks {
		if (task.ID == parent.ID || task.ID == child.ID) && task.Status != "cancelled" {
			t.Fatalf("not cancelled: %+v", task)
		}
	}
}

func TestArtifactOpenAndPathAuthorization(t *testing.T) {
	gate := make(chan struct{})
	started := make(chan catagent.TurnOptions, 1)
	s, _ := testService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		started <- o
		select {
		case <-gate:
		case <-o.Ctx.Done():
		}
		return catagent.TurnResponse{}, nil
	})
	r, ms := testRoom(t, s, 1)
	_, e := s.Command(context.Background(), Command{RequestID: "start", RoomID: r.ID, Intent: "new_task", Content: "生成文件", TargetMemberIDs: []string{employeeMembers(ms)[0].ID}})
	if e != nil {
		t.Fatal(e)
	}
	var o catagent.TurnOptions
	select {
	case o = <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("not started")
	}
	if e = os.WriteFile(filepath.Join(r.Workspace, "report.md"), []byte("# report"), 0600); e != nil {
		t.Fatal(e)
	}
	raw, e := toolCall(o, "team_open_file", map[string]any{"path": "report.md"})
	if e != nil {
		t.Fatal(e)
	}
	var a Artifact
	_ = json.Unmarshal(raw, &a)
	if a.RoomID != r.ID || a.TaskID == "" {
		t.Fatal(string(raw))
	}
	for _, path := range []string{"../outside.md", "sub/../../outside.md", "C:/outside.md", "report.md:stream", "/outside.md"} {
		if _, e = toolCall(o, "team_open_file", map[string]any{"path": path}); e == nil {
			t.Errorf("unsafe path accepted: %s", path)
		}
	}
	v, e := s.Snapshot(context.Background(), r.ID, 0)
	if e != nil || len(v.Artifacts) != 1 {
		t.Fatalf("artifacts %v %v", v.Artifacts, e)
	}
	files, e := s.Files(context.Background(), r.ID)
	if e != nil || len(files.Files) != 1 {
		t.Fatalf("files=%+v %v", files, e)
	}
	close(gate)
}

func TestNoLogicalFinishIsNotCompletion(t *testing.T) {
	s, _ := testService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		o.OnTextDelta("一轮结束")
		return catagent.TurnResponse{}, nil
	})
	r, ms := testRoom(t, s, 1)
	_, e := s.Command(context.Background(), Command{RequestID: "start", RoomID: r.ID, Intent: "new_task", Content: "任务", TargetMemberIDs: []string{employeeMembers(ms)[0].ID}})
	if e != nil {
		t.Fatal(e)
	}
	eventually(t, func() bool {
		v, _ := s.Snapshot(context.Background(), r.ID, 0)
		return len(v.Tasks) == 1 && v.Tasks[0].Status == "needs_review"
	})
}

func TestRestartDoesNotReplayUnconfirmedWork(t *testing.T) {
	var calls atomic.Int32
	s, _ := testService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		calls.Add(1)
		return catagent.TurnResponse{}, nil
	})
	r, ms := testRoom(t, s, 1)
	r.Paused = true
	if _, e := s.UpdateRoom(context.Background(), r); e != nil {
		t.Fatal(e)
	}
	_, e := s.Command(context.Background(), Command{RequestID: "seed", RoomID: r.ID, Intent: "new_task", Content: "不能自动重放", TargetMemberIDs: []string{employeeMembers(ms)[0].ID}})
	if e != nil {
		t.Fatal(e)
	}
	s.Close()
	resumed, e := New(context.Background(), s.cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer resumed.Close()
	v, e := resumed.Snapshot(context.Background(), r.ID, 0)
	if e != nil || v.Tasks[0].Status != "needs_review" || calls.Load() != 0 {
		t.Fatalf("restart=%+v calls=%d err=%v", v.Tasks, calls.Load(), e)
	}
}

func TestHumanMentionNotificationOutboxIsNotDuplicated(t *testing.T) {
	s, _ := testService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		_, ms := inputTask(o)
		human := ""
		for _, m := range ms {
			if m.Kind == "human" {
				human = m.ID
			}
		}
		body := map[string]any{"requestId": "report", "toMemberId": human, "content": "进度汇报", "category": "progress"}
		if _, e := toolCall(o, "team_report", body); e != nil {
			return catagent.TurnResponse{}, e
		}
		if _, e := toolCall(o, "team_report", body); e != nil {
			return catagent.TurnResponse{}, e
		}
		_, e := toolCall(o, "team_finish", map[string]any{"requestId": "finish", "result": "完成", "validation": "not_run"})
		return catagent.TurnResponse{}, e
	})
	var notified atomic.Int32
	s.cfg.Notify = func(n Notification) error {
		if n.RoomID == "" || n.MessageID == "" {
			return errors.New("missing notification route")
		}
		notified.Add(1)
		return nil
	}
	r, ms := testRoom(t, s, 1)
	_, e := s.Command(context.Background(), Command{RequestID: "task", RoomID: r.ID, Intent: "new_task", Content: "汇报任务", TargetMemberIDs: []string{employeeMembers(ms)[0].ID}})
	if e != nil {
		t.Fatal(e)
	}
	eventually(t, func() bool {
		v, _ := s.Snapshot(context.Background(), r.ID, 0)
		return len(v.Inbox) == 2 && v.Inbox[0].NotificationStatus == "submitted" && v.Inbox[1].NotificationStatus == "submitted"
	})
	if notified.Load() != 2 {
		t.Fatalf("got %d notifications for one progress report and one logical result", notified.Load())
	}
}

package cat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/store"
)

type groupWorkAdapter struct {
	kind string
	run  func(catagent.TurnOptions) (catagent.TurnResponse, error)
}

func (a *groupWorkAdapter) Kind() string           { return a.kind }
func (a *groupWorkAdapter) ExecutablePath() string { return "" }
func (a *groupWorkAdapter) ProtocolVersion() int   { return 1 }
func (a *groupWorkAdapter) Status() catagent.Status {
	return catagent.Status{State: catagent.StateReady}
}
func (a *groupWorkAdapter) Recheck() catagent.Status                        { return a.Status() }
func (a *groupWorkAdapter) ListModels() ([]catagent.Model, error)           { return nil, nil }
func (a *groupWorkAdapter) ListThinkLevels() ([]catagent.ThinkLevel, error) { return nil, nil }
func (a *groupWorkAdapter) RunTurn(o catagent.TurnOptions) (catagent.TurnResponse, error) {
	return a.run(o)
}

func groupWorkTestService(t *testing.T, run func(catagent.TurnOptions) (catagent.TurnResponse, error)) (*Service, store.CatGroup, []store.CatGroupMember, string) {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	reg := catagent.NewRegistry()
	for _, kind := range catagent.CreatableKinds() {
		reg.Register(&groupWorkAdapter{kind: kind, run: run})
	}
	s := New(Config{Store: st, Registry: reg, DataRoot: t.TempDir(), CLIHome: t.TempDir()})
	p, _, err := st.InsertOrGetCatProject(ctx, store.CatProjectRow{Path: root, PathKey: root, Name: "项目"})
	if err != nil {
		t.Fatal(err)
	}
	g, err := st.InsertCatGroup(ctx, store.CatGroup{ProjectID: p.ID, Name: "协作", AutoRoute: true})
	if err != nil {
		t.Fatal(err)
	}
	var members []store.CatGroupMember
	for i, kind := range catagent.CreatableKinds() {
		c, err := st.InsertCatConversation(ctx, store.CatConversation{ProjectID: p.ID, Title: kind, AgentKind: kind})
		if err != nil {
			t.Fatal(err)
		}
		m, err := st.InsertCatGroupMember(ctx, store.CatGroupMember{ID: c.ID, GroupID: g.ID, Name: fmt.Sprintf("员工%d", i), Title: "工程师", AgentKind: kind, Order: i, Charter: store.CatCharter{Status: "ok"}})
		if err != nil {
			t.Fatal(err)
		}
		members = append(members, m)
	}
	t.Cleanup(func() { _ = s.StopCatGroup(ctx, g.ID); s.Wait(); _ = st.Close() })
	return s, g, members, root
}

func groupPlanTasks(ms []store.CatGroupMember) []store.CatGroupWorkTask {
	return []store.CatGroupWorkTask{
		{ID: "a", Title: "后端接口", MemberID: ms[0].ID, Instruction: "实现并测试接口", Kind: "develop", DependsOn: []string{}, Status: "pending"},
		{ID: "b", Title: "前端页面", MemberID: ms[1].ID, Instruction: "实现并测试页面", Kind: "develop", DependsOn: []string{}, Status: "pending"},
		{ID: "c", Title: "联调", MemberID: ms[2].ID, Instruction: "对接前后端", Kind: "integrate", DependsOn: []string{"a", "b"}, Status: "pending"},
		{ID: "v", Title: "验收", MemberID: ms[0].ID, Instruction: "检查原始需求与测试结果", Kind: "verify", DependsOn: []string{"c"}, Status: "pending"},
	}
}

func waitGroupWork(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("协作状态未按期更新")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestGroupWorkflow_ReassignRunningTaskIgnoresOldCompletion(t *testing.T) {
	ctx := context.Background()
	started := make(chan catagent.TurnOptions, 4)
	s, g, ms, _ := groupWorkTestService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		started <- o
		<-o.Ctx.Done()
		return catagent.TurnResponse{}, o.Ctx.Err()
	})
	tasks := []store.CatGroupWorkTask{
		{ID: "a", Title: "接口", MemberID: ms[0].ID, Kind: "develop", Instruction: "实现接口", Status: "pending"},
		{ID: "v", Title: "验收", MemberID: ms[2].ID, Kind: "verify", Instruction: "检查", Status: "pending", DependsOn: []string{"a"}},
	}
	if err := s.startGroupWorkflow(ctx, g, store.CatGroupMessage{ChainID: "reassign", Text: "开发接口"}, tasks); err != nil {
		t.Fatal(err)
	}
	old := <-started
	_, err := s.ReassignCatGroupRoute(ctx, ReassignGroupRouteRequest{GroupID: g.ID, ChainID: "reassign", TaskID: "a", ToMemberID: ms[1].ID})
	if err != nil {
		t.Fatal(err)
	}
	var next catagent.TurnOptions
	select {
	case next = <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("reassigned task did not start")
	}
	if next.ConversationID != ms[1].ID || next.TurnID == old.TurnID {
		t.Fatalf("wrong execution after reassignment: %s", next.ConversationID)
	}
	s.finishGroupWorkTask(g, "reassign", "a", "done", "旧结果", "", old.TurnID)
	w, err := s.cfg.Store.GetCatGroupWorkflow(ctx, "reassign")
	if err != nil || w.Tasks[0].Status != "running" || w.Tasks[0].MemberID != ms[1].ID || w.Tasks[0].Result == "旧结果" || w.Tasks[1].Status != "pending" {
		t.Fatalf("stale completion changed graph: %+v, %v", w, err)
	}
	if _, err := s.ReassignCatGroupRoute(ctx, ReassignGroupRouteRequest{GroupID: g.ID, ChainID: "reassign", ToMemberID: ms[0].ID}); err == nil {
		t.Fatal("ambiguous graph reassignment should require task id")
	}
}

func TestGroupWorkflow_ServiceJoinRetryAndPermissions(t *testing.T) {
	for _, failB := range []bool{false, true} {
		t.Run(fmt.Sprint("failure=", failB), func(t *testing.T) {
			ctx := context.Background()
			started := make(chan string, 20)
			finishA, finishB := make(chan struct{}, 1), make(chan struct{}, 1)
			var mu sync.Mutex
			var calls []catagent.TurnOptions
			var tasks []store.CatGroupWorkTask
			bCalls := 0
			s, g, ms, root := groupWorkTestService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
				if o.JSONSchema != "" {
					body, _ := json.Marshal(map[string]any{"memberIds": []string{tasks[0].MemberID, tasks[1].MemberID}, "everyone": false, "reason": "开发后联调验收", "tasks": tasks})
					return catagent.TurnResponse{Message: catagent.WireMessage{Content: string(body)}}, nil
				}
				id := ""
				for _, task := range tasks {
					if strings.Contains(o.Messages[0].Content, "你本轮只负责任务 "+task.ID+"：") {
						id = task.ID
					}
				}
				mu.Lock()
				calls = append(calls, o)
				if id == "b" {
					bCalls++
				}
				attempt := bCalls
				mu.Unlock()
				started <- id
				if id == "a" || id == "b" {
					ch := finishA
					if id == "b" {
						ch = finishB
					}
					select {
					case <-ch:
					case <-o.Ctx.Done():
						return catagent.TurnResponse{}, o.Ctx.Err()
					}
				}
				if id == "b" && failB && attempt == 1 {
					return catagent.TurnResponse{}, errors.New("模拟后端未完成")
				}
				if id == "v" && o.OnActivity != nil {
					o.OnActivity("tool", "verify-check", "exec_command\ngo test ./...", true)
				}
				body := "## 完成了什么\n已完成 " + id + `\n` + groupTaskResultTag + `{"status":"done","summary":"` + id + `完成","evidence":"go test ./... PASS"}</cat_task_result>`
				return catagent.TurnResponse{Message: catagent.WireMessage{Content: body}}, nil
			})
			tasks = groupPlanTasks(ms)
			res, err := s.SendCatGroupMessage(ctx, SendGroupMessageRequest{GroupID: g.ID, Text: "完成开发并联调验收"})
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				select {
				case id := <-started:
					if id != "a" && id != "b" {
						t.Fatalf("依赖提前启动 %s", id)
					}
				case <-time.After(8 * time.Second):
					t.Fatal("独立任务未启动")
				}
			}
			finishA <- struct{}{}
			waitGroupWork(t, func() bool {
				w, _ := s.cfg.Store.GetCatGroupWorkflow(ctx, res.ChainID)
				return len(w.Tasks) > 0 && w.Tasks[0].Status == "done"
			})
			select {
			case id := <-started:
				t.Fatalf("另一人未完成时启动了 %s", id)
			default:
			}
			finishB <- struct{}{}
			if failB {
				waitGroupWork(t, func() bool { w, _ := s.cfg.Store.GetCatGroupWorkflow(ctx, res.ChainID); return w.Status == "blocked" })
				select {
				case id := <-started:
					t.Fatalf("失败依赖放行了 %s", id)
				default:
				}
				if err := s.RetryCatGroupMemberTurn(ctx, RetryGroupMemberTurnRequest{GroupID: g.ID, MemberID: ms[1].ID, ChainID: res.ChainID}); err != nil {
					t.Fatal(err)
				}
				select {
				case id := <-started:
					if id != "b" {
						t.Fatalf("重试错任务 %s", id)
					}
				case <-time.After(8 * time.Second):
					t.Fatal("没有重试")
				}
				finishB <- struct{}{}
			}
			for _, want := range []string{"c", "v"} {
				select {
				case id := <-started:
					if id != want {
						t.Fatalf("阶段=%s want=%s", id, want)
					}
				case <-time.After(8 * time.Second):
					t.Fatal("联调/验收没启动")
				}
			}
			waitGroupWork(t, func() bool { w, _ := s.cfg.Store.GetCatGroupWorkflow(ctx, res.ChainID); return w.Status == "completed" })
			s.Wait()
			mu.Lock()
			defer mu.Unlock()
			for _, o := range calls {
				var kind string
				for _, m := range ms {
					if m.ID == o.ConversationID {
						kind = m.AgentKind
					}
				}
				if o.ProjectPath != root || o.PermissionMode != catagent.NormalizeMode(kind, "full") {
					t.Fatalf("项目或权限不正确: %q %q kind=%s", o.ProjectPath, o.PermissionMode, kind)
				}
			}
			msgs, _ := s.cfg.Store.ListCatGroupMessages(ctx, g.ID, 0, 200)
			summaries := 0
			for _, m := range msgs {
				if m.SystemKind == "workflow_summary" {
					summaries++
				}
				if m.Kind == "member" && strings.Contains(m.Text, groupTaskResultTag) {
					t.Fatal("内部状态块泄漏到群消息")
				}
			}
			if summaries != 1 {
				t.Fatalf("最终汇总次数=%d", summaries)
			}
		})
	}
}

func TestGroupWorkflow_PlanValidationAndJoin(t *testing.T) {
	ms := []store.CatGroupMember{{ID: "m1", Name: "甲"}, {ID: "m2", Name: "乙"}, {ID: "m3", Name: "丙"}}
	for _, tc := range []struct {
		name string
		edit func([]store.CatGroupWorkTask)
	}{
		{"valid", func([]store.CatGroupWorkTask) {}},
		{"unknown member", func(ts []store.CatGroupWorkTask) { ts[0].MemberID = "missing" }},
		{"unknown dependency", func(ts []store.CatGroupWorkTask) { ts[2].DependsOn = []string{"missing"} }},
		{"cycle", func(ts []store.CatGroupWorkTask) { ts[0].DependsOn = []string{"c"} }},
		{"missing final join", func(ts []store.CatGroupWorkTask) { ts[3].DependsOn = []string{"a"} }},
		{"no verification", func(ts []store.CatGroupWorkTask) { ts[3].Kind = "develop" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ts := groupPlanTasks(ms)
			tc.edit(ts)
			raw, _ := json.Marshal(map[string]any{"tasks": ts})
			_, err := parseGroupWorkPlan(string(raw), ms)
			if (err == nil) != (tc.name == "valid") {
				t.Fatalf("validation: %v", err)
			}
		})
	}
	w := store.CatGroupWorkflow{Tasks: groupPlanTasks(ms)}
	ready := readyGroupTasks(&w)
	if len(ready) != 2 {
		t.Fatalf("roots=%v", ready)
	}
	w.Tasks[0].Status = "done"
	w.Tasks[1].Status = "running"
	if got := readyGroupTasks(&w); len(got) != 0 {
		t.Fatalf("premature join %v", got)
	}
	w.Tasks[1].Status = "done"
	if got := readyGroupTasks(&w); len(got) != 1 || got[0].ID != "c" {
		t.Fatalf("join not released: %v", got)
	}
}

func TestGroupWorkflow_ReworkBoundAndPreservesOtherResults(t *testing.T) {
	ms := []store.CatGroupMember{{ID: "m1"}, {ID: "m2"}, {ID: "m3"}}
	w := store.CatGroupWorkflow{Tasks: groupPlanTasks(ms)}
	for i := range w.Tasks {
		w.Tasks[i].Status = "done"
		w.Tasks[i].Result = "已有产物"
	}
	v := w.Tasks[3]
	v.Attempts = 1
	v.Status = "failed"
	v.Result = "前端参数不匹配"
	if !resetGroupRework(&w, v, []string{"b"}) {
		t.Fatal("返工未安排")
	}
	if w.Tasks[0].Status != "done" || w.Tasks[0].Result != "已有产物" {
		t.Fatal("无关任务被重跑")
	}
	for _, i := range []int{1, 2, 3} {
		if w.Tasks[i].Status != "pending" {
			t.Fatal("相关后续未重置")
		}
	}
	v.Attempts = 3
	if resetGroupRework(&w, v, []string{"a"}) {
		t.Fatal("超过返工次数仍自动重跑")
	}
}

func TestGroupWorkflow_RecoveryStopAndVerificationEvidence(t *testing.T) {
	s, g, ms, _ := groupWorkTestService(t, func(catagent.TurnOptions) (catagent.TurnResponse, error) {
		return catagent.TurnResponse{}, errors.New("unexpected call")
	})
	ctx := context.Background()
	w := store.CatGroupWorkflow{ChainID: "recover", GroupID: g.ID, Status: "running", Tasks: groupPlanTasks(ms)}
	w.Tasks[0].Status = "running"
	w.Tasks[1].Status = "done"
	if err := s.cfg.Store.SaveCatGroupWorkflow(ctx, &w); err != nil {
		t.Fatal(err)
	}
	s.recoverGroupWorkflows(ctx, g.ID)
	w, _ = s.cfg.Store.GetCatGroupWorkflow(ctx, w.ChainID)
	if w.Status != "blocked" || w.Tasks[0].Status != "blocked" || w.Tasks[1].Status != "done" {
		t.Fatalf("恢复状态 %v", w)
	}
	// Verification without evidence cannot complete the request.
	for i := range w.Tasks {
		w.Tasks[i].Status = "done"
	}
	w.Tasks[3].Status = "running"
	w.Status = "running"
	_ = s.cfg.Store.SaveCatGroupWorkflow(ctx, &w)
	s.finishGroupWorkTask(g, w.ChainID, "v", "done", "都好了", "", "turn")
	w, _ = s.cfg.Store.GetCatGroupWorkflow(ctx, w.ChainID)
	if w.Status != "blocked" || w.Tasks[3].Status != "blocked" {
		t.Fatal("没有证据也标成已验收")
	}
	w.Tasks[3].Status = "running"
	_ = s.cfg.Store.SaveCatGroupWorkflow(ctx, &w)
	s.finishGroupWorkTask(g, w.ChainID, "v", "done", "都好了", "声称测试通过", "turn", groupTaskResult{ToolCount: 0})
	w, _ = s.cfg.Store.GetCatGroupWorkflow(ctx, w.ChainID)
	if w.Status != "blocked" || w.Tasks[3].Status != "blocked" {
		t.Fatal("没有实际工具调用也标成已验收")
	}
	s.stopGroupWorkflows(ctx, g.ID, "")
	w, _ = s.cfg.Store.GetCatGroupWorkflow(ctx, w.ChainID)
	if w.Status != "stopped" {
		t.Fatal("停止没有保存")
	}
	s.finishGroupWorkTask(g, w.ChainID, "v", "done", "迟到的成功", "PASS", "turn")
	w, _ = s.cfg.Store.GetCatGroupWorkflow(ctx, w.ChainID)
	if w.Status != "stopped" {
		t.Fatal("停止后被迟到结果重启")
	}
}

func TestGroupWorkflow_ResultAndPinnedContext(t *testing.T) {
	if _, r := parseGroupTaskResult("已经好了"); r.Status == "done" {
		t.Fatal("普通回复被当作任务完成")
	}
	s, g, ms, _ := groupWorkTestService(t, nil)
	ctx := context.Background()
	origin, _ := s.cfg.Store.InsertCatGroupMessage(ctx, store.CatGroupMessage{GroupID: g.ID, ChainID: "chain", Kind: "user", Text: "原始目标"})
	for i := 0; i < 130; i++ {
		_, _ = s.cfg.Store.InsertCatGroupMessage(ctx, store.CatGroupMessage{GroupID: g.ID, ChainID: "other", Kind: "system", Text: fmt.Sprint("背景", i)})
	}
	trigger, _ := s.cfg.Store.InsertCatGroupMessage(ctx, store.CatGroupMessage{GroupID: g.ID, ChainID: "chain", Kind: "member", MemberID: ms[1].ID, Text: "请核对接口产物"})
	prompt, readThrough := s.buildMemberPromptSnapshot(ctx, g, ms[0], ms, trigger.Seq)
	if !strings.Contains(prompt, origin.Text) || !strings.Contains(prompt, trigger.Text) || !strings.Contains(prompt, "背景129") {
		t.Fatal("原始任务、交接消息或最新背景丢失")
	}
	late, _ := s.cfg.Store.InsertCatGroupMessage(ctx, store.CatGroupMessage{GroupID: g.ID, ChainID: "chain", Kind: "user", Text: "工作期间的新要求"})
	_ = s.cfg.Store.UpdateCatGroupMemberLastSeen(ctx, ms[0].ID, readThrough)
	m, _ := s.cfg.Store.GetCatGroupMember(ctx, ms[0].ID)
	next := s.buildMemberPrompt(ctx, g, m, ms, late.Seq)
	if !strings.Contains(next, late.Text) || m.LastSeenSeq >= late.Seq {
		t.Fatal("工作期间的新消息被标成已读")
	}
}

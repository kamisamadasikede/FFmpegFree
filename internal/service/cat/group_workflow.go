package cat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/id"
	"FFmpegFree/internal/store"
)

var groupTaskIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,48}$`)

// Validate before persisting or waking anyone. Every plan ends in a verification
// task that transitively waits for all development and integration work.
func parseGroupWorkPlan(raw string, members []store.CatGroupMember) ([]store.CatGroupWorkTask, error) {
	var payload struct {
		Tasks []store.CatGroupWorkTask `json:"tasks"`
	}
	if err := json.Unmarshal([]byte(extractJSONObject(raw)), &payload); err != nil {
		return nil, err
	}
	tasks := payload.Tasks
	if len(tasks) == 0 {
		return nil, nil
	}
	if len(tasks) > 12 {
		return nil, errors.New("任务最多拆成 12 项")
	}
	byID := map[string]int{}
	verify := ""
	for i := range tasks {
		t := &tasks[i]
		if !groupTaskIDPattern.MatchString(t.ID) {
			return nil, errors.New("任务编号无效")
		}
		if _, ok := byID[t.ID]; ok {
			return nil, errors.New("任务编号重复")
		}
		byID[t.ID] = i
		mid, ok := resolveRouteMemberID(members, t.MemberID)
		if !ok {
			return nil, errors.New("任务负责人不在本群")
		}
		t.MemberID = mid
		t.Title = strings.TrimSpace(t.Title)
		t.Instruction = strings.TrimSpace(t.Instruction)
		if t.Title == "" || t.Instruction == "" || len([]rune(t.Instruction)) > 3000 {
			return nil, errors.New("任务内容缺失或过长")
		}
		if t.Kind != "develop" && t.Kind != "integrate" && t.Kind != "verify" {
			return nil, errors.New("任务阶段无效")
		}
		if t.Kind == "verify" {
			if verify != "" {
				return nil, errors.New("只允许一个最终验收任务")
			}
			verify = t.ID
		}
		t.Status = "pending"
		t.Attempts = 0
		t.Result = ""
		t.Evidence = ""
		t.TurnID = ""
	}
	if verify == "" {
		return nil, errors.New("缺少最终联调验收任务")
	}
	seen := map[string]int{}
	var visit func(string) error
	visit = func(id string) error {
		if seen[id] == 1 {
			return errors.New("任务依赖形成循环")
		}
		if seen[id] == 2 {
			return nil
		}
		seen[id] = 1
		for _, dep := range tasks[byID[id]].DependsOn {
			if _, ok := byID[dep]; !ok {
				return errors.New("依赖的任务不存在")
			}
			if err := visit(dep); err != nil {
				return err
			}
		}
		seen[id] = 2
		return nil
	}
	if err := visit(verify); err != nil {
		return nil, err
	}
	if len(seen) != len(tasks) {
		return nil, errors.New("最终验收没有等待全部任务")
	}
	return tasks, nil
}

func (s *Service) emitGroupWorkflow(w store.CatGroupWorkflow) {
	s.emit("cat:group-workflow", w)
}

// Reassignment changes a single graph node. Keep it reserved until its previous
// execution exits, and leave this employee's unrelated work and queue intact.
func (s *Service) reassignGroupWorkTask(ctx context.Context, g store.CatGroup, req ReassignGroupRouteRequest, to store.CatGroupMember) (bool, ReassignGroupRouteResult, error) {
	s.groupStartMu.Lock()
	defer s.groupStartMu.Unlock()
	s.groupWorkMu.Lock()
	w, err := s.cfg.Store.GetCatGroupWorkflow(ctx, req.ChainID)
	if errors.Is(err, sql.ErrNoRows) {
		s.groupWorkMu.Unlock()
		return false, ReassignGroupRouteResult{}, nil
	}
	if err != nil || w.GroupID != g.ID {
		s.groupWorkMu.Unlock()
		if err == nil {
			err = s.errGroupNotFound()
		}
		return true, ReassignGroupRouteResult{}, err
	}
	index := -1
	for i, task := range w.Tasks {
		if task.Status == "done" || (req.TaskID != "" && task.ID != req.TaskID) {
			continue
		}
		if index >= 0 {
			s.groupWorkMu.Unlock()
			return true, ReassignGroupRouteResult{}, apperr.New(apperr.TaskConflict, "本次协作有多项任务，请在协作进度中指定要换人的任务")
		}
		index = i
	}
	if index < 0 || w.Status == "stopped" || w.Dismissed {
		s.groupWorkMu.Unlock()
		return true, ReassignGroupRouteResult{}, apperr.New(apperr.TaskConflict, "任务已完成或整项协作已停止，请先重试再改派")
	}
	task := &w.Tasks[index]
	if task.MemberID == to.ID {
		s.groupWorkMu.Unlock()
		return true, ReassignGroupRouteResult{}, apperr.New(apperr.TaskConflict, "请选择其他成员接手这项任务")
	}
	fromID, oldTurn, taskID := task.MemberID, task.TurnID, task.ID
	assignment, managed, err := s.assignmentFor(ctx, w.ChainID, fromID, taskID)
	if err != nil {
		s.groupWorkMu.Unlock()
		return true, ReassignGroupRouteResult{}, err
	}
	if managed && assignment.Status == "held" {
		s.groupWorkMu.Unlock()
		return true, ReassignGroupRouteResult{}, apperr.New(apperr.TaskConflict, "这项任务正在移交，请等待原成员退出后再改派")
	}
	next := assignment
	if managed {
		next.ID, next.MemberID, next.TurnID = groupAssignmentID(w.ChainID, to.ID, taskID), to.ID, ""
		next = s.materializeGroupDecision(ctx, store.CatGroupDecision{Assignments: []store.CatGroupAssignment{next}}).Assignments[0]
	}
	reservation := id.New()
	task.MemberID, task.Status, task.TurnID = to.ID, "queued", reservation
	task.Result = "正在移交给 " + to.Name
	task.Evidence = ""
	task.RetryAt = 0
	if managed {
		err = s.cfg.Store.MoveCatGroupAssignment(ctx, assignment.ID, next, &w)
	} else {
		err = s.cfg.Store.SaveCatGroupWorkflow(ctx, &w)
	}
	s.groupWorkMu.Unlock()
	if err != nil {
		return true, ReassignGroupRouteResult{}, err
	}
	s.emitGroupWorkflow(w)
	groupQueueMu.Lock()
	if st := groupQueues[fromID]; st != nil {
		kept := st.items[:0]
		for _, item := range st.items {
			if item.chainID != w.ChainID || item.taskID != taskID {
				kept = append(kept, item)
			}
		}
		st.items = kept
	}
	groupQueueMu.Unlock()
	s.mu.Lock()
	previous := s.turns[groupConvID(g.ID, fromID)]
	if previous != nil && previous.turnID != oldTurn {
		previous = nil
	}
	s.mu.Unlock()
	if previous != nil {
		previous.cancel()
	}
	sys := s.postSystem(ctx, g.ID, w.ChainID, 0, "system", "已将「"+task.Title+"」交给 "+to.Name, "reassign", store.CatRouteInfo{Mode: "reassign", MemberID: to.ID, Reason: "任务改派"}, fromID, to.ID)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if previous != nil {
			<-previous.done
		}
		s.groupDispatchMu.Lock()
		defer s.groupDispatchMu.Unlock()
		if groupContextStopped(ctx) {
			return
		}
		s.groupWorkMu.Lock()
		current, err := s.cfg.Store.GetCatGroupWorkflow(ctx, w.ChainID)
		if err != nil || current.Status == "stopped" || current.Dismissed {
			s.groupWorkMu.Unlock()
			return
		}
		changed := false
		for i := range current.Tasks {
			t := &current.Tasks[i]
			if t.ID == taskID && t.Status == "queued" && t.TurnID == reservation {
				t.Status, t.TurnID = "pending", ""
				changed = true
			}
		}
		if changed {
			if managed {
				changed, err = s.cfg.Store.ReleaseCatGroupAssignment(ctx, next.ID, &current)
			} else {
				err = s.cfg.Store.SaveCatGroupWorkflow(ctx, &current)
			}
		}
		s.groupWorkMu.Unlock()
		if err != nil {
			s.cfg.Logf("group workflow handoff release: %v", err)
		}
		if changed && err == nil {
			s.advanceGroupWorkflow(g, w.ChainID, ctx)
		}
	}()
	return true, ReassignGroupRouteResult{System: sys}, nil
}

func (s *Service) currentGroupWorkExecution(chainID, taskID, turnID string) bool {
	s.groupWorkMu.Lock()
	defer s.groupWorkMu.Unlock()
	w, err := s.cfg.Store.GetCatGroupWorkflow(context.Background(), chainID)
	if err != nil || w.Status == "stopped" || w.Dismissed {
		return false
	}
	for _, t := range w.Tasks {
		if t.ID == taskID {
			return t.Status == "running" && t.TurnID == turnID
		}
	}
	return false
}

func (s *Service) startGroupWorkflow(ctx context.Context, g store.CatGroup, msg store.CatGroupMessage, tasks []store.CatGroupWorkTask) error {
	ctx = s.groupContext(ctx, g.ID)
	if groupContextStopped(ctx) {
		return context.Canceled
	}
	s.recoverGroupWorkflows(ctx, g.ID)
	w := store.CatGroupWorkflow{ChainID: msg.ChainID, GroupID: g.ID, OriginSeq: msg.Seq, Request: msg.Text, Status: "running", Tasks: tasks, UpdatedAt: time.Now().UnixMilli()}
	s.groupWorkMu.Lock()
	_, err := s.cfg.Store.GetCatGroupWorkflow(ctx, w.ChainID)
	if err == nil {
		s.groupWorkMu.Unlock()
		return apperr.New(apperr.TaskConflict, "这条任务已经安排过")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		s.groupWorkMu.Unlock()
		return err
	}
	if request, requestErr := s.cfg.Store.GetCatGroupRequest(ctx, w.ChainID); requestErr == nil {
		detail, _ := s.cfg.Store.LatestCatGroupRouteForChain(ctx, w.GroupID, w.ChainID)
		d := s.materializeGroupDecision(ctx, groupDecision(request.Message, nil, tasks, detail.OutputText))
		err = s.cfg.Store.CommitCatGroupDecision(ctx, w.ChainID, d, &w)
	} else if errors.Is(requestErr, sql.ErrNoRows) {
		err = s.cfg.Store.SaveCatGroupWorkflow(ctx, &w)
	} else {
		err = requestErr
	}
	if err == nil {
		if s.groupWorkContexts == nil {
			s.groupWorkContexts = map[string]context.Context{}
		}
		s.groupWorkContexts[w.ChainID] = context.WithoutCancel(ctx)
	}
	s.groupWorkMu.Unlock()
	if err != nil {
		return err
	}
	s.emitGroupWorkflow(w)
	s.advanceGroupWorkflow(g, w.ChainID, ctx)
	return nil
}

// readyGroupTasks implements the join: A finishing cannot release C while B is
// still running, failed or blocked. One member's FIFO handles multiple ready jobs.
func readyGroupTasks(w *store.CatGroupWorkflow) []store.CatGroupWorkTask {
	states := map[string]string{}
	for _, t := range w.Tasks {
		states[t.ID] = t.Status
	}
	var ready []store.CatGroupWorkTask
	for i := range w.Tasks {
		t := &w.Tasks[i]
		if t.Status != "pending" {
			continue
		}
		ok := true
		for _, dep := range t.DependsOn {
			if states[dep] != "done" {
				ok = false
				break
			}
		}
		if ok {
			t.Status = "queued"
			t.TurnID = ""
			ready = append(ready, *t)
		}
	}
	return ready
}

func (s *Service) advanceGroupWorkflow(g store.CatGroup, chainID string, contexts ...context.Context) {
	ctx := context.Background()
	if len(contexts) > 0 {
		ctx = contexts[0]
	}
	// Active workflows already carry a lifetime. Read it before binding a new
	// one: synchronous startup failures can advance while dispatchMu is held.
	s.groupWorkMu.Lock()
	if saved := s.groupWorkContexts[chainID]; saved != nil {
		ctx = saved
	}
	s.groupWorkMu.Unlock()
	ctx = s.groupContext(ctx, g.ID)
	s.groupWorkMu.Lock()
	if saved := s.groupWorkContexts[chainID]; saved != nil {
		ctx = saved
	}
	if groupContextStopped(ctx) {
		s.groupWorkMu.Unlock()
		return
	}
	w, err := s.cfg.Store.GetCatGroupWorkflow(ctx, chainID)
	if err != nil || w.GroupID != g.ID || w.Status == "completed" || w.Status == "stopped" || w.Dismissed {
		s.groupWorkMu.Unlock()
		return
	}
	ready := readyGroupTasks(&w)
	done, blocked := true, false
	for _, t := range w.Tasks {
		if t.Status != "done" {
			done = false
		}
		if t.Status == "failed" || t.Status == "blocked" || t.Status == "stopped" {
			blocked = true
		}
	}
	w.Status = "running"
	if blocked {
		w.Status = "blocked"
	}
	if done {
		w.Status = "completed"
	}
	if done {
		delete(s.groupWorkContexts, chainID)
	}
	err = s.cfg.Store.SaveCatGroupWorkflow(ctx, &w)
	s.groupWorkMu.Unlock()
	if err != nil {
		s.cfg.Logf("group workflow save failed: %v", err)
		return
	}
	s.emitGroupWorkflow(w)
	if done {
		var b strings.Builder
		b.WriteString("协作任务已通过联调验收。\n")
		for _, t := range w.Tasks {
			fmt.Fprintf(&b, "\n%s：%s", t.Title, t.Result)
			if t.Evidence != "" {
				fmt.Fprintf(&b, "\n验证：%s", t.Evidence)
			}
		}
		s.postSystem(ctx, g.ID, chainID, 0, "system", b.String(), "workflow_summary", store.CatRouteInfo{Mode: "done"}, "", "")
		return
	}
	for _, t := range ready {
		s.wakeMember(ctx, g, t.MemberID, chainID, 0, w.OriginSeq, false, "workflow", t.ID)
	}
}

func (s *Service) beginGroupWorkTask(chainID, taskID string, ownerTurn ...string) bool {
	s.groupWorkMu.Lock()
	defer s.groupWorkMu.Unlock()
	w, err := s.cfg.Store.GetCatGroupWorkflow(context.Background(), chainID)
	if err != nil || w.Status == "stopped" || w.Dismissed {
		return false
	}
	for i := range w.Tasks {
		t := &w.Tasks[i]
		if t.ID != taskID {
			continue
		}
		if t.Status != "queued" || t.TurnID != "" {
			return false
		}
		if len(ownerTurn) == 2 {
			if t.MemberID != ownerTurn[0] {
				return false
			}
			t.TurnID = ownerTurn[1]
		}
		t.Status = "running"
		t.RetryAt = 0
		t.Result = ""
		t.Attempts++
		if err := s.cfg.Store.SaveCatGroupWorkflow(context.Background(), &w); err != nil {
			return false
		}
		s.emitGroupWorkflow(w)
		return true
	}
	return false
}

func (s *Service) finishGroupWorkTask(g store.CatGroup, chainID, taskID, status, summary, evidence, turnID string, result ...groupTaskResult) {
	if taskID == "" {
		return
	}
	ctx := context.Background()
	s.groupWorkMu.Lock()
	w, err := s.cfg.Store.GetCatGroupWorkflow(ctx, chainID)
	if err != nil || w.Status == "stopped" || w.Dismissed {
		s.groupWorkMu.Unlock()
		return
	}
	var task store.CatGroupWorkTask
	for i := range w.Tasks {
		t := &w.Tasks[i]
		if t.ID != taskID {
			continue
		}
		if t.Status != "running" || (t.TurnID != "" && t.TurnID != turnID) {
			s.groupWorkMu.Unlock()
			return
		}
		if status != "done" && status != "blocked" && status != "failed" && status != "stopped" {
			status = "blocked"
			summary = "没有收到明确的任务完成状态，请检查过程后重试"
		}
		if t.Kind == "verify" && status == "done" && strings.TrimSpace(evidence) == "" {
			status = "blocked"
			summary = "验收未提供实际验证结果，不能标记整体完成"
		}
		if t.Kind == "verify" && status == "done" && (len(result) == 0 || result[0].ToolCount == 0) {
			status = "blocked"
			summary = "验收没有实际检查工具记录，不能确认通过"
		}
		t.Status = status
		t.Result = truncateRunes(summary, 2000)
		t.Evidence = truncateRunes(evidence, 2000)
		t.TurnID = turnID
		task = *t
		break
	}
	if task.Kind == "verify" && task.Status == "failed" && len(result) > 0 && len(result[0].Rework) > 0 {
		if resetGroupRework(&w, task, result[0].Rework) {
			if err := s.resetGroupWorkflowAssignments(ctx, w); err != nil {
				s.groupWorkMu.Unlock()
				s.cfg.Logf("group rework outbox reset: %v", err)
				return
			}
			status = "pending"
		}
	}
	err = s.cfg.Store.SaveCatGroupWorkflow(ctx, &w)
	s.groupWorkMu.Unlock()
	if err != nil {
		s.cfg.Logf("group workflow completion save failed: %v", err)
		return
	}
	if task.ID == "" {
		return
	}
	if status == "blocked" || (status == "failed" && turnID == "") {
		s.postFail(ctx, g.ID, chainID, 0, store.CatGroupMember{ID: task.MemberID, Name: task.Title}, summary)
	}
	s.advanceGroupWorkflow(g, chainID)
}

type groupTaskResult struct {
	Status    string   `json:"status"`
	Summary   string   `json:"summary"`
	Evidence  string   `json:"evidence"`
	Rework    []string `json:"rework"`
	ToolCount int      `json:"-"`
}

// Verification may request two bounded repair rounds. Only named predecessors
// and their dependents are rerun; unrelated completed work is preserved.
func resetGroupRework(w *store.CatGroupWorkflow, verify store.CatGroupWorkTask, ids []string) bool {
	if verify.Attempts >= 3 {
		return false
	}
	reset := map[string]bool{}
	for _, id := range ids {
		found := false
		for _, t := range w.Tasks {
			if t.ID == id && t.Kind != "verify" && t.Status == "done" {
				found = true
			}
		}
		if !found {
			return false
		}
		reset[id] = true
	}
	for changed := true; changed; {
		changed = false
		for _, t := range w.Tasks {
			for _, dep := range t.DependsOn {
				if reset[dep] && !reset[t.ID] {
					reset[t.ID] = true
					changed = true
				}
			}
		}
	}
	for i := range w.Tasks {
		if reset[w.Tasks[i].ID] {
			w.Tasks[i].Status = "pending"
			w.Tasks[i].Result = "验收反馈：" + verify.Result
			w.Tasks[i].Evidence = ""
		}
	}
	return true
}

const groupTaskResultTag = "<cat_task_result>"

func parseGroupTaskResult(text string) (string, groupTaskResult) {
	r := groupTaskResult{Status: "blocked", Summary: "没有收到明确的任务完成状态，请检查过程后重试"}
	start := strings.LastIndex(text, groupTaskResultTag)
	if start < 0 {
		return text, r
	}
	rest := text[start+len(groupTaskResultTag):]
	end := strings.Index(rest, "</cat_task_result>")
	if end < 0 {
		return text, r
	}
	var parsed groupTaskResult
	if json.Unmarshal([]byte(rest[:end]), &parsed) == nil {
		if parsed.Status == "done" || parsed.Status == "blocked" || parsed.Status == "failed" {
			r = parsed
		}
	}
	visible := strings.TrimSpace(text[:start] + rest[end+len("</cat_task_result>"):])
	if visible == "" {
		visible = r.Summary
	}
	return visible, r
}

func (s *Service) groupWorkTaskPrompt(chainID, taskID string) string {
	w, err := s.cfg.Store.GetCatGroupWorkflow(context.Background(), chainID)
	if err != nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "本次开发协作的原始需求：\n%s\n\n任务清单：\n", w.Request)
	for _, t := range w.Tasks {
		fmt.Fprintf(&b, "- %s [%s] %s（负责人 id=%s，阶段=%s，依赖=%s）\n", t.ID, t.Status, t.Title, t.MemberID, t.Kind, strings.Join(t.DependsOn, ","))
	}
	for _, t := range w.Tasks {
		if t.ID == taskID {
			fmt.Fprintf(&b, "\n你本轮只负责任务 %s：%s\n%s\n", t.ID, t.Title, t.Instruction)
			for _, dep := range t.DependsOn {
				for _, other := range w.Tasks {
					if other.ID == dep {
						fmt.Fprintf(&b, "\n已完成依赖 %s：%s\n验证：%s\n", dep, other.Result, other.Evidence)
					}
				}
			}
			if t.Result != "" {
				fmt.Fprintf(&b, "\n上次未完成原因：%s\n", t.Result)
			}
			if t.Kind == "verify" {
				b.WriteString("\n这是最终验收：必须实际运行适用测试/检查，检查各部分对接与原始需求。测试失败、依赖不齐或无法验证时，不得标记 done。证据中列出执行过的命令、结果及未覆盖范围。若能明确定位需返工的前置任务，在状态块增加 rework:[\"任务id\"]、status=failed，调度器最多自动返工两轮；不要把验收任务本身放入 rework。\n")
			}
		}
	}
	b.WriteString(`
协作调度规则：下列群聊记录仅供背景，优先执行本轮指定任务，不替别人做整项任务。
依赖产物缺失、需要澄清、还没完成时，明确报告 blocked 或 failed；结束回复不代表任务完成。
调度器会按依赖自动通知下一位，本轮不要用「交给谁」重复派活。不要因为看到别人在干活就声称全部完成。
正常写面向用户的汇报，在最终回复末尾另附以下状态块（不要放在代码围栏内）：
<cat_task_result>{"status":"done 或 blocked 或 failed","summary":"本任务交付或阻塞原因","evidence":"实际执行的测试/检查及结果，没有则为空"}</cat_task_result>
`)
	return b.String()
}

func (s *Service) retryGroupWorkTask(ctx context.Context, g store.CatGroup, chainID, memberID string, taskIDs ...string) (bool, error) {
	ctx = s.groupContext(ctx, g.ID)
	s.groupWorkMu.Lock()
	w, err := s.cfg.Store.GetCatGroupWorkflow(ctx, chainID)
	if errors.Is(err, sql.ErrNoRows) {
		s.groupWorkMu.Unlock()
		return false, nil
	}
	if err != nil {
		s.groupWorkMu.Unlock()
		return true, err
	}
	if w.GroupID != g.ID {
		s.groupWorkMu.Unlock()
		return true, s.errGroupNotFound()
	}
	if w.Dismissed {
		s.groupWorkMu.Unlock()
		return true, apperr.New(apperr.TaskConflict, "该任务已从记录中移除，请发送新消息发起任务")
	}
	found := false
	taskID := ""
	if len(taskIDs) > 0 {
		taskID = taskIDs[0]
	}
	for i := range w.Tasks {
		t := &w.Tasks[i]
		if t.MemberID == memberID && (taskID == "" || t.ID == taskID) && (t.Status == "blocked" || t.Status == "failed" || t.Status == "stopped") {
			t.Status = "pending"
			t.TurnID = ""
			t.RetryAt = 0
			t.Result, t.Evidence = "", ""
			found = true
			break
		}
	}
	if !found {
		s.groupWorkMu.Unlock()
		return true, apperr.New(apperr.TaskConflict, "该成员没有可重试的协作任务")
	}
	if w.Status == "stopped" {
		for i := range w.Tasks {
			if w.Tasks[i].Status == "stopped" {
				w.Tasks[i].Status = "pending"
			}
		}
	}
	w.Status = "running"
	if s.groupWorkContexts == nil {
		s.groupWorkContexts = map[string]context.Context{}
	}
	s.groupWorkContexts[chainID] = context.WithoutCancel(ctx)
	if err = s.resetGroupWorkflowAssignments(ctx, w); err != nil {
		s.groupWorkMu.Unlock()
		return true, err
	}
	err = s.cfg.Store.SaveCatGroupWorkflow(ctx, &w)
	s.groupWorkMu.Unlock()
	if err == nil {
		s.advanceGroupWorkflow(g, chainID, ctx)
	}
	return true, err
}

func (s *Service) stopGroupWorkflows(ctx context.Context, groupID, memberID string) {
	s.groupWorkMu.Lock()
	defer s.groupWorkMu.Unlock()
	list, err := s.cfg.Store.ListCatGroupWorkflows(ctx, groupID)
	if err != nil {
		return
	}
	for _, w := range list {
		if w.Status == "completed" || w.Status == "stopped" || w.Dismissed {
			continue
		}
		changed := false
		for i := range w.Tasks {
			t := &w.Tasks[i]
			if (memberID == "" || t.MemberID == memberID) && (t.Status == "running" || t.Status == "queued" || t.Status == "pending") {
				t.Status = "stopped"
				t.Result = "已停止，后续依赖不会自动开工"
				changed = true
			}
		}
		if !changed && memberID != "" {
			continue
		}
		w.Status = "blocked"
		if memberID == "" {
			w.Status = "stopped"
		}
		if err := s.cfg.Store.SaveCatGroupWorkflow(ctx, &w); err == nil {
			s.emitGroupWorkflow(w)
		}
	}
}

// On a new service instance, keep the durable plan but do not replay writes whose
// outcome was lost during shutdown. An explicit retry can continue the graph.
func (s *Service) recoverGroupWorkflows(ctx context.Context, groupID string) {
	s.groupWorkMu.Lock()
	defer s.groupWorkMu.Unlock()
	if s.groupWorkRecovered == nil {
		s.groupWorkRecovered = map[string]bool{}
	}
	if s.groupWorkRecovered[groupID] {
		return
	}
	list, err := s.cfg.Store.ListCatGroupWorkflows(ctx, groupID)
	if err != nil {
		return
	}
	for _, w := range list {
		if w.Status == "completed" || w.Status == "stopped" || w.Dismissed {
			continue
		}
		changed := false
		states := map[string]string{}
		for _, t := range w.Tasks {
			states[t.ID] = t.Status
		}
		for i := range w.Tasks {
			t := &w.Tasks[i]
			readyPending := t.Status == "pending"
			for _, dep := range t.DependsOn {
				readyPending = readyPending && states[dep] == "done"
			}
			if t.Status == "running" || t.Status == "queued" || readyPending {
				t.Status = "blocked"
				t.Result = "应用中断，结果尚未确认，请检查后重试"
				changed = true
			}
		}
		allDone := len(w.Tasks) > 0
		for _, t := range w.Tasks {
			allDone = allDone && t.Status == "done"
		}
		if allDone {
			for i := range w.Tasks {
				if w.Tasks[i].Kind == "verify" {
					w.Tasks[i].Status = "blocked"
					w.Tasks[i].Result = "验收结束时应用中断，尚未汇总，请重试确认"
					changed = true
				}
			}
		}
		if changed {
			w.Status = "blocked"
			if err := s.cfg.Store.SaveCatGroupWorkflow(ctx, &w); err != nil {
				return
			}
		}
	}
	s.groupWorkRecovered[groupID] = true
}

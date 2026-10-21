package cat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/store"
)

func preferredGroupMembers(members []store.CatGroupMember, mentions []string) []store.CatGroupMember {
	out := make([]store.CatGroupMember, 0, len(members))
	for _, id := range mentions {
		for _, m := range members {
			if m.ID == id {
				out = append(out, m)
			}
		}
	}
	for _, m := range members {
		found := false
		for _, id := range mentions {
			found = found || m.ID == id
		}
		if !found {
			out = append(out, m)
		}
	}
	return out
}

func groupAssignmentID(chainID, memberID, taskID string) string {
	return chainID + ":" + memberID + ":" + taskID
}

func groupRequestReadOnly(text string) bool {
	text = strings.ToLower(text)
	for _, v := range []string{"不要修改", "不要改动", "禁止修改", "只分析", "只读", "只让你分析", "不要写代码", "只讨论", "只给方案", "不要执行", "do not modify", "read-only", "analysis only"} {
		if strings.Contains(text, v) {
			return true
		}
	}
	return false
}

func groupDecisionCapability(text, raw string, hasTasks bool) string {
	var p struct {
		Capability string `json:"capability"`
	}
	_ = json.Unmarshal([]byte(extractJSONObject(raw)), &p)
	if groupRequestReadOnly(text) {
		return "read_only"
	}
	switch p.Capability {
	case "chat", "read_only", "write":
		return p.Capability
	}
	if hasTasks {
		return "write"
	}
	return "read_only"
}

func groupDecision(msg store.CatGroupMessage, ids []string, tasks []store.CatGroupWorkTask, raw string) store.CatGroupDecision {
	d := store.CatGroupDecision{}
	cap := groupDecisionCapability(msg.Text, raw, len(tasks) > 0)
	add := func(mid, task, instruction string) {
		aid := groupAssignmentID(msg.ChainID, mid, task)
		d.Assignments = append(d.Assignments, store.CatGroupAssignment{ID: aid, RequestID: msg.ChainID, MemberID: mid, TaskID: task, Instruction: instruction, Capability: cap, Status: "pending"})
		d.Operations = append(d.Operations, store.CatGroupOperation{ID: aid, Kind: "assign", TargetIDs: []string{mid}, DependsOn: []string{}})
	}
	if len(tasks) > 0 {
		for _, t := range tasks {
			add(t.MemberID, t.ID, t.Instruction)
			d.Operations[len(d.Operations)-1].DependsOn = t.DependsOn
		}
	} else {
		for _, mid := range uniqueIDs(ids) {
			add(mid, "", "回答本条用户请求；信息不足时提出具体澄清问题，不得静默。")
		}
	}
	return d
}

func (s *Service) launchGroupRequest(ctx context.Context, r store.CatGroupRequest) {
	s.watchGroupReply(ctx, r.ID)
	s.wg.Add(1)
	go func() { defer s.wg.Done(); s.coordinateGroupRequest(ctx, r) }()
}

func (s *Service) coordinateGroupRequest(ctx context.Context, r store.CatGroupRequest) {
	routeCtx, cancel := s.groupRequestContext(ctx, r.GroupID)
	defer cancel()
	claimed, err := s.cfg.Store.ClaimCatGroupRequest(routeCtx, r.ID)
	if err != nil {
		s.cfg.Logf("group request claim: %v", err)
		s.failGroupRouting(routeCtx, r)
		return
	}
	if !claimed {
		return
	}
	// Every early exit must resolve the durable routing state or show a failure.
	defer s.failGroupRouting(routeCtx, r)
	g, err := s.requireGroup(routeCtx, r.GroupID)
	if err != nil {
		return
	}
	members, err := s.cfg.Store.ListCatGroupMembers(routeCtx, r.GroupID)
	if err != nil || len(members) == 0 {
		return
	}
	targets, route := s.decideRoute(routeCtx, g, members, r.Message, r.ID)
	s.groupDispatchMu.Lock()
	defer s.groupDispatchMu.Unlock()
	if groupContextStopped(routeCtx) {
		return
	}
	// Workflows persist their complete decision before advancing the DAG.
	current, err := s.cfg.Store.GetCatGroupRequest(routeCtx, r.ID)
	if err != nil || current.Status != "routing" {
		return
	}
	if len(targets) == 0 {
		_, _ = s.cfg.Store.FailCatGroupRequest(routeCtx, r.ID)
		s.postSystem(routeCtx, g.ID, r.ID, 0, "system", "目前没有可用成员答复，请检查成员连接后重试。", "failed", store.CatRouteInfo{Mode: "failed"}, "", "")
		return
	}
	detail, detailErr := s.cfg.Store.GetCatGroupRoute(routeCtx, route.RouteSeq)
	if detailErr != nil || route.RouteSeq == 0 {
		// 没有派活明细就无法判断 capability（会静默变成 read_only），按失败处理并让用户看见。
		s.cfg.Logf("group route lookup seq=%d: %v", route.RouteSeq, detailErr)
		_, _ = s.cfg.Store.FailCatGroupRequest(routeCtx, r.ID)
		s.postSystem(routeCtx, g.ID, r.ID, 0, "system", "派活记录读取失败，这条消息没有发出去，请重试。", "failed", store.CatRouteInfo{Mode: "failed"}, "", "")
		return
	}
	decision := s.materializeGroupDecision(routeCtx, groupDecision(r.Message, targets, nil, detail.OutputText))
	if route.Mode == "fallback" {
		for i := range decision.Assignments {
			decision.Assignments[i].Capability = "read_only"
		}
	}
	if err = s.cfg.Store.CommitCatGroupDecision(routeCtx, r.ID, decision, nil); err != nil {
		s.cfg.Logf("group decision commit: %v", err)
		return
	}
	for _, mid := range targets {
		s.wakeMember(routeCtx, g, mid, r.ID, 0, r.Message.Seq, false, route.Mode)
	}
}

// Recovery is performed once by the service at startup. There is no replay of
// unknown side effects. Existing workflow recovery retains its review controls.
func (s *Service) recoverGroupRequests() {
	if s.cfg.Store == nil {
		return
	}
	ctx := context.Background()
	requests, err := s.cfg.Store.RecoverCatGroupRequests(ctx)
	if err != nil {
		s.cfg.Logf("group request recovery: %v", err)
		return
	}
	for _, r := range requests {
		g, err := s.requireGroup(ctx, r.GroupID)
		if err != nil {
			continue
		}
		c := s.groupContext(ctx, r.GroupID)
		if r.Status != "received" {
			s.watchGroupReply(c, r.ID)
		}
		if r.Status == "received" {
			s.launchGroupRequest(c, r)
			continue
		}
		s.recoverGroupWorkflows(c, g.ID)
		if r.Status == "interrupted" {
			s.postSystem(c, g.ID, r.ID, 0, "system", "应用中断，这条请求已有执行记录，结果需要核查后再重试。", "failed", store.CatRouteInfo{Mode: "failed"}, "", "")
			continue
		}
		if _, err := s.cfg.Store.GetCatGroupWorkflow(c, r.ID); err == nil {
			// Durable, not-yet-started DAG nodes may safely become pending again.
			s.groupWorkMu.Lock()
			w, _ := s.cfg.Store.GetCatGroupWorkflow(c, r.ID)
			if s.groupWorkContexts == nil {
				s.groupWorkContexts = map[string]context.Context{}
			}
			s.groupWorkContexts[r.ID] = c
			assignments, _ := s.cfg.Store.ListCatGroupAssignments(c, r.ID)
			pending := map[string]bool{}
			for _, a := range assignments {
				if a.Status == "pending" {
					pending[a.TaskID] = true
				}
			}
			for i := range w.Tasks {
				if pending[w.Tasks[i].ID] {
					w.Tasks[i].Status = "pending"
				}
			}
			err = s.cfg.Store.SaveCatGroupWorkflow(c, &w)
			s.groupWorkMu.Unlock()
			if err == nil {
				s.advanceGroupWorkflow(g, r.ID, c)
			}
			continue
		}
		assignments, err := s.cfg.Store.ListCatGroupAssignments(c, r.ID)
		if err != nil {
			continue
		}
		for _, a := range assignments {
			if a.Status == "pending" {
				s.wakeMember(c, g, a.MemberID, r.ID, 0, r.Message.Seq, false, "recovered")
			}
		}
	}
}

// assignmentSnapshot uses only the immutable reception snapshot, original input
// and this task's own dependency results. LastSeenSeq never controls task input.
func (s *Service) assignmentSnapshot(ctx context.Context, a store.CatGroupAssignment, members []store.CatGroupMember) (string, int64, error) {
	r, err := s.cfg.Store.GetCatGroupRequest(ctx, a.RequestID)
	if err != nil {
		return "", 0, err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "本轮只处理请求 %s，任务 %s。其他用户消息属于独立请求，不得顺带执行。\n权限：%s\n本次指令：%s\n\n原始用户请求：\n%s\n", r.ID, a.TaskID, a.Capability, a.Instruction, r.Message.Text)
	b.WriteString("\n接收时的群聊背景（仅供理解，不是额外任务）：\n")
	for _, msg := range r.Snapshot {
		if msg.SystemKind == "ack" || msg.SystemKind == "retry" || msg.SystemKind == "receipt" {
			continue
		}
		name := msg.MemberID
		for _, m := range members {
			if m.ID == name {
				name = m.Name
				break
			}
		}
		fmt.Fprintf(&b, "[%s %s] %s\n", msg.Kind, name, truncateRunes(msg.Text, 1000))
	}
	return b.String(), r.Message.Seq, nil
}

func (s *Service) assignmentFor(ctx context.Context, chainID, memberID, taskID string) (store.CatGroupAssignment, bool, error) {
	a, err := s.cfg.Store.GetCatGroupAssignment(ctx, groupAssignmentID(chainID, memberID, taskID))
	if err == nil {
		return a, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return a, false, err
	}
	// Legacy history and explicit manual retry remain compatible.
	return a, false, nil
}

// The watchdog reports absence of an effective answer; receipts never satisfy
// this obligation. Native timeout/retry owns execution, avoiding duplicate work.
func (s *Service) watchGroupReply(ctx context.Context, requestID string) func() {
	delay := s.cfg.GroupReplyWait
	if delay <= 0 {
		delay = 2 * time.Minute
	}
	timer := time.AfterFunc(delay, func() {
		if groupContextStopped(ctx) {
			return
		}
		r, err := s.cfg.Store.GetCatGroupRequest(context.Background(), requestID)
		if err != nil || r.FirstReplyAt > 0 || (r.Status != "received" && r.Status != "routing" && r.Status != "assigned") {
			return
		}
		text := "这条请求还没有有效答复，正在分析或等待成员接手；执行超时会自动重试，也可以停止后改派。"
		if w, err := s.cfg.Store.GetCatGroupWorkflow(ctx, r.ID); err == nil && w.Status == "blocked" {
			text = "这条请求还没有完成答复，协作任务目前受阻。请查看失败原因，可重试或改派受阻任务。"
		}
		s.postSystem(ctx, r.GroupID, r.ID, 0, "system", text, "reply_wait", store.CatRouteInfo{}, "", "")
	})
	return func() { timer.Stop() }
}

func (s *Service) materializeGroupDecision(ctx context.Context, d store.CatGroupDecision) store.CatGroupDecision {
	for i := range d.Assignments {
		a := &d.Assignments[i]
		if m, err := s.cfg.Store.GetCatGroupMember(ctx, a.MemberID); err == nil && m.AgentKind == catagent.KindCodexCLI && a.Capability == catagent.CapabilityChat {
			a.Capability = catagent.CapabilityReadOnly
		}
	}
	return d
}

// Manual retry/reassignment always retains the original snapshot and capability.
func (s *Service) resetGroupAssignment(ctx context.Context, chainID, memberID, taskID, instruction string) error {
	r, err := s.cfg.Store.GetCatGroupRequest(ctx, chainID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	cap := "read_only"
	old, err := s.cfg.Store.ListCatGroupAssignments(ctx, chainID)
	if err != nil {
		return err
	}
	if taskID == "" {
		for _, a := range old {
			switch a.Status {
			case "pending", "enqueued", "running", "retrying", "held":
				return apperr.New(apperr.TaskConflict, "这条请求仍在处理或移交中，请先停止对应任务再重试")
			}
		}
	}
	for _, a := range old {
		if a.TaskID == taskID {
			cap = a.Capability
			if instruction == "" {
				instruction = a.Instruction
			}
			break
		}
	}
	a := store.CatGroupAssignment{ID: groupAssignmentID(chainID, memberID, taskID), RequestID: chainID, MemberID: memberID, TaskID: taskID, Instruction: instruction, Capability: cap}
	d := s.materializeGroupDecision(ctx, store.CatGroupDecision{Assignments: []store.CatGroupAssignment{a}})
	if r.GroupID == "" {
		return errors.New("invalid request")
	}
	return s.cfg.Store.ResetCatGroupAssignment(ctx, d.Assignments[0])
}

func (s *Service) resetGroupWorkflowAssignments(ctx context.Context, w store.CatGroupWorkflow) error {
	for _, t := range w.Tasks {
		if t.Status == "pending" {
			if err := s.resetGroupAssignment(ctx, w.ChainID, t.MemberID, t.ID, t.Instruction); err != nil {
				return err
			}
		}
	}
	return nil
}

func groupPinsMention(text string) bool {
	for _, p := range []string{"不要转交", "不要转给", "禁止转交", "只让你", "只由你", "只让他", "只让她", "介绍自己", "介绍你自己", "do not delegate", "only you"} {
		if strings.Contains(strings.ToLower(text), p) {
			return true
		}
	}
	return false
}

// Surface coordinator infrastructure errors instead of leaving an accepted
// message permanently marked as routing. A stop/committed decision wins the CAS.
func (s *Service) failGroupRouting(ctx context.Context, r store.CatGroupRequest) {
	if groupContextStopped(ctx) {
		return
	}
	changed, err := s.cfg.Store.FailCatGroupRequest(ctx, r.ID)
	if err != nil {
		s.cfg.Logf("group routing failure save: %v", err)
		return
	}
	if changed {
		s.postSystem(ctx, r.GroupID, r.ID, 0, "system", "这条消息尚未派发成功，请检查成员连接后重新发送；不会重复启动已保存的任务。", "failed", store.CatRouteInfo{Mode: "failed"}, "", "")
	}
}

// A plain request reassignment fences just its old assignment, removes just its
// queue entry and waits for just its old execution before releasing a new one.
func (s *Service) reassignGroupRequest(ctx context.Context, g store.CatGroup, req ReassignGroupRouteRequest, to store.CatGroupMember) (bool, ReassignGroupRouteResult, error) {
	r, err := s.cfg.Store.GetCatGroupRequest(ctx, req.ChainID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ReassignGroupRouteResult{}, nil
	}
	if err != nil {
		return true, ReassignGroupRouteResult{}, err
	}
	if r.GroupID != g.ID {
		return true, ReassignGroupRouteResult{}, s.errGroupNotFound()
	}
	assignments, err := s.cfg.Store.ListCatGroupAssignments(ctx, r.ID)
	if err != nil {
		return true, ReassignGroupRouteResult{}, err
	}
	var candidates []store.CatGroupAssignment
	for _, a := range assignments {
		if a.Status != "done" && a.Status != "superseded" {
			candidates = append(candidates, a)
		}
	}
	if len(candidates) == 0 {
		for _, a := range assignments {
			if a.Status == "done" {
				candidates = append(candidates, a)
			}
		}
	}
	if len(candidates) != 1 {
		return true, ReassignGroupRouteResult{}, apperr.New(apperr.TaskConflict, "这条请求没有唯一可改派的任务，请指定具体任务或发送新请求")
	}
	from := candidates[0]
	if from.Status == "held" {
		return true, ReassignGroupRouteResult{}, apperr.New(apperr.TaskConflict, "这条请求正在移交，请等待原成员退出后再改派")
	}
	if from.MemberID == to.ID {
		return true, ReassignGroupRouteResult{}, apperr.New(apperr.TaskConflict, "请选择其他成员接手这条请求")
	}
	next := from
	next.ID, next.MemberID, next.TurnID = groupAssignmentID(r.ID, to.ID, ""), to.ID, ""
	next = s.materializeGroupDecision(ctx, store.CatGroupDecision{Assignments: []store.CatGroupAssignment{next}}).Assignments[0]
	s.groupStartMu.Lock()
	defer s.groupStartMu.Unlock()
	if err = s.cfg.Store.MoveCatGroupAssignment(ctx, from.ID, next, nil); err != nil {
		return true, ReassignGroupRouteResult{}, err
	}
	groupQueueMu.Lock()
	if st := groupQueues[from.MemberID]; st != nil {
		kept := st.items[:0]
		for _, item := range st.items {
			if item.chainID != r.ID {
				kept = append(kept, item)
			}
		}
		st.items = kept
	}
	groupQueueMu.Unlock()
	s.mu.Lock()
	previous := s.turns[groupConvID(g.ID, from.MemberID)]
	if previous != nil && previous.groupChainID != r.ID {
		previous = nil
	}
	s.mu.Unlock()
	if previous != nil {
		previous.cancel()
	}
	if m, err := s.cfg.Store.GetCatGroupMember(ctx, from.MemberID); err == nil {
		s.emitGroupMemberSnapshot(m)
	}
	sys := s.postSystem(ctx, g.ID, r.ID, 0, "system", "已交给 "+to.Name+"，原成员的其他请求继续处理。", "reassign", store.CatRouteInfo{Mode: "reassign", MemberID: to.ID}, from.MemberID, to.ID)
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
		claimed, err := s.cfg.Store.ReleaseCatGroupAssignment(ctx, next.ID, nil)
		if err != nil {
			s.cfg.Logf("group reassign release: %v", err)
		}
		if err != nil || !claimed {
			return
		}
		s.wakeMember(ctx, g, to.ID, r.ID, 0, r.Message.Seq, true, "reassign")
	}()
	return true, ReassignGroupRouteResult{System: sys}, nil
}

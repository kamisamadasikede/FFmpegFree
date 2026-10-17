package cat

import (
	"context"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/i18n"
	"FFmpegFree/internal/id"
	"FFmpegFree/internal/store"
)

type groupQueueItem struct {
	chainID  string
	hop      int
	afterSeq int64
	reassign bool
	fromSeq  int64
}

// memberQueueState：每个成员的内存 FIFO（契约 6.21.6 ⑥ / 6.21.17 ④）。不落库，应用重启丢失（一期接受）。
// active = 已叫起一轮（含启动中），同一成员第二个任务进 items 排队；不同成员互不影响（并行）。
type memberQueueState struct {
	active  bool
	items   []groupQueueItem
	running *GroupRunning // 最近一次推送的干活状态；nil = 没在干活
}

var (
	groupQueueMu sync.Mutex
	groupQueues  = map[string]*memberQueueState{}
)

func queueStateLocked(memberID string) *memberQueueState {
	st := groupQueues[memberID]
	if st == nil {
		st = &memberQueueState{}
		groupQueues[memberID] = st
	}
	return st
}

func queueTasksLocked(st *memberQueueState) []store.CatGroupQueuedTask {
	out := make([]store.CatGroupQueuedTask, 0, len(st.items))
	for i, it := range st.items {
		out = append(out, store.CatGroupQueuedTask{ChainID: it.chainID, Hop: it.hop, FromSeq: it.fromSeq, QueuePosition: i})
	}
	return out
}

// decorateGroupMember 填运行时只读字段 status / queueLength / queue（契约 6.21.17 ④）。
func decorateGroupMember(m *store.CatGroupMember) {
	groupQueueMu.Lock()
	defer groupQueueMu.Unlock()
	st := groupQueues[m.ID]
	m.Status, m.QueueLength, m.Queue = "idle", 0, []store.CatGroupQueuedTask{}
	if st == nil {
		return
	}
	m.Queue = queueTasksLocked(st)
	m.QueueLength = len(m.Queue)
	switch {
	case st.running != nil:
		m.Status = "running"
	case m.QueueLength > 0:
		m.Status = "queued"
	}
}

func decorateGroupMembers(list []store.CatGroupMember) {
	for i := range list {
		decorateGroupMember(&list[i])
	}
}

func groupRunningSnapshot(memberID string) *GroupRunning {
	groupQueueMu.Lock()
	defer groupQueueMu.Unlock()
	if st := groupQueues[memberID]; st != nil && st.running != nil {
		r := *st.running
		if r.StartedAt > 0 {
			r.ElapsedMs = time.Now().UnixMilli() - r.StartedAt
		}
		return &r
	}
	return nil
}

func (s *Service) wakeMember(ctx context.Context, g store.CatGroup, memberID, chainID string, hop int, afterSeq int64, reassign bool, mode string) {
	m, err := s.cfg.Store.GetCatGroupMember(ctx, memberID)
	if err != nil {
		return
	}
	groupQueueMu.Lock()
	st := queueStateLocked(m.ID)
	if st.active {
		fromSeq := afterSeq
		if fromSeq == 0 {
			fromSeq = s.chainOriginSeq(ctx, g.ID, chainID)
		}
		st.items = append(st.items, groupQueueItem{chainID: chainID, hop: hop, afterSeq: afterSeq, reassign: reassign, fromSeq: fromSeq})
		groupQueueMu.Unlock()
		// 入队灰字（契约 6.21.17 ④ 5）：systemKind=route，route.mode 沿用本次派活来源，queued=true。
		s.postSystem(ctx, g.ID, chainID, hop, "system",
			i18n.T(i18n.KeyCatGroupRouteQueued, map[string]string{"name": m.Name}), "route",
			store.CatRouteInfo{Mode: mode, MemberID: m.ID, Queued: true}, "", m.ID)
		s.emitGroupMemberSnapshot(m)
		return
	}
	st.active = true
	groupQueueMu.Unlock()
	s.startGroupMemberTurn(ctx, g, m, chainID, hop, afterSeq, reassign)
}

// chainOriginSeq：换人 / 重试入队时没有 afterSeq，用本链用户消息的 seq 当 fromSeq。
func (s *Service) chainOriginSeq(ctx context.Context, groupID, chainID string) int64 {
	list, err := s.cfg.Store.ListCatGroupMessages(ctx, groupID, 0, 200)
	if err != nil {
		return 0
	}
	var last int64
	for _, m := range list {
		if m.ChainID != chainID {
			continue
		}
		if m.Kind == "user" {
			return m.Seq
		}
		last = m.Seq
	}
	return last
}

func (s *Service) startGroupMemberTurn(ctx context.Context, g store.CatGroup, m store.CatGroupMember, chainID string, hop int, afterSeq int64, reassign bool) {
	a, err := s.adapter(m.AgentKind)
	if err != nil {
		s.postFail(ctx, g.ID, chainID, hop, m, "助手未就绪")
		s.drainGroupQueue(g, m.ID)
		return
	}
	if a.Status().State != catagent.StateReady {
		s.postFail(ctx, g.ID, chainID, hop, m, "助手未就绪")
		s.drainGroupQueue(g, m.ID)
		return
	}
	projectRoot := ""
	if row, err := s.cfg.Store.GetCatProject(ctx, g.ProjectID); err == nil {
		projectRoot = row.Path
	}
	members, _ := s.cfg.Store.ListCatGroupMembers(ctx, g.ID)
	prompt := s.buildMemberPrompt(ctx, g, m, members, afterSeq)
	rules := ""
	if s.cfg.Store != nil {
		// workspace rules + group rules
		rules = strings.TrimSpace(s.workspaceRulesForTurn(ctx, g.ProjectID) + "\n\n" + groupRulesText(m, members))
	} else {
		rules = groupRulesText(m, members)
	}
	convID := groupConvID(g.ID, m.ID)
	turnCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	nowMs := time.Now().UnixMilli()
	t := &turnState{
		convID: convID, turnID: id.New(), cancel: cancel, msgID: id.New(), done: make(chan struct{}),
		usageCwd: projectRoot, agentKind: m.AgentKind, startedAt: nowMs,
		leaseKey: groupExecutionScope(projectRoot, g.ID, m.ID),
	}
	// track claimed tool write paths + tool ids for toolCount
	var (
		claimedMu  sync.Mutex
		claimed    []string
		toolSeen   = map[string]bool{}
		toolCount  int
		snapBefore string
	)
	if projectRoot != "" {
		snapBefore = createGitSnapshot(turnCtx, projectRoot)
	}
	s.setTurn(t)
	s.lastMu.Lock()
	s.lastTurns[convID] = &lastTurn{turnID: t.turnID, status: catagent.TurnRunning}
	s.lastMu.Unlock()
	s.emit(catagent.EventTurn, catagent.TurnEvent{ConvID: convID, TurnID: t.turnID, Status: catagent.TurnRunning})
	s.emitGroupMember(m, &GroupRunning{TurnID: t.turnID, StartedAt: nowMs, ToolCount: 0, ElapsedMs: 0})

	provider := s.providerOverlayFor(ctx, m.AgentKind)
	s.applyProviderOverlay(ctx, a, m.AgentKind)
	opts := catagent.TurnOptions{
		Ctx: turnCtx, ConversationID: m.ID, TurnID: t.turnID,
		ModelID: m.ModelID, ThinkLevelID: s.effortForTurn(ctx, m.AgentKind, m.ModelID, m.ThinkLevel),
		PermissionMode: catagent.NormalizeMode(m.AgentKind, ""),
		ProjectPath:    projectRoot,
		Resume:         s.sessionResumable(m.AgentKind, m.ID, projectRoot),
		Messages:       []catagent.WireMessage{{Role: "user", Content: prompt}},
		Rules:          rules,
		Provider:       provider,
		Skills:         skillsOverlayForProject(m.AgentKind, projectRoot),
	}
	opts.OnActivity = func(kind, idv, text string, replace bool) {
		if kind == "tool" {
			claimedMu.Lock()
			if !toolSeen[idv] {
				toolSeen[idv] = true
				toolCount++
			}
			for _, p := range extractWritePathsFromTool(text, projectRoot) {
				claimed = append(claimed, p)
			}
			tc := toolCount
			claimedMu.Unlock()
			s.appendActivity(turnCtx, t, kind, idv, text, replace)
			s.emitGroupMember(m, &GroupRunning{
				TurnID: t.turnID, StartedAt: nowMs, ToolCount: tc,
				ElapsedMs: time.Now().UnixMilli() - nowMs,
			})
			return
		}
		s.appendActivity(turnCtx, t, kind, idv, text, replace)
	}
	opts.OnTextDelta = func(delta string) {
		s.appendText(turnCtx, t, delta)
	}
	opts.OnError = func(msg string) {
		s.noteTurnError(turnCtx, t, msg)
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		// 先 clearTurn 再取下一条：否则下一轮 setTurn 会把还没清掉的这一轮当旧轮去停。
		defer s.drainGroupQueue(g, m.ID)
		defer close(t.done)
		defer s.clearTurn(t)

		stopWatch := s.watchTurnWorkspace(turnCtx, t, projectRoot, &opts)
		s.syncSharingBeforeTurn(turnCtx, projectRoot)
		s.runTurn(turnCtx, t, a, opts)
		stopWatch()

		claimedMu.Lock()
		claimedCopy := append([]string{}, claimed...)
		tc := toolCount
		claimedMu.Unlock()

		status := ""
		s.lastMu.Lock()
		if lt := s.lastTurns[convID]; lt != nil && lt.turnID == t.turnID {
			status = lt.status
		}
		s.lastMu.Unlock()
		// finish already set lastTurns via noteLastTurnStatus in finishLocked — read from turn
		t.mu.Lock()
		finished := t.finished
		failMsg := t.failMsg
		text := strings.TrimSpace(t.text.String())
		footer := store.CatMessageFooter{
			FinishedAt: t.finishedAt, TokenTotal: int64Ptr(t.tokenTotal),
			OutputTokens: int64Ptr(t.outputTokens), TokensPerSec: t.tokensPerSec,
			FirstTokenMs: msPtr(t.startedAt, t.firstTokenAt), TotalMs: msPtr(t.startedAt, t.finishedAt),
		}
		t.mu.Unlock()
		_ = finished
		_ = tc

		snapAfter := ""
		if projectRoot != "" {
			snapAfter = createGitSnapshot(context.Background(), projectRoot)
		}
		_ = s.cfg.Store.SaveCatGroupTurnMeta(context.Background(), store.CatGroupTurnMeta{
			TurnID: t.turnID, GroupID: g.ID, MemberID: m.ID, ChainID: chainID, Hop: hop,
			SnapshotBefore: snapBefore, SnapshotAfter: snapAfter, ClaimedFiles: claimedCopy,
		})

		// clear running
		s.emitGroupMember(m, nil)

		if status == "" {
			// infer from turn
			s.lastMu.Lock()
			if lt := s.lastTurns[convID]; lt != nil {
				status = lt.status
			}
			s.lastMu.Unlock()
		}
		stoppedByReassign := takeReassignStop(convID)
		if status == catagent.TurnCancelled {
			note := m.Name + " 已停止"
			if stoppedByReassign {
				note = m.Name + " 已停止（换人时停下的）"
			}
			s.postSystem(context.Background(), g.ID, chainID, hop, "system", note, "stopped",
				store.CatRouteInfo{Mode: "stopped", MemberID: m.ID}, "", "")
			return
		}
		if status == catagent.TurnFailed || text == "" {
			reason := failMsg
			if reason == "" {
				reason = catagent.MsgReplyFailed
			}
			msg, _ := s.cfg.Store.InsertCatGroupMessage(context.Background(), store.CatGroupMessage{
				GroupID: g.ID, Kind: "system", MemberID: m.ID, Text: m.Name + " 没完成：" + reason,
				ChainID: chainID, Hop: hop, TurnID: t.turnID, SystemKind: "failed", FailReason: reason,
				Route: &store.CatRouteInfo{Mode: "failed", MemberID: m.ID, Reason: reason},
			})
			s.emitGroupMessage(msg)
			return
		}

		report := parseGroupReport(text)
		report.Footer = &footer
		// files from turn changes + overreach tags
		changes, _ := s.GetCatTurnChangeDetail(context.Background(), m.ID, t.turnID)
		report.Files = s.tagReportFiles(context.Background(), g, m, claimedCopy, changes, members)
		if projectRoot != "" {
			// 撤销越界时按轮末内容判断「之后又被改过」（6.21.14 ⑥）。
			s.persistEndHashes(context.Background(), t.turnID, projectRoot, report.Files)
		}
		mentions := matchMentionsInText(text, members, m.ID)

		outMsg, err := s.cfg.Store.InsertCatGroupMessage(context.Background(), store.CatGroupMessage{
			GroupID: g.ID, Kind: "member", MemberID: m.ID, Text: text, Mentions: mentions,
			ChainID: chainID, Hop: hop + 1, TurnID: t.turnID, Report: &report,
		})
		if err == nil {
			s.emitGroupMessage(outMsg)
			if !report.Parsed {
				// 6.21.15 / 产品确认：解析失败时整段当 did，并发灰字提示
				s.postSystem(context.Background(), g.ID, chainID, hop+1, "system",
					i18n.T(i18n.KeyCatGroupReportUnparsed, nil), "report_format",
					store.CatRouteInfo{Mode: "done", MemberID: m.ID, Reason: "unparsed"}, "", "")
			}
		}
		_ = s.cfg.Store.UpdateCatGroupMemberLastSeen(context.Background(), m.ID, outMsg.Seq)

		if len(mentions) == 0 {
			s.postSystem(context.Background(), g.ID, chainID, hop+1, "system",
				m.Name+" 已完成，没有交给下一位", "no_handoff",
				store.CatRouteInfo{Mode: "done", MemberID: m.ID}, "", "")
			return
		}
		// 换人 / 重试叫起的这一轮已经沿用原来的 hop（不加），它自己再 @ 下一位照常 +1。
		nextHop := hop + 1
		// Contract: one reply @ two people → hop += 1 only (already nextHop = hop+1 once)
		if nextHop >= maxGroupHops {
			s.postSystem(context.Background(), g.ID, chainID, nextHop, "system",
				"已连续转交 6 次，先停在这里。需要继续请 @ 对应成员。", "hop_limit",
				store.CatRouteInfo{Mode: "hop_limit"}, "", "")
			return
		}
		for _, mid := range mentions {
			name := mid
			for _, om := range members {
				if om.ID == mid {
					name = om.Name
					break
				}
			}
			// 6.21.16：已转交给 {成员}（第 n/6 次）；n = 当前 hop
			s.postSystem(context.Background(), g.ID, chainID, nextHop, "system",
				"已转交给 "+name+"（第 "+strconv.Itoa(nextHop)+"/6 次）", "route",
				store.CatRouteInfo{Mode: "mention", MemberID: mid, Reason: "接力"}, "", mid)
			s.wakeMember(context.Background(), g, mid, chainID, nextHop, outMsg.Seq, false, "mention")
		}
	}()
}

// drainGroupQueue：一轮结束（或没起来）后取下一条开跑；队列空则释放。轮到开跑不发灰字（6.21.17 ④ 5），
// 只由 startGroupMemberTurn 推 running 非空的 cat:group-member（快照里队列已减一）。
func (s *Service) drainGroupQueue(g store.CatGroup, memberID string) {
	for {
		groupQueueMu.Lock()
		st := queueStateLocked(memberID)
		if len(st.items) == 0 {
			delete(groupQueues, memberID)
			groupQueueMu.Unlock()
			return
		}
		item := st.items[0]
		st.items = st.items[1:]
		groupQueueMu.Unlock()
		m, err := s.cfg.Store.GetCatGroupMember(context.Background(), memberID)
		if err != nil {
			continue // 成员已删：丢掉剩下的任务
		}
		s.startGroupMemberTurn(context.Background(), g, m, item.chainID, item.hop, item.afterSeq, item.reassign)
		return
	}
}

func (s *Service) buildMemberPrompt(ctx context.Context, g store.CatGroup, m store.CatGroupMember, members []store.CatGroupMember, afterSeq int64) string {
	var b strings.Builder
	b.WriteString("以下是群里你还没看过的新消息，请按职责处理并按汇报格式回复。\n\n")
	limit := 100
	firstTime := m.LastSeenSeq == 0
	if firstTime {
		limit = 20
	}
	msgs, _ := s.cfg.Store.ListCatGroupMessagesAfter(ctx, g.ID, m.LastSeenSeq, limit)
	if afterSeq > 0 && m.LastSeenSeq == 0 {
		// include up to afterSeq
	}
	total := 0
	for _, msg := range msgs {
		line := formatGroupContextLine(msg, members)
		if firstTime {
			total += utf8.RuneCountInString(line)
			if total > 8000 {
				break
			}
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func formatGroupContextLine(msg store.CatGroupMessage, members []store.CatGroupMember) string {
	switch msg.Kind {
	case "user":
		return "【用户】" + msg.Text
	case "member":
		name, title := msg.MemberID, ""
		for _, m := range members {
			if m.ID == msg.MemberID {
				name, title = m.Name, m.Title
				break
			}
		}
		return "【" + name + "（" + title + "）】" + msg.Text
	default:
		return "【系统】" + msg.Text
	}
}

func (s *Service) postFail(ctx context.Context, groupID, chainID string, hop int, m store.CatGroupMember, reason string) {
	msg, _ := s.cfg.Store.InsertCatGroupMessage(ctx, store.CatGroupMessage{
		GroupID: groupID, Kind: "system", MemberID: m.ID, Text: m.Name + " 没完成：" + reason,
		ChainID: chainID, Hop: hop, SystemKind: "failed", FailReason: reason,
	})
	s.emitGroupMessage(msg)
}

func (s *Service) StopCatGroup(ctx context.Context, groupID string) error {
	g, err := s.requireGroup(ctx, groupID)
	if err != nil {
		return err
	}
	members, _ := s.cfg.Store.ListCatGroupMembers(ctx, g.ID)
	for _, m := range members {
		_ = s.StopCatGroupMember(ctx, m.ID)
	}
	return nil
}

func (s *Service) StopCatGroupMember(ctx context.Context, memberID string) error {
	m, err := s.requireMember(ctx, memberID)
	if err != nil {
		return err
	}
	groupQueueMu.Lock()
	cleared := false
	if st := groupQueues[m.ID]; st != nil && len(st.items) > 0 {
		st.items = nil
		cleared = true
	}
	groupQueueMu.Unlock()
	if cleared {
		s.emitGroupMemberSnapshot(m) // 清队也推一次（6.21.17 ④ 2）
	}
	convID := groupConvID(m.GroupID, m.ID)
	s.cancelTurn(convID, "")
	// Also stop by member.ID conversation key used as ConversationID for CLI
	s.cancelTurn(m.ID, "")
	catagent.StopLiveConv(m.ID)
	return nil
}

var (
	// 只认写入类工具；读文件 / 搜索 / 跑命令看过的文件不算「自己改的」，否则别人改的文件也能被撤销。
	reWriteTool = regexp.MustCompile(`(?i)(?:^|[^a-z])(?:write|writes|writing|edit|edits|editing|multiedit|notebookedit|create|apply_patch|patch|str_replace|search_replace|update|delete|move|rename)(?:[^a-z]|$)`)
	// Claude / MCP 的参数是 JSON（标签被截到 200 字时 JSON 可能不完整，所以用正则取）。
	reToolPathArg = regexp.MustCompile(`"(?:file_path|filePath|path|notebook_path)"\s*:\s*"((?:[^"\\]|\\.)+)"`)
)

// extractWritePathsFromTool 从工具标签里取出写过的文件，转成相对项目根的路径。
// 标签格式见 catagent：grok「标题\n路径…」、codex「apply_patch\nadd 路径」、claude「Write {json}」。
func extractWritePathsFromTool(label, root string) []string {
	header, rest, _ := strings.Cut(label, "\n")
	if !reWriteTool.MatchString(header) {
		return nil
	}
	var out []string
	add := func(p string) {
		p = strings.Trim(strings.TrimSpace(p), `"'`)
		if p == "" {
			return
		}
		if root != "" && filepath.IsAbs(p) {
			if rel, err := filepath.Rel(root, p); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				p = filepath.ToSlash(rel)
			}
		}
		out = append(out, p)
	}
	if strings.Contains(header, "{") {
		for _, m := range reToolPathArg.FindAllStringSubmatch(header, -1) {
			if v, err := strconv.Unquote(`"` + m[1] + `"`); err == nil {
				add(v)
			} else {
				add(m[1])
			}
		}
		header = ""
	}
	lines := strings.Split(rest, "\n")
	if header != "" {
		lines = append([]string{header}, lines...)
	}
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			break // 空行之后是改动对比，不是路径
		}
		for _, pre := range []string{"add ", "Add ", "delete ", "update ", "modified "} {
			line = strings.TrimPrefix(line, pre)
		}
		fields := strings.Fields(line)
		if i == 0 && header != "" && len(fields) > 0 {
			fields = fields[1:] // 标题第一个词是工具名
		}
		for _, f := range fields {
			if strings.Contains(f, "/") || strings.Contains(f, ".") {
				add(f)
				break
			}
		}
	}
	return out
}

func groupBackingConvID(convID string) string {
	if strings.HasPrefix(convID, "group:") {
		parts := strings.Split(convID, ":")
		if len(parts) >= 3 {
			return parts[len(parts)-1]
		}
	}
	return convID
}

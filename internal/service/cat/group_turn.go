package cat

import (
	"context"
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
	chainID   string
	hop       int
	afterSeq  int64
	reassign  bool
}

// groupQueues: memberID → pending wakes
func (s *Service) ensureGroupQueues() {
	// lazily stored on Service via sync.Map pattern using existing fields — use a dedicated map.
}

var (
	groupQueueMu sync.Mutex
	groupQueues  = map[string][]groupQueueItem{}
)

func (s *Service) wakeMember(ctx context.Context, g store.CatGroup, memberID, chainID string, hop int, afterSeq int64, reassign bool) {
	m, err := s.cfg.Store.GetCatGroupMember(ctx, memberID)
	if err != nil {
		return
	}
	convID := groupConvID(g.ID, m.ID)
	s.mu.Lock()
	_, busy := s.turns[convID]
	s.mu.Unlock()
	if busy {
		groupQueueMu.Lock()
		groupQueues[m.ID] = append(groupQueues[m.ID], groupQueueItem{chainID, hop, afterSeq, reassign})
		groupQueueMu.Unlock()
		return
	}
	s.startGroupMemberTurn(ctx, g, m, chainID, hop, afterSeq, reassign)
}

func (s *Service) startGroupMemberTurn(ctx context.Context, g store.CatGroup, m store.CatGroupMember, chainID string, hop int, afterSeq int64, reassign bool) {
	a, err := s.adapter(m.AgentKind)
	if err != nil {
		s.postFail(ctx, g.ID, chainID, hop, m, "助手未就绪")
		return
	}
	if a.Status().State != catagent.StateReady {
		s.postFail(ctx, g.ID, chainID, hop, m, "助手未就绪")
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
	}
	// track claimed tool write paths + tool ids for toolCount
	var (
		claimedMu   sync.Mutex
		claimed     []string
		toolSeen    = map[string]bool{}
		toolCount   int
		snapBefore  string
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
			for _, p := range extractWritePathsFromTool(text) {
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
		defer close(t.done)
		defer s.clearTurn(t)
		defer s.drainGroupQueue(g, m.ID)

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
		if status == catagent.TurnCancelled {
			note := m.Name + " 已停止"
			if reassign {
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
		nextHop := hop + 1
		if reassign {
			nextHop = hop // reassign / retry: hop does not increase for the woken turn; but this is after completion
		}
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
			s.wakeMember(context.Background(), g, mid, chainID, nextHop, outMsg.Seq, false)
		}
	}()
}

func (s *Service) drainGroupQueue(g store.CatGroup, memberID string) {
	groupQueueMu.Lock()
	q := groupQueues[memberID]
	if len(q) == 0 {
		groupQueueMu.Unlock()
		return
	}
	item := q[0]
	groupQueues[memberID] = q[1:]
	groupQueueMu.Unlock()
	m, err := s.cfg.Store.GetCatGroupMember(context.Background(), memberID)
	if err != nil {
		return
	}
	s.startGroupMemberTurn(context.Background(), g, m, item.chainID, item.hop, item.afterSeq, item.reassign)
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
	delete(groupQueues, m.ID)
	groupQueueMu.Unlock()
	convID := groupConvID(m.GroupID, m.ID)
	s.cancelTurn(convID, "")
	// Also stop by member.ID conversation key used as ConversationID for CLI
	s.cancelTurn(m.ID, "")
	catagent.StopLiveConv(m.ID)
	return nil
}

func extractWritePathsFromTool(label string) []string {
	// Heuristic from tool labels: lines with path-like tokens after apply_patch / Write / Edit
	var out []string
	lower := strings.ToLower(label)
	if !(strings.Contains(lower, "apply_patch") || strings.Contains(lower, "write") ||
		strings.Contains(lower, "edit") || strings.Contains(lower, "create") ||
		strings.Contains(label, "\n")) {
		// still try path-looking tokens
	}
	for _, line := range strings.Split(label, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "add ")
		line = strings.TrimPrefix(line, "Add ")
		line = strings.TrimPrefix(line, "delete ")
		line = strings.TrimPrefix(line, "update ")
		line = strings.TrimPrefix(line, "modified ")
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, " ") && !strings.Contains(line, "/") {
			continue
		}
		// take first path-like token
		fields := strings.Fields(line)
		for _, f := range fields {
			if strings.Contains(f, "/") || strings.Contains(f, ".") {
				f = strings.Trim(f, `"'`)
				if f != "" {
					out = append(out, f)
				}
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

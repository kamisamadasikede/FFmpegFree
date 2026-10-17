package cat

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/id"
	"FFmpegFree/internal/store"
)

const maxGroupHops = 6

// aiRouteTimeout：派活判断硬超时。Grok 结构化偶发 >20s；产品/架构锁定 45s。
const aiRouteTimeout = 45 * time.Second

// undecidedRouteBase：产品/设计锁定的未定人灰字主句，后缀另拼，勿改主句。
const undecidedRouteBase = "没判断出该谁接，请 @ 对应成员，或再说具体要做什么。"

func (s *Service) SendCatGroupMessage(ctx context.Context, req SendGroupMessageRequest) (SendGroupMessageResult, error) {
	g, err := s.requireGroup(ctx, req.GroupID)
	if err != nil {
		return SendGroupMessageResult{}, err
	}
	text := strings.TrimSpace(req.Text)
	if text == "" && len(req.Files) == 0 {
		return SendGroupMessageResult{}, apperr.New(apperr.InvalidArgument, "消息不能为空")
	}
	if utf8.RuneCountInString(text) > 100_000 {
		return SendGroupMessageResult{}, apperr.New(apperr.InvalidArgument, "内容太长")
	}
	members, err := s.cfg.Store.ListCatGroupMembers(ctx, g.ID)
	if err != nil {
		return SendGroupMessageResult{}, apperr.Wrap(apperr.IOError, "列出群成员失败", err)
	}
	if len(members) == 0 {
		return SendGroupMessageResult{}, apperr.New(apperr.InvalidArgument, "群里还没有成员")
	}
	mentions := uniqueIDs(req.Mentions)
	for _, mid := range mentions {
		found := false
		for _, m := range members {
			if m.ID == mid {
				found = true
				break
			}
		}
		if !found {
			return SendGroupMessageResult{}, s.errMemberNotFound()
		}
	}
	chainID := id.New()
	userMsg, err := s.cfg.Store.InsertCatGroupMessage(ctx, store.CatGroupMessage{
		GroupID: g.ID, Kind: "user", Text: text, Mentions: mentions, ChainID: chainID, Hop: 0,
	})
	if err != nil {
		return SendGroupMessageResult{}, apperr.Wrap(apperr.IOError, "写入群消息失败", err)
	}
	s.emitGroupMessage(userMsg)

	targets := mentions
	var route store.CatRouteInfo
	if len(targets) > 0 {
		route = store.CatRouteInfo{Mode: "mention", MemberID: strings.Join(targets, ","), Reason: "用户指定"}
	} else if !g.AutoRoute {
		first := members[0]
		targets = []string{first.ID}
		route = store.CatRouteInfo{Mode: "off", MemberID: first.ID, Reason: "没开自动派活，交给排第一的成员"}
		s.postRouteSystem(ctx, g.ID, chainID, first, route, "交给 "+first.Name+"：没开自动派活，交给排第一的成员")
	} else {
		targets, route = s.decideRoute(ctx, g, members, userMsg, chainID)
	}
	for _, mid := range targets {
		s.wakeMember(ctx, g, mid, chainID, 0, userMsg.Seq, false, route.Mode)
	}
	return SendGroupMessageResult{UserMessage: userMsg, ChainID: chainID}, nil
}

func (s *Service) decideRoute(ctx context.Context, g store.CatGroup, members []store.CatGroupMember, userMsg store.CatGroupMessage, chainID string) ([]string, store.CatRouteInfo) {
	// followup: last member reply on this chain within 10 min
	if last, ok := s.lastMemberOnChain(ctx, g.ID, chainID); ok {
		if time.Now().UnixMilli()-last.CreatedAt <= 10*60*1000 && isFollowupText(userMsg.Text) {
			route := store.CatRouteInfo{Mode: "followup", MemberID: last.MemberID, Reason: "接着上一条"}
			var name string
			for _, m := range members {
				if m.ID == last.MemberID {
					name = m.Name
					break
				}
			}
			s.postRouteSystem(ctx, g.ID, chainID, store.CatGroupMember{ID: last.MemberID, Name: name}, route, "交给 "+name+"：接着上一条")
			return []string{last.MemberID}, route
		}
	}
	// also check recent member messages in group (any chain) for followup feel — contract says 本链
	// 6.21.17 ③：started_at = 发「正在安排谁来接…」时。
	startedAt := time.Now().UnixMilli()
	routingMsg := s.postSystem(ctx, g.ID, chainID, 0, "system", "正在安排谁来接…", "route", store.CatRouteInfo{Mode: "ai"}, "", "")
	_ = routingMsg
	s.emitGroupRoute(GroupRouteEvent{GroupID: g.ID, ChainID: chainID, State: "routing", Route: store.CatRouteInfo{Mode: "ai"}})

	memberID, reason, detail := s.runAIRoute(ctx, g, members, userMsg, startedAt)
	if memberID == "" {
		// v0.55.9 / 6.21.18：自动派活开着但判断不可用 → 不定人、不叫醒任何人（不再交给排第一）。
		// detail.Reason 已是机器码（timeout/empty/parse_error/…）；灰字主句不动，后缀 ·判断超时 等给 FE 读。
		failCode := detail.Reason
		grey := undecidedGreyText(failCode)
		route := store.CatRouteInfo{Mode: "undecided", Reason: failCode, RouteSeq: detail.RouteSeq, DurationMs: routeDurationMs(detail)}
		s.postSystem(ctx, g.ID, chainID, 0, "system", grey, "route", route, "", "")
		s.emitGroupRoute(GroupRouteEvent{GroupID: g.ID, ChainID: chainID, State: "routed", Route: route})
		return nil, route
	}
	var name string
	for _, m := range members {
		if m.ID == memberID {
			name = m.Name
			break
		}
	}
	route := store.CatRouteInfo{Mode: "ai", MemberID: memberID, Reason: reason, RouteSeq: detail.RouteSeq, DurationMs: routeDurationMs(detail)}
	s.postRouteSystem(ctx, g.ID, chainID, store.CatGroupMember{ID: memberID, Name: name}, route, "交给 "+name+"："+reason)
	s.emitGroupRoute(GroupRouteEvent{GroupID: g.ID, ChainID: chainID, State: "routed", Route: route})
	return []string{memberID}, route
}

func (s *Service) lastMemberOnChain(ctx context.Context, groupID, chainID string) (store.CatGroupMessage, bool) {
	list, err := s.cfg.Store.ListCatGroupMessages(ctx, groupID, 0, 50)
	if err != nil {
		return store.CatGroupMessage{}, false
	}
	for i := len(list) - 1; i >= 0; i-- {
		m := list[i]
		if m.ChainID == chainID && m.Kind == "member" {
			return m, true
		}
	}
	return store.CatGroupMessage{}, false
}

// routeDurationMs = finishedAt - startedAt；任一为 0 → 0（json omitempty 省略，前端不显示耗时）。
func routeDurationMs(d store.CatGroupRouteDetail) int64 {
	if d.StartedAt == 0 || d.FinishedAt == 0 || d.FinishedAt < d.StartedAt {
		return 0
	}
	return d.FinishedAt - d.StartedAt
}

func (s *Service) runAIRoute(ctx context.Context, g store.CatGroup, members []store.CatGroupMember, userMsg store.CatGroupMessage, startedAt int64) (memberID, reason string, detail store.CatGroupRouteDetail) {
	first := members[0]
	recent, _ := s.cfg.Store.ListCatGroupMessages(ctx, g.ID, 0, 10)
	var b strings.Builder
	b.WriteString("根据用户消息和群成员职责，选出最合适的一个人。只输出 JSON。\n")
	b.WriteString("硬性要求：memberId 必须是下面「成员」列表里 id= 后面的原样字符串（UUID），禁止用人名、职位名或其它别名。\n")
	b.WriteString("输出格式：{\"memberId\":\"<id= 的值>\",\"reason\":\"≤20字理由\"}\n")
	b.WriteString("用户消息：" + userMsg.Text + "\n")
	b.WriteString("最近群消息：\n")
	for _, m := range recent {
		b.WriteString("- [" + m.Kind + "] " + m.Text + "\n")
	}
	b.WriteString("成员：\n")
	for _, m := range members {
		b.WriteString("- id=" + m.ID + " name=" + m.Name + " title=" + m.Title + " owns=" + m.Charter.Owns + "\n")
	}
	input := b.String()
	if utf8.RuneCountInString(input) > 8000 {
		r := []rune(input)
		input = string(r[:8000])
	}

	outText := ""
	failReason := ""
	resolvedID := ""
	resolvedReason := ""
	finishedAt := time.Now().UnixMilli()

	a, err := s.adapter(first.AgentKind)
	if err != nil {
		failReason = "run_error"
	} else {
		turnCtx, cancel := context.WithTimeout(ctx, aiRouteTimeout)
		defer cancel()
		s.applyProviderOverlay(ctx, a, first.AgentKind)
		opts := catagent.TurnOptions{
			Ctx: turnCtx, ConversationID: "group-route-" + g.ID + "-" + id.New(),
			ModelID: first.ModelID, ThinkLevelID: s.effortForTurn(ctx, first.AgentKind, first.ModelID, first.ThinkLevel),
			Ephemeral: true, NoTools: true, JSONSchema: catagent.RouteSchema,
			Messages: []catagent.WireMessage{{Role: "user", Content: input}},
			Provider: s.providerOverlayFor(ctx, first.AgentKind),
			Timeout:  aiRouteTimeout,
		}
		if row, err := s.cfg.Store.GetCatProject(ctx, g.ProjectID); err == nil {
			opts.ProjectPath = row.Path
		}
		resp, runErr := a.RunTurn(opts)
		finishedAt = time.Now().UnixMilli() // 拿到结果 / 超时 / 出错
		outText = strings.TrimSpace(resp.Message.Content)
		if runErr != nil {
			if isAIRouteTimeout(runErr) {
				failReason = "timeout"
				// 超时但已有部分正文时先试解析，能认出人就当成功。
				if outText != "" {
					if idv, why, ok := parseAIRouteOutput(members, outText); ok {
						resolvedID, resolvedReason, failReason = idv, why, ""
					}
				}
			} else {
				failReason = "run_error"
			}
		} else if outText == "" {
			failReason = "empty"
		} else if idv, why, ok := parseAIRouteOutput(members, outText); ok {
			resolvedID, resolvedReason = idv, why
		} else {
			failReason = routeParseFailReason(members, outText)
			if failReason == "" {
				failReason = "parse_error"
			}
		}
	}

	mode := "ai"
	storeReason := resolvedReason
	if resolvedID == "" {
		mode = "undecided"
		storeReason = failReason
	}
	detail, _ = s.cfg.Store.InsertCatGroupRoute(ctx, store.CatGroupRouteDetail{
		GroupID: g.ID, ChainID: userMsg.ChainID, Mode: mode, MemberID: resolvedID, Reason: storeReason,
		InputText: input, OutputText: outText, StartedAt: startedAt, FinishedAt: finishedAt,
	})
	return resolvedID, resolvedReason, detail
}


// isAIRouteTimeout：context 截止或 CLI 带回 reason=timeout / reason=idle_timeout。
func isAIRouteTimeout(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ae *apperr.AppError
	if errors.As(err, &ae) {
		d := ae.Detail + "\n" + ae.Message
		if strings.Contains(d, "reason=timeout") || strings.Contains(d, "reason=idle_timeout") {
			return true
		}
	}
	msg := err.Error()
	return strings.Contains(msg, "reason=timeout") ||
		strings.Contains(msg, "reason=idle_timeout") ||
		strings.Contains(msg, "context deadline exceeded") ||
		strings.Contains(msg, "DeadlineExceeded")
}

// undecidedReasonSuffix：机器码 → 灰字 · 后中文；空码不拼。
func undecidedReasonSuffix(code string) string {
	switch strings.TrimSpace(code) {
	case "timeout":
		return "判断超时"
	case "empty":
		return "输出为空"
	case "parse_error":
		return "解析失败"
	case "run_error":
		return "判断执行失败"
	case "unknown_id":
		return "判断结果对不上成员"
	default:
		return ""
	}
}

// undecidedGreyText：主句锁定；有机器码时追加「·判断超时」等，不改主句。
func undecidedGreyText(code string) string {
	suf := undecidedReasonSuffix(code)
	if suf == "" {
		return undecidedRouteBase
	}
	return undecidedRouteBase + "·" + suf
}

func parseAIRouteOutput(members []store.CatGroupMember, outText string) (memberID, reason string, ok bool) {
	var parsed struct {
		MemberID string `json:"memberId"`
		Reason   string `json:"reason"`
	}
	if json.Unmarshal([]byte(extractJSONObject(outText)), &parsed) != nil {
		return "", "", false
	}
	parsed.MemberID = strings.TrimSpace(parsed.MemberID)
	parsed.Reason = strings.TrimSpace(parsed.Reason)
	if utf8.RuneCountInString(parsed.Reason) > 20 {
		r := []rune(parsed.Reason)
		parsed.Reason = string(r[:20])
	}
	if idv, found := resolveRouteMemberID(members, parsed.MemberID); found {
		return idv, parsed.Reason, true
	}
	return "", "", false
}

// routeParseFailReason：解析失败时的机器码（empty / parse_error / unknown_id）。
func routeParseFailReason(members []store.CatGroupMember, outText string) string {
	var parsed struct {
		MemberID string `json:"memberId"`
		Reason   string `json:"reason"`
	}
	if json.Unmarshal([]byte(extractJSONObject(outText)), &parsed) != nil {
		return "parse_error"
	}
	parsed.MemberID = strings.TrimSpace(parsed.MemberID)
	if parsed.MemberID == "" {
		return "empty"
	}
	if _, ok := resolveRouteMemberID(members, parsed.MemberID); !ok {
		return "unknown_id"
	}
	return ""
}

// resolveRouteMemberID：先按 id= 精确匹配；否则用人名、再用职位名；同名或同职位不唯一则拒绝。
func resolveRouteMemberID(members []store.CatGroupMember, raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	for _, m := range members {
		if m.ID == raw {
			return m.ID, true
		}
	}
	var byName []string
	for _, m := range members {
		if m.Name == raw {
			byName = append(byName, m.ID)
		}
	}
	if len(byName) == 1 {
		return byName[0], true
	}
	if len(byName) > 1 {
		return "", false
	}
	var byTitle []string
	for _, m := range members {
		if m.Title == raw {
			byTitle = append(byTitle, m.ID)
		}
	}
	if len(byTitle) == 1 {
		return byTitle[0], true
	}
	return "", false
}

func (s *Service) postRouteSystem(ctx context.Context, groupID, chainID string, member store.CatGroupMember, route store.CatRouteInfo, text string) {
	s.postSystem(ctx, groupID, chainID, 0, "system", text, "route", route, "", "")
}

func (s *Service) postSystem(ctx context.Context, groupID, chainID string, hop int, kind, text, systemKind string, route store.CatRouteInfo, fromID, toID string) store.CatGroupMessage {
	msg, err := s.cfg.Store.InsertCatGroupMessage(ctx, store.CatGroupMessage{
		GroupID: groupID, Kind: kind, Text: text, ChainID: chainID, Hop: hop,
		SystemKind: systemKind, Route: &route, FromMemberID: fromID, ToMemberID: toID,
		MemberID: route.MemberID,
	})
	if err != nil {
		return store.CatGroupMessage{}
	}
	s.emitGroupMessage(msg)
	return msg
}

func (s *Service) ReassignCatGroupRoute(ctx context.Context, req ReassignGroupRouteRequest) (ReassignGroupRouteResult, error) {
	g, err := s.requireGroup(ctx, req.GroupID)
	if err != nil {
		return ReassignGroupRouteResult{}, err
	}
	to, err := s.requireMember(ctx, req.ToMemberID)
	if err != nil {
		return ReassignGroupRouteResult{}, err
	}
	if to.GroupID != g.ID {
		return ReassignGroupRouteResult{}, s.errMemberNotFound()
	}
	// find original running or last routed member on chain
	fromID, fromName := s.findChainActiveMember(ctx, g.ID, req.ChainID)
	// 换人不计入 hop：新成员接着原成员所在的那一跳，不归零（否则能绕过 6 次上限）。
	hop := 0
	if fromID != "" {
		hop = s.chainHopFor(ctx, g.ID, req.ChainID, fromID)
		markReassignStop(groupConvID(g.ID, fromID))
		_ = s.StopCatGroupMember(ctx, fromID)
	}
	sys := s.postSystem(ctx, g.ID, req.ChainID, 0, "system",
		"已换人："+fromName+" → "+to.Name, "reassign",
		store.CatRouteInfo{Mode: "reassign", MemberID: to.ID, Reason: "换人"},
		fromID, to.ID)
	// 6.21.16：交给 {新成员}：你指定的（换人时，不是用户 @ 发送时）
	s.postSystem(ctx, g.ID, req.ChainID, 0, "system",
		"交给 "+to.Name+"：你指定的", "route",
		store.CatRouteInfo{Mode: "reassign", MemberID: to.ID, Reason: "你指定的"},
		fromID, to.ID)
	// wake new member; hop not counted
	s.wakeMember(ctx, g, to.ID, req.ChainID, hop, 0, true, "reassign")
	return ReassignGroupRouteResult{System: sys}, nil
}

func (s *Service) findChainActiveMember(ctx context.Context, groupID, chainID string) (id, name string) {
	members, _ := s.cfg.Store.ListCatGroupMembers(ctx, groupID)
	for _, m := range members {
		conv := groupConvID(groupID, m.ID)
		s.mu.Lock()
		_, busy := s.turns[conv]
		s.mu.Unlock()
		if busy {
			return m.ID, m.Name
		}
	}
	if last, ok := s.lastMemberOnChain(ctx, groupID, chainID); ok {
		for _, m := range members {
			if m.ID == last.MemberID {
				return m.ID, m.Name
			}
		}
		return last.MemberID, last.MemberID
	}
	return "", ""
}

func (s *Service) RetryCatGroupMemberTurn(ctx context.Context, req RetryGroupMemberTurnRequest) error {
	g, err := s.requireGroup(ctx, req.GroupID)
	if err != nil {
		return err
	}
	m, err := s.requireMember(ctx, req.MemberID)
	if err != nil {
		return err
	}
	if m.GroupID != g.ID {
		return s.errMemberNotFound()
	}
	// 重试：同一任务在原来那一跳再跑一轮，hop 不加（也不归零）。
	s.wakeMember(ctx, g, m.ID, req.ChainID, s.chainHopFor(ctx, g.ID, req.ChainID, m.ID), 0, true, "retry")
	return nil
}

func uniqueIDs(ids []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, idv := range ids {
		idv = strings.TrimSpace(idv)
		if idv == "" || seen[idv] {
			continue
		}
		seen[idv] = true
		out = append(out, idv)
	}
	return out
}

func groupConvID(groupID, memberID string) string {
	return "group:" + groupID + ":" + memberID
}

// chainHopFor 返回成员在这条链上最近一次被叫起时所在的 hop：
// 转交给他的灰字（toMemberId）或他自己的失败 / 停止灰字；都没有（用户直接 @ / 派活）就是 0。
func (s *Service) chainHopFor(ctx context.Context, groupID, chainID, memberID string) int {
	list, err := s.cfg.Store.ListCatGroupMessages(ctx, groupID, 0, 200)
	if err != nil {
		return 0
	}
	for i := len(list) - 1; i >= 0; i-- {
		m := list[i]
		if m.ChainID != chainID || m.Kind != "system" {
			continue
		}
		if (m.SystemKind == "route" && m.ToMemberID == memberID && m.Route != nil && m.Route.Mode == "mention") ||
			((m.SystemKind == "failed" || m.SystemKind == "stopped") && m.MemberID == memberID) {
			return m.Hop
		}
	}
	return 0
}

var (
	reassignStopMu sync.Mutex
	reassignStops  = map[string]bool{}
)

func markReassignStop(convID string) {
	reassignStopMu.Lock()
	reassignStops[convID] = true
	reassignStopMu.Unlock()
}

func takeReassignStop(convID string) bool {
	reassignStopMu.Lock()
	defer reassignStopMu.Unlock()
	v := reassignStops[convID]
	delete(reassignStops, convID)
	return v
}

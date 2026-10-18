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
	// 6.21.17 ③ / v0.57：started_at = 发「正在安排谁来接…」时；成功/失败用协调气泡或 undecided **替换**该灰字（不叠放）。
	startedAt := time.Now().UnixMilli()
	routingMsg := s.postSystem(ctx, g.ID, chainID, 0, "system", "正在安排谁来接…", "route", store.CatRouteInfo{Mode: "ai"}, "", "")
	s.emitGroupRoute(GroupRouteEvent{GroupID: g.ID, ChainID: chainID, State: "routing", Route: store.CatRouteInfo{Mode: "ai"}})

	memberIDs, everyone, reason, detail := s.runAIRoute(ctx, g, members, userMsg, startedAt)
	if len(memberIDs) == 0 {
		// v0.55.9 / 6.21.18：不定人、不叫醒、不发协调气泡；加载灰字替换为 undecided。
		failCode := detail.Reason
		grey := undecidedGreyText(failCode)
		route := store.CatRouteInfo{Mode: "undecided", Reason: failCode, RouteSeq: detail.RouteSeq, DurationMs: routeDurationMs(detail)}
		s.replaceRouteSystem(ctx, routingMsg, grey, "route", route, nil)
		s.emitGroupRoute(GroupRouteEvent{GroupID: g.ID, ChainID: chainID, State: "routed", Route: route})
		return nil, route
	}
	route := store.CatRouteInfo{
		Mode: "ai", MemberID: memberIDs[0], MemberIDs: memberIDs, Everyone: everyone,
		Reason: reason, RouteSeq: detail.RouteSeq, DurationMs: routeDurationMs(detail),
	}
	coordText := formatCoordBubbleText(reason, everyone, members, memberIDs)
	s.replaceRouteSystem(ctx, routingMsg, coordText, "coord", route, memberIDs)
	s.emitGroupRoute(GroupRouteEvent{GroupID: g.ID, ChainID: chainID, State: "routed", Route: route})
	// 唤醒由 SendCatGroupMessage 走与用户 @ 同一套 wakeMember（hop=0，协调不计 hop）。
	return memberIDs, route
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

func (s *Service) runAIRoute(ctx context.Context, g store.CatGroup, members []store.CatGroupMember, userMsg store.CatGroupMessage, startedAt int64) (memberIDs []string, everyone bool, reason string, detail store.CatGroupRouteDetail) {
	first := members[0]
	recent, _ := s.cfg.Store.ListCatGroupMessages(ctx, g.ID, 0, 10)
	var b strings.Builder
	b.WriteString("根据用户消息和群成员职责，选出最合适的一个或多个成员。只输出 JSON。\n")
	b.WriteString("硬性要求：memberIds 里每一项必须是下面「成员」列表里 id= 后面的原样字符串（UUID）；也可用人名/职位（须唯一）。\n")
	b.WriteString("输出格式：{\"memberIds\":[\"<id>\",...],\"reason\":\"≤40字理由\",\"everyone\":false}\n")
	b.WriteString("若应通知全员（如「介绍下自己」「你们好」），设 everyone=true（可省略 memberIds）。招呼/介绍类优先多人或全员，不要压成单人。\n")
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
	var resolvedIDs []string
	resolvedEveryone := false
	resolvedReason := ""
	finishedAt := time.Now().UnixMilli()

	a, err := s.adapter(first.AgentKind)
	if err != nil {
		failReason = "run_error"
	} else {
		// 只用 TurnOptions.Timeout 控 45s：勿再 WithTimeout 包一层 Ctx，否则截止会被
		// BuildAdapter 判成「已取消」而非 reason=timeout（DispatchTimeout e2e）。
		s.applyProviderOverlay(ctx, a, first.AgentKind)
		opts := catagent.TurnOptions{
			Ctx: ctx, ConversationID: "group-route-" + g.ID + "-" + id.New(),
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
					if ids, all, why, ok := parseAIRouteOutput(members, outText); ok {
						resolvedIDs, resolvedEveryone, resolvedReason, failReason = ids, all, why, ""
					}
				}
			} else {
				failReason = "run_error"
			}
		} else if outText == "" {
			failReason = "empty"
		} else if ids, all, why, ok := parseAIRouteOutput(members, outText); ok {
			resolvedIDs, resolvedEveryone, resolvedReason = ids, all, why
		} else {
			failReason = routeParseFailReason(members, outText)
			if failReason == "" {
				failReason = "parse_error"
			}
		}
	}

	mode := "ai"
	storeReason := resolvedReason
	storeMemberID := ""
	if len(resolvedIDs) == 0 {
		mode = "undecided"
		storeReason = failReason
	} else {
		storeMemberID = resolvedIDs[0]
	}
	detail, _ = s.cfg.Store.InsertCatGroupRoute(ctx, store.CatGroupRouteDetail{
		GroupID: g.ID, ChainID: userMsg.ChainID, Mode: mode, MemberID: storeMemberID, Reason: storeReason,
		InputText: input, OutputText: outText, StartedAt: startedAt, FinishedAt: finishedAt,
	})
	return resolvedIDs, resolvedEveryone, resolvedReason, detail
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

// truncateRunes caps s to at most n runes.
func truncateRunes(s string, n int) string {
	if n <= 0 || utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n])
}

// parseAIRouteOutput：v0.57 解析 {memberIds[], reason, everyone?}；兼容旧 {memberId, reason}。
// 任一 id 对不上 → 失败；everyone=true → 展开全员。
func parseAIRouteOutput(members []store.CatGroupMember, outText string) (memberIDs []string, everyone bool, reason string, ok bool) {
	var parsed struct {
		MemberIDs []string `json:"memberIds"`
		MemberID  string   `json:"memberId"`
		Everyone  bool     `json:"everyone"`
		Reason    string   `json:"reason"`
	}
	if json.Unmarshal([]byte(extractJSONObject(outText)), &parsed) != nil {
		return nil, false, "", false
	}
	parsed.Reason = strings.TrimSpace(parsed.Reason)
	parsed.Reason = truncateRunes(parsed.Reason, 40)
	parsed.MemberID = strings.TrimSpace(parsed.MemberID)

	rawIDs := parsed.MemberIDs
	if len(rawIDs) == 0 && parsed.MemberID != "" {
		rawIDs = []string{parsed.MemberID}
	}

	everyone = parsed.Everyone
	// 哨兵：memberIds 含 @全体 / everyone / * → 全员
	filtered := make([]string, 0, len(rawIDs))
	for _, raw := range rawIDs {
		raw = strings.TrimSpace(raw)
		low := strings.ToLower(raw)
		if raw == "@全体" || raw == "全体" || low == "everyone" || raw == "*" {
			everyone = true
			continue
		}
		if raw != "" {
			filtered = append(filtered, raw)
		}
	}

	if everyone {
		ids := make([]string, 0, len(members))
		for _, m := range members {
			ids = append(ids, m.ID)
		}
		if len(ids) == 0 {
			return nil, false, "", false
		}
		return ids, true, parsed.Reason, true
	}

	if len(filtered) == 0 {
		return nil, false, "", false
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(filtered))
	for _, raw := range filtered {
		idv, found := resolveRouteMemberID(members, raw)
		if !found {
			return nil, false, "", false
		}
		if seen[idv] {
			continue
		}
		seen[idv] = true
		out = append(out, idv)
	}
	if len(out) == 0 {
		return nil, false, "", false
	}
	return out, false, parsed.Reason, true
}

// routeParseFailReason：解析失败时的机器码（empty / parse_error / unknown_id）。
func routeParseFailReason(members []store.CatGroupMember, outText string) string {
	var parsed struct {
		MemberIDs []string `json:"memberIds"`
		MemberID  string   `json:"memberId"`
		Everyone  bool     `json:"everyone"`
		Reason    string   `json:"reason"`
	}
	if json.Unmarshal([]byte(extractJSONObject(outText)), &parsed) != nil {
		return "parse_error"
	}
	if parsed.Everyone {
		if len(members) == 0 {
			return "unknown_id"
		}
		return ""
	}
	rawIDs := parsed.MemberIDs
	if len(rawIDs) == 0 && strings.TrimSpace(parsed.MemberID) != "" {
		rawIDs = []string{parsed.MemberID}
	}
	if len(rawIDs) == 0 {
		return "empty"
	}
	for _, raw := range rawIDs {
		raw = strings.TrimSpace(raw)
		low := strings.ToLower(raw)
		if raw == "@全体" || raw == "全体" || low == "everyone" || raw == "*" {
			continue
		}
		if raw == "" {
			return "empty"
		}
		if _, ok := resolveRouteMemberID(members, raw); !ok {
			return "unknown_id"
		}
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

// formatCoordBubbleText：协调短气泡正文 = reason≤40 + @名… 或 @全体。
func formatCoordBubbleText(reason string, everyone bool, members []store.CatGroupMember, memberIDs []string) string {
	reason = strings.TrimSpace(truncateRunes(reason, 40))
	if reason == "" {
		reason = "请接一下"
	}
	if everyone {
		return reason + " @全体"
	}
	byID := map[string]string{}
	for _, m := range members {
		byID[m.ID] = m.Name
	}
	parts := make([]string, 0, len(memberIDs))
	for _, idv := range memberIDs {
		name := byID[idv]
		if name == "" {
			name = idv
		}
		parts = append(parts, "@"+name)
	}
	if len(parts) == 0 {
		return reason
	}
	return reason + " " + strings.Join(parts, " ")
}

func (s *Service) postRouteSystem(ctx context.Context, groupID, chainID string, member store.CatGroupMember, route store.CatRouteInfo, text string) {
	s.postSystem(ctx, groupID, chainID, 0, "system", text, "route", route, "", "")
}

// replaceRouteSystem：用新正文/systemKind/mentions 就地替换加载灰字（同 seq），再 emit；失败则插入新行。
func (s *Service) replaceRouteSystem(ctx context.Context, loading store.CatGroupMessage, text, systemKind string, route store.CatRouteInfo, mentions []string) store.CatGroupMessage {
	if loading.Seq == 0 || loading.GroupID == "" {
		msg, err := s.cfg.Store.InsertCatGroupMessage(ctx, store.CatGroupMessage{
			GroupID: loading.GroupID, Kind: "system", Text: text, ChainID: loading.ChainID, Hop: 0,
			SystemKind: systemKind, Route: &route, Mentions: mentions, MemberID: route.MemberID,
		})
		if err != nil {
			return store.CatGroupMessage{}
		}
		s.emitGroupMessage(msg)
		return msg
	}
	upd := loading
	upd.Kind = "system"
	upd.Text = text
	upd.SystemKind = systemKind
	upd.Route = &route
	upd.Mentions = mentions
	upd.MemberID = route.MemberID
	upd.FromMemberID = ""
	upd.ToMemberID = ""
	upd.FailReason = ""
	msg, err := s.cfg.Store.UpdateCatGroupMessage(ctx, upd)
	if err != nil {
		// 更新失败则插入，避免丢结果（可能短暂叠两条，列表刷新后以 DB 为准）。
		return s.postSystem(ctx, loading.GroupID, loading.ChainID, 0, "system", text, systemKind, route, "", "")
	}
	s.emitGroupMessage(msg)
	return msg
}

func (s *Service) postSystem(ctx context.Context, groupID, chainID string, hop int, kind, text, systemKind string, route store.CatRouteInfo, fromID, toID string) store.CatGroupMessage {
	msg, err := s.cfg.Store.InsertCatGroupMessage(ctx, store.CatGroupMessage{
		GroupID: groupID, Kind: kind, Text: text, ChainID: chainID, Hop: hop,
		SystemKind: systemKind, Route: &route, FromMemberID: fromID, ToMemberID: toID,
		MemberID: route.MemberID, Mentions: append([]string(nil), route.MemberIDs...),
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

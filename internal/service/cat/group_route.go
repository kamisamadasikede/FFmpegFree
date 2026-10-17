package cat

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/id"
	"FFmpegFree/internal/store"
)

const maxGroupHops = 6

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
		s.wakeMember(ctx, g, mid, chainID, 0, userMsg.Seq, false)
	}
	return SendGroupMessageResult{UserMessage: userMsg, ChainID: chainID}, nil
}

func (s *Service) decideRoute(ctx context.Context, g store.CatGroup, members []store.CatGroupMember, userMsg store.CatGroupMessage, chainID string) ([]string, store.CatRouteInfo) {
	first := members[0]
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
	routingMsg := s.postSystem(ctx, g.ID, chainID, 0, "system", "正在安排谁来接…", "route", store.CatRouteInfo{Mode: "ai"}, "", "")
	_ = routingMsg
	s.emitGroupRoute(GroupRouteEvent{GroupID: g.ID, ChainID: chainID, State: "routing", Route: store.CatRouteInfo{Mode: "ai"}})

	memberID, reason, detail := s.runAIRoute(ctx, g, members, userMsg)
	if memberID == "" {
		route := store.CatRouteInfo{Mode: "fallback", MemberID: first.ID, Reason: "没判断出该谁接，先交给排第一的成员", RouteSeq: detail.RouteSeq}
		s.postRouteSystem(ctx, g.ID, chainID, first, route, "交给 "+first.Name+"：没判断出该谁接，先交给排第一的成员")
		s.emitGroupRoute(GroupRouteEvent{GroupID: g.ID, ChainID: chainID, State: "routed", Route: route})
		return []string{first.ID}, route
	}
	var name string
	for _, m := range members {
		if m.ID == memberID {
			name = m.Name
			break
		}
	}
	route := store.CatRouteInfo{Mode: "ai", MemberID: memberID, Reason: reason, RouteSeq: detail.RouteSeq}
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

func (s *Service) runAIRoute(ctx context.Context, g store.CatGroup, members []store.CatGroupMember, userMsg store.CatGroupMessage) (memberID, reason string, detail store.CatGroupRouteDetail) {
	first := members[0]
	a, err := s.adapter(first.AgentKind)
	if err != nil {
		return "", "", detail
	}
	recent, _ := s.cfg.Store.ListCatGroupMessages(ctx, g.ID, 0, 10)
	var b strings.Builder
	b.WriteString("根据用户消息和群成员职责，选出最合适的一个人。只输出 JSON。\n")
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
	turnCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	s.applyProviderOverlay(ctx, a, first.AgentKind)
	opts := catagent.TurnOptions{
		Ctx: turnCtx, ConversationID: "group-route-" + g.ID + "-" + id.New(),
		ModelID: first.ModelID, ThinkLevelID: s.effortForTurn(ctx, first.AgentKind, first.ModelID, first.ThinkLevel),
		Ephemeral: true, NoTools: true, JSONSchema: catagent.RouteSchema,
		Messages: []catagent.WireMessage{{Role: "user", Content: input}},
		Provider: s.providerOverlayFor(ctx, first.AgentKind),
		Timeout:  20 * time.Second,
	}
	if row, err := s.cfg.Store.GetCatProject(ctx, g.ProjectID); err == nil {
		opts.ProjectPath = row.Path
	}
	resp, runErr := a.RunTurn(opts)
	outText := strings.TrimSpace(resp.Message.Content)
	detail, _ = s.cfg.Store.InsertCatGroupRoute(ctx, store.CatGroupRouteDetail{
		GroupID: g.ID, ChainID: userMsg.ChainID, Mode: "ai", InputText: input, OutputText: outText,
	})
	if runErr != nil || outText == "" {
		return "", "", detail
	}
	var parsed struct {
		MemberID string `json:"memberId"`
		Reason   string `json:"reason"`
	}
	if json.Unmarshal([]byte(extractJSONObject(outText)), &parsed) != nil {
		return "", "", detail
	}
	parsed.MemberID = strings.TrimSpace(parsed.MemberID)
	parsed.Reason = strings.TrimSpace(parsed.Reason)
	if utf8.RuneCountInString(parsed.Reason) > 20 {
		r := []rune(parsed.Reason)
		parsed.Reason = string(r[:20])
	}
	ok := false
	for _, m := range members {
		if m.ID == parsed.MemberID {
			ok = true
			break
		}
	}
	if !ok {
		return "", "", detail
	}
	detail.MemberID = parsed.MemberID
	detail.Reason = parsed.Reason
	_ = detail // already inserted; RouteSeq set
	return parsed.MemberID, parsed.Reason, detail
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
	if fromID != "" {
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
	s.wakeMember(ctx, g, to.ID, req.ChainID, 0, 0, true)
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
	// hop does not increase
	s.wakeMember(ctx, g, m.ID, req.ChainID, 0, 0, true)
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

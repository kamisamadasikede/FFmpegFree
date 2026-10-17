package cat

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"unicode/utf8"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/id"
	"FFmpegFree/internal/store"
)

func (s *Service) errGroupNotFound() error {
	return apperr.New(apperr.NotFound, "找不到这个群聊").WithDetail("reason=group_not_found")
}
func (s *Service) errMemberNotFound() error {
	return apperr.New(apperr.NotFound, "找不到这个成员").WithDetail("reason=member_not_found")
}
func (s *Service) errMemberNameTaken() error {
	return apperr.New(apperr.InvalidArgument, "群里已有同名成员").WithDetail("reason=member_name_taken")
}

func (s *Service) requireGroup(ctx context.Context, groupID string) (store.CatGroup, error) {
	if s.cfg.Store == nil {
		return store.CatGroup{}, apperr.New(apperr.Internal, "本地存储尚未初始化")
	}
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return store.CatGroup{}, apperr.New(apperr.InvalidArgument, "群聊编号不能为空")
	}
	g, err := s.cfg.Store.GetCatGroup(ctx, groupID)
	if errors.Is(err, sql.ErrNoRows) {
		return store.CatGroup{}, s.errGroupNotFound()
	}
	if err != nil {
		return store.CatGroup{}, apperr.Wrap(apperr.IOError, "读取群聊失败", err)
	}
	return g, nil
}

func (s *Service) requireMember(ctx context.Context, memberID string) (store.CatGroupMember, error) {
	if s.cfg.Store == nil {
		return store.CatGroupMember{}, apperr.New(apperr.Internal, "本地存储尚未初始化")
	}
	memberID = strings.TrimSpace(memberID)
	if memberID == "" {
		return store.CatGroupMember{}, apperr.New(apperr.InvalidArgument, "成员编号不能为空")
	}
	m, err := s.cfg.Store.GetCatGroupMember(ctx, memberID)
	if errors.Is(err, sql.ErrNoRows) {
		return store.CatGroupMember{}, s.errMemberNotFound()
	}
	if err != nil {
		return store.CatGroupMember{}, apperr.Wrap(apperr.IOError, "读取群成员失败", err)
	}
	return m, nil
}

func (s *Service) ListCatGroups(ctx context.Context, projectID string) ([]Group, error) {
	if s.cfg.Store == nil {
		return nil, apperr.New(apperr.Internal, "本地存储尚未初始化")
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, apperr.New(apperr.InvalidArgument, "项目编号不能为空")
	}
	if _, err := s.cfg.Store.GetCatProject(ctx, projectID); errors.Is(err, sql.ErrNoRows) {
		return nil, errProjectNotFound()
	} else if err != nil {
		return nil, apperr.Wrap(apperr.IOError, "读取项目失败", err)
	}
	list, err := s.cfg.Store.ListCatGroups(ctx, projectID)
	if err != nil {
		return nil, apperr.Wrap(apperr.IOError, "列出群聊失败", err)
	}
	// 6.21.17 ②：一条 SQL 带出所有群的成员摘要。
	ids := make([]string, 0, len(list))
	for _, g := range list {
		ids = append(ids, g.ID)
	}
	briefs, err := s.cfg.Store.ListCatGroupMemberBriefs(ctx, ids)
	if err != nil {
		return nil, apperr.Wrap(apperr.IOError, "列出群成员失败", err)
	}
	for i := range list {
		list[i].Members = briefs[list[i].ID]
	}
	return list, nil
}

func (s *Service) CreateCatGroup(ctx context.Context, req CreateGroupRequest) (Group, error) {
	if s.cfg.Store == nil {
		return Group{}, apperr.New(apperr.Internal, "本地存储尚未初始化")
	}
	projectID := strings.TrimSpace(req.ProjectID)
	name := strings.TrimSpace(req.Name)
	if projectID == "" || name == "" {
		return Group{}, apperr.New(apperr.InvalidArgument, "项目或群名不能为空")
	}
	if n := utf8.RuneCountInString(name); n < 1 || n > 40 {
		return Group{}, apperr.New(apperr.InvalidArgument, "群名长度须为 1–40 字")
	}
	row, err := s.cfg.Store.GetCatProject(ctx, projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return Group{}, errProjectNotFound()
	}
	if err != nil {
		return Group{}, apperr.Wrap(apperr.IOError, "读取项目失败", err)
	}
	if s.checkProject(row).Missing {
		return Group{}, errProjectMissing()
	}
	g, err := s.cfg.Store.InsertCatGroup(ctx, store.CatGroup{
		ProjectID: projectID,
		Name:      name,
		AutoRoute: true,
	})
	if err != nil {
		return Group{}, apperr.Wrap(apperr.IOError, "创建群聊失败", err)
	}
	return g, nil
}

func (s *Service) RenameCatGroup(ctx context.Context, req RenameGroupRequest) (Group, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return Group{}, apperr.New(apperr.InvalidArgument, "群名不能为空")
	}
	if n := utf8.RuneCountInString(name); n < 1 || n > 40 {
		return Group{}, apperr.New(apperr.InvalidArgument, "群名长度须为 1–40 字")
	}
	g, err := s.requireGroup(ctx, req.GroupID)
	if err != nil {
		return Group{}, err
	}
	if err := s.cfg.Store.RenameCatGroup(ctx, g.ID, name); err != nil {
		return Group{}, apperr.Wrap(apperr.IOError, "重命名群聊失败", err)
	}
	g.Name = name
	s.fillGroupMembers(ctx, &g)
	return g, nil
}

func (s *Service) DeleteCatGroup(ctx context.Context, groupID string) error {
	g, err := s.requireGroup(ctx, groupID)
	if err != nil {
		return err
	}
	_ = s.StopCatGroup(ctx, g.ID)
	members, _ := s.cfg.Store.ListCatGroupMembers(ctx, g.ID)
	if err := s.cfg.Store.DeleteCatGroup(ctx, g.ID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return apperr.Wrap(apperr.IOError, "删除群聊失败", err)
	}
	// Delete backing conversations; leave CLI session files alone (契约 6.21.3).
	for _, m := range members {
		_ = s.cfg.Store.DeleteCatConversation(ctx, m.ID)
	}
	return nil
}

func (s *Service) SetCatGroupAutoRoute(ctx context.Context, req SetGroupAutoRouteRequest) (Group, error) {
	g, err := s.requireGroup(ctx, req.GroupID)
	if err != nil {
		return Group{}, err
	}
	if err := s.cfg.Store.SetCatGroupAutoRoute(ctx, g.ID, req.On); err != nil {
		return Group{}, apperr.Wrap(apperr.IOError, "更新派活开关失败", err)
	}
	g.AutoRoute = req.On
	s.fillGroupMembers(ctx, &g)
	return g, nil
}

// fillGroupMembers 给单个群带上成员摘要（GetCatGroup / Rename / SetAutoRoute 返回的 CatGroup 同样带，6.21.17 ②）。
func (s *Service) fillGroupMembers(ctx context.Context, g *Group) {
	g.Members = []store.CatGroupMemberBrief{}
	if briefs, err := s.cfg.Store.ListCatGroupMemberBriefs(ctx, []string{g.ID}); err == nil {
		g.Members = briefs[g.ID]
	}
}

func memberBriefs(list []store.CatGroupMember) []store.CatGroupMemberBrief {
	out := make([]store.CatGroupMemberBrief, 0, len(list))
	for _, m := range list {
		out = append(out, store.CatGroupMemberBrief{
			ID: m.ID, Name: m.Name, Title: m.Title, AgentKind: m.AgentKind, Order: m.Order,
			AvatarIndex: store.CatMemberAvatarIndex(m.Order),
		})
	}
	return out
}

func (s *Service) ListCatGroupMembers(ctx context.Context, groupID string) ([]GroupMember, error) {
	if _, err := s.requireGroup(ctx, groupID); err != nil {
		return nil, err
	}
	list, err := s.cfg.Store.ListCatGroupMembers(ctx, groupID)
	if err != nil {
		return nil, apperr.Wrap(apperr.IOError, "列出群成员失败", err)
	}
	decorateGroupMembers(list)
	return list, nil
}

func normalizeMemberName(name string) string {
	return strings.TrimSpace(name)
}

func (s *Service) AddCatGroupMember(ctx context.Context, req AddGroupMemberRequest) (GroupMember, error) {
	g, err := s.requireGroup(ctx, req.GroupID)
	if err != nil {
		return GroupMember{}, err
	}
	name := normalizeMemberName(req.Name)
	title := strings.TrimSpace(req.Title)
	kind := strings.TrimSpace(req.AgentKind)
	if name == "" || title == "" || kind == "" {
		return GroupMember{}, apperr.New(apperr.InvalidArgument, "姓名、职位和助手不能为空")
	}
	if !catagent.Phase1Creatable(kind) {
		return GroupMember{}, apperr.New(apperr.InvalidArgument, "不支持这种助手模式")
	}
	modelID := strings.TrimSpace(req.ModelID)
	think := strings.TrimSpace(req.ThinkLevelID)
	if err := s.validateGroupMemberModel(ctx, kind, modelID, think); err != nil {
		return GroupMember{}, err
	}
	order, err := s.cfg.Store.NextCatGroupMemberOrder(ctx, g.ID)
	if err != nil {
		return GroupMember{}, apperr.Wrap(apperr.IOError, "写入群成员失败", err)
	}
	memberID := id.New()
	// Backing conversation so turn-changes / file tools can resolve the member.
	if _, err := s.cfg.Store.InsertCatConversation(ctx, store.CatConversation{
		ID: memberID, Title: name, AgentKind: kind, AccessMode: catagent.AccessAsk, ProjectID: g.ProjectID,
	}); err != nil {
		return GroupMember{}, apperr.Wrap(apperr.IOError, "创建成员会话失败", err)
	}
	m, err := s.cfg.Store.InsertCatGroupMember(ctx, store.CatGroupMember{
		ID: memberID, GroupID: g.ID, Name: name, Title: title, AgentKind: kind,
		ModelID: modelID, ThinkLevel: think,
		Order: order, Charter: store.CatCharter{Status: "generating", Dirs: []string{}, Edited: []string{}},
	})
	if err != nil {
		_ = s.cfg.Store.DeleteCatConversation(ctx, memberID)
		if store.IsMemberNameTaken(err) {
			return GroupMember{}, s.errMemberNameTaken()
		}
		return GroupMember{}, apperr.Wrap(apperr.IOError, "写入群成员失败", err)
	}
	s.emitGroupMember(m, nil)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.generateCharter(context.Background(), m.ID, false)
	}()
	decorateGroupMember(&m)
	return m, nil
}

func (s *Service) GetCatGroup(ctx context.Context, groupID string) (GroupDetail, error) {
	g, err := s.requireGroup(ctx, groupID)
	if err != nil {
		return GroupDetail{}, err
	}
	members, err := s.cfg.Store.ListCatGroupMembers(ctx, g.ID)
	if err != nil {
		return GroupDetail{}, apperr.Wrap(apperr.IOError, "列出群成员失败", err)
	}
	decorateGroupMembers(members)
	g.Members = memberBriefs(members)
	return GroupDetail{Group: g, Members: members}, nil
}

func (s *Service) UpdateCatGroupMember(ctx context.Context, req UpdateGroupMemberRequest) (GroupMember, error) {
	m, err := s.requireMember(ctx, req.MemberID)
	if err != nil {
		return GroupMember{}, err
	}
	oldKind := m.AgentKind
	if req.Name != nil {
		name := normalizeMemberName(*req.Name)
		if name == "" {
			return GroupMember{}, apperr.New(apperr.InvalidArgument, "姓名不能为空")
		}
		m.Name = name
	}
	if req.Title != nil {
		m.Title = strings.TrimSpace(*req.Title)
	}
	if req.AgentKind != nil {
		k := strings.TrimSpace(*req.AgentKind)
		if !catagent.Phase1Creatable(k) {
			return GroupMember{}, apperr.New(apperr.InvalidArgument, "不支持这种助手模式")
		}
		m.AgentKind = k
	}
	if req.ModelID != nil {
		m.ModelID = strings.TrimSpace(*req.ModelID)
	}
	if req.ThinkLevelID != nil {
		m.ThinkLevel = strings.TrimSpace(*req.ThinkLevelID)
	}
	if err := s.validateGroupMemberModel(ctx, m.AgentKind, m.ModelID, m.ThinkLevel); err != nil {
		return GroupMember{}, err
	}
	if m.AgentKind != oldKind {
		m.SessionID = ""
		if a, err := s.adapter(oldKind); err == nil {
			if clearer, ok := a.(interface{ ClearSession(string) }); ok {
				clearer.ClearSession(m.ID)
			}
		}
	}
	if err := s.cfg.Store.UpdateCatGroupMember(ctx, m); err != nil {
		if store.IsMemberNameTaken(err) {
			return GroupMember{}, s.errMemberNameTaken()
		}
		return GroupMember{}, apperr.Wrap(apperr.IOError, "更新群成员失败", err)
	}
	if req.Name != nil {
		s.refreshHandoffs(ctx, m.GroupID)
	}
	s.emitGroupMemberSnapshot(m)
	decorateGroupMember(&m)
	return m, nil
}

func (s *Service) RemoveCatGroupMember(ctx context.Context, memberID string) error {
	m, err := s.requireMember(ctx, memberID)
	if err != nil {
		return err
	}
	_ = s.StopCatGroupMember(ctx, m.ID)
	if err := s.cfg.Store.RemoveCatGroupMember(ctx, m.ID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return apperr.Wrap(apperr.IOError, "删除群成员失败", err)
	}
	_ = s.cfg.Store.DeleteCatConversation(ctx, m.ID)
	s.emitGroupMember(m, nil) // last snapshot; member already removed — emit with zero running
	s.refreshHandoffs(ctx, m.GroupID)
	return nil
}

func (s *Service) ReorderCatGroupMembers(ctx context.Context, req ReorderGroupMembersRequest) ([]GroupMember, error) {
	if _, err := s.requireGroup(ctx, req.GroupID); err != nil {
		return nil, err
	}
	cur, err := s.cfg.Store.ListCatGroupMembers(ctx, req.GroupID)
	if err != nil {
		return nil, apperr.Wrap(apperr.IOError, "列出群成员失败", err)
	}
	if len(req.MemberIDs) != len(cur) {
		return nil, apperr.New(apperr.InvalidArgument, "成员顺序必须包含全部成员")
	}
	seen := map[string]bool{}
	for _, idv := range req.MemberIDs {
		seen[idv] = true
	}
	for _, m := range cur {
		if !seen[m.ID] {
			return nil, apperr.New(apperr.InvalidArgument, "成员顺序必须包含全部成员")
		}
	}
	if err := s.cfg.Store.ReorderCatGroupMembers(ctx, req.GroupID, req.MemberIDs); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, s.errMemberNotFound()
		}
		return nil, apperr.Wrap(apperr.IOError, "调整成员顺序失败", err)
	}
	return s.ListCatGroupMembers(ctx, req.GroupID)
}

// emitGroupMember 推 cat:group-member；running 非空 = 正在干活，nil = 这一轮结束（契约 6.21.14 ④）。
// 同时带 status / queueLength / queue（6.21.17 ④）。
func (s *Service) emitGroupMember(m GroupMember, running *GroupRunning) {
	groupQueueMu.Lock()
	if st := groupQueues[m.ID]; st != nil {
		if running != nil {
			r := *running
			st.running = &r
		} else {
			st.running = nil
		}
	}
	groupQueueMu.Unlock()
	s.publishGroupMember(m, running)
}

// emitGroupMemberSnapshot 推成员最新快照，干活状态沿用当前（入队 / 清队 / 规范变化时用，不把正在干活的人误报成结束）。
func (s *Service) emitGroupMemberSnapshot(m GroupMember) {
	s.publishGroupMember(m, groupRunningSnapshot(m.ID))
}

func (s *Service) publishGroupMember(m GroupMember, running *GroupRunning) {
	decorateGroupMember(&m)
	s.emit("cat:group-member", GroupMemberEvent{GroupID: m.GroupID, Member: m, Running: running, QueueLength: m.QueueLength, Queue: m.Queue})
}

func (s *Service) emitGroupMessage(m GroupMessage) {
	s.emit("cat:group-message", m)
}

func (s *Service) emitGroupRoute(ev GroupRouteEvent) {
	s.emit("cat:group-route", ev)
}

func (s *Service) ListCatGroupMessages(ctx context.Context, req ListGroupMessagesRequest) (ListGroupMessagesResult, error) {
	if _, err := s.requireGroup(ctx, req.GroupID); err != nil {
		return ListGroupMessagesResult{}, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	// fetch one extra to detect hasMore
	list, err := s.cfg.Store.ListCatGroupMessages(ctx, req.GroupID, req.BeforeSeq, limit+1)
	if err != nil {
		return ListGroupMessagesResult{}, apperr.Wrap(apperr.IOError, "列出群消息失败", err)
	}
	hasMore := len(list) > limit
	if hasMore {
		list = list[:limit]
	}
	return ListGroupMessagesResult{Messages: list, HasMore: hasMore}, nil
}

func (s *Service) GetCatRouteDetail(ctx context.Context, req GetRouteDetailRequest) (GetRouteDetailResult, error) {
	if _, err := s.requireGroup(ctx, req.GroupID); err != nil {
		return GetRouteDetailResult{}, err
	}
	msg, err := s.cfg.Store.GetCatGroupMessage(ctx, req.GroupID, req.Seq)
	if errors.Is(err, sql.ErrNoRows) {
		return GetRouteDetailResult{}, apperr.New(apperr.NotFound, "找不到这条消息")
	}
	if err != nil {
		return GetRouteDetailResult{}, apperr.Wrap(apperr.IOError, "读取消息失败", err)
	}
	if msg.Route == nil || msg.Route.RouteSeq == 0 {
		return GetRouteDetailResult{}, apperr.New(apperr.NotFound, "找不到这次派活记录")
	}
	d, err := s.cfg.Store.GetCatGroupRoute(ctx, msg.Route.RouteSeq)
	if errors.Is(err, sql.ErrNoRows) {
		return GetRouteDetailResult{}, apperr.New(apperr.NotFound, "找不到这次派活记录")
	}
	if err != nil {
		return GetRouteDetailResult{}, apperr.Wrap(apperr.IOError, "读取派活记录失败", err)
	}
	res := GetRouteDetailResult{Input: d.InputText, Output: d.OutputText, Mode: d.Mode, MemberID: d.MemberID, Reason: d.Reason,
		StartedAt: d.StartedAt, FinishedAt: d.FinishedAt}
	if ms := routeDurationMs(d); ms > 0 {
		res.DurationMs = &ms
	}
	return res, nil
}

// validateGroupMemberModel：非空 modelId 必须在 ListCatModels(agentKind) 里（与设置页同源，含自定义服务商）。
// 空 modelId = 跟随 CLI 默认，始终合法。非空 thinkLevel 须在该模型 efforts 内（列不出模型时只校格式）。
func (s *Service) validateGroupMemberModel(ctx context.Context, agentKind, modelID, thinkLevel string) error {
	modelID = strings.TrimSpace(modelID)
	thinkLevel = strings.TrimSpace(thinkLevel)
	if modelID != "" {
		list, err := s.ListCatModels(ctx, agentKind)
		if err != nil {
			return err
		}
		if _, ok := catagent.FindModel(list, modelID); !ok {
			return apperr.New(apperr.InvalidArgument, "这个模型当前不可用，请重选。").WithDetail("reason=model")
		}
	}
	if thinkLevel == "" {
		return nil
	}
	if !s.validEffortFor(ctx, agentKind, modelID, thinkLevel) {
		return apperr.New(apperr.InvalidArgument, "这个思考强度当前不可用，请重选。").WithDetail("reason=think")
	}
	return nil
}

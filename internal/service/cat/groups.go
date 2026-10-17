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
	return g, nil
}

func (s *Service) ListCatGroupMembers(ctx context.Context, groupID string) ([]GroupMember, error) {
	if _, err := s.requireGroup(ctx, groupID); err != nil {
		return nil, err
	}
	list, err := s.cfg.Store.ListCatGroupMembers(ctx, groupID)
	if err != nil {
		return nil, apperr.Wrap(apperr.IOError, "列出群成员失败", err)
	}
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
		ModelID: strings.TrimSpace(req.ModelID), ThinkLevel: strings.TrimSpace(req.ThinkLevelID),
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
	s.emitGroupMember(m, nil)
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

func (s *Service) emitGroupMember(m GroupMember, running *GroupRunning) {
	s.emit("cat:group-member", GroupMemberEvent{GroupID: m.GroupID, Member: m, Running: running})
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
	return GetRouteDetailResult{Input: d.InputText, Output: d.OutputText, Mode: d.Mode, MemberID: d.MemberID, Reason: d.Reason}, nil
}

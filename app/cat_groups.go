package app

import "FFmpegFree/internal/service/cat"

func (s *CatService) ListCatGroups(projectId string) ([]cat.Group, error) {
	c, err := s.svc()
	if err != nil {
		return nil, err
	}
	return c.ListCatGroups(s.rootCtx(), projectId)
}
func (s *CatService) GetCatGroup(groupId string) (cat.GroupDetail, error) {
	c, err := s.svc()
	if err != nil {
		return cat.GroupDetail{}, err
	}
	return c.GetCatGroup(s.rootCtx(), groupId)
}
func (s *CatService) CreateCatGroup(req cat.CreateGroupRequest) (cat.Group, error) {
	c, err := s.svc()
	if err != nil {
		return cat.Group{}, err
	}
	return c.CreateCatGroup(s.rootCtx(), req)
}
func (s *CatService) RenameCatGroup(req cat.RenameGroupRequest) (cat.Group, error) {
	c, err := s.svc()
	if err != nil {
		return cat.Group{}, err
	}
	return c.RenameCatGroup(s.rootCtx(), req)
}
func (s *CatService) DeleteCatGroup(id string) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.DeleteCatGroup(s.rootCtx(), id)
}
func (s *CatService) SetCatGroupAutoRoute(groupId string, on bool) (cat.Group, error) {
	c, err := s.svc()
	if err != nil {
		return cat.Group{}, err
	}
	return c.SetCatGroupAutoRoute(s.rootCtx(), cat.SetGroupAutoRouteRequest{GroupID: groupId, On: on})
}
func (s *CatService) ListCatGroupMembers(groupId string) ([]cat.GroupMember, error) {
	c, err := s.svc()
	if err != nil {
		return nil, err
	}
	return c.ListCatGroupMembers(s.rootCtx(), groupId)
}
func (s *CatService) AddCatGroupMember(req cat.AddGroupMemberRequest) (cat.GroupMember, error) {
	c, err := s.svc()
	if err != nil {
		return cat.GroupMember{}, err
	}
	return c.AddCatGroupMember(s.rootCtx(), req)
}
func (s *CatService) UpdateCatGroupMember(req cat.UpdateGroupMemberRequest) (cat.GroupMember, error) {
	c, err := s.svc()
	if err != nil {
		return cat.GroupMember{}, err
	}
	return c.UpdateCatGroupMember(s.rootCtx(), req)
}
func (s *CatService) RemoveCatGroupMember(id string) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.RemoveCatGroupMember(s.rootCtx(), id)
}
func (s *CatService) ReorderCatGroupMembers(req cat.ReorderGroupMembersRequest) ([]cat.GroupMember, error) {
	c, err := s.svc()
	if err != nil {
		return nil, err
	}
	return c.ReorderCatGroupMembers(s.rootCtx(), req)
}
func (s *CatService) RegenerateCatCharter(memberId string) (cat.GroupMember, error) {
	c, err := s.svc()
	if err != nil {
		return cat.GroupMember{}, err
	}
	return c.RegenerateCatCharter(s.rootCtx(), memberId)
}
func (s *CatService) UpdateCatCharter(req cat.UpdateCharterRequest) (cat.GroupMember, error) {
	c, err := s.svc()
	if err != nil {
		return cat.GroupMember{}, err
	}
	return c.UpdateCatCharter(s.rootCtx(), req)
}
func (s *CatService) SendCatGroupMessage(req cat.SendGroupMessageRequest) (cat.SendGroupMessageResult, error) {
	c, err := s.svc()
	if err != nil {
		return cat.SendGroupMessageResult{}, err
	}
	return c.SendCatGroupMessage(s.rootCtx(), req)
}
func (s *CatService) ReassignCatGroupRoute(req cat.ReassignGroupRouteRequest) (cat.ReassignGroupRouteResult, error) {
	c, err := s.svc()
	if err != nil {
		return cat.ReassignGroupRouteResult{}, err
	}
	return c.ReassignCatGroupRoute(s.rootCtx(), req)
}
func (s *CatService) StopCatGroup(groupId string) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.StopCatGroup(s.rootCtx(), groupId)
}
func (s *CatService) StopCatGroupMember(memberId string) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.StopCatGroupMember(s.rootCtx(), memberId)
}
func (s *CatService) ListCatGroupMessages(req cat.ListGroupMessagesRequest) (cat.ListGroupMessagesResult, error) {
	c, err := s.svc()
	if err != nil {
		return cat.ListGroupMessagesResult{}, err
	}
	return c.ListCatGroupMessages(s.rootCtx(), req)
}
func (s *CatService) GetCatRouteDetail(req cat.GetRouteDetailRequest) (cat.GetRouteDetailResult, error) {
	c, err := s.svc()
	if err != nil {
		return cat.GetRouteDetailResult{}, err
	}
	return c.GetCatRouteDetail(s.rootCtx(), req)
}
func (s *CatService) RevertCatOverreach(req cat.RevertOverreachRequest) (cat.RevertOverreachResult, error) {
	c, err := s.svc()
	if err != nil {
		return cat.RevertOverreachResult{}, err
	}
	return c.RevertCatOverreach(s.rootCtx(), req)
}
func (s *CatService) RetryCatGroupMemberTurn(req cat.RetryGroupMemberTurnRequest) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.RetryCatGroupMemberTurn(s.rootCtx(), req)
}
func (s *CatService) ListCatGroupFiles(groupId string) ([]cat.GroupFile, error) {
	c, err := s.svc()
	if err != nil {
		return nil, err
	}
	return c.ListCatGroupFiles(s.rootCtx(), groupId)
}

func (s *CatService) DismissCatGroupWorkflow(req cat.DismissGroupWorkflowRequest) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.DismissCatGroupWorkflow(s.rootCtx(), req)
}

func (s *CatService) ClearCatGroupWorkflowHistory(groupID string) ([]string, error) {
	c, err := s.svc()
	if err != nil {
		return nil, err
	}
	return c.ClearCatGroupWorkflowHistory(s.rootCtx(), groupID)
}

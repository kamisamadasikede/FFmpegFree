package app

import (
	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/genoffice"
	"FFmpegFree/internal/team"
	"context"
)

type TeamService struct {
	get func() *team.Service
	ctx func() context.Context
}

func NewTeamService(get func() *team.Service, ctx func() context.Context) *TeamService {
	return &TeamService{get, ctx}
}
func (s *TeamService) service() (*team.Service, error) {
	if s.get != nil {
		if v := s.get(); v != nil {
			return v, nil
		}
	}
	return nil, apperr.New(apperr.Internal, "群聊服务尚未初始化")
}
func (s *TeamService) GetTeamBootstrap() (team.Bootstrap, error) {
	v, e := s.service()
	if e != nil {
		return team.Bootstrap{}, e
	}
	return v.Bootstrap(s.ctx())
}
func (s *TeamService) GetTeamRoom(roomID string, before int64) (team.Snapshot, error) {
	v, e := s.service()
	if e != nil {
		return team.Snapshot{}, e
	}
	return v.Snapshot(s.ctx(), roomID, before)
}
func (s *TeamService) GetTeamEvents(roomID string, after int64) ([]team.Event, error) {
	v, e := s.service()
	if e != nil {
		return nil, e
	}
	return v.Events(s.ctx(), roomID, after)
}
func (s *TeamService) SaveTeamHuman(req team.Human) (team.Human, error) {
	v, e := s.service()
	if e != nil {
		return req, e
	}
	return v.SaveHuman(s.ctx(), req)
}
func (s *TeamService) SaveTeamEmployee(req team.Employee) (team.Employee, error) {
	v, e := s.service()
	if e != nil {
		return req, e
	}
	return v.SaveEmployee(s.ctx(), req)
}
func (s *TeamService) CreateTeamRoom(req team.CreateRoom) (team.Room, error) {
	v, e := s.service()
	if e != nil {
		return team.Room{}, e
	}
	return v.CreateRoom(s.ctx(), req)
}
func (s *TeamService) UpdateTeamRoom(req team.Room) (team.Room, error) {
	v, e := s.service()
	if e != nil {
		return req, e
	}
	return v.UpdateRoom(s.ctx(), req)
}
func (s *TeamService) SetTeamMember(roomID, employeeID string, active bool) error {
	v, e := s.service()
	if e != nil {
		return e
	}
	return v.SetMember(s.ctx(), roomID, employeeID, active)
}
func (s *TeamService) SendTeamMessage(req team.Command) (team.CommandResult, error) {
	v, e := s.service()
	if e != nil {
		return team.CommandResult{}, e
	}
	return v.Command(s.ctx(), req)
}
func (s *TeamService) StopTeamTask(taskID string) error {
	v, e := s.service()
	if e != nil {
		return e
	}
	return v.Stop(s.ctx(), taskID)
}
func (s *TeamService) ContinueTeamTask(taskID, content string) error {
	v, e := s.service()
	if e != nil {
		return e
	}
	return v.Continue(s.ctx(), taskID, content)
}
func (s *TeamService) MarkTeamMessagesRead(roomID string, messageIDs []string) error {
	v, e := s.service()
	if e != nil {
		return e
	}
	return v.MarkRead(s.ctx(), roomID, messageIDs)
}

func (s *TeamService) ListTeamFiles(roomID string) (team.FileList, error) {
	v, e := s.service()
	if e != nil {
		return team.FileList{}, e
	}
	return v.Files(s.ctx(), roomID)
}
func (s *TeamService) StartTeamPreview(roomID, path string) (genoffice.Preview, error) {
	v, e := s.service()
	if e != nil {
		return genoffice.Preview{}, e
	}
	return v.StartPreview(s.ctx(), roomID, path)
}
func (s *TeamService) ReleaseTeamPreview(id string) error {
	v, e := s.service()
	if e != nil {
		return e
	}
	v.ReleasePreview(id)
	return nil
}

func (s *TeamService) GetTeamTaskRuns(taskID string) ([]team.RunRecord, error) {
	v, e := s.service()
	if e != nil {
		return nil, e
	}
	return v.Runs(s.ctx(), taskID)
}

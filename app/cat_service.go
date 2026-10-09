package app

import (
	"context"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/service/cat"
)

// CatService 是 Cat 助手的 Wails 绑定（契约 6.19）。
type CatService struct {
	get func() *cat.Service
	ctx func() context.Context
}

// NewCatService 创建绑定。get 在 OnStartup 完成前可能返回 nil。
func NewCatService(get func() *cat.Service, rootCtx func() context.Context) *CatService {
	return &CatService{get: get, ctx: rootCtx}
}

func (s *CatService) rootCtx() context.Context {
	if s.ctx != nil {
		if c := s.ctx(); c != nil {
			return c
		}
	}
	return context.Background()
}

func (s *CatService) svc() (*cat.Service, error) {
	if s.get != nil {
		if c := s.get(); c != nil {
			return c, nil
		}
	}
	return nil, apperr.New(apperr.Internal, "助手服务尚未初始化")
}

func (s *CatService) GetCatStatus() (catagent.Status, error) {
	c, err := s.svc()
	if err != nil {
		return catagent.Status{}, err
	}
	return c.GetCatStatus(s.rootCtx())
}

func (s *CatService) RecheckCat() (catagent.Status, error) {
	c, err := s.svc()
	if err != nil {
		return catagent.Status{}, err
	}
	return c.RecheckCat(s.rootCtx())
}

func (s *CatService) ListCatModels(agentKind string) ([]catagent.Model, error) {
	c, err := s.svc()
	if err != nil {
		return nil, err
	}
	return c.ListCatModels(s.rootCtx(), agentKind)
}

func (s *CatService) ListCatThinkLevels(agentKind string) ([]catagent.ThinkLevel, error) {
	c, err := s.svc()
	if err != nil {
		return nil, err
	}
	return c.ListCatThinkLevels(s.rootCtx(), agentKind)
}

func (s *CatService) ListCatConversations() ([]cat.Conversation, error) {
	c, err := s.svc()
	if err != nil {
		return nil, err
	}
	return c.ListCatConversations(s.rootCtx())
}

func (s *CatService) GetCatConversation(id string) (cat.ConversationDetail, error) {
	c, err := s.svc()
	if err != nil {
		return cat.ConversationDetail{}, err
	}
	return c.GetCatConversation(s.rootCtx(), id)
}

func (s *CatService) CreateCatConversation(req cat.CreateConversationRequest) (cat.Conversation, error) {
	c, err := s.svc()
	if err != nil {
		return cat.Conversation{}, err
	}
	return c.CreateCatConversation(s.rootCtx(), req)
}

func (s *CatService) DeleteCatConversation(id string) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.DeleteCatConversation(s.rootCtx(), id)
}

func (s *CatService) SendCatMessage(req cat.SendMessageRequest) (cat.SendMessageResult, error) {
	c, err := s.svc()
	if err != nil {
		return cat.SendMessageResult{}, err
	}
	return c.SendCatMessage(s.rootCtx(), req)
}

// CancelCatTurn 立即停止 { convId, turnId } 这一轮（契约 6.19.9）；重复取消幂等。
func (s *CatService) CancelCatTurn(req catagent.CancelCatTurnRequest) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.CancelCatTurn(s.rootCtx(), req)
}

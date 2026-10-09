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

// ---------- 项目（契约 v0.31，6.19.10） ----------

// ListCatProjects 返回全部项目，missing 实时计算；没有项目返回空数组。
func (s *CatService) ListCatProjects() ([]cat.Project, error) {
	c, err := s.svc()
	if err != nil {
		return nil, err
	}
	return c.ListCatProjects(s.rootCtx())
}

// CreateCatProject 建项目；同一文件夹已建过 → 返回已有项目 existed=true。
func (s *CatService) CreateCatProject(req cat.CreateProjectRequest) (cat.CreateProjectResult, error) {
	c, err := s.svc()
	if err != nil {
		return cat.CreateProjectResult{}, err
	}
	return c.CreateCatProject(s.rootCtx(), req)
}

// RenameCatProject 改项目名。
func (s *CatService) RenameCatProject(req cat.RenameProjectRequest) (cat.Project, error) {
	c, err := s.svc()
	if err != nil {
		return cat.Project{}, err
	}
	return c.RenameCatProject(s.rootCtx(), req)
}

// DeleteCatProject 删除项目及其下对话（不动文件夹里的文件）；未知 id 幂等返回 nil。
func (s *CatService) DeleteCatProject(req cat.DeleteProjectRequest) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.DeleteCatProject(s.rootCtx(), req)
}

// RelocateCatProject 换项目文件夹（v0.31.1）：对话保留、名字不变；missing 的项目也可以。
func (s *CatService) RelocateCatProject(req cat.RelocateProjectRequest) (cat.Project, error) {
	c, err := s.svc()
	if err != nil {
		return cat.Project{}, err
	}
	return c.RelocateCatProject(s.rootCtx(), req)
}

// RevealCatProject 在系统文件管理器里打开项目文件夹（v0.31.1）；只收 id。
func (s *CatService) RevealCatProject(req cat.RevealProjectRequest) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.RevealCatProject(s.rootCtx(), req)
}

// ListCatFiles 列当前对话根下的一层（契约 6.19.11）。不读文件内容。
func (s *CatService) ListCatFiles(req cat.ListFilesRequest) (cat.ListFilesResult, error) {
	c, err := s.svc()
	if err != nil {
		return cat.ListFilesResult{}, err
	}
	return c.ListCatFiles(s.rootCtx(), req)
}

// RevealCatConversationFolder 在系统文件管理器里打开对话的根（契约 6.19.11.5）。
func (s *CatService) RevealCatConversationFolder(req cat.RevealConversationFolderRequest) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.RevealCatConversationFolder(s.rootCtx(), req)
}

// ReadCatFile 读取对话根下的一个文件，供侧边预览。
func (s *CatService) ReadCatFile(req cat.ReadFileRequest) (cat.ReadFileResult, error) {
	c, err := s.svc()
	if err != nil {
		return cat.ReadFileResult{}, err
	}
	return c.ReadCatFile(s.rootCtx(), req)
}

// WriteCatFile 覆盖对话根下已有的文本文件。
func (s *CatService) WriteCatFile(req cat.WriteFileRequest) (cat.WriteFileResult, error) {
	c, err := s.svc()
	if err != nil {
		return cat.WriteFileResult{}, err
	}
	return c.WriteCatFile(s.rootCtx(), req)
}

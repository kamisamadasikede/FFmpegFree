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

// SetCatWindowFocused 前端上报主窗口键盘焦点（v0.48 6.19.12.11-1）；服务未就绪时忽略。
func (s *CatService) SetCatWindowFocused(focused bool) {
	if c, err := s.svc(); err == nil {
		c.SetWindowFocused(focused)
	}
}

// GetCatNotifyStatus 系统通知是否可用 / 已授权（v0.48 6.19.12.11-5）。
func (s *CatService) GetCatNotifyStatus() (cat.NotifyStatus, error) {
	c, err := s.svc()
	if err != nil {
		return cat.NotifyStatus{}, err
	}
	return c.GetCatNotifyStatus(), nil
}

// RecheckCat 重新检测；agentKind 为空 = 全部（v0.47.1）。
func (s *CatService) RecheckCat(agentKind string) (catagent.Status, error) {
	c, err := s.svc()
	if err != nil {
		return catagent.Status{}, err
	}
	return c.RecheckCat(s.rootCtx(), agentKind)
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

// RetryCatTurn 原地重试最后一轮失败的 turn（契约 v0.44 6.19.18.7）。
func (s *CatService) RetryCatTurn(req cat.RetryCatTurnRequest) (cat.SendMessageResult, error) {
	c, err := s.svc()
	if err != nil {
		return cat.SendMessageResult{}, err
	}
	return c.RetryCatTurn(s.rootCtx(), req)
}

// PreviewCatMedia 返回本机图片、视频或文档的预览地址。
func (s *CatService) PreviewCatMedia(path string) (cat.MediaPreview, error) {
	c, err := s.svc()
	if err != nil {
		return cat.MediaPreview{}, err
	}
	return c.PreviewCatMedia(s.rootCtx(), path)
}

// ListLiveCatTurns 返回刷新后仍在回复的对话。
func (s *CatService) ListLiveCatTurns() ([]cat.LiveTurn, error) {
	c, err := s.svc()
	if err != nil {
		return nil, err
	}
	return c.ListLiveCatTurns(s.rootCtx())
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

// GitCatStatus 读取对话文件夹的分支和改动。不是仓库时 Repo 为 false。
func (s *CatService) GitCatStatus(req cat.GitStatusRequest) (cat.GitStatusResult, error) {
	c, err := s.svc()
	if err != nil {
		return cat.GitStatusResult{}, err
	}
	return c.GitCatStatus(s.rootCtx(), req)
}

// GitCatStage 暂存改动。Paths 为空时暂存对话根下的全部改动。
func (s *CatService) GitCatStage(req cat.GitPathsRequest) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.GitCatStage(s.rootCtx(), req)
}

// GitCatUnstage 取消暂存指定路径。
func (s *CatService) GitCatUnstage(req cat.GitPathsRequest) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.GitCatUnstage(s.rootCtx(), req)
}

// GitCatCommit 用说明提交。没有已暂存的改动时先暂存再提交。
func (s *CatService) GitCatCommit(req cat.GitPathsRequest) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.GitCatCommit(s.rootCtx(), req)
}

// GitCatDiff 返回这个文件和上一版提交的行对比。
func (s *CatService) GitCatDiff(req cat.GitDiffRequest) (cat.GitDiffResult, error) {
	c, err := s.svc()
	if err != nil {
		return cat.GitDiffResult{}, err
	}
	return c.GitCatDiff(s.rootCtx(), req)
}

// GitCatPullRequest 返回 GitHub 比较页地址，不推送。
func (s *CatService) GitCatPullRequest(req cat.GitStatusRequest) (cat.GitPRResult, error) {
	c, err := s.svc()
	if err != nil {
		return cat.GitPRResult{}, err
	}
	return c.GitCatPullRequest(s.rootCtx(), req)
}

// GitCatRemote 推送、拉取、获取或同步当前分支。
func (s *CatService) GitCatRemote(req cat.GitRemoteRequest) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.GitCatRemote(s.rootCtx(), req)
}

// GitCatBranch 新建分支并切过去。
func (s *CatService) GitCatBranch(req cat.GitBranchRequest) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.GitCatBranch(s.rootCtx(), req)
}

// GitCatConflictResolve 单文件采用当前/传入/双方或手改后暂存（契约 6.19.15）。
func (s *CatService) GitCatConflictResolve(req cat.GitCatConflictResolveRequest) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.GitCatConflictResolve(s.rootCtx(), req)
}

// GitCatConflictComplete 完成合并/变基/拣选。
func (s *CatService) GitCatConflictComplete(req cat.GitCatConflictCompleteRequest) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.GitCatConflictComplete(s.rootCtx(), req)
}

// GitCatConflictAbort 中止合并/变基/拣选。
func (s *CatService) GitCatConflictAbort(req cat.GitCatConflictAbortRequest) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.GitCatConflictAbort(s.rootCtx(), req)
}

// GitCatCheckout 切已有本地分支；脏工作区须 Force。
func (s *CatService) GitCatCheckout(req cat.GitCatCheckoutRequest) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.GitCatCheckout(s.rootCtx(), req)
}

// SearchCatFiles 在对话根下按名称或内容查找。
func (s *CatService) SearchCatFiles(req cat.SearchFilesRequest) (cat.SearchFilesResult, error) {
	c, err := s.svc()
	if err != nil {
		return cat.SearchFilesResult{}, err
	}
	return c.SearchCatFiles(s.rootCtx(), req)
}

// StartCatTerm 在这个对话的文件夹里打开本机命令行。
func (s *CatService) StartCatTerm(req cat.TermStartRequest) (cat.TermStartResult, error) {
	c, err := s.svc()
	if err != nil {
		return cat.TermStartResult{}, err
	}
	return c.StartCatTerm(s.rootCtx(), req)
}

// WriteCatTerm 把输入写进命令行。
func (s *CatService) WriteCatTerm(req cat.TermWriteRequest) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.WriteCatTerm(s.rootCtx(), req)
}

// StopCatTerm 结束命令行。
func (s *CatService) StopCatTerm(req cat.TermStopRequest) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.StopCatTerm(s.rootCtx(), req)
}

// ResizeCatTerm 按字符数调整命令行宽高。
func (s *CatService) ResizeCatTerm(req cat.TermResizeRequest) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.ResizeCatTerm(s.rootCtx(), req)
}

// OpenCatBrowser 取回侧栏浏览器要嵌的网页。
func (s *CatService) OpenCatBrowser(req cat.BrowserOpenRequest) (cat.BrowserPage, error) {
	c, err := s.svc()
	if err != nil {
		return cat.BrowserPage{}, err
	}
	return c.OpenCatBrowser(s.rootCtx(), req)
}

// CloneCatProject 从 Git 地址克隆并建项目（契约 6.19.10.11 / v0.40）。
func (s *CatService) CloneCatProject(req cat.CloneCatProjectRequest) (cat.CloneCatProjectResult, error) {
	c, err := s.svc()
	if err != nil {
		return cat.CloneCatProjectResult{}, err
	}
	return c.CloneCatProject(s.rootCtx(), req)
}

// CancelCloneCatProject 取消进行中的克隆；未知 / 已结束幂等返回 nil。
func (s *CatService) CancelCloneCatProject(req cat.CancelCloneCatProjectRequest) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.CancelCloneCatProject(s.rootCtx(), req)
}

// ListCatBranches 返回对话文件夹路径和所在仓库的分支（契约 v0.49，6.19.24）。
func (s *CatService) ListCatBranches(req cat.CatBranchesRequest) (cat.CatBranchesResult, error) {
	c, err := s.svc()
	if err != nil {
		return cat.CatBranchesResult{}, err
	}
	return c.ListCatBranches(s.rootCtx(), req)
}

// SwitchCatBranch 切本地分支，或从远程分支建跟踪分支（契约 v0.49，6.19.24）。
func (s *CatService) SwitchCatBranch(req cat.SwitchCatBranchRequest) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.SwitchCatBranch(s.rootCtx(), req)
}

// GitCatMerge 把分支合并到当前分支（契约 v0.50，6.19.25）。
func (s *CatService) GitCatMerge(req cat.GitCatMergeRequest) (cat.GitCatMergeResult, error) {
	c, err := s.svc()
	if err != nil {
		return cat.GitCatMergeResult{}, err
	}
	return c.GitCatMerge(s.rootCtx(), req)
}

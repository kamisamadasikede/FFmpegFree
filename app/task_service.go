package app

import (
	"context"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/service/convert"
	"FFmpegFree/internal/store"
	"FFmpegFree/internal/task"
)

// TaskService 是任务中心的 Wails 绑定（契约第 4 节）。只做转发，逻辑在 internal/task。
//
// 任务管理器依赖数据库和事件通道，要等 OnStartup 之后才存在，所以这里用 getter 延迟取；
// 启动完成之前调用会返回 INTERNAL。
type TaskService struct {
	get  func() *task.Manager
	conv func() *convert.Service
	ctx  func() context.Context
}

// NewTaskService 创建 TaskService。get 每次调用时返回当前的任务管理器（未就绪返回 nil）；
// conv 返回转换服务（GetPreviewURL / OpenWithSystem 用它的 convert 登记表和系统打开，未就绪返回 nil，可为 nil）；
// rootCtx 返回应用根 ctx（可为 nil）。
func NewTaskService(get func() *task.Manager, conv func() *convert.Service, rootCtx func() context.Context) *TaskService {
	return &TaskService{get: get, conv: conv, ctx: rootCtx}
}

func (s *TaskService) rootCtx() context.Context {
	if s.ctx != nil {
		if c := s.ctx(); c != nil {
			return c
		}
	}
	return context.Background()
}

func (s *TaskService) files() (*convert.Service, error) {
	if s.conv != nil {
		if c := s.conv(); c != nil {
			return c, nil
		}
	}
	return nil, apperr.New(apperr.Internal, "任务管理器尚未初始化")
}

func (s *TaskService) mgr() (*task.Manager, error) {
	if s.get != nil {
		if m := s.get(); m != nil {
			return m, nil
		}
	}
	return nil, apperr.New(apperr.Internal, "任务管理器尚未初始化")
}

// ListActive 返回所有排队和运行中的任务（含实时进度），供前端任务 store 启动时拉取。
func (s *TaskService) ListActive() ([]store.Task, error) {
	m, err := s.mgr()
	if err != nil {
		return nil, err
	}
	return m.ListActive(), nil
}

// List 按类型、状态分页查询历史任务，按创建时间倒序。limit 默认 50，最大 200。
// 默认不返回在任务中心隐藏的任务（filter.includeHidden=true 时才返回，契约 v0.23）。
func (s *TaskService) List(filter store.TaskFilter) (store.TaskPage, error) {
	m, err := s.mgr()
	if err != nil {
		return store.TaskPage{}, err
	}
	return m.List(filter)
}

// Get 返回单个任务，不存在返回 NOT_FOUND。
func (s *TaskService) Get(id string) (store.Task, error) {
	m, err := s.mgr()
	if err != nil {
		return store.Task{}, err
	}
	return m.Get(id)
}

// Cancel 取消任务：排队中的立即变为 canceled；运行中的取消后异步收尾，终态通过 task:status 通知；
// 已结束的返回 TASK_CONFLICT。
func (s *TaskService) Cancel(id string) error {
	m, err := s.mgr()
	if err != nil {
		return err
	}
	return m.Cancel(id)
}

// Retry 原地重试（契约 v0.23，6.6）：复用任务 id 和原输出名，把记录重置回 queued 重新排队，发一条 retried 的 task:status。
// 允许 failed / interrupted / canceled；进行中或已成功返回 TASK_CONFLICT；没有重试工厂的类型 UNSUPPORTED。
func (s *TaskService) Retry(id string) (store.Task, error) {
	m, err := s.mgr()
	if err != nil {
		return store.Task{}, err
	}
	return m.Retry(id)
}

// Remove 删除已结束任务的记录和日志；deleteOutput 为 true 时同时删除成功任务的输出文件。
// ids 里有进行中的任务时整体失败（TASK_CONFLICT）；有 convert 任务时整体 INVALID_ARGUMENT（转换记录在格式转换页删除）。发 task:removed 事件。
func (s *TaskService) Remove(ids []string, deleteOutput bool) error {
	m, err := s.mgr()
	if err != nil {
		return err
	}
	return m.Remove(ids, deleteOutput)
}

// ClearFinished 已废弃（契约 v0.23）：等同 HideFinishedInTaskCenter，不再删除任何记录。
func (s *TaskService) ClearFinished() error {
	m, err := s.mgr()
	if err != nil {
		return err
	}
	return m.ClearFinished()
}

// GetLog 返回任务日志的最后 tailLines 行（<=0 表示全部）。
func (s *TaskService) GetLog(id string, tailLines int) (string, error) {
	m, err := s.mgr()
	if err != nil {
		return "", err
	}
	return m.GetLog(id, tailLines)
}

// HideFinishedInTaskCenter 任务中心“隐藏已结束”：所有类型、所有已结束且未隐藏的任务设为隐藏，返回本次隐藏的条数。
// 不删任何东西、version 不变、不发事件（任务中心随后自己重新 List）。
func (s *TaskService) HideFinishedInTaskCenter() (int64, error) {
	m, err := s.mgr()
	if err != nil {
		return 0, err
	}
	return m.HideFinishedInTaskCenter()
}

// UnhideInTaskCenter 取消隐藏：1~500 个 id，已隐藏的清成未隐藏、version +1、各发一条 task:status（hiddenInTaskCenter: false）。
// 幂等；任一 id 不存在或是旧类型 NOT_FOUND（reason=record），整体不改。
func (s *TaskService) UnhideInTaskCenter(ids []string) error {
	m, err := s.mgr()
	if err != nil {
		return err
	}
	return m.UnhideInTaskCenter(ids)
}

// CheckPaths 检查任务登记的输入 / 输出文件现在是否还在：1~500 个，结果一一对应，不存在的 id found=false。
func (s *TaskService) CheckPaths(taskIDs []string) ([]task.TaskPathCheck, error) {
	m, err := s.mgr()
	if err != nil {
		return nil, err
	}
	return m.CheckPaths(taskIDs)
}

// GetPreviewURL 返回任务输入（which="input"）或输出（which="output"，只有 succeeded）的 /local/<token> 预览地址。
// 错误：which 不合法 INVALID_ARGUMENT；任务不存在 NOT_FOUND（reason=record）；文件不在 NOT_FOUND（reason=file）；
// 扩展名不在预览白名单 UNSUPPORTED（reason=format）。
func (s *TaskService) GetPreviewURL(taskID string, which string) (convert.PreviewURL, error) {
	c, err := s.files()
	if err != nil {
		return convert.PreviewURL{}, err
	}
	return c.TaskPreviewURL(s.rootCtx(), taskID, which)
}

// OpenWithSystem 用系统默认程序打开任务的输入 / 输出文件（只允许音视频扩展名）。
// 错误：同 GetPreviewURL；没有能打开它的程序 NOT_FOUND（reason=no_app）；启动失败 PROCESS_FAILED。
func (s *TaskService) OpenWithSystem(taskID string, which string) error {
	c, err := s.files()
	if err != nil {
		return err
	}
	return c.TaskOpenWithSystem(s.rootCtx(), taskID, which)
}

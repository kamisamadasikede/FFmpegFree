package app

import (
	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/store"
	"FFmpegFree/internal/task"
)

// TaskService 是任务中心的 Wails 绑定（契约第 4 节）。只做转发，逻辑在 internal/task。
//
// 任务管理器依赖数据库和事件通道，要等 OnStartup 之后才存在，所以这里用 getter 延迟取；
// 启动完成之前调用会返回 INTERNAL。
type TaskService struct {
	get func() *task.Manager
}

// NewTaskService 创建 TaskService。get 每次调用时返回当前的任务管理器（未就绪返回 nil）。
func NewTaskService(get func() *task.Manager) *TaskService { return &TaskService{get: get} }

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

// Retry 用原任务的参数重新提交，生成新任务；原任务仍在进行时返回 TASK_CONFLICT。
func (s *TaskService) Retry(id string) (store.Task, error) {
	m, err := s.mgr()
	if err != nil {
		return store.Task{}, err
	}
	return m.Retry(id)
}

// Remove 删除已结束任务的记录和日志；deleteOutput 为 true 时同时删除成功任务的输出文件。
// ids 里有进行中的任务时整体失败（TASK_CONFLICT）。发 task:removed 事件。
func (s *TaskService) Remove(ids []string, deleteOutput bool) error {
	m, err := s.mgr()
	if err != nil {
		return err
	}
	return m.Remove(ids, deleteOutput)
}

// ClearFinished 清空所有已结束任务的记录和日志（不删输出文件）。
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

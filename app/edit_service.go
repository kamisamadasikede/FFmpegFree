package app

import (
	"context"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/service/edit"
	"FFmpegFree/internal/store"
)

// EditService 是多轨时间线剪辑的 Wails 绑定（契约 6.11）。只做转发，逻辑在 internal/service/edit。
// 底层服务依赖数据库、任务管理器、媒体服务，要等 OnStartup 之后才存在，启动完成之前调用返回 INTERNAL。
// 素材探测、缩略图、最近素材用 MediaService，选文件用 SystemService.PickFiles，不在这里。
type EditService struct {
	get func() *edit.Service
	ctx func() context.Context
}

// NewEditService 创建 EditService。get 每次调用时返回当前的服务（未就绪返回 nil）；rootCtx 返回应用根 ctx（可为 nil）。
func NewEditService(get func() *edit.Service, rootCtx func() context.Context) *EditService {
	return &EditService{get: get, ctx: rootCtx}
}

func (s *EditService) rootCtx() context.Context {
	if s.ctx != nil {
		if c := s.ctx(); c != nil {
			return c
		}
	}
	return context.Background()
}

func (s *EditService) svc() (*edit.Service, error) {
	if s.get != nil {
		if e := s.get(); e != nil {
			return e, nil
		}
	}
	return nil, apperr.New(apperr.Internal, "剪辑服务尚未初始化")
}

// ValidateProject 不落盘、不启动导出：探测素材并做全部校验，返回规范化后的时长与警告。
func (s *EditService) ValidateProject(project edit.EditProject) (edit.EditPlan, error) {
	e, err := s.svc()
	if err != nil {
		return edit.EditPlan{}, err
	}
	return e.ValidateProject(s.rootCtx(), project)
}

// Export 提交一个 edit_export 任务（batch 池排队，进度走 task:progress，取消走 TaskService.Cancel）。
// 先整体校验再提交，任何一项失败都不产生任务；错误 detail 第一行是 `clip=<id> path=<path>` 或 `project`。
func (s *EditService) Export(project edit.EditProject, opts edit.EditExportOptions) (store.Task, error) {
	e, err := s.svc()
	if err != nil {
		return store.Task{}, err
	}
	return e.Export(s.rootCtx(), project, opts)
}

// GetPreviewURL 登记素材并返回预览用的 /local/<token>。token 失效（404）时前端应重新调用。
func (s *EditService) GetPreviewURL(path string) (edit.PreviewURL, error) {
	e, err := s.svc()
	if err != nil {
		return edit.PreviewURL{}, err
	}
	return e.GetPreviewURL(path)
}

// SaveProject 保存工程（id 空 = 新建）。只校验数量 / 大小上限，不校验重叠和素材是否存在（草稿可保存）。
func (s *EditService) SaveProject(project edit.EditProject) (edit.EditProjectMeta, error) {
	e, err := s.svc()
	if err != nil {
		return edit.EditProjectMeta{}, err
	}
	return e.SaveProject(s.rootCtx(), project)
}

// LoadProject 读取工程；素材丢失不报错，路径放 missingPaths。
func (s *EditService) LoadProject(id string) (edit.LoadedProject, error) {
	e, err := s.svc()
	if err != nil {
		return edit.LoadedProject{}, err
	}
	return e.LoadProject(s.rootCtx(), id)
}

// ListProjects 按 updatedAt 倒序返回工程摘要，limit 默认 50，最大 200。
func (s *EditService) ListProjects(limit int) ([]edit.EditProjectMeta, error) {
	e, err := s.svc()
	if err != nil {
		return nil, err
	}
	return e.ListProjects(s.rootCtx(), limit)
}

// DeleteProject 删除工程（不删素材和导出文件），不存在返回 NOT_FOUND。
func (s *EditService) DeleteProject(id string) error {
	e, err := s.svc()
	if err != nil {
		return err
	}
	return e.DeleteProject(s.rootCtx(), id)
}

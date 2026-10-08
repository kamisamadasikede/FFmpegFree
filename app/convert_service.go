package app

import (
	"context"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/service/convert"
	"FFmpegFree/internal/store"
	"FFmpegFree/internal/task"
)

// ConvertService 是格式转换的 Wails 绑定（契约第 4 节）。只做转发，逻辑在 internal/service/convert。
// 底层服务依赖数据库、任务管理器和媒体服务，要等 OnStartup 之后才存在，启动完成之前调用返回 INTERNAL。
//
// 提交前的探测用应用的根 ctx（应用退出时取消，正在跑的 ffprobe 会被结束），被取消时返回 CANCELED。
type ConvertService struct {
	get func() *convert.Service
	ctx func() context.Context
}

// NewConvertService 创建 ConvertService。get 每次调用时返回当前的服务（未就绪返回 nil）；
// rootCtx 返回应用根 ctx（可为 nil，此时用 s.rootCtx()）。
func NewConvertService(get func() *convert.Service, rootCtx func() context.Context) *ConvertService {
	return &ConvertService{get: get, ctx: rootCtx}
}

func (s *ConvertService) rootCtx() context.Context {
	if s.ctx != nil {
		if c := s.ctx(); c != nil {
			return c
		}
	}
	return context.Background()
}

func (s *ConvertService) svc() (*convert.Service, error) {
	if s.get != nil {
		if c := s.get(); c != nil {
			return c, nil
		}
	}
	return nil, apperr.New(apperr.Internal, "转换服务尚未初始化")
}

// ListPresets 返回全部预设，内置预设（builtIn=true）在前，用户预设按创建顺序在后。
func (s *ConvertService) ListPresets() ([]convert.Preset, error) {
	c, err := s.svc()
	if err != nil {
		return nil, err
	}
	return c.ListPresets(s.rootCtx())
}

// SavePreset 保存用户预设：id 为空是新建，否则更新该用户预设；返回保存后的预设（含新 id）。
// 名称不能为空（最多 60 字），options 必须合法（INVALID_ARGUMENT，message 说明哪里不对），
// 不能修改内置预设，更新不存在的 id 返回 NOT_FOUND，用户预设最多 100 个。
func (s *ConvertService) SavePreset(p convert.Preset) (convert.Preset, error) {
	c, err := s.svc()
	if err != nil {
		return convert.Preset{}, err
	}
	return c.SavePreset(s.rootCtx(), p)
}

// DeletePreset 删除用户预设；内置预设 INVALID_ARGUMENT，不存在 NOT_FOUND。
func (s *ConvertService) DeletePreset(id string) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.DeletePreset(s.rootCtx(), id)
}

// Submit 为每个输入文件提交一个 convert 任务（进入 batch 池排队），返回的任务与 inputs 一一对应；
// 进度、状态、取消、重试走任务中心（task:* 事件、TaskService）。
//
// inputs 最多 50 个（更多的由前端分批）。提交前会校验所有输入（探测文件、检查参数与输入是否兼容），
// 任何一个不通过整体失败、不提交任何任务，错误 detail 指出是哪个文件。
// outputDir 为空时使用 Settings.defaultOutputDir，仍为空则输出到各自源文件所在的文件夹；
// 输出文件名为「源文件名.新扩展名」，重名自动追加 " (1)"、" (2)"（提交时定名并占位），不会覆盖已有文件。
// 兼容保留（契约 v0.23）：按路径自动找到 / 创建源文件行，每个任务都有 sourceId。
// 错误码：INVALID_ARGUMENT、NOT_FOUND、PROBE_FAILED、FFMPEG_NOT_FOUND、CANCELED（应用退出）；任务运行中的失败见任务的 error
// （CONVERT_DISK_FULL、IO_ERROR、PROBE_FAILED、PROCESS_FAILED）。
// options.targetSizeMb 暂缓：大于 0 返回 INVALID_ARGUMENT。
func (s *ConvertService) Submit(inputs []string, opts ffmpeg.ConvertOptions, outputDir string) ([]store.Task, error) {
	c, err := s.svc()
	if err != nil {
		return nil, err
	}
	return c.Submit(s.rootCtx(), inputs, opts, outputDir)
}

// ---------- 转换记录（契约 v0.23 / v0.23.1，6.14）：除 AddSources 外只收 id，不收路径 ----------

// AddSources 登记源文件（1~500 个，前端约 50 个一批）：同一文件已有行只更新 lastActivityAt（existed=true）。不探测。
// 单项错误放进该项 error：NOT_FOUND（reason=file）/ INVALID_ARGUMENT / IO_ERROR。
func (s *ConvertService) AddSources(paths []string) ([]convert.AddSourceResult, error) {
	c, err := s.svc()
	if err != nil {
		return nil, err
	}
	return c.AddSources(s.rootCtx(), paths)
}

// ListSources 按 lastActivityAt 倒序分页列出全部源文件行，每行内嵌最新的 recordLimit 条记录和记录总数。
func (s *ConvertService) ListSources(filter convert.ConvertSourceFilter) (convert.ConvertSourcePage, error) {
	c, err := s.svc()
	if err != nil {
		return convert.ConvertSourcePage{}, err
	}
	return c.ListSources(s.rootCtx(), filter)
}

// GetSource 返回一行，与 ListSources 的一项相同（最新 20 条记录 + 记录总数）；任务中心“在转换页查看”用。
// 不存在 NOT_FOUND（reason=record）。
func (s *ConvertService) GetSource(sourceID string) (convert.ConvertSourceEntry, error) {
	c, err := s.svc()
	if err != nil {
		return convert.ConvertSourceEntry{}, err
	}
	return c.GetSource(s.rootCtx(), sourceID)
}

// ListSourceRecords 返回某一行的更多记录（limit 默认 50 最大 200）。行不存在 NOT_FOUND（reason=record）。
func (s *ConvertService) ListSourceRecords(sourceID string, limit, offset int) (store.TaskPage, error) {
	c, err := s.svc()
	if err != nil {
		return store.TaskPage{}, err
	}
	return c.ListSourceRecords(s.rootCtx(), sourceID, limit, offset)
}

// SearchSources 按文件名搜索（源文件名或输出文件名包含关键字，不区分大小写）。
func (s *ConvertService) SearchSources(filter convert.ConvertSearchFilter) (convert.ConvertSourcePage, error) {
	c, err := s.svc()
	if err != nil {
		return convert.ConvertSourcePage{}, err
	}
	return c.SearchSources(s.rootCtx(), filter)
}

// CheckSources 检查源文件现在是否还在（1~500 个，结果一一对应）。
func (s *ConvertService) CheckSources(sourceIDs []string) ([]convert.SourcePathCheck, error) {
	c, err := s.svc()
	if err != nil {
		return nil, err
	}
	return c.CheckSources(s.rootCtx(), sourceIDs)
}

// PreviewOutputName 返回“将保存为”的完整输出路径（不占位，真正提交时可能不同）。
func (s *ConvertService) PreviewOutputName(sourceID string, opts ffmpeg.ConvertOptions, outputDir string) (string, error) {
	c, err := s.svc()
	if err != nil {
		return "", err
	}
	return c.PreviewOutputName(s.rootCtx(), sourceID, opts, outputDir)
}

// SubmitSources 按源文件行提交转换（规则同 Submit），写 sourceId 和 presetId / presetName / paramsSummary 快照，提交时定名并占位。
// v0.24：返回 {tasks, skipped}，副本没就绪的行跳过；一行都没就绪时 TASK_CONFLICT（reason=copying / copy_failed）。
func (s *ConvertService) SubmitSources(req convert.ConvertSubmitRequest) (convert.ConvertSubmitResult, error) {
	c, err := s.svc()
	if err != nil {
		return convert.ConvertSubmitResult{Tasks: []store.Task{}, Skipped: []convert.SkippedSource{}}, err
	}
	return c.SubmitSources(s.rootCtx(), req)
}

// Reconvert 在同一条已成功的记录上原地重新转换（契约 v0.24 / v0.24.1，6.17）：id、输出路径不变，成功后才原子替换旧输出；
// 失败 / 取消时记录恢复成 succeeded（失败带 lastReconvertError）。
func (s *ConvertService) Reconvert(req convert.ReconvertRequest) (store.Task, error) {
	c, err := s.svc()
	if err != nil {
		return store.Task{}, err
	}
	return c.Reconvert(s.rootCtx(), req)
}

// CancelCopy 取消这一行副本的复制（契约 6.15.6）：copying → canceled，行保留。
func (s *ConvertService) CancelCopy(sourceID string) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.CancelCopy(s.rootCtx(), sourceID)
}

// RetryCopy 重新复制这一行的副本（契约 6.15.6），返回更新后的行。
func (s *ConvertService) RetryCopy(sourceID string) (convert.ConvertSource, error) {
	c, err := s.svc()
	if err != nil {
		return convert.ConvertSource{}, err
	}
	return c.RetryCopy(s.rootCtx(), sourceID)
}

// GetFormatCatalog 返回格式目录（契约 6.16）：转换组件没就绪时照样返回，全部 encodable=false。
func (s *ConvertService) GetFormatCatalog() ([]convert.FormatEntry, error) {
	c, err := s.svc()
	if err != nil {
		return nil, err
	}
	return c.GetFormatCatalog(s.rootCtx())
}

// TakeInterruptedReconverts 返回本次启动时恢复的“上次退出时被中断的重转”条数，第一次调用后清零（契约 v0.24.1）。
// 签名按架构师定名：TakeInterruptedReconverts() (int)；服务没启动时返回 0。
func (s *ConvertService) TakeInterruptedReconverts() int {
	c, err := s.svc()
	if err != nil {
		return 0
	}
	return c.TakeInterruptedReconverts()
}

// DeleteRecords 删除转换记录：进行中的先取消（最多等 10 秒），deleteOutputs 时尽量删输出文件，没删成的放进 failures；不删源文件。
func (s *ConvertService) DeleteRecords(taskIDs []string, deleteOutputs bool) (task.DeleteResult, error) {
	c, err := s.svc()
	if err != nil {
		return task.NewDeleteResult(), err
	}
	return c.DeleteRecords(s.rootCtx(), taskIDs, deleteOutputs)
}

// DeleteSource 删除一行的全部转换记录，全部删掉后再删这一行；不删源文件。
func (s *ConvertService) DeleteSource(sourceID string, deleteOutputs bool) (task.DeleteResult, error) {
	c, err := s.svc()
	if err != nil {
		return task.NewDeleteResult(), err
	}
	return c.DeleteSource(s.rootCtx(), sourceID, deleteOutputs)
}

// GetSourcePreviewURL 返回源文件的 /local/<token> 预览地址（扩展名白名单见契约 6.14.7）。
func (s *ConvertService) GetSourcePreviewURL(sourceID string) (convert.PreviewURL, error) {
	c, err := s.svc()
	if err != nil {
		return convert.PreviewURL{}, err
	}
	return c.GetSourcePreviewURL(s.rootCtx(), sourceID)
}

// OpenSourceWithSystem 用系统默认程序打开源文件（规则同 TaskService.OpenWithSystem）。
func (s *ConvertService) OpenSourceWithSystem(sourceID string) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.OpenSourceWithSystem(s.rootCtx(), sourceID)
}

// RevealSource 在文件管理器里显示源文件。
func (s *ConvertService) RevealSource(sourceID string) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.RevealSource(s.rootCtx(), sourceID)
}

// RevealRecord 在文件管理器里显示转换记录的输出文件（契约 v0.23.1；转换页不再用 SystemService.RevealInFolder）。
// 记录不存在 NOT_FOUND（reason=record）；输出文件不在 NOT_FOUND（reason=file）；不是转换记录 INVALID_ARGUMENT。
func (s *ConvertService) RevealRecord(taskID string) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.RevealRecord(s.rootCtx(), taskID)
}

// GetRecordThumbnail 返回转换记录输出文件的缩略图（data:image/jpeg;base64,...），首次调用时生成并缓存。
func (s *ConvertService) GetRecordThumbnail(taskID string) (string, error) {
	c, err := s.svc()
	if err != nil {
		return "", err
	}
	return c.GetRecordThumbnail(s.rootCtx(), taskID)
}

// GetSourceThumbnail 返回源文件的缩略图（data:image/jpeg;base64,...），首次调用时生成并缓存。
func (s *ConvertService) GetSourceThumbnail(sourceID string) (string, error) {
	c, err := s.svc()
	if err != nil {
		return "", err
	}
	return c.GetSourceThumbnail(s.rootCtx(), sourceID)
}

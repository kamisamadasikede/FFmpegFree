package app

import (
	"context"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/service/convert"
	"FFmpegFree/internal/store"
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
// 输出文件名为「源文件名.新扩展名」，重名自动追加 (1)、(2)，不会覆盖已有文件。
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

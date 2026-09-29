package app

import (
	"context"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/service/live"
	"FFmpegFree/internal/store"
)

// LiveService 是直播推流的 Wails 绑定（契约第 4 节 LiveService、6.10）。只做转发，逻辑在 internal/service/live。
// 底层服务依赖任务管理器和媒体服务，要等 OnStartup 之后才存在，启动完成之前调用返回 INTERNAL。
//
// 推流会话就是 live 池里的任务，会话 id = 任务 id；停止用 TaskService.Cancel，没有 StopPush。
// Start* 用应用根 ctx，被取消返回 CANCELED。
type LiveService struct {
	get func() *live.Service
	ctx func() context.Context
}

// NewLiveService 创建 LiveService。get 每次调用时返回当前服务（未就绪返回 nil）；rootCtx 返回应用根 ctx（可为 nil）。
func NewLiveService(get func() *live.Service, rootCtx func() context.Context) *LiveService {
	return &LiveService{get: get, ctx: rootCtx}
}

func (s *LiveService) rootCtx() context.Context {
	if s.ctx != nil {
		if c := s.ctx(); c != nil {
			return c
		}
	}
	return context.Background()
}

func (s *LiveService) svc() (*live.Service, error) {
	if s.get != nil {
		if c := s.get(); c != nil {
			return c, nil
		}
	}
	return nil, apperr.New(apperr.Internal, "直播服务尚未初始化")
}

// StartFilePush 开始文件推流（可循环），立即返回入队前的任务快照（queued、version=1）；连接 / 鉴权失败体现在任务 failed + error。
// 同步错误：FFMPEG_NOT_FOUND、INVALID_ARGUMENT、NOT_FOUND / PROBE_FAILED、LIVE_URL_INVALID（detail 首行 reason=…）、
// UNSUPPORTED（缺 srt / rtmps，detail 写 missing=…）、TASK_CONFLICT（detail 首行 reason=max_sessions | duplicate_url）、CANCELED。
func (s *LiveService) StartFilePush(req live.FilePushRequest) (store.Task, error) {
	l, err := s.svc()
	if err != nil {
		return store.Task{}, err
	}
	return l.StartFilePush(s.rootCtx(), req)
}

// CheckPushURL 只校验地址并返回脱敏后的显示文本，不联网。不通过返回 LIVE_URL_INVALID（detail 首行 reason=…）。
func (s *LiveService) CheckPushURL(url string) (live.PushURLInfo, error) {
	l, err := s.svc()
	if err != nil {
		return live.PushURLInfo{}, err
	}
	return l.CheckPushURL(url)
}

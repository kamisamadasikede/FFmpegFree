package app

import (
	"context"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/service/media"
	"FFmpegFree/internal/store"
)

// MediaService 是媒体探测与缩略图的 Wails 绑定（契约第 4 节）。只做转发，逻辑在 internal/service/media。
//
// 底层服务依赖数据库，要等 OnStartup 之后才存在，所以用 getter 延迟取；启动完成之前调用返回 INTERNAL。
//
// 探测和缩略图用应用的根 ctx（应用退出时取消），退出时正在跑的 ffprobe / ffmpeg 会被结束，不会遗留子进程。
type MediaService struct {
	get func() *media.Service
	ctx func() context.Context
}

// NewMediaService 创建 MediaService。get 每次调用时返回当前的服务（未就绪返回 nil）；
// rootCtx 返回应用根 ctx（可为 nil，此时用 s.rootCtx()）。
func NewMediaService(get func() *media.Service, rootCtx func() context.Context) *MediaService {
	return &MediaService{get: get, ctx: rootCtx}
}

func (s *MediaService) rootCtx() context.Context {
	if s.ctx != nil {
		if c := s.ctx(); c != nil {
			return c
		}
	}
	return context.Background()
}

func (s *MediaService) svc() (*media.Service, error) {
	if s.get != nil {
		if m := s.get(); m != nil {
			return m, nil
		}
	}
	return nil, apperr.New(apperr.Internal, "媒体服务尚未初始化")
}

// Probe 批量探测本地文件（一次最多 500 个，建议前端每批不超过 50 个，界面能更快看到结果、也方便取消），返回值与入参一一对应。
// 单个文件失败时该项的 error 有值（NOT_FOUND / PROBE_FAILED / IO_ERROR），其他文件不受影响；
// 成功的写入最近媒体记录，并带默认缩略图（thumbUrl 是 data URL）。
// ffmpeg / ffprobe 缺失时整个调用返回 FFMPEG_NOT_FOUND。
func (s *MediaService) Probe(paths []string) ([]store.MediaInfo, error) {
	m, err := s.svc()
	if err != nil {
		return nil, err
	}
	return m.Probe(s.rootCtx(), paths)
}

// Thumbnail 返回 path 在 atSec 秒处的缩略图（jpg，最大宽度 width，width<=0 用 320），带磁盘缓存。
func (s *MediaService) Thumbnail(path string, atSec float64, width int) (media.Thumb, error) {
	m, err := s.svc()
	if err != nil {
		return media.Thumb{}, err
	}
	return m.Thumbnail(s.rootCtx(), path, atSec, width)
}

// ListRecent 返回最近探测过的媒体，按探测时间倒序。limit 默认 20，最大 200。
func (s *MediaService) ListRecent(limit int) ([]store.MediaInfo, error) {
	m, err := s.svc()
	if err != nil {
		return nil, err
	}
	return m.ListRecent(s.rootCtx(), limit)
}

// RemoveRecent 只删最近媒体记录，不删文件。一次最多 500 个 id，超过返回 INVALID_ARGUMENT。
func (s *MediaService) RemoveRecent(ids []string) error {
	m, err := s.svc()
	if err != nil {
		return err
	}
	return m.RemoveRecent(s.rootCtx(), ids)
}

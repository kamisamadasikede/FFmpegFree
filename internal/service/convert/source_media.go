package convert

import (
	"context"
	"os"
	"sync"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/store"
)

// ---------- 源文件行的媒体信息持久化（契约 v0.23.4，6.14.2 / 6.14.8；走查 G3） ----------

// SourceMediaStore 是持久化源文件行探测结果的能力（*store.Store 实现）；Sources 没实现它时不持久化（退回 media 表关联）。
type SourceMediaStore interface {
	SetConvertSourceMedia(ctx context.Context, id, fp string, m *store.MediaInfo) error
}

var _ SourceMediaStore = (*store.Store)(nil)

const (
	// sourceProbeTimeout 是懒探测单个文件的上限（比 MediaService 默认的 30 秒短：列表不能被一个坏文件拖太久）。
	sourceProbeTimeout = 15 * time.Second
	// sourceProbeConcurrency 是一次列表 / 添加里同时探测的文件数。
	sourceProbeConcurrency = 4
	// sourceDetectWait 是懒探测前最多等转换组件检测的时间（只在检测进行中才等，通常是应用刚启动的几秒）。
	sourceDetectWait = 15 * time.Second
)

// refreshSourceMedia 保证 src.Media 是登记文件当前版本的探测结果：
//   - 文件不在 / 不是普通文件 / 路径为空：不动（保留上次持久化的结果）；
//   - 指纹和持久化的一致：不探测；
//   - 否则探测并写回：成功写结果；PROBE_FAILED 写“探测失败”标记（文件不变就不再重探）；
//     转换组件没就绪、被取消、无权限等暂时性错误什么都不写，下次再试。
//
// 转换组件还在检测（应用启动时转换页是首页，ListSources 比首次检测先到）时先等检测有结果再探测：
// 不等的话这一次拿到 FFMPEG_NOT_FOUND 什么都不写，而前端一次会话只列一次，行就一直只有 media 表的退回结果
// （包 18 / 19 在 Windows 上 convert_sources.media 一直是空的就是这个原因）。
func (s *Service) refreshSourceMedia(ctx context.Context, ss SourceStore, src *ConvertSource) {
	if src.Kind == store.SourceKindDoc { // 文档页的行不探测媒体信息（v0.26）
		return
	}
	ms, ok := ss.(SourceMediaStore)
	p := displayPath(*src) // v0.24：副本就绪时探测副本（mtime / 大小和原文件一致，指纹相同），原文件拔掉了也能探
	if !ok || s.cfg.Media == nil || p == "" {
		return
	}
	fi, err := os.Stat(p)
	if err != nil || !fi.Mode().IsRegular() {
		return
	}
	fp := store.FileFingerprint(fi)
	if fp == src.MediaFP {
		return
	}
	if s.cfg.WaitFFmpeg != nil {
		wctx, wcancel := context.WithTimeout(ctx, sourceDetectWait)
		s.cfg.WaitFFmpeg(wctx)
		wcancel()
	}
	pctx, cancel := context.WithTimeout(ctx, sourceProbeTimeout)
	m, err := s.cfg.Media.Inspect(pctx, p)
	timedOut := pctx.Err() != nil // 必须在 cancel 之前看
	cancel()
	var save *store.MediaInfo
	switch {
	case err == nil:
		m.ID, m.ThumbURL, m.Error = "", "", nil
		m.FillCodecNames()
		save = &m
	case apperr.Is(err, apperr.ProbeFailed) && ctx.Err() == nil && !timedOut:
		save = nil // 文件本身解析不了：记下这个指纹，不再反复探测
	default:
		return
	}
	if err := ms.SetConvertSourceMedia(ctx, src.SourceID, fp, save); err != nil {
		return
	}
	src.Media, src.MediaFP = save, fp
}

// refreshSourcesMedia 并发刷新多行（最多 sourceProbeConcurrency 个同时探测）。
func (s *Service) refreshSourcesMedia(ctx context.Context, ss SourceStore, srcs []*ConvertSource) {
	if len(srcs) == 0 {
		return
	}
	sem := make(chan struct{}, sourceProbeConcurrency)
	var wg sync.WaitGroup
	for _, src := range srcs {
		wg.Add(1)
		sem <- struct{}{}
		go func(src *ConvertSource) {
			defer wg.Done()
			defer func() { <-sem }()
			s.refreshSourceMedia(ctx, ss, src)
		}(src)
	}
	wg.Wait()
}

// previewGate 在扩展名白名单之后按编码挡一层（playable.go）：m 有值且编码 WebView 解不了时 UNSUPPORTED（reason=format）。
func previewGate(m *store.MediaInfo) error {
	if unplayableCodec(m) != "" {
		return formatUnsupported("无法在应用内播放这种编码，请用系统播放器打开")
	}
	return nil
}

// inspectForPreview 探测任务的输入 / 输出文件，给预览门控用；转换组件没就绪、探测失败返回 nil（不挡，交给前端兜底）。
func (s *Service) inspectForPreview(ctx context.Context, p string) *store.MediaInfo {
	if s.cfg.Media == nil {
		return nil
	}
	pctx, cancel := context.WithTimeout(ctx, sourceProbeTimeout)
	defer cancel()
	m, err := s.cfg.Media.Inspect(pctx, p)
	if err != nil {
		return nil
	}
	return &m
}

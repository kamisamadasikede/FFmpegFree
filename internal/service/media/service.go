package media

import (
	"context"
	"errors"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/id"
	"FFmpegFree/internal/paths"
	"FFmpegFree/internal/proc"
	"FFmpegFree/internal/store"
)

const (
	defaultProbeTimeout = 30 * time.Second
	maxProbeBatch       = 500
	maxRemoveIDs        = 500
	probeConcurrency    = 4
	thumbConcurrency    = 2
	// 每生成这么多张新缩略图顺带清理一次缓存。
	cleanupEvery = 50
)

// Store 是 MediaService 用到的持久化能力，*store.Store 实现它。
type Store interface {
	UpsertMedia(ctx context.Context, pathKey string, m store.MediaInfo) (store.MediaInfo, error)
	ListRecentMedia(ctx context.Context, limit int) ([]store.MediaInfo, error)
	DeleteMedia(ctx context.Context, ids []string) error
}

// Config 是 Service 的依赖。测试可以替换 Require 和 Now。
type Config struct {
	Store     Store
	ThumbsDir string // <数据目录>/thumbs

	// Require 返回当前 ffmpeg / ffprobe，默认 ffmpeg.RequireProbe（未就绪返回 FFMPEG_NOT_FOUND）。
	Require func() (ffmpeg.Binaries, error)

	// OnRemoved 在 RemoveRecent 删除记录之后被调用，参数是被移除记录的路径。用来撤销这些文件的 /local/<token> 预览
	// （契约 6.13：被 RemoveRecent 撤销后一律 404）。可为 nil。
	OnRemoved func(paths []string)

	// OnProbed 在 Probe 成功探测并写入 media 表之后被调用（契约 v0.23.4）：用来刷新转换页同一文件的源文件行
	// （convert_sources.media）。key 是 path_key，fi 是探测前 stat 到的文件信息。可为 nil；失败只影响源文件行，不影响 Probe。
	OnProbed func(ctx context.Context, key string, m store.MediaInfo, fi os.FileInfo)

	ProbeTimeout  time.Duration
	ThumbTimeout  time.Duration
	CacheMaxFiles int
	CacheMaxBytes int64
	Now           func() time.Time
}

// Service 实现媒体探测和缩略图。它没有后台协程，可以随意创建多个。
type Service struct {
	cfg        Config
	cache      *thumbCache
	thumbSem   chan struct{}
	generated  atomic.Int64
	ensureOnce sync.Once
}

// New 创建 Service。
func New(cfg Config) *Service {
	if cfg.Require == nil {
		cfg.Require = ffmpeg.RequireProbe
	}
	if cfg.ProbeTimeout <= 0 {
		cfg.ProbeTimeout = defaultProbeTimeout
	}
	if cfg.ThumbTimeout <= 0 {
		cfg.ThumbTimeout = defaultThumbTimeout
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Service{
		cfg:      cfg,
		cache:    newThumbCache(cfg.ThumbsDir, cfg.CacheMaxFiles, cfg.CacheMaxBytes, cfg.Now),
		thumbSem: make(chan struct{}, thumbConcurrency),
	}
}

// CleanupCache 立即清理一次缩略图缓存（应用启动时调用一次）。
func (s *Service) CleanupCache() { s.cache.cleanup() }

// Probe 批量探测。返回值与入参一一对应：单个文件失败时该项的 Error 有值（NOT_FOUND / PROBE_FAILED / IO_ERROR …），
// 不影响其他文件，失败的文件不写入 media 表。只有整个调用无法进行（参数为空、超过 500 个、ffmpeg / ffprobe 缺失）才返回 error。
// 成功的项写入 media 表（同一文件再次探测保留原 id），并附带默认位置的缩略图（ThumbURL，失败时为空，不算错误）。
func (s *Service) Probe(ctx context.Context, in []string) ([]store.MediaInfo, error) {
	if len(in) == 0 {
		return nil, apperr.New(apperr.InvalidArgument, "没有要探测的文件")
	}
	if len(in) > maxProbeBatch {
		return nil, apperr.New(apperr.InvalidArgument, "一次最多探测 500 个文件")
	}
	bin, err := s.cfg.Require()
	if err != nil {
		return nil, err
	}
	out := make([]store.MediaInfo, len(in))
	sem := make(chan struct{}, probeConcurrency)
	var wg sync.WaitGroup
	for i, p := range in {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, p string) {
			defer wg.Done()
			defer func() { <-sem }()
			m, err := s.probeOne(ctx, bin, p)
			if err != nil {
				m = store.MediaInfo{Path: p, Name: filepath.Base(p), Error: apperr.From(err)}
			}
			out[i] = m
		}(i, p)
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, apperr.Wrap(apperr.Internal, "探测被取消", err)
	}
	return out, nil
}

// ProbeOne 探测单个文件，失败直接返回 error。
func (s *Service) ProbeOne(ctx context.Context, path string) (store.MediaInfo, error) {
	bin, err := s.cfg.Require()
	if err != nil {
		return store.MediaInfo{}, err
	}
	return s.probeOne(ctx, bin, path)
}

func (s *Service) probeOne(ctx context.Context, bin ffmpeg.Binaries, raw string) (store.MediaInfo, error) {
	m, key, fi, err := s.inspect(ctx, bin, raw)
	if err != nil {
		return store.MediaInfo{}, err
	}
	m.ID = id.New()
	m.ProbedAt = s.cfg.Now().UnixMilli()
	if s.cfg.Store != nil {
		saved, err := s.cfg.Store.UpsertMedia(ctx, key, m)
		if err != nil {
			return store.MediaInfo{}, apperr.Wrap(apperr.IOError, "保存媒体记录失败", err)
		}
		m.ID = saved.ID
	}
	if s.cfg.OnProbed != nil {
		s.cfg.OnProbed(ctx, key, m, fi)
	}
	if m.HasVideo {
		if t, err := s.thumbnail(ctx, bin, m.Path, key, fi, defaultThumbAt(m.Duration), DefaultThumbWidth); err == nil {
			m.ThumbURL = t.DataURL
		}
	}
	return m, nil
}

// inspect 只做探测：不写 media 表，不生成缩略图，ID 为空。转换等服务需要时长和流信息时用它（见 Inspect）。
func (s *Service) inspect(ctx context.Context, bin ffmpeg.Binaries, raw string) (store.MediaInfo, string, os.FileInfo, error) {
	p, key, fi, err := statMedia(raw)
	if err != nil {
		return store.MediaInfo{}, "", nil, err
	}
	data, err := runProbe(ctx, bin.FFprobe, p, s.cfg.ProbeTimeout)
	if err != nil {
		return store.MediaInfo{}, "", nil, err
	}
	m, err := ParseProbe(data, p)
	if err != nil {
		return store.MediaInfo{}, "", nil, err
	}
	m.Path, m.Name, m.Size = p, filepath.Base(p), fi.Size()
	return m, key, fi, nil
}

// Inspect 探测单个文件并返回结果，但不写入最近媒体记录、不生成缩略图（ID 为空）。
// 错误码与 Probe 一致；ffmpeg / ffprobe 缺失返回 FFMPEG_NOT_FOUND。
func (s *Service) Inspect(ctx context.Context, path string) (store.MediaInfo, error) {
	bin, err := s.cfg.Require()
	if err != nil {
		return store.MediaInfo{}, err
	}
	m, _, _, err := s.inspect(ctx, bin, path)
	return m, err
}

// statMedia 规范化路径并检查文件：不存在 NOT_FOUND，目录和非普通文件（FIFO / 设备）INVALID_ARGUMENT，打不开 IO_ERROR。
// 必须先 Stat 再 Open：Open 一个没有写端的 FIFO 会永远阻塞。
func statMedia(raw string) (p, key string, fi os.FileInfo, err error) {
	p, key, err = paths.Normalize(raw)
	if err != nil {
		return "", "", nil, apperr.Wrap(apperr.InvalidArgument, "路径不合法", err)
	}
	fi, err = os.Stat(p)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "", "", nil, apperr.New(apperr.NotFound, "文件不存在").WithDetail(p)
	case err != nil:
		return "", "", nil, apperr.Wrap(apperr.IOError, "无法读取文件信息", err)
	case fi.IsDir():
		return "", "", nil, apperr.New(apperr.InvalidArgument, "这是一个文件夹，不是媒体文件").WithDetail(p)
	case !fi.Mode().IsRegular():
		// FIFO、设备文件、socket：os.Open 会一直阻塞（FIFO 等写端），ffprobe 也会卡住，必须在打开之前拒绝。
		return "", "", nil, apperr.New(apperr.InvalidArgument, "不是普通文件（管道、设备等），不能作为媒体文件").WithDetail(p)
	}
	f, err := os.Open(p)
	if err != nil {
		// 走查 X8：说清楚是哪个文件、不用问号；detail 是系统错误（含完整路径），提交时 withInput 再在第一行加上路径。
		return "", "", nil, apperr.Wrap(apperr.IOError, "无法读取文件“"+filepath.Base(p)+"”，可能没有读取权限。", err)
	}
	f.Close()
	return p, key, fi, nil
}

// probeArgs 生成 ffprobe 命令行。输入带 file: 前缀，文件名以 - 开头、含冒号、空格或中日韩字符都安全。
func probeArgs(path string) []string {
	a := []string{"-v", "error", "-print_format", "json", "-show_format", "-show_streams"}
	a = append(a, ffmpegPatternArgs(path)...)
	return append(a, "-i", "file:"+path)
}

func runProbe(ctx context.Context, ffprobeExe, path string, timeout time.Duration) ([]byte, error) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := ffmpeg.NewCommand(cctx, ffprobeExe, probeArgs(path)...)
	// 限制输出大小：畸形文件可能让 ffprobe 输出海量流 / 标签，不能无限占内存。
	stdout, stderr := &headWriter{max: maxProbeStdoutBytes}, newTailWriter(maxStderrBytes)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err := proc.Run(cmd)
	if ctx.Err() != nil {
		return nil, apperr.Wrap(apperr.Internal, "探测被取消", ctx.Err())
	}
	if cctx.Err() == context.DeadlineExceeded {
		return nil, apperr.New(apperr.ProbeFailed, "探测超时").WithDetail(path)
	}
	if stdout.over {
		return nil, apperr.New(apperr.ProbeFailed, "文件的元数据太大，无法解析").WithDetail(path)
	}
	if err != nil {
		tail := lastLines(stderr.String(), 50)
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			return nil, apperr.Wrap(apperr.FFmpegNotFound, "无法运行转换组件，读取不了媒体信息", err)
		}
		return nil, apperr.New(apperr.ProbeFailed, "无法解析这个文件，可能已损坏或不是音视频文件").WithDetail(tail)
	}
	return stdout.buf.Bytes(), nil
}

// Thumbnail 返回 path 在 atSec 秒处的缩略图（jpg，最大宽度 width，不放大，保持比例并按旋转元数据转正）。
// 结果缓存在 <数据目录>/thumbs，命中缓存不启动 ffmpeg。atSec 超出视频长度时退回第 0 秒。
// 没有视频画面的文件返回 INVALID_ARGUMENT。width<=0 用 320，范围限制在 16~1920。
func (s *Service) Thumbnail(ctx context.Context, path string, atSec float64, width int) (Thumb, error) {
	if math.IsNaN(atSec) || math.IsInf(atSec, 0) || atSec < 0 {
		return Thumb{}, apperr.New(apperr.InvalidArgument, "atSec 必须是不小于 0 的数字")
	}
	if atSec > maxThumbAt {
		atSec = maxThumbAt // 再大也超出任何视频长度，会退回第 0 秒；封顶避免换算毫秒时 int64 溢出
	}
	bin, err := s.cfg.Require()
	if err != nil {
		return Thumb{}, err
	}
	p, key, fi, err := statMedia(path)
	if err != nil {
		return Thumb{}, err
	}
	return s.thumbnail(ctx, bin, p, key, fi, atSec, clampWidth(width))
}

func (s *Service) thumbnail(ctx context.Context, bin ffmpeg.Binaries, p, key string, fi os.FileInfo, atSec float64, width int) (Thumb, error) {
	if err := os.MkdirAll(s.cache.dir, 0o755); err != nil {
		return Thumb{}, apperr.Wrap(apperr.IOError, "创建缩略图目录失败", err)
	}
	name := cacheName(key, fi.ModTime(), fi.Size(), atSec, width)
	// hit 读取缓存命中的文件。缓存清理可能在 lookup 和读取之间删掉它：读不到（文件已不存在）按未命中处理，重新生成。
	hit := func(name string, at float64) (Thumb, bool, error) {
		path, ok := s.cache.lookup(name)
		if !ok {
			return Thumb{}, false, nil
		}
		u, err := dataURL(path)
		if errors.Is(err, fs.ErrNotExist) {
			return Thumb{}, false, nil
		}
		if err != nil {
			return Thumb{}, false, apperr.Wrap(apperr.IOError, "读取缩略图失败", err)
		}
		return Thumb{Path: path, DataURL: u, AtSec: at, Width: width}, true, nil
	}
	if th, ok, err := hit(name, atSec); err != nil || ok {
		return th, err
	}
	unlock := s.cache.lock(name)
	defer unlock()
	if th, ok, err := hit(name, atSec); err != nil || ok { // 等锁期间别人已经生成
		return th, err
	}
	select {
	case s.thumbSem <- struct{}{}:
		defer func() { <-s.thumbSem }()
	case <-ctx.Done():
		return Thumb{}, apperr.Wrap(apperr.Internal, "已取消", ctx.Err())
	}
	part := strings.TrimSuffix(s.cache.path(name), ".jpg") + ".part.jpg"
	usedAt, err := runThumb(ctx, bin.FFmpeg, p, part, atSec, width, s.cfg.ThumbTimeout)
	if err != nil {
		_ = os.Remove(part)
		return Thumb{}, apperr.From(err)
	}
	// 先把内容读进内存再改名：改名之后缓存清理随时可能删掉这个文件，不能再依赖它存在。
	u, err := dataURL(part)
	if err != nil {
		_ = os.Remove(part)
		return Thumb{}, apperr.Wrap(apperr.IOError, "读取缩略图失败", err)
	}
	// 退回第 0 秒时，缓存名和返回的 AtSec 都按 0 算（否则同一张图会以不同的 atSec 重复缓存，返回值也对不上实际画面）。
	if usedAt != atSec {
		name = cacheName(key, fi.ModTime(), fi.Size(), usedAt, width)
	}
	final := s.cache.path(name)
	if err := os.Rename(part, final); err != nil {
		_ = os.Remove(part)
		return Thumb{}, apperr.Wrap(apperr.IOError, "保存缩略图失败", err)
	}
	if s.generated.Add(1)%cleanupEvery == 0 {
		go s.cache.cleanup()
	}
	return Thumb{Path: final, DataURL: u, AtSec: usedAt, Width: width}, nil
}

// ListRecent 返回最近探测过的媒体（按探测时间倒序，limit 默认 20，最大 200）。
// 记录只包含 media 表里的列；文件已经不存在的记录照常返回（前端可提示"文件丢失"），
// ThumbURL 只在缩略图缓存命中时有值，不会为此启动 ffmpeg。
func (s *Service) ListRecent(ctx context.Context, limit int) ([]store.MediaInfo, error) {
	if s.cfg.Store == nil {
		return []store.MediaInfo{}, nil
	}
	items, err := s.cfg.Store.ListRecentMedia(ctx, limit)
	if err != nil {
		return nil, apperr.Wrap(apperr.IOError, "读取最近媒体失败", err)
	}
	for i := range items {
		it := &items[i]
		if !it.HasVideo {
			continue
		}
		_, key, err := paths.Normalize(it.Path)
		if err != nil {
			continue
		}
		fi, err := os.Stat(it.Path)
		if err != nil {
			continue
		}
		name := cacheName(key, fi.ModTime(), fi.Size(), defaultThumbAt(it.Duration), DefaultThumbWidth)
		if hit, ok := s.cache.lookup(name); ok {
			if u, err := dataURL(hit); err == nil {
				it.ThumbURL = u
			}
		}
	}
	return items, nil
}

// RemoveRecent 只删 media 记录，不删文件，也不删缩略图缓存（由容量上限回收）。
func (s *Service) RemoveRecent(ctx context.Context, ids []string) error {
	if len(ids) > maxRemoveIDs {
		return apperr.New(apperr.InvalidArgument, "一次最多删除 500 条记录")
	}
	if s.cfg.Store == nil {
		return nil
	}
	var removed []string
	if pl, ok := s.cfg.Store.(interface {
		MediaPaths(ctx context.Context, ids []string) ([]string, error)
	}); ok && s.cfg.OnRemoved != nil {
		removed, _ = pl.MediaPaths(ctx, ids) // 查不到路径只是少撤销几个预览，不影响删除
	}
	if err := s.cfg.Store.DeleteMedia(ctx, ids); err != nil {
		return apperr.Wrap(apperr.IOError, "删除媒体记录失败", err)
	}
	if len(removed) > 0 {
		s.cfg.OnRemoved(removed)
	}
	return nil
}

// DefaultThumbnailDataURL 返回 path 的默认缩略图（与 Probe 附带的相同：时长的 10%、最多 10 秒，宽 320），只返回 data URL
// （契约 v0.23，6.14.10：ConvertService.GetRecordThumbnail / GetSourceThumbnail 用，路径由调用方从表里取）。
// durationHint > 0 时直接用它算截图时间点；否则先探测一次（同时判断有没有画面）。
// 缓存键包含文件当前的 mtime 和 size（每次调用都先 stat），文件被替换后一定重新生成。
// 错误：文件不存在 / 不是普通文件 NOT_FOUND（reason=file）；没有画面 UNSUPPORTED（reason=format）；
// 其余沿用 Thumbnail（FFMPEG_NOT_FOUND、PROBE_FAILED、INTERNAL 等）。
func (s *Service) DefaultThumbnailDataURL(ctx context.Context, path string, durationHint float64) (string, error) {
	bin, err := s.cfg.Require()
	if err != nil {
		return "", err
	}
	p, key, fi, err := statMedia(path)
	if err != nil {
		if apperr.Is(err, apperr.NotFound) || apperr.Is(err, apperr.InvalidArgument) {
			return "", apperr.New(apperr.NotFound, "文件不存在").WithDetail("reason=file")
		}
		return "", err
	}
	dur := durationHint
	if dur <= 0 || math.IsNaN(dur) || math.IsInf(dur, 0) {
		m, _, _, err := s.inspect(ctx, bin, p)
		if err != nil {
			return "", err
		}
		if !m.HasVideo {
			return "", apperr.New(apperr.Unsupported, "这个文件没有画面").WithDetail("reason=format")
		}
		dur = m.Duration
	}
	th, err := s.thumbnail(ctx, bin, p, key, fi, defaultThumbAt(dur), DefaultThumbWidth)
	if err != nil {
		if apperr.Is(err, apperr.InvalidArgument) { // runThumbOnce："该文件没有视频画面"
			return "", apperr.New(apperr.Unsupported, "这个文件没有画面").WithDetail("reason=format")
		}
		return "", err
	}
	return th.DataURL, nil
}

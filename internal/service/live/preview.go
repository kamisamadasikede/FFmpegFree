package live

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/id"
	livepkg "FFmpegFree/internal/live"
	"FFmpegFree/internal/proc"
)

// 直播预览（契约 v0.14 / 6.10「预览画面」）：推流会话在主输出之外多一路 JPEG 预览输出，
// 拉流预览会话只读远端流、只输出预览。GetPreview 读取最新一帧。

const (
	// PreviewDirName 是数据目录 tmp 下的预览临时子目录名。
	PreviewDirName = "live-preview"
	// MaxPullPreviews 是同时进行的拉流预览会话上限（与推流会话上限分开计）。
	MaxPullPreviews = 4
	// maxPreviewBytes 是单帧预览的大小上限；640 宽的 JPEG 通常只有几十 KB，超过说明文件不对。
	maxPreviewBytes = 4 << 20
	pullProbeWait   = 12 * time.Second
)

// Preview 是 GetPreview 的返回。没有可用画面时 Data 为空、TS 为 0（不是错误）。
type Preview struct {
	// Data 是最新一帧 JPEG 的 base64（标准编码、无 data: 前缀），没有画面时为空串。
	Data string `json:"data"`
	// TS 是这一帧写入的时间（毫秒时间戳）；没有画面时为 0。前端可据此判断画面是否停滞。
	TS int64 `json:"ts"`
	// Active 表示会话还在进行（推流任务未结束 / 拉流预览会话未结束）。前端在 Active=false 时停止轮询。
	Active bool `json:"active"`
}

// PullPreviewRequest 是 StartPullPreview 的参数。
type PullPreviewRequest struct {
	URL string `json:"url"` // rtmp / rtmps / srt / http / https 地址
	// Preview 为 nil（缺省）或 true 时出预览；false 时会话不启动 ffmpeg（GetPreview 恒为空）。
	Preview *bool `json:"preview"`
}

// PullSession 是拉流预览会话。
type PullSession struct {
	ID       string `json:"id"`
	Redacted string `json:"redacted"` // 脱敏后的地址，可直接显示
	Preview  bool   `json:"preview"`  // 是否真的在出预览
}

type pullSession struct {
	key     string
	preview string // 预览 JPEG 路径；空 = 没有预览
	cancel  context.CancelFunc
	done    chan struct{}
}

// CleanupPreviewDir 清理并重建预览临时目录（应用启动时调用：上次异常退出遗留的预览文件）。
// 目录名必须是 live-preview，避免误删别的目录。
func CleanupPreviewDir(dir string) error {
	if dir == "" || filepath.Base(dir) != PreviewDirName {
		return fmt.Errorf("预览目录不合法: %q", dir)
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return os.MkdirAll(dir, 0o755)
}

// planPreview 决定这个会话要不要出预览：返回预览 JPEG 路径，"" 表示不加预览输出。
// 降级不报错：调用方关闭、没配预览目录、ffmpeg 不支持 mjpeg / image2 / fps / scale、目录不可写，都只是不加预览（记日志）。
func (s *Service) planPreview(ctx context.Context, bin ffmpeg.Binaries, sessionID string, want *bool) string {
	if want != nil && !*want {
		return ""
	}
	dir := s.cfg.PreviewDir
	if dir == "" {
		return ""
	}
	if !s.cfg.Preview.Supported(ctx, bin.FFmpeg) {
		s.logf("当前 ffmpeg 不支持预览输出，本次会话不加预览")
		return ""
	}
	if err := ensureWritableDir(dir); err != nil {
		s.logf("预览目录不可用，本次会话不加预览: %v", err)
		return ""
	}
	return filepath.Join(dir, sessionID+".jpg")
}

func ensureWritableDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".probe-*")
	if err != nil {
		return err
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}

// removePreviewFiles 删除预览文件和 ffmpeg 原子写入用的 .tmp 文件。
func removePreviewFiles(path string) {
	if path == "" {
		return
	}
	os.Remove(path)
	os.Remove(path + ".tmp")
}

// readPreviewFrame 读取并校验预览帧：必须以 JPEG SOI（FF D8）开头、以 EOI（FF D9）结尾。半帧或空文件返回 ok=false。
// ts 取文件修改时间（毫秒）。
func readPreviewFrame(path string) (data []byte, ts int64, ok bool) {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() || fi.Size() < 4 || fi.Size() > maxPreviewBytes {
		return nil, 0, false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, false
	}
	if !validJPEG(b) {
		return nil, 0, false
	}
	return b, fi.ModTime().UnixMilli(), true
}

// validJPEG 检查 SOI 开头、EOI 结尾（允许结尾有少量 0 填充字节）。
func validJPEG(b []byte) bool {
	if len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 {
		return false
	}
	end := len(b)
	for end > 2 && b[end-1] == 0 && len(b)-end < 16 {
		end--
	}
	return b[end-2] == 0xFF && b[end-1] == 0xD9
}

// GetPreview 返回会话（推流任务 id 或拉流预览会话 id）最新一帧预览。会话不存在、已结束、预览关闭、还没有画面、
// 读到半帧，都返回空 Preview（Data 为空），不是错误。
func (s *Service) GetPreview(sessionID string) (Preview, error) {
	path, active := s.previewPath(sessionID)
	out := Preview{Active: active}
	if path == "" {
		return out, nil
	}
	if b, ts, ok := readPreviewFrame(path); ok {
		out.Data = base64.StdEncoding.EncodeToString(b)
		out.TS = ts
	}
	return out, nil
}

func (s *Service) previewPath(sessionID string) (path string, active bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if x, ok := s.sessions[sessionID]; ok {
		return x.preview, true
	}
	if p, ok := s.pulls[sessionID]; ok {
		return p.preview, true
	}
	return "", false
}

// ---------- 拉流预览会话 ----------

// StartPullPreview 开始一个拉流预览会话：后端用 ffmpeg 读远端流，只输出预览 JPEG（不推流、不落盘存档）。
// 播放本身仍由前端播放器直接拉远端地址。同一个标准化地址已有会话时返回已有会话（幂等）。
// 纯音频的流没有预览：会话很快自然结束（GetPreview 的 Active 变为 false）。
func (s *Service) StartPullPreview(_ context.Context, req PullPreviewRequest) (PullSession, error) {
	bin, err := s.cfg.Require()
	if err != nil {
		return PullSession{}, err
	}
	u, err := livepkg.ParsePullURL(req.URL)
	if err != nil {
		return PullSession{}, urlError(err)
	}
	wantPreview := req.Preview == nil || *req.Preview
	sid := id.New()
	path := ""
	if wantPreview {
		if s.cfg.PreviewDir == "" || !s.cfg.Preview.Supported(context.Background(), bin.FFmpeg) {
			return PullSession{}, apperr.New(apperr.Unsupported, "当前 ffmpeg 不支持生成预览画面").WithDetail("missing=preview")
		}
		if err := ensureWritableDir(s.cfg.PreviewDir); err != nil {
			return PullSession{}, apperr.Wrap(apperr.IOError, "预览目录不可用", err)
		}
		path = filepath.Join(s.cfg.PreviewDir, sid+".jpg")
	}

	s.mu.Lock()
	for pid, p := range s.pulls {
		if p.key == u.Key {
			s.mu.Unlock()
			return PullSession{ID: pid, Redacted: u.Redacted, Preview: p.preview != ""}, nil
		}
	}
	if len(s.pulls) >= MaxPullPreviews {
		s.mu.Unlock()
		return PullSession{}, apperr.New(apperr.TaskConflict, "同时进行的拉流预览已达上限").
			WithDetail(fmt.Sprintf("reason=max_pull_previews\n最多同时预览 %d 路", MaxPullPreviews))
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &pullSession{key: u.Key, preview: path, cancel: cancel, done: make(chan struct{})}
	if s.pulls == nil {
		s.pulls = map[string]*pullSession{}
	}
	s.pulls[sid] = p
	s.mu.Unlock()

	if path == "" { // preview=false：不启动 ffmpeg，会话只是占位
		close(p.done)
		return PullSession{ID: sid, Redacted: u.Redacted, Preview: false}, nil
	}
	go s.runPull(ctx, sid, p, bin, u)
	return PullSession{ID: sid, Redacted: u.Redacted, Preview: true}, nil
}

func (s *Service) runPull(ctx context.Context, sid string, p *pullSession, bin ffmpeg.Binaries, u livepkg.PullURL) {
	defer close(p.done)
	defer s.endPull(sid, p)
	wl := ffmpeg.PullInputWhitelist(u.Scheme)
	hasVideo := true // 探测不出来（没有 ffprobe、连不上）就让 ffmpeg 自己试
	if bin.FFprobe != "" {
		if v, err := s.cfg.ProbeStreams(ctx, bin.FFprobe, u.FFmpeg, wl); err == nil {
			hasVideo = v
		}
	}
	args, ok := ffmpeg.BuildPullPreviewArgs(ffmpeg.PullPreviewPlan{URL: u.FFmpeg, InputWhitelist: wl, HasVideo: hasVideo, PreviewPath: p.preview})
	if !ok {
		return // 纯音频：没有预览
	}
	redact := livepkg.NewRedactor(u.FFmpeg)
	_, err := ffmpeg.Run(ctx, ffmpeg.RunOptions{
		Exe: bin.FFmpeg, Args: args, Redact: redact,
		OnStderr: func(line string) { s.logf("拉流预览 %s: %s", sid, line) },
	})
	if err != nil && ctx.Err() == nil {
		s.logf("拉流预览 %s 结束: %v", sid, redact(err.Error()))
	}
}

func (s *Service) endPull(sid string, p *pullSession) {
	s.mu.Lock()
	if cur, ok := s.pulls[sid]; ok && cur == p {
		delete(s.pulls, sid)
	}
	s.mu.Unlock()
	removePreviewFiles(p.preview)
}

// StopPullPreview 停止拉流预览会话并等待 ffmpeg 退出、清理预览文件。会话不存在（已结束）时什么也不做，返回 nil。
func (s *Service) StopPullPreview(sessionID string) error {
	s.mu.Lock()
	p, ok := s.pulls[sessionID]
	s.mu.Unlock()
	if !ok {
		return nil
	}
	p.cancel()
	select {
	case <-p.done:
	case <-time.After(6 * time.Second):
	}
	return nil
}

// Close 停止全部拉流预览会话（应用退出时调用）。推流会话由任务管理器的 Shutdown 负责。
func (s *Service) Close() {
	s.mu.Lock()
	var ps []*pullSession
	for _, p := range s.pulls {
		ps = append(ps, p)
	}
	s.mu.Unlock()
	for _, p := range ps {
		p.cancel()
	}
	for _, p := range ps {
		select {
		case <-p.done:
		case <-time.After(3 * time.Second):
		}
	}
}

// probeStreams 用 ffprobe 看远端流里有没有视频；连不上 / 超时返回错误（调用方按"未知"处理）。
func probeStreams(ctx context.Context, ffprobe, url, whitelist string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, pullProbeWait)
	defer cancel()
	cmd := ffmpeg.NewCommand(ctx, ffprobe, "-v", "error", "-protocol_whitelist", whitelist, "-rw_timeout", "8000000",
		"-show_entries", "stream=codec_type", "-of", "csv=p=0", url)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := proc.Run(cmd); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) || ctx.Err() != nil {
			return false, fmt.Errorf("探测失败: %v", err)
		}
		return false, err
	}
	kinds := strings.Fields(out.String())
	if len(kinds) == 0 {
		return false, errors.New("探测不到任何流")
	}
	for _, k := range kinds {
		if strings.HasPrefix(k, "video") {
			return true, nil
		}
	}
	return false, nil
}

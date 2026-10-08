package live

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
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

// 直播预览视频流（契约 v0.25 / 6.10.3）。v0.17 的 JPEG 预览已删除。

const (
	// PreviewDirName 是旧版预览临时目录名。v0.25 启动时删掉它，不再重建。
	PreviewDirName = "live-preview"
	// MaxPullPreviews 是同时进行的拉流预览会话上限。
	MaxPullPreviews = 4
	pullProbeWait   = 12 * time.Second
	previewMIME     = "video/x-flv"
)

// defaultDevPreview 由带 dev 标签的文件在 init 里打开。
var defaultDevPreview bool

// PreviewStream 是 GetPreviewStream 的返回。
type PreviewStream struct {
	URL      string `json:"url"`
	MIME     string `json:"mime"`
	HasVideo bool   `json:"hasVideo"`
	HasAudio bool   `json:"hasAudio"`
}

// PullPreviewRequest 是 StartPullPreview 的参数。Preview 保留，后端忽略。
type PullPreviewRequest struct {
	URL     string `json:"url"`
	Preview *bool  `json:"preview"`
}

// PullSession 是拉流预览会话。
type PullSession struct {
	ID         string `json:"id"`
	Redacted   string `json:"redacted"`
	Preview    bool   `json:"preview"`
	PreviewURL string `json:"previewUrl"`
}

// PullEvent 是 live:pull 的 payload。
type PullEvent struct {
	ID    string           `json:"id"`
	State string           `json:"state"`
	Error *apperr.AppError `json:"error,omitempty"`
}

// StreamProbe 是拉流地址里的编码名（空 = 没有这条流）。
type StreamProbe struct {
	Video string
	Audio string
	// Format 是 ffprobe 的 format_name（如 "hls"、"flv"、"mpegts"）；HLS 要匀速送出（见 flvPacer）。
	Format string
}

type pullSession struct {
	key      string
	feed     *previewFeed
	cancel   context.CancelFunc
	done     chan struct{}
	redacted string
}

// CleanupPreviewDir 删除旧版预览目录（v0.25 不再使用）。目录名必须是 live-preview。不存在不算错。
func CleanupPreviewDir(dir string) error {
	if dir == "" || filepath.Base(dir) != PreviewDirName {
		return fmt.Errorf("预览目录不合法: %q", dir)
	}
	return os.RemoveAll(dir)
}

func (s *Service) server() *previewHTTP {
	s.httpOnce.Do(func() {
		s.http = &previewHTTP{dev: s.cfg.DevPreview}
	})
	return s.http
}

func (s *Service) hasPreviewMux(ctx context.Context, bin ffmpeg.Binaries) bool {
	p, err := s.cfg.Protocols.OutputProtocols(ctx, bin.FFmpeg)
	if err != nil {
		s.logf("检查预览所需协议失败，本次不加预览: %v", err)
		return false
	}
	if !p["tee"] || !p["tcp"] {
		s.logf("当前转换组件不支持预览视频流（缺少 tee 或 tcp），本次不加预览")
		return false
	}
	return true
}

// openFeed 打开预览分支的监听。做不到（缺协议、端口绑不上）返回 nil，推流照常。
func (s *Service) openFeed(ctx context.Context, bin ffmpeg.Binaries, push, hasVideo, hasAudio bool) *previewFeed {
	if !s.hasPreviewMux(ctx, bin) {
		return nil
	}
	h := s.server()
	httpPort, err := h.start()
	if err != nil {
		s.logf("预览服务启动失败，本次不加预览: %v", err)
		return nil
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		s.logf("预览监听失败，本次不加预览: %v", err)
		return nil
	}
	tcpLn := ln.(*net.TCPListener)
	token, err := newPreviewToken()
	if err != nil {
		tcpLn.Close()
		s.logf("预览令牌生成失败，本次不加预览: %v", err)
		return nil
	}
	f := &previewFeed{
		token: token, port: ln.Addr().(*net.TCPAddr).Port,
		url:      fmt.Sprintf("http://127.0.0.1:%d/live/%s.flv", httpPort, token),
		hasVideo: hasVideo, hasAudio: hasAudio, push: push,
		hub: newFLVHub(), ln: tcpLn, done: make(chan struct{}),
	}
	h.register(f)
	go f.acceptIngest()
	return f
}

func (s *Service) dropFeed(f *previewFeed) {
	if f == nil {
		return
	}
	if s.http != nil {
		s.http.revoke(f.token)
	}
	f.stop()
}

func sessionNotFound() *apperr.AppError {
	return apperr.New(apperr.NotFound, "会话不存在或已结束").WithDetail("reason=session")
}

func previewUnavailable(push bool) *apperr.AppError {
	msg := "这路视频暂时无法在应用内播放。"
	if push {
		msg = "这路视频暂时无法预览，推流不受影响。"
	}
	return apperr.New(apperr.Unsupported, msg).WithDetail("reason=preview_unavailable")
}

func codecUnsupported(push bool, video, audio string) *apperr.AppError {
	msg := "这路视频无法在应用内播放。"
	if push {
		msg = "这路视频无法在应用内预览，推流不受影响。"
	}
	line := "audio=" + audio
	if video != "" {
		line = "video=" + video
	}
	return apperr.New(apperr.Unsupported, msg).WithDetail("reason=codec\n" + line)
}

// GetPreviewStream 返回会话的预览视频流地址。
func (s *Service) GetPreviewStream(sessionID string) (PreviewStream, error) {
	s.mu.Lock()
	sess, pushOK := s.sessions[sessionID]
	pull, pullOK := s.pulls[sessionID]
	s.mu.Unlock()
	var f *previewFeed
	switch {
	case pushOK:
		f = sess.feed
	case pullOK:
		f = pull.feed
	default:
		return PreviewStream{}, sessionNotFound()
	}
	if f == nil || f.state.Load() == 2 || f.state.Load() == 3 {
		return PreviewStream{}, previewUnavailable(pushOK)
	}
	return PreviewStream{URL: f.url, MIME: previewMIME, HasVideo: f.hasVideo, HasAudio: f.hasAudio}, nil
}

func (s *Service) emitPull(id, state string, err *apperr.AppError) {
	if s.cfg.Emit == nil || s.closing.Load() {
		return
	}
	s.cfg.Emit("live:pull", PullEvent{ID: id, State: state, Error: err})
}

// StartPullPreview 开始拉流预览：转封装成 FLV，经本机 HTTP 提供。请求里的 Preview 被忽略。
func (s *Service) StartPullPreview(_ context.Context, req PullPreviewRequest) (PullSession, error) {
	bin, err := s.cfg.Require()
	if err != nil {
		return PullSession{}, err
	}
	u, err := livepkg.ParsePullURL(req.URL)
	if err != nil {
		return PullSession{}, urlError(err)
	}
	s.mu.Lock()
	for pid, p := range s.pulls {
		if p.key == u.Key {
			s.mu.Unlock()
			return pullView(pid, p), nil
		}
	}
	if len(s.pulls) >= MaxPullPreviews {
		s.mu.Unlock()
		return PullSession{}, apperr.New(apperr.TaskConflict, "同时进行的拉流预览已达上限").
			WithDetail(fmt.Sprintf("reason=max_pull_previews\n最多同时预览 %d 路", MaxPullPreviews))
	}
	s.mu.Unlock()

	feed := s.openFeed(context.Background(), bin, false, true, true)
	sid := id.New()
	ctx, cancel := context.WithCancel(context.Background())
	p := &pullSession{key: u.Key, feed: feed, cancel: cancel, done: make(chan struct{}), redacted: u.Redacted}
	s.mu.Lock()
	for pid, cur := range s.pulls {
		if cur.key == u.Key {
			s.mu.Unlock()
			cancel()
			s.dropFeed(feed)
			return pullView(pid, cur), nil
		}
	}
	if s.pulls == nil {
		s.pulls = map[string]*pullSession{}
	}
	s.pulls[sid] = p
	s.mu.Unlock()
	go s.runPull(ctx, sid, p, bin, u)
	return pullView(sid, p), nil
}

func pullView(id string, p *pullSession) PullSession {
	out := PullSession{ID: id, Redacted: p.redacted, Preview: p.feed != nil}
	if p.feed != nil {
		out.PreviewURL = p.feed.url
	}
	return out
}

func (s *Service) runPull(ctx context.Context, sid string, p *pullSession, bin ffmpeg.Binaries, u livepkg.PullURL) {
	defer close(p.done)
	defer s.endPull(sid, p)
	if p.feed == nil {
		s.emitPull(sid, "failed", previewUnavailable(false))
		return
	}
	wl := ffmpeg.PullInputWhitelist(u.Scheme)
	hls := ffmpeg.LooksLikeHLS(u.FFmpeg, "")
	unknown := true
	video, audio := true, true
	if bin.FFprobe != "" {
		if pr, err := s.cfg.ProbeStreams(ctx, bin.FFprobe, u.FFmpeg, wl); err == nil {
			unknown = false
			sendV, sendA, bad := ffmpeg.PreviewPlayable(pr.Video, pr.Audio)
			if bad {
				s.emitPull(sid, "unsupported", codecUnsupported(false, pr.Video, pr.Audio))
				return
			}
			video, audio = sendV, sendA
			hls = hls || ffmpeg.LooksLikeHLS("", pr.Format)
			p.feed.hasVideo, p.feed.hasAudio = sendV, sendA
		}
	}
	p.feed.paced.Store(hls)
	args, ok := ffmpeg.BuildPullRemuxArgs(ffmpeg.PullRemuxPlan{
		URL: u.FFmpeg, InputWhitelist: wl, Port: p.feed.port, Unknown: unknown, Video: video, Audio: audio, HLS: hls,
	})
	if !ok {
		s.emitPull(sid, "failed", previewUnavailable(false))
		return
	}
	// playing：收到第一个 FLV 头时发一次（契约 6.10.3.7）。hub 关闭时 ready 也会关，用 hasHeader 区分。
	go func() {
		select {
		case <-p.feed.hub.ready:
			if p.feed.hub.hasHeader() && ctx.Err() == nil {
				s.emitPull(sid, "playing", nil)
			}
		case <-ctx.Done():
		}
	}()
	redact := livepkg.NewRedactor(u.FFmpeg)
	s.logf("拉流预览 %s ffmpeg 参数: %s", sid, redact(strings.ReplaceAll(strings.Join(args, " "), u.FFmpeg, "<拉流地址>")))
	res, runErr := ffmpeg.Run(ctx, ffmpeg.RunOptions{
		Exe: bin.FFmpeg, Args: args, Redact: redact,
		OnStderr: func(line string) { s.logf("拉流预览 %s: %s", sid, line) },
	})
	if s.closing.Load() {
		return
	}
	switch {
	case ctx.Err() != nil:
		s.emitPull(sid, "ended", nil)
	case runErr == nil:
		s.emitPull(sid, "ended", nil)
	case p.feed.state.Load() == 1 || p.feed.hub.hasHeader():
		s.emitPull(sid, "interrupted", nil)
	default:
		// 拉流有自己的分类和文字（ClassifyPullError），不能用推流的“推流启动失败”。分类看 stderr 尾部（已脱敏）。
		tail := res.StderrTail
		if tail == "" {
			tail = apperr.From(runErr).Detail
		}
		s.emitPull(sid, "failed", ffmpeg.ClassifyPullError(ffmpeg.LiveClassifyInput{Tail: tail, Scheme: u.Scheme}))
	}
}

func (s *Service) endPull(sid string, p *pullSession) {
	s.mu.Lock()
	if cur, ok := s.pulls[sid]; ok && cur == p {
		delete(s.pulls, sid)
	}
	s.mu.Unlock()
	s.dropFeed(p.feed)
}

// StopPullPreview 停止拉流预览。会话不存在时什么也不做。
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

// Close 停止全部拉流预览并关掉预览 HTTP 服务。应用退出时不发 live:pull。
func (s *Service) Close() {
	s.closing.Store(true)
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
	if s.http != nil {
		s.http.close()
	}
}

func probeStreams(ctx context.Context, ffprobe, url, whitelist string) (StreamProbe, error) {
	ctx, cancel := context.WithTimeout(ctx, pullProbeWait)
	defer cancel()
	cmd := ffmpeg.NewCommand(ctx, ffprobe, "-v", "error", "-protocol_whitelist", whitelist, "-rw_timeout", "8000000",
		"-analyzeduration", "5000000", "-probesize", "5000000",
		"-show_entries", "stream=codec_type,codec_name:format=format_name", "-of", "json", url)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := proc.Run(cmd); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) || ctx.Err() != nil {
			return StreamProbe{}, fmt.Errorf("探测失败: %v", err)
		}
		return StreamProbe{}, err
	}
	return parseStreamProbe(out.Bytes())
}

// parseStreamProbe 解析 ffprobe -of json 的输出（streams 的 codec_type / codec_name，format 的 format_name）。
func parseStreamProbe(b []byte) (StreamProbe, error) {
	var v struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
			CodecName string `json:"codec_name"`
		} `json:"streams"`
		Format struct {
			FormatName string `json:"format_name"`
		} `json:"format"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return StreamProbe{}, fmt.Errorf("探测结果解析失败: %v", err)
	}
	if len(v.Streams) == 0 {
		return StreamProbe{}, errors.New("探测不到任何流")
	}
	pr := StreamProbe{Format: v.Format.FormatName}
	for _, st := range v.Streams {
		switch st.CodecType {
		case "video":
			if pr.Video == "" {
				pr.Video = st.CodecName
			}
		case "audio":
			if pr.Audio == "" {
				pr.Audio = st.CodecName
			}
		}
	}
	return pr, nil
}

package live

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"strconv"
	"strings"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/id"
	"FFmpegFree/internal/proc"
	"FFmpegFree/internal/task"
)

const defaultScreenFps = 30

// ScreenPushRequest 是 StartScreenPush 的参数。
type ScreenPushRequest struct {
	URL        string      `json:"url"`
	ScreenID   string      `json:"screenId"`
	HideCursor bool        `json:"hideCursor"`
	Audio      string      `json:"audio"` // none（默认）| silent
	ArchiveDir string      `json:"archiveDir"`
	Options    PushOptions `json:"options"`
	// Preview 保留给旧前端，v0.25 起后端忽略。
	Preview *bool `json:"preview"`
	// CaptureSourceID 可选：ListCaptureSources 返回的 id（screen:<序号> | window:<hwnd 十进制>）。不传 = 沿用 ScreenID（原行为）；
	// 传了以它为准（同时给了 ScreenID 时忽略 ScreenID）。
	CaptureSourceID string `json:"captureSourceId"`
}

// CaptureCapabilities 是 GetCaptureCapabilities 的返回。
type CaptureCapabilities struct {
	Supported    bool   `json:"supported"`
	Platform     string `json:"platform"`     // windows | darwin | linux
	Backend      string `json:"backend"`      // gdigrab | avfoundation | x11grab；不支持时 ""
	SessionType  string `json:"sessionType"`  // linux：x11 | wayland | unknown；其他平台 ""
	Permission   string `json:"permission"`   // granted | denied | unknown | notRequired
	AudioCapture bool   `json:"audioCapture"` // v1 恒为 false
	Reason       string `json:"reason"`
}

// ScreenInfo 是 ListScreens 的一项。
type ScreenInfo struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Primary bool    `json:"primary"`
	X       int     `json:"x"`
	Y       int     `json:"y"`
	Width   int     `json:"width"`
	Height  int     `json:"height"`
	Scale   float64 `json:"scale"`
}

// GetCaptureCapabilities 检测屏幕采集能不能用。Linux 读 XDG_SESSION_TYPE 和 DISPLAY，Wayland（即使有 XWayland）或没有 DISPLAY
// 都不支持（不去录黑屏）；macOS 的屏幕录制授权查不出来，permission=unknown，开始时按 ffmpeg 报错分类；Windows 无需授权。
func (s *Service) GetCaptureCapabilities() (CaptureCapabilities, error) {
	if _, err := s.cfg.Require(); err != nil {
		return CaptureCapabilities{}, err
	}
	return s.capabilities(), nil
}

func (s *Service) capabilities() CaptureCapabilities {
	c := CaptureCapabilities{Platform: s.cfg.GOOS}
	switch s.cfg.GOOS {
	case "windows":
		c.Supported, c.Backend, c.Permission = true, "gdigrab", "notRequired"
	case "darwin":
		c.Supported, c.Backend, c.Permission = true, "avfoundation", "unknown"
	case "linux":
		c.Permission = "notRequired"
		switch strings.ToLower(s.cfg.Getenv("XDG_SESSION_TYPE")) {
		case "x11":
			c.SessionType = "x11"
		case "wayland":
			c.SessionType = "wayland"
		default:
			c.SessionType = "unknown"
		}
		switch {
		case c.SessionType == "wayland":
			c.Reason = "当前是 Wayland 会话，暂不支持屏幕采集，请切换到 X11 会话后再试"
		case s.cfg.Getenv("DISPLAY") == "":
			c.Reason = "没有可用的图形显示（DISPLAY 为空），无法采集屏幕"
		default:
			c.Supported, c.Backend = true, "x11grab"
		}
	default:
		c.Platform = s.cfg.GOOS
		c.Reason = "当前系统不支持屏幕采集"
	}
	return c
}

func (s *Service) unsupportedErr(c CaptureCapabilities) error {
	return apperr.New(apperr.UnsupportedPlatform, c.Reason)
}

// ListScreens 返回可采集的显示器。不支持采集的平台 / 会话返回 UNSUPPORTED_PLATFORM。
func (s *Service) ListScreens(ctx context.Context) ([]ScreenInfo, error) {
	bin, err := s.cfg.Require()
	if err != nil {
		return nil, err
	}
	c := s.capabilities()
	if !c.Supported {
		return nil, s.unsupportedErr(c)
	}
	switch s.cfg.GOOS {
	case "windows":
		return s.cfg.Monitors()
	case "darwin":
		return s.listDarwinScreens(ctx, bin)
	}
	return s.listX11Screens(ctx)
}

var xrandrLine = regexp.MustCompile(`^(\S+) connected( primary)? (\d+)x(\d+)\+(-?\d+)\+(-?\d+)`)

// parseXrandr 解析 xrandr --query 的已连接且有分辨率的输出。
func parseXrandr(out string) []ScreenInfo {
	var res []ScreenInfo
	for _, line := range strings.Split(out, "\n") {
		m := xrandrLine.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		w, _ := strconv.Atoi(m[3])
		h, _ := strconv.Atoi(m[4])
		x, _ := strconv.Atoi(m[5])
		y, _ := strconv.Atoi(m[6])
		res = append(res, ScreenInfo{ID: "x11:" + m[1], Primary: m[2] != "", X: x, Y: y, Width: w, Height: h, Scale: 1})
	}
	primary := false
	for _, r := range res {
		primary = primary || r.Primary
	}
	if !primary && len(res) > 0 {
		res[0].Primary = true // 没有标 primary 时第一个当主显示器
	}
	for i := range res {
		res[i].Name = fmt.Sprintf("屏幕 %d", i+1)
		if res[i].Primary {
			res[i].Name += "（主显示器）"
		}
	}
	return res
}

func (s *Service) listX11Screens(ctx context.Context) ([]ScreenInfo, error) {
	out, err := s.cfg.Run(ctx, "xrandr", "--display", s.cfg.Getenv("DISPLAY"), "--query")
	if err == nil {
		if r := parseXrandr(out); len(r) > 0 {
			return r, nil
		}
	}
	// 没有 xrandr（或解析不出）：只返回整个桌面。
	return []ScreenInfo{{ID: "x11:desktop", Name: "整个桌面", Primary: true, Scale: 1}}, nil
}

var avfScreen = regexp.MustCompile(`\[(\d+)\]\s+Capture screen (\d+)`)

// parseAVFoundationScreens 解析 `ffmpeg -f avfoundation -list_devices true -i ""` 输出里的 Capture screen N。
func parseAVFoundationScreens(out string) []ScreenInfo {
	var res []ScreenInfo
	for _, line := range strings.Split(out, "\n") {
		if m := avfScreen.FindStringSubmatch(line); m != nil {
			n, _ := strconv.Atoi(m[2])
			si := ScreenInfo{ID: "avf:" + m[1], Name: fmt.Sprintf("屏幕 %d", n+1), Primary: n == 0, Scale: 1}
			if si.Primary {
				si.Name += "（主显示器）"
			}
			res = append(res, si)
		}
	}
	return res
}

func (s *Service) listDarwinScreens(ctx context.Context, bin ffmpeg.Binaries) ([]ScreenInfo, error) {
	// 这个命令把设备列表打在 stderr 并以非零码退出，所以不能用只返回 stdout 的 Runner。
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := ffmpeg.NewCommand(ctx, bin.FFmpeg, "-hide_banner", "-f", "avfoundation", "-list_devices", "true", "-i", "")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	_ = proc.Run(cmd)
	r := parseAVFoundationScreens(stderr.String())
	if len(r) == 0 {
		return nil, apperr.New(apperr.UnsupportedPlatform, "没有找到可采集的屏幕，请确认已在系统设置里授予屏幕录制权限")
	}
	return r, nil
}

// resolveScreen 按 ScreenID 找到要采集的屏幕；"" = 主显示器；不存在 INVALID_ARGUMENT。
func resolveScreen(list []ScreenInfo, screenID string) (ScreenInfo, error) {
	if len(list) == 0 {
		return ScreenInfo{}, apperr.New(apperr.UnsupportedPlatform, "没有可采集的显示器")
	}
	if screenID == "" {
		for _, sc := range list {
			if sc.Primary {
				return sc, nil
			}
		}
		return list[0], nil
	}
	for _, sc := range list {
		if sc.ID == screenID {
			return sc, nil
		}
	}
	return ScreenInfo{}, invalidArg("屏幕不存在，请重新选择显示器")
}

type screenPushParams struct {
	Kind       string      `json:"kind"`
	ScreenID   string      `json:"screenId"`
	URL        string      `json:"url"`
	HideCursor bool        `json:"hideCursor"`
	Audio      string      `json:"audio"`
	ArchiveDir string      `json:"archiveDir"`
	Options    PushOptions `json:"options"`
	// CaptureSourceID 只在传了时写入（旧任务的 params 不变）。
	CaptureSourceID string `json:"captureSourceId,omitempty"`
}

// StartScreenPush 开始屏幕推流。立即返回入队前的任务快照。
func (s *Service) StartScreenPush(ctx context.Context, req ScreenPushRequest) (task.Task, error) {
	t, err := s.startScreenPush(ctx, req)
	if err != nil && ctx.Err() != nil && !apperr.Is(err, apperr.Canceled) {
		return task.Task{}, apperr.Wrap(apperr.Canceled, "操作已取消", ctx.Err())
	}
	return t, err
}

func (s *Service) startScreenPush(ctx context.Context, req ScreenPushRequest) (task.Task, error) {
	if s.cfg.Tasks == nil {
		return task.Task{}, apperr.New(apperr.Internal, "直播服务尚未初始化")
	}
	bin, err := s.cfg.Require()
	if err != nil {
		return task.Task{}, err
	}
	u, err := parseURL(req.URL)
	if err != nil {
		return task.Task{}, err
	}
	if err := req.Options.validate(); err != nil {
		return task.Task{}, err
	}
	switch req.Audio {
	case "", "none", "silent":
	default:
		return task.Task{}, invalidArg("audio 只能是 none 或 silent")
	}
	// "" = 不存档。v0.24.3：打开存档且用户没有另选文件夹时，调用方传入实际输出目录
	// （defaultOutputDir 为空即 <base>/output，含 6.15.1 的可写性回退），这里不把空目录当成存档位置。
	archiveDir := ""
	if req.ArchiveDir != "" {
		if archiveDir, err = checkArchiveDir(req.ArchiveDir); err != nil {
			return task.Task{}, err
		}
	}
	c := s.capabilities()
	if !c.Supported {
		return task.Task{}, s.unsupportedErr(c)
	}
	if err := s.checkProtocols(ctx, bin, u.Scheme, archiveDir != ""); err != nil {
		return task.Task{}, err
	}
	var sc ScreenInfo
	var windowTitle string
	name := ""
	if req.CaptureSourceID != "" {
		// 开始时再校验一次来源还在：窗口已关闭 / 最小化、屏幕序号不存在 → LIVE_SOURCE_GONE
		src, err := s.resolveCaptureSource(ctx, req.CaptureSourceID)
		if err != nil {
			return task.Task{}, err
		}
		sc, windowTitle, name = src.Screen, src.WindowTitle, src.Name
	} else {
		screens, err := s.ListScreens(ctx)
		if err != nil {
			return task.Task{}, err
		}
		if sc, err = resolveScreen(screens, req.ScreenID); err != nil {
			return task.Task{}, err
		}
		name = sc.Name
	}
	fps := req.Options.Fps
	if fps == 0 {
		fps = defaultScreenFps
	}
	taskID := id.New()
	feed := s.openFeed(ctx, bin, true, true, req.Audio == "silent")
	plan := ffmpeg.ScreenPushPlan{
		PreviewPort: feed.Port(),
		GOOS:        s.cfg.GOOS, Display: s.cfg.Getenv("DISPLAY"), HideCursor: req.HideCursor, Silent: req.Audio == "silent",
		Scheme: u.Scheme, URL: u.FFmpeg, FPS: fps,
		Region: ffmpeg.ScreenRegion{X: sc.X, Y: sc.Y, Width: sc.Width, Height: sc.Height, Desktop: sc.ID == "x11:desktop", WindowTitle: windowTitle},
	}
	baseEnc := ffmpeg.LiveEncode{
		Width: req.Options.Width, Height: req.Options.Height, GOPFps: fps,
		VideoKbps: req.Options.videoKbps(), AudioKbps: req.Options.audioKbps(),
	}
	if strings.HasPrefix(sc.ID, "avf:") {
		plan.Region.DeviceIndex, _ = strconv.Atoi(strings.TrimPrefix(sc.ID, "avf:"))
	}
	archive := archiveDir != ""
	if err := s.reserve(taskID, u.Key, archive, true, feed); err != nil {
		s.dropFeed(feed)
		return task.Task{}, err
	}
	// 存档：先占会话再建占位文件（冲突时不留下空文件）。占位文件是本任务自己用 O_EXCL 创建的，之后只有它可能被删。
	var archivePath string
	if archive {
		if archivePath, err = s.reserveArchive(archiveDir); err != nil {
			s.release(taskID)
			return task.Task{}, err
		}
		tee, terr := ffmpeg.TeePath(s.cfg.GOOS, archivePath)
		if terr != nil {
			s.release(taskID)
			s.removePlaceholder(archivePath)
			return task.Task{}, invalidArg("存档路径不合法")
		}
		plan.ArchiveTee = tee
	}
	args, encoding := s.resolveLiveEncoder(ctx, baseEnc, func(e ffmpeg.LiveEncode) []string {
		p := plan
		p.Enc = e
		return ffmpeg.BuildScreenPushArgs(p)
	})
	pj, _ := json.Marshal(screenPushParams{Kind: "screen", ScreenID: req.ScreenID, URL: u.Redacted, HideCursor: req.HideCursor,
		Audio: firstNonEmpty(req.Audio, "none"), ArchiveDir: archiveDir, Options: req.Options, CaptureSourceID: req.CaptureSourceID})
	spec := task.Spec{
		ID:         taskID,
		Type:       task.TypeLiveScreenPush,
		Title:      "屏幕推流：" + name + " → " + u.Redacted,
		InputPaths: []string{},
		OutputPath: archivePath,
		Params:     string(pj),
	}
	var r task.Runner
	base := s.newRunner(taskID, bin, u, req.URL, args, encoding, true, archive, feed)
	if archive {
		r = &archiveRunner{runner: base, g: &archiveGuard{s: s, ffprobe: bin.FFprobe, path: archivePath}}
	} else {
		r = base
	}
	t, err := s.cfg.Tasks.Submit(spec, r)
	if err != nil {
		s.release(taskID)
		if archive {
			s.removePlaceholder(archivePath)
		}
		return task.Task{}, err
	}
	return t, nil
}

// removePlaceholder 删除本任务刚创建、还没交给 ffmpeg 的空占位文件（启动失败的回滚）。
func (s *Service) removePlaceholder(path string) {
	if err := s.cfg.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		s.logf("删除存档占位文件失败: %s: %v", path, err)
	}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

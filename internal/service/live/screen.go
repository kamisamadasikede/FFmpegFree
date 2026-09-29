package live

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
		return listWindowsMonitors()
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
		res[i].Name = fmt.Sprintf("显示器 %d", i+1)
		if res[i].Primary {
			res[i].Name += "（主）"
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
			si := ScreenInfo{ID: "avf:" + m[1], Name: fmt.Sprintf("显示器 %d", n+1), Primary: n == 0, Scale: 1}
			if si.Primary {
				si.Name += "（主）"
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
	if req.ArchiveDir != "" {
		// 存档（tee + 分片 mp4）在 #22（文件名净化函数）合入 v2 之后实现。
		return task.Task{}, apperr.New(apperr.Unsupported, "屏幕推流的本地存档暂未实现")
	}
	c := s.capabilities()
	if !c.Supported {
		return task.Task{}, s.unsupportedErr(c)
	}
	if err := s.checkProtocols(ctx, bin, u.Scheme, false); err != nil {
		return task.Task{}, err
	}
	screens, err := s.ListScreens(ctx)
	if err != nil {
		return task.Task{}, err
	}
	sc, err := resolveScreen(screens, req.ScreenID)
	if err != nil {
		return task.Task{}, err
	}
	fps := req.Options.Fps
	if fps == 0 {
		fps = defaultScreenFps
	}
	plan := ffmpeg.ScreenPushPlan{
		GOOS: s.cfg.GOOS, Display: s.cfg.Getenv("DISPLAY"), HideCursor: req.HideCursor, Silent: req.Audio == "silent",
		Scheme: u.Scheme, URL: u.FFmpeg, FPS: fps,
		Region: ffmpeg.ScreenRegion{X: sc.X, Y: sc.Y, Width: sc.Width, Height: sc.Height, Desktop: sc.ID == "x11:desktop"},
		Enc: ffmpeg.LiveEncode{
			Width: req.Options.Width, Height: req.Options.Height, GOPFps: fps,
			VideoKbps: req.Options.videoKbps(), AudioKbps: req.Options.audioKbps(),
		},
	}
	if strings.HasPrefix(sc.ID, "avf:") {
		plan.Region.DeviceIndex, _ = strconv.Atoi(strings.TrimPrefix(sc.ID, "avf:"))
	}
	args := ffmpeg.BuildScreenPushArgs(plan)
	taskID := id.New()
	if err := s.reserve(taskID, u.Key, false, true); err != nil {
		return task.Task{}, err
	}
	pj, _ := json.Marshal(screenPushParams{Kind: "screen", ScreenID: sc.ID, URL: u.Redacted, HideCursor: req.HideCursor,
		Audio: firstNonEmpty(req.Audio, "none"), ArchiveDir: "", Options: req.Options})
	spec := task.Spec{
		ID:         taskID,
		Type:       task.TypeLiveScreenPush,
		Title:      "屏幕推流：" + sc.Name + " → " + u.Redacted,
		InputPaths: []string{},
		Params:     string(pj),
	}
	t, err := s.cfg.Tasks.Submit(spec, s.newRunner(taskID, bin, u, req.URL, args, true, false))
	if err != nil {
		s.release(taskID)
		return task.Task{}, err
	}
	return t, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

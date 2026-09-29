package live

import (
	"context"
	"os"
	"strconv"
	"strings"

	"FFmpegFree/internal/apperr"
)

// 采集来源（契约 v0.14，见 6.10「采集来源」）。id 约定：screen:<序号>（ListScreens 结果里的位置，从 0 起）、
// window:<hwnd 十进制>（只有 Windows 有窗口）。前端只把 id 原样传回 StartScreenPush.captureSourceId。
const (
	SourceKindScreen = "screen"
	SourceKindWindow = "window"
)

// CaptureSource 是 ListCaptureSources 的一项。
type CaptureSource struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"` // screen | window
	Title  string `json:"title"`
	Width  int    `json:"width"`  // 物理像素；查不到为 0
	Height int    `json:"height"` // 同上
}

// RawWindow 是平台层枚举到的一个顶层窗口（未过滤）。过滤逻辑在纯函数 filterCaptureWindows 里，方便测试。
type RawWindow struct {
	HWND          uint64
	Title, Class  string
	PID           uint32
	Visible       bool
	Minimized     bool
	Cloaked       bool   // DWM 隐藏（别的虚拟桌面上的窗口、挂起的 UWP 窗口）
	ExStyle       uint32 // GWL_EXSTYLE
	Owner         uint64 // GW_OWNER，0 = 没有
	Width, Height int
}

const (
	wsExToolWindow = 0x00000080
	wsExAppWindow  = 0x00040000
	wsExNoActivate = 0x08000000
)

// 系统壳窗口：桌面、任务栏等。标题为 "Program Manager" 的是桌面（Progman）。
var shellWindowClasses = map[string]bool{
	"Progman": true, "WorkerW": true, "Shell_TrayWnd": true, "Shell_SecondaryTrayWnd": true,
	"Windows.UI.Core.CoreWindow": true,
}

// filterCaptureWindows 只保留适合采集的窗口：有标题、可见、没有最小化、没有被 DWM 隐藏、不是工具窗口 / 被拥有的子窗口 /
// 不激活的浮层（带 WS_EX_APPWINDOW 的例外）、不是系统壳窗口、不是本进程的窗口、有大小。最小化的窗口 gdigrab 采不到内容，
// 所以不列出（选了也会 LIVE_SOURCE_GONE）。保持枚举顺序（EnumWindows 是 Z 序）。
func filterCaptureWindows(raw []RawWindow, selfPID uint32) []CaptureSource {
	var res []CaptureSource
	for _, w := range raw {
		if strings.TrimSpace(w.Title) == "" || !w.Visible || w.Minimized || w.Cloaked {
			continue
		}
		if w.Width <= 0 || w.Height <= 0 {
			continue
		}
		if selfPID != 0 && w.PID == selfPID {
			continue
		}
		if shellWindowClasses[w.Class] || w.Title == "Program Manager" {
			continue
		}
		if w.ExStyle&wsExAppWindow == 0 && (w.ExStyle&wsExToolWindow != 0 || w.ExStyle&wsExNoActivate != 0 || w.Owner != 0) {
			continue
		}
		res = append(res, CaptureSource{ID: "window:" + strconv.FormatUint(w.HWND, 10), Kind: SourceKindWindow, Title: w.Title, Width: w.Width, Height: w.Height})
	}
	return res
}

// parseSourceID 解析 screen:<序号> / window:<hwnd>。数字必须是规范十进制（无符号、无前导零），否则 INVALID_ARGUMENT。
func parseSourceID(id string) (kind string, n uint64, err error) {
	k, num, ok := strings.Cut(id, ":")
	if ok && (k == SourceKindScreen || k == SourceKindWindow) {
		if v, perr := strconv.ParseUint(num, 10, 64); perr == nil && strconv.FormatUint(v, 10) == num {
			return k, v, nil
		}
	}
	return "", 0, invalidArg("captureSourceId 格式不对，应为 screen:<序号> 或 window:<窗口句柄>")
}

// sourceGone 是 LIVE_SOURCE_GONE：detail 只有一行 kind=window|screen（不带标题，避免把窗口标题写进日志）。
func sourceGone(kind string) *apperr.AppError {
	msg := "所选窗口已不可用，请重新选择"
	if kind == SourceKindScreen {
		msg = "所选屏幕已不可用，请重新选择"
	}
	return apperr.New(apperr.LiveSourceGone, msg).WithDetail("kind=" + kind)
}

// windowsEnabled 只有 Windows 有窗口来源；其他系统即使注入了枚举函数也不返回 window。
func (s *Service) windowsEnabled() bool { return s.cfg.GOOS == "windows" && s.cfg.EnumWindows != nil }

func (s *Service) listWindowSources() ([]CaptureSource, error) {
	raw, err := s.cfg.EnumWindows()
	if err != nil {
		return nil, err
	}
	return filterCaptureWindows(raw, uint32(os.Getpid())), nil
}

// ListCaptureSources 返回可选的采集来源：先是屏幕（同 ListScreens，id 为 screen:<序号>），Windows 上再加应用窗口。
// 不支持屏幕采集的平台 / 会话返回 UNSUPPORTED_PLATFORM（同 ListScreens）；枚举窗口失败时只返回屏幕并记日志。
func (s *Service) ListCaptureSources(ctx context.Context) ([]CaptureSource, error) {
	screens, err := s.ListScreens(ctx)
	if err != nil {
		return nil, err
	}
	res := make([]CaptureSource, 0, len(screens)+8)
	for i, sc := range screens {
		res = append(res, CaptureSource{ID: "screen:" + strconv.Itoa(i), Kind: SourceKindScreen, Title: sc.Name, Width: sc.Width, Height: sc.Height})
	}
	if s.windowsEnabled() {
		ws, werr := s.listWindowSources()
		if werr != nil {
			s.logf("枚举窗口失败: %v", werr)
		}
		res = append(res, ws...)
	}
	return res, nil
}

// resolvedSource 是开始推流时校验过的采集来源。
type resolvedSource struct {
	Screen      ScreenInfo // Kind=screen
	WindowTitle string     // Kind=window：此刻的窗口标题（gdigrab title= 用它）
	Name        string     // 任务标题里显示的名字
}

// resolveCaptureSource 校验 captureSourceId 指向的来源此刻仍然存在。格式不对 INVALID_ARGUMENT；屏幕序号越界、
// 窗口不存在 / 已最小化 / 已不可见 → LIVE_SOURCE_GONE（detail 首行 kind=screen|window）。
func (s *Service) resolveCaptureSource(ctx context.Context, id string) (resolvedSource, error) {
	kind, n, err := parseSourceID(id)
	if err != nil {
		return resolvedSource{}, err
	}
	if kind == SourceKindScreen {
		screens, err := s.ListScreens(ctx)
		if err != nil {
			return resolvedSource{}, err
		}
		if n >= uint64(len(screens)) {
			return resolvedSource{}, sourceGone(SourceKindScreen)
		}
		sc := screens[n]
		return resolvedSource{Screen: sc, Name: sc.Name}, nil
	}
	if s.cfg.GOOS != "windows" {
		return resolvedSource{}, invalidArg("当前系统不支持采集窗口")
	}
	if s.cfg.EnumWindows == nil {
		return resolvedSource{}, apperr.New(apperr.UnsupportedPlatform, "当前系统不支持采集窗口")
	}
	ws, err := s.listWindowSources()
	if err != nil {
		return resolvedSource{}, apperr.Wrap(apperr.Internal, "枚举窗口失败", err)
	}
	for _, w := range ws {
		if w.ID == id {
			return resolvedSource{WindowTitle: w.Title, Name: w.Title}, nil
		}
	}
	return resolvedSource{}, sourceGone(SourceKindWindow)
}

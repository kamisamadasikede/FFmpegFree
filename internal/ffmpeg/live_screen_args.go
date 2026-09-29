package ffmpeg

import (
	"fmt"
	"strconv"
)

// ScreenRegion 是要采集的显示器区域。
type ScreenRegion struct {
	X, Y          int
	Width, Height int // 0 = 采集整个桌面 / 让采集端自己决定
	DeviceIndex   int // macOS avfoundation 的设备序号
	Desktop       bool
}

// ScreenPushPlan 描述一次屏幕推流（不含存档）。
type ScreenPushPlan struct {
	GOOS       string // windows | darwin | linux
	Display    string // linux：DISPLAY，如 ":0" 或 ":99.0"
	Region     ScreenRegion
	HideCursor bool
	Silent     bool // audio=silent：补静音音轨
	Scheme     string
	URL        string
	FPS        float64 // 采集帧率
	Enc        LiveEncode
}

// ScreenInputArgs 生成屏幕采集输入参数（各平台）。
func ScreenInputArgs(p ScreenPushPlan) []string {
	fps := fnum(p.FPS)
	cursor := "1"
	if p.HideCursor {
		cursor = "0"
	}
	switch p.GOOS {
	case "windows":
		a := []string{"-f", "gdigrab", "-framerate", fps}
		if p.HideCursor {
			a = append(a, "-draw_mouse", "0")
		}
		if !p.Region.Desktop && p.Region.Width > 0 && p.Region.Height > 0 {
			a = append(a, "-offset_x", strconv.Itoa(p.Region.X), "-offset_y", strconv.Itoa(p.Region.Y),
				"-video_size", fmt.Sprintf("%dx%d", p.Region.Width, p.Region.Height))
		}
		return append(a, "-i", "desktop")
	case "darwin":
		return []string{"-f", "avfoundation", "-framerate", fps, "-capture_cursor", cursor,
			"-i", fmt.Sprintf("%d:none", p.Region.DeviceIndex)}
	default: // linux
		a := []string{"-f", "x11grab", "-framerate", fps, "-draw_mouse", cursor}
		if !p.Region.Desktop && p.Region.Width > 0 && p.Region.Height > 0 {
			a = append(a, "-video_size", fmt.Sprintf("%dx%d", p.Region.Width, p.Region.Height))
		}
		return append(a, "-i", fmt.Sprintf("%s+%d,%d", p.Display, p.Region.X, p.Region.Y))
	}
}

// BuildScreenPushArgs 生成无存档的屏幕推流参数。
func BuildScreenPushArgs(p ScreenPushPlan) []string {
	a := ScreenInputArgs(p)
	audioMap := ""
	if p.Silent {
		a = append(a, "-protocol_whitelist", inputWhitelist, "-f", "lavfi", "-i", anullsrc)
		audioMap = "1:a"
	}
	a = append(a, "-map", "0:v:0")
	if p.Silent {
		a = append(a, "-map", audioMap) // 屏幕采集是无限流，不需要 -shortest
	}
	a = append(a, liveEncodeArgs(p.Enc, p.Silent)...)
	a = append(a, "-protocol_whitelist", ProtocolWhitelist(p.Scheme), "-f", OutputFormat(p.Scheme))
	if p.Scheme != "srt" {
		a = append(a, "-flvflags", "no_duration_filesize")
	}
	return append(a, p.URL)
}

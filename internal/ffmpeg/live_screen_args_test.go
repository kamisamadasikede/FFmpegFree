package ffmpeg

import (
	"strings"
	"testing"
)

func TestScreenInputArgsPerPlatform(t *testing.T) {
	base := ScreenPushPlan{FPS: 30, Display: ":99", Region: ScreenRegion{X: 10, Y: 20, Width: 1920, Height: 1080}}
	j := func(p ScreenPushPlan) string { return strings.Join(ScreenInputArgs(p), " ") }

	p := base
	p.GOOS, p.HideCursor = "windows", true
	if got := j(p); got != "-f gdigrab -framerate 30 -draw_mouse 0 -offset_x 10 -offset_y 20 -video_size 1920x1080 -i desktop" {
		t.Errorf("windows: %s", got)
	}
	p.HideCursor = false
	if got := j(p); strings.Contains(got, "draw_mouse") {
		t.Errorf("windows 带指针不写 draw_mouse: %s", got)
	}
	p.Region.Desktop = true
	if got := j(p); got != "-f gdigrab -framerate 30 -i desktop" {
		t.Errorf("windows desktop: %s", got)
	}

	p = base
	p.GOOS, p.HideCursor = "darwin", true
	p.Region.DeviceIndex = 2
	if got := j(p); got != "-f avfoundation -framerate 30 -capture_cursor 0 -i 2:none" {
		t.Errorf("darwin: %s", got)
	}
	p.HideCursor = false
	if got := j(p); got != "-f avfoundation -framerate 30 -capture_cursor 1 -i 2:none" {
		t.Errorf("darwin cursor: %s", got)
	}

	p = base
	p.GOOS = "linux"
	if got := j(p); got != "-f x11grab -framerate 30 -draw_mouse 1 -video_size 1920x1080 -i :99+10,20" {
		t.Errorf("linux: %s", got)
	}
	p.HideCursor = true
	p.Region = ScreenRegion{Desktop: true}
	if got := j(p); got != "-f x11grab -framerate 30 -draw_mouse 0 -i :99+0,0" {
		t.Errorf("linux desktop: %s", got)
	}
}

func TestBuildScreenPushArgs(t *testing.T) {
	p := ScreenPushPlan{GOOS: "linux", Display: ":99", FPS: 15, Scheme: "rtmp", URL: "rtmp://h/a/k", Enc: LiveEncode{GOPFps: 15, VideoKbps: 1000, AudioKbps: 64}, Region: ScreenRegion{Desktop: true}}
	line := strings.Join(BuildScreenPushArgs(p), " ")
	if !strings.Contains(line, "-an") || strings.Contains(line, "anullsrc") || strings.Contains(line, "-shortest") || strings.Contains(line, "-c:a") {
		t.Fatalf("audio=none 视频流里没有音轨: %s", line)
	}
	if !strings.Contains(line, "-g 30") || !strings.HasSuffix(line, "-protocol_whitelist rtmp,tcp -f flv -flvflags no_duration_filesize rtmp://h/a/k") {
		t.Fatalf("%s", line)
	}
	p.Silent = true
	line = strings.Join(BuildScreenPushArgs(p), " ")
	if !strings.Contains(line, "anullsrc") || !strings.Contains(line, "-c:a aac") || strings.Contains(line, "-shortest") {
		t.Fatalf("silent 补静音、屏幕采集是无限流不加 -shortest: %s", line)
	}
}

// 表驱动：gdigrab 窗口（title=）与桌面（offset / video_size，含负偏移）的 argv。标题永远是单个 argv 元素。
func TestScreenInputArgsWindowAndOffset(t *testing.T) {
	tests := []struct {
		name string
		plan ScreenPushPlan
		want []string
	}{
		{"窗口", ScreenPushPlan{GOOS: "windows", FPS: 30, Region: ScreenRegion{WindowTitle: "记事本"}},
			[]string{"-f", "gdigrab", "-framerate", "30", "-i", "title=记事本"}},
		{"窗口标题带空格引号等号（一个 argv）", ScreenPushPlan{GOOS: "windows", FPS: 15, HideCursor: true, Region: ScreenRegion{WindowTitle: `a "b" = c; d&e|f`}},
			[]string{"-f", "gdigrab", "-framerate", "15", "-draw_mouse", "0", "-i", `title=a "b" = c; d&e|f`}},
		{"窗口忽略 offset / 尺寸", ScreenPushPlan{GOOS: "windows", FPS: 30, Region: ScreenRegion{WindowTitle: "x", X: 10, Y: 20, Width: 800, Height: 600}},
			[]string{"-f", "gdigrab", "-framerate", "30", "-i", "title=x"}},
		{"窗口标题以 title= 开头也原样", ScreenPushPlan{GOOS: "windows", FPS: 30, Region: ScreenRegion{WindowTitle: "title=x"}},
			[]string{"-f", "gdigrab", "-framerate", "30", "-i", "title=title=x"}},
		{"副屏在左侧（负 offset）", ScreenPushPlan{GOOS: "windows", FPS: 30, Region: ScreenRegion{X: -1920, Y: 0, Width: 1920, Height: 1080}},
			[]string{"-f", "gdigrab", "-framerate", "30", "-offset_x", "-1920", "-offset_y", "0", "-video_size", "1920x1080", "-i", "desktop"}},
		{"副屏在右下", ScreenPushPlan{GOOS: "windows", FPS: 30, Region: ScreenRegion{X: 2560, Y: 360, Width: 1280, Height: 720}},
			[]string{"-f", "gdigrab", "-framerate", "30", "-offset_x", "2560", "-offset_y", "360", "-video_size", "1280x720", "-i", "desktop"}},
		{"宽高为 0 采整个桌面", ScreenPushPlan{GOOS: "windows", FPS: 30},
			[]string{"-f", "gdigrab", "-framerate", "30", "-i", "desktop"}},
		{"非 Windows 忽略窗口标题", ScreenPushPlan{GOOS: "linux", Display: ":0", FPS: 30, Region: ScreenRegion{WindowTitle: "x", Desktop: true}},
			[]string{"-f", "x11grab", "-framerate", "30", "-draw_mouse", "1", "-i", ":0+0,0"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ScreenInputArgs(tc.plan)
			if strings.Join(got, "\x00") != strings.Join(tc.want, "\x00") {
				t.Fatalf("\n got %q\nwant %q", got, tc.want)
			}
		})
	}
}

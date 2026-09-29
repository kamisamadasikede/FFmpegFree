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

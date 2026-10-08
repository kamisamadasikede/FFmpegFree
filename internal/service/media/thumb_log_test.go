package media

import (
	"bytes"
	"context"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
)

// captureLog 把标准库 log 的输出接到缓冲区，测试结束时还原。
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old, flags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old); log.SetFlags(flags) })
	return &buf
}

// 包 19 Windows：缩略图失败要在应用日志里留一行，带输入 / 输出路径、退出码和 stderr 末尾；失败不进缓存，下次重新生成。
func TestThumbnailFailureIsLoggedAndNotCached(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell 脚本假 ffmpeg")
	}
	dir := t.TempDir()
	counter := filepath.Join(dir, "count")
	bin := filepath.Join(dir, "ffmpeg")
	os.WriteFile(bin, []byte("#!/bin/sh\necho x >> '"+counter+"'\necho 'line one' >&2\necho 'Error opening output file: Permission denied' >&2\nexit 3\n"), 0o755)
	f := filepath.Join(dir, "我的 视频.mp4")
	os.WriteFile(f, []byte("x"), 0o644)
	thumbs := filepath.Join(dir, "thumbs")
	svc := New(Config{ThumbsDir: thumbs,
		Require: func() (ffmpeg.Binaries, error) { return ffmpeg.Binaries{FFmpeg: bin, FFprobe: bin}, nil }})
	buf := captureLog(t)

	_, err := svc.DefaultThumbnailDataURL(context.Background(), f, 23)
	if !apperr.Is(err, apperr.ProcessFailed) {
		t.Fatalf("%v", err)
	}
	out := buf.String()
	for _, want := range []string{"缩略图: ffmpeg 失败", "exit=3(0x3)", "Permission denied", "at=auto", "at=0.000s", `in="` + f + `"`, `out="` + thumbs} {
		if !strings.Contains(out, want) {
			t.Errorf("日志缺少 %q:\n%s", want, out)
		}
	}
	// 找不黑的帧失败后退回第一帧再试一次：两次各一行
	if n := strings.Count(out, "缩略图: ffmpeg 失败"); n != 2 {
		t.Errorf("应记两行（auto + 退回第一帧），实际 %d:\n%s", n, out)
	}
	es, _ := os.ReadDir(thumbs)
	if len(es) != 0 {
		t.Fatalf("失败不应留下缓存文件: %v", es)
	}
	// 失败不缓存：再调一次会重新启动 ffmpeg
	svc.DefaultThumbnailDataURL(context.Background(), f, 23)
	b, _ := os.ReadFile(counter)
	if n := strings.Count(string(b), "x"); n != 4 {
		t.Fatalf("第二次调用应重新生成（共 4 次 ffmpeg），实际 %d", n)
	}
}

// 转换组件未就绪（启动检测中 / 缺失）时 DefaultThumbnailDataURL 返回 FFMPEG_NOT_FOUND，并记一行日志。
func TestDefaultThumbnailLogsWhenFFmpegNotReady(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "v.mp4")
	os.WriteFile(f, []byte("x"), 0o644)
	svc := New(Config{ThumbsDir: filepath.Join(dir, "t"), Require: func() (ffmpeg.Binaries, error) {
		return ffmpeg.Binaries{}, apperr.New(apperr.FFmpegNotFound, "未找到可用的转换组件")
	}})
	buf := captureLog(t)
	if _, err := svc.DefaultThumbnailDataURL(context.Background(), f, 23); !apperr.Is(err, apperr.FFmpegNotFound) {
		t.Fatalf("%v", err)
	}
	if out := buf.String(); !strings.Contains(out, "缩略图: 转换组件未就绪") || !strings.Contains(out, f) {
		t.Fatalf("日志: %s", out)
	}
}

func TestExitText(t *testing.T) {
	if got := exitText(nil); got != "exit=0" {
		t.Fatal(got)
	}
	if got := exitText(os.ErrNotExist); !strings.HasPrefix(got, "err=") {
		t.Fatal(got)
	}
}

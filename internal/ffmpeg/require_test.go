package ffmpeg

import (
	"testing"

	"FFmpegFree/internal/apperr"
)

func TestRequire(t *testing.T) {
	SetCurrent(nil)
	if _, err := Require(); !apperr.Is(err, apperr.FFmpegNotFound) {
		t.Fatalf("未就绪应返回 FFMPEG_NOT_FOUND, got %v", err)
	}
	SetCurrent(&Binaries{FFmpeg: "/a/ffmpeg", FFprobe: "/a/ffprobe"})
	t.Cleanup(func() { SetCurrent(nil) })
	b, err := Require()
	if err != nil || b.FFmpeg != "/a/ffmpeg" || b.FFprobe != "/a/ffprobe" {
		t.Fatalf("就绪应返回路径: %+v %v", b, err)
	}
	SetCurrent(nil)
	if _, err := Require(); err == nil {
		t.Fatal("清除后应再次报错")
	}
}

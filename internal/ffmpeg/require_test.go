package ffmpeg

import (
	"context"
	"testing"
	"time"

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

// 启动时的检测：SetChecking 之后 WaitDetected 等到 SetCurrent 才返回；没在检测时不等；ctx 到期不再等。
func TestWaitDetected(t *testing.T) {
	t.Cleanup(func() { SetCurrent(nil) })
	SetCurrent(nil)
	start := time.Now()
	WaitDetected(context.Background()) // 没在检测：立即返回
	if time.Since(start) > time.Second {
		t.Fatal("没在检测时不应等待")
	}

	SetChecking()
	if _, err := Require(); !apperr.Is(err, apperr.FFmpegNotFound) {
		t.Fatalf("检测期间 Require 仍返回 FFMPEG_NOT_FOUND: %v", err)
	}
	done := make(chan struct{})
	go func() {
		WaitDetected(context.Background())
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("检测还没结果就返回了")
	case <-time.After(50 * time.Millisecond):
	}
	SetChecking() // 重复调用不影响等待
	SetCurrent(&Binaries{FFmpeg: "/a/ffmpeg", FFprobe: "/a/ffprobe"})
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("SetCurrent 之后应结束等待")
	}
	if _, err := RequireProbe(); err != nil {
		t.Fatalf("检测结束后可用: %v", err)
	}

	SetChecking() // 重新检测：清掉当前路径
	if _, ok := Current(); ok {
		t.Fatal("检测期间没有可用路径")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	WaitDetected(ctx) // ctx 到期返回
	SetCurrent(nil)   // 检测结果：不可用，同样结束等待
	WaitDetected(context.Background())
}

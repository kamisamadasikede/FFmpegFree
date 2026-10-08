package media

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
)

func detailOf(err error) string {
	var ae *apperr.AppError
	if errors.As(err, &ae) {
		return ae.Detail
	}
	return ""
}

// 契约 6.14.10：转换页缩略图 = 默认缩略图（v0.23.6 起第一帧，太暗取前 3 秒里第一张不黑的；宽 320），缓存键含 mtime / size。
func TestDefaultThumbnailDataURL(t *testing.T) {
	e := newEnv(t, nil)
	ctx := context.Background()
	p := e.video(t, "v.mp4")
	u1, err := e.svc.DefaultThumbnailDataURL(ctx, p, 0) // 没有时长提示：先探测
	if err != nil || !strings.HasPrefix(u1, "data:image/jpeg;base64,") {
		t.Fatalf("%v", err)
	}
	fi, _ := os.Stat(p)
	_, key, _, _ := statMedia(p)
	cached := filepath.Join(e.thumbs, cacheName(key, fi.ModTime(), fi.Size(), autoThumbAt, DefaultThumbWidth))
	if _, err := os.Stat(cached); err != nil {
		t.Fatalf("缓存文件名应按 (path_key, mtime, size, atSec, 320) 计算: %v", err)
	}
	// 命中缓存不启动 ffmpeg / ffprobe
	e.svc.cfg.Require = func() (ffmpeg.Binaries, error) {
		return ffmpeg.Binaries{FFmpeg: "/nonexistent/ffmpeg", FFprobe: "/nonexistent/ffprobe"}, nil
	}
	if u2, err := e.svc.DefaultThumbnailDataURL(ctx, p, 3); err != nil || u2 != u1 {
		t.Fatalf("应命中缓存: %v", err)
	}
	e.svc.cfg.Require = func() (ffmpeg.Binaries, error) { return e.bin, nil }
	// 同一路径的文件被替换（mtime / size 变）→ 新键、重新生成
	e.video(t, "v.mp4", "-vf", "negate")
	future := time.Now().Add(time.Hour)
	os.Chtimes(p, future, future)
	u3, err := e.svc.DefaultThumbnailDataURL(ctx, p, 3)
	if err != nil || u3 == u1 {
		t.Fatalf("文件替换后应返回新的缩略图: %v", err)
	}
	// 文件不存在 / 是目录 → NOT_FOUND reason=file
	for _, bad := range []string{filepath.Join(e.dir, "nope.mp4"), e.dir} {
		if _, err := e.svc.DefaultThumbnailDataURL(ctx, bad, 0); !apperr.Is(err, apperr.NotFound) || detailOf(err) != "reason=file" {
			t.Fatalf("%s: %v", bad, err)
		}
	}
	// 纯音频 → UNSUPPORTED reason=format
	a := filepath.Join(e.dir, "a.mp3")
	gen(t, e.bin, a, "-f", "lavfi", "-i", "sine=frequency=440:duration=1", "-c:a", "libmp3lame")
	if _, err := e.svc.DefaultThumbnailDataURL(ctx, a, 0); !apperr.Is(err, apperr.Unsupported) || detailOf(err) != "reason=format" {
		t.Fatalf("%v", err)
	}
}

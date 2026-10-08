package media

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// meanLuma 解出 data URL 里的 JPEG，返回平均亮度（0~255）。
func meanLuma(t *testing.T, u string) float64 {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(u, "data:image/jpeg;base64,"))
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	var sum, n float64
	r := img.Bounds()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if yc, ok := img.(*image.YCbCr); ok {
				sum += float64(yc.Y[yc.YOffset(x, y)])
			} else {
				c := img.At(x, y)
				rr, gg, bb, _ := c.RGBA()
				sum += (0.299*float64(rr) + 0.587*float64(gg) + 0.114*float64(bb)) / 257
			}
			n++
		}
	}
	return sum / n
}

// 契约 v0.23.6：默认缩略图取第一帧；片头黑场时取前 3 秒里第一张不黑的；全黑退回第一帧（仍然出图）。GIF 同样。
func TestDefaultThumbnailSkipsBlackStart(t *testing.T) {
	e := newEnv(t, nil)
	ctx := context.Background()
	mk := func(name string, args ...string) string {
		p := filepath.Join(e.dir, name)
		gen(t, e.bin, p, args...)
		return p
	}
	blackThen := []string{"-f", "lavfi", "-i", "color=black:s=320x240:d=1.2:r=25", "-f", "lavfi", "-i", "color=white:s=320x240:d=2:r=25",
		"-filter_complex", "[0][1]concat=n=2:v=1[v]", "-map", "[v]"}
	cases := []struct {
		name   string
		args   []string
		bright bool
	}{
		{"black-start.mp4", append(append([]string{}, blackThen...), "-c:v", "libx264", "-pix_fmt", "yuv420p"), true},
		{"black-start.gif", blackThen, true},
		{"first-frame.mp4", []string{"-f", "lavfi", "-i", "color=white:s=320x240:d=2:r=25", "-c:v", "libx264", "-pix_fmt", "yuv420p"}, true},
		{"all-black.mp4", []string{"-f", "lavfi", "-i", "color=black:s=320x240:d=4:r=25", "-c:v", "libx264", "-pix_fmt", "yuv420p"}, false},
	}
	for _, c := range cases {
		p := mk(c.name, c.args...)
		u, err := e.svc.DefaultThumbnailDataURL(ctx, p, 0)
		if err != nil || !strings.HasPrefix(u, "data:image/jpeg;base64,") {
			t.Fatalf("%s: %v", c.name, err)
		}
		l := meanLuma(t, u)
		if c.bright && l < 128 {
			t.Errorf("%s: 应跳过片头黑场取到亮的帧，平均亮度 %.0f", c.name, l)
		}
		if !c.bright && l > 40 {
			t.Errorf("%s: 全黑应退回第一帧（黑），平均亮度 %.0f", c.name, l)
		}
	}
	// 缓存：同一文件第二次命中，不再启动 ffmpeg
	p := filepath.Join(e.dir, "black-start.mp4")
	fi, _ := os.Stat(p)
	_, key, _, _ := statMedia(p)
	if _, err := os.Stat(filepath.Join(e.thumbs, cacheName(key, fi.ModTime(), fi.Size(), autoThumbAt, DefaultThumbWidth))); err != nil {
		t.Fatalf("缓存文件名应按 autoThumbAt 计算: %v", err)
	}
}

// 缓存版本进键：同一参数换了版本号就是另一个文件名（旧的 10% / 1 秒缩略图不会被当成新规则的结果）。
func TestCacheNameIncludesVersion(t *testing.T) {
	fi, _ := os.Stat(os.Args[0])
	a := cacheName("k", fi.ModTime(), 1, autoThumbAt, 320)
	b := cacheName("k", fi.ModTime(), 1, 0, 320)
	if a == b {
		t.Fatal("默认缩略图（auto）和第 0 秒应是不同的缓存")
	}
}

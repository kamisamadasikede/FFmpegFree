package media

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/store"
)

// realBins 找系统里的 ffmpeg / ffprobe，找不到就跳过（CI 不保证有）。
func realBins(t *testing.T) ffmpeg.Binaries {
	t.Helper()
	fm, err1 := exec.LookPath("ffmpeg")
	fp, err2 := exec.LookPath("ffprobe")
	if err1 != nil || err2 != nil {
		t.Skip("没有 ffmpeg / ffprobe，跳过集成测试")
	}
	return ffmpeg.Binaries{FFmpeg: fm, FFprobe: fp}
}

func gen(t *testing.T, bin ffmpeg.Binaries, out string, args ...string) {
	t.Helper()
	full := append([]string{"-v", "error", "-y"}, args...)
	full = append(full, "file:"+out)
	if b, err := exec.Command(bin.FFmpeg, full...).CombinedOutput(); err != nil {
		t.Skipf("生成测试媒体失败（编码器缺失？）: %v\n%s", err, b)
	}
}

type env struct {
	svc    *Service
	st     *store.Store
	bin    ffmpeg.Binaries
	dir    string
	thumbs string
}

func newEnv(t *testing.T, mut func(*Config)) *env {
	t.Helper()
	bin := realBins(t)
	dir := t.TempDir()
	st, err := store.Open(context.Background(), filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	cfg := Config{Store: st, ThumbsDir: filepath.Join(dir, "thumbs"),
		Require: func() (ffmpeg.Binaries, error) { return bin, nil }}
	if mut != nil {
		mut(&cfg)
	}
	return &env{svc: New(cfg), st: st, bin: bin, dir: dir, thumbs: cfg.ThumbsDir}
}

func (e *env) video(t *testing.T, name string, extra ...string) string {
	t.Helper()
	p := filepath.Join(e.dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	args := []string{"-f", "lavfi", "-i", "testsrc=size=320x240:rate=25:duration=3",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=44100:duration=3",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest"}
	gen(t, e.bin, p, append(args, extra...)...)
	return p
}

func TestIntegrationProbeAndThumbnail(t *testing.T) {
	e := newEnv(t, nil)
	p := e.video(t, "clip.mp4")
	ctx := context.Background()

	res, err := e.svc.Probe(ctx, []string{p})
	if err != nil || len(res) != 1 {
		t.Fatalf("%v %v", res, err)
	}
	m := res[0]
	if m.Error != nil || m.VideoCodec != "h264" || m.AudioCodec != "aac" || m.Width != 320 || m.Height != 240 ||
		m.Duration < 2.9 || m.Duration > 3.3 || m.Fps != 25 || m.ID == "" || m.Name != "clip.mp4" || m.Size <= 0 {
		t.Fatalf("%+v", m)
	}
	if !strings.HasPrefix(m.ThumbURL, "data:image/jpeg;base64,") {
		t.Fatalf("默认缩略图缺失: %q", m.ThumbURL[:min(30, len(m.ThumbURL))])
	}
	// 写入了 media 表
	recent, err := e.svc.ListRecent(ctx, 10)
	if err != nil || len(recent) != 1 || recent[0].ID != m.ID || recent[0].Path != m.Path {
		t.Fatalf("%+v %v", recent, err)
	}
	if !strings.HasPrefix(recent[0].ThumbURL, "data:image/jpeg") {
		t.Fatal("ListRecent 应带上已缓存的缩略图")
	}
	// 再次探测同一文件：id 不变
	res2, _ := e.svc.Probe(ctx, []string{p})
	if res2[0].ID != m.ID {
		t.Fatalf("id 应保持不变: %s vs %s", res2[0].ID, m.ID)
	}

	// 指定时间点 + 宽度，返回真实的 jpg 文件
	th, err := e.svc.Thumbnail(ctx, p, 1.5, 160)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(th.Path)
	if err != nil || len(b) < 100 || b[0] != 0xFF || b[1] != 0xD8 {
		t.Fatalf("不是 jpg: %v len=%d", err, len(b))
	}
	if filepath.Dir(th.Path) != e.thumbs || th.Width != 160 || th.AtSec != 1.5 {
		t.Fatalf("%+v", th)
	}
	// 尺寸：160x120
	out, err := exec.Command(e.bin.FFprobe, "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=width,height", "-of", "csv=p=0", "file:"+th.Path).Output()
	if err != nil || strings.TrimSpace(string(out)) != "160,120" {
		t.Fatalf("缩略图尺寸 %q %v", out, err)
	}
	// 缓存命中：文件没被重新生成（mtime 之外再删掉 ffmpeg 也应成功）
	e.svc.cfg.Require = func() (ffmpeg.Binaries, error) {
		return ffmpeg.Binaries{FFmpeg: "/nonexistent/ffmpeg", FFprobe: e.bin.FFprobe}, nil
	}
	th2, err := e.svc.Thumbnail(ctx, p, 1.5, 160)
	if err != nil || th2.Path != th.Path {
		t.Fatalf("应命中缓存: %+v %v", th2, err)
	}
	// 不放大
	e.svc.cfg.Require = func() (ffmpeg.Binaries, error) { return e.bin, nil }
	big, err := e.svc.Thumbnail(ctx, p, 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	out, _ = exec.Command(e.bin.FFprobe, "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=width,height", "-of", "csv=p=0", "file:"+big.Path).Output()
	if strings.TrimSpace(string(out)) != "320,240" {
		t.Fatalf("不应放大: %q", out)
	}
	// 超出时长退回第 0 秒
	if _, err := e.svc.Thumbnail(ctx, p, 999, 100); err != nil {
		t.Fatalf("超出时长应退回 0 秒: %v", err)
	}
}

func TestIntegrationCacheInvalidatesOnFileChange(t *testing.T) {
	e := newEnv(t, nil)
	p := e.video(t, "a.mp4")
	t1, err := e.svc.Thumbnail(context.Background(), p, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	// 重写文件（大小、mtime 都变）→ 新的缓存文件
	e.video(t, "a.mp4", "-t", "1")
	future := time.Now().Add(time.Hour)
	_ = os.Chtimes(p, future, future)
	t2, err := e.svc.Thumbnail(context.Background(), p, 0, 100)
	if err != nil || t2.Path == t1.Path {
		t.Fatalf("文件变化后应重新生成: %+v %v", t2, err)
	}
}

func TestIntegrationSpecialFilenames(t *testing.T) {
	e := newEnv(t, nil)
	names := []string{
		"with space.mp4",
		"中文 视频 名.mp4",
		"日本語のファイル.mp4",
		"a:b.mp4",
		"100%.mp4",
		"it's [x] (1) & y.mp4",
		filepath.Join("目录 一", "子 dir", "z.mp4"),
	}
	if runtime.GOOS == "windows" { // 冒号在 Windows 文件名里非法
		names[3] = "a_b.mp4"
	}
	for _, n := range names {
		p := e.video(t, n)
		res, err := e.svc.Probe(context.Background(), []string{p})
		if err != nil || res[0].Error != nil || res[0].VideoCodec != "h264" {
			t.Fatalf("%q: %+v %v", n, res, err)
		}
		if !strings.HasPrefix(res[0].ThumbURL, "data:image/jpeg") {
			t.Fatalf("%q: 没有缩略图", n)
		}
	}

	// 以 - 开头：用相对路径调用（Normalize 会转绝对路径，但 file: 前缀保证即使直接传入也不被当选项）
	if runtime.GOOS != "windows" {
		dash := e.video(t, "-dash lead.mp4")
		wd, _ := os.Getwd()
		t.Cleanup(func() { os.Chdir(wd) })
		if err := os.Chdir(filepath.Dir(dash)); err != nil {
			t.Fatal(err)
		}
		res, err := e.svc.Probe(context.Background(), []string{"-dash lead.mp4"})
		if err != nil || res[0].Error != nil || res[0].VideoCodec != "h264" {
			t.Fatalf("以 - 开头的相对路径: %+v %v", res, err)
		}
		if _, err := e.svc.Thumbnail(context.Background(), "-dash lead.mp4", 0, 64); err != nil {
			t.Fatalf("以 - 开头的缩略图: %v", err)
		}
	}
}

func TestIntegrationProbeErrors(t *testing.T) {
	e := newEnv(t, nil)
	good := e.video(t, "ok.mp4")
	garbage := filepath.Join(e.dir, "garbage.mp4")
	os.WriteFile(garbage, []byte("this is not a media file at all"), 0o644)
	empty := filepath.Join(e.dir, "empty.mp4")
	os.WriteFile(empty, nil, 0o644)
	trunc := filepath.Join(e.dir, "trunc.mp4")
	if b, err := os.ReadFile(good); err == nil {
		os.WriteFile(trunc, b[:len(b)/8], 0o644)
	}
	res, err := e.svc.Probe(context.Background(), []string{
		filepath.Join(e.dir, "nope.mp4"), garbage, empty, e.dir, good, trunc,
	})
	if err != nil || len(res) != 6 {
		t.Fatalf("%v %v", res, err)
	}
	wantCode := []apperr.Code{apperr.NotFound, apperr.ProbeFailed, apperr.ProbeFailed, apperr.InvalidArgument, "", ""}
	for i, w := range wantCode {
		if w == "" {
			continue
		}
		if res[i].Error == nil || res[i].Error.Code != w {
			t.Errorf("#%d 期望 %s, got %+v", i, w, res[i].Error)
		}
	}
	if res[4].Error != nil || res[4].VideoCodec != "h264" {
		t.Fatalf("好文件不受影响: %+v", res[4])
	}
	// 失败的文件不入库
	recent, _ := e.svc.ListRecent(context.Background(), 50)
	for _, r := range recent {
		if strings.Contains(r.Path, "garbage") || strings.Contains(r.Path, "nope") {
			t.Fatalf("失败文件不应入库: %+v", r)
		}
	}
	// 单个 Thumbnail 对不存在文件直接返回 NOT_FOUND
	if _, err := e.svc.Thumbnail(context.Background(), filepath.Join(e.dir, "nope.mp4"), 0, 100); !apperr.Is(err, apperr.NotFound) {
		t.Fatalf("%v", err)
	}
	if _, err := e.svc.Thumbnail(context.Background(), garbage, 0, 100); err == nil {
		t.Fatal("损坏文件应报错")
	}
	if _, err := e.svc.Thumbnail(context.Background(), good, -1, 100); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("负数时间: %v", err)
	}
}

func TestIntegrationAudioOnlyHasNoThumbnail(t *testing.T) {
	e := newEnv(t, nil)
	p := filepath.Join(e.dir, "a.mp3")
	gen(t, e.bin, p, "-f", "lavfi", "-i", "sine=frequency=440:duration=1", "-c:a", "libmp3lame")
	res, err := e.svc.Probe(context.Background(), []string{p})
	if err != nil || res[0].Error != nil || res[0].HasVideo || !res[0].HasAudio || res[0].ThumbURL != "" {
		t.Fatalf("%+v %v", res, err)
	}
	if _, err := e.svc.Thumbnail(context.Background(), p, 0, 100); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("音频文件应返回 INVALID_ARGUMENT: %v", err)
	}
}

func TestIntegrationRotatedThumbnailIsUpright(t *testing.T) {
	e := newEnv(t, nil)
	base := e.video(t, "base.mp4")
	rot := filepath.Join(e.dir, "rot.mp4")
	if b, err := exec.Command(e.bin.FFmpeg, "-v", "error", "-y", "-display_rotation", "90", "-i", "file:"+base, "-c", "copy", "file:"+rot).CombinedOutput(); err != nil {
		t.Skipf("生成旋转文件失败: %v %s", err, b)
	}
	res, err := e.svc.Probe(context.Background(), []string{rot})
	if err != nil || res[0].Rotation != 90 || res[0].Width != 240 || res[0].Height != 320 {
		t.Fatalf("%+v %v", res, err)
	}
	th, err := e.svc.Thumbnail(context.Background(), rot, 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := exec.Command(e.bin.FFprobe, "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=width,height", "-of", "csv=p=0", "file:"+th.Path).Output()
	if strings.TrimSpace(string(out)) != "240,320" {
		t.Fatalf("缩略图应已转正为 240x320: %q", out)
	}
}

func TestRemoveRecentKeepsFile(t *testing.T) {
	e := newEnv(t, nil)
	p := e.video(t, "keep.mp4")
	res, _ := e.svc.Probe(context.Background(), []string{p})
	if err := e.svc.RemoveRecent(context.Background(), []string{res[0].ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatal("文件不应被删除")
	}
	if r, _ := e.svc.ListRecent(context.Background(), 10); len(r) != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestBatchOrderAndLimits(t *testing.T) {
	e := newEnv(t, nil)
	if _, err := e.svc.Probe(context.Background(), nil); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("空列表: %v", err)
	}
	if _, err := e.svc.Probe(context.Background(), make([]string, maxProbeBatch+1)); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("超限: %v", err)
	}
	// 顺序与入参一致
	var in []string
	for _, n := range []string{"1.mp4", "2.mp4", "3.mp4", "4.mp4", "5.mp4", "6.mp4"} {
		in = append(in, e.video(t, n, "-t", "1"))
	}
	res, err := e.svc.Probe(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	for i := range in {
		if res[i].Path != in[i] || res[i].Error != nil {
			t.Fatalf("#%d: %+v", i, res[i])
		}
	}
}

func TestRequireGate(t *testing.T) {
	dir := t.TempDir()
	calls := 0
	svc := New(Config{ThumbsDir: dir, Require: func() (ffmpeg.Binaries, error) {
		calls++
		return ffmpeg.Binaries{}, apperr.New(apperr.FFmpegNotFound, "未找到")
	}})
	f := filepath.Join(dir, "x.mp4")
	os.WriteFile(f, []byte("x"), 0o644)
	if _, err := svc.Probe(context.Background(), []string{f}); !apperr.Is(err, apperr.FFmpegNotFound) {
		t.Fatalf("Probe: %v", err)
	}
	if _, err := svc.Thumbnail(context.Background(), f, 0, 100); !apperr.Is(err, apperr.FFmpegNotFound) {
		t.Fatalf("Thumbnail: %v", err)
	}
	if _, err := svc.ProbeOne(context.Background(), f); !apperr.Is(err, apperr.FFmpegNotFound) {
		t.Fatalf("ProbeOne: %v", err)
	}
	if calls != 3 {
		t.Fatalf("calls = %d", calls)
	}
	// 默认门控：未设置 ffmpeg.Current 时返回 FFMPEG_NOT_FOUND
	ffmpeg.SetCurrent(nil)
	if _, err := New(Config{ThumbsDir: dir}).Probe(context.Background(), []string{f}); !apperr.Is(err, apperr.FFmpegNotFound) {
		t.Fatalf("默认门控: %v", err)
	}
	// 只有 ffmpeg 没有 ffprobe（v1 目录）
	ffmpeg.SetCurrent(&ffmpeg.Binaries{FFmpeg: "/x/ffmpeg"})
	defer ffmpeg.SetCurrent(nil)
	if _, err := New(Config{ThumbsDir: dir}).Thumbnail(context.Background(), f, 0, 100); !apperr.Is(err, apperr.FFmpegNotFound) {
		t.Fatalf("缺 ffprobe: %v", err)
	}
}

func TestArgsAreSafe(t *testing.T) {
	for _, in := range []string{"-rf.mp4", `C:\视频 库\a b.mp4`, "/a/b:c.mp4", "http://x/y.mp4"} {
		a := probeArgs(in)
		if a[len(a)-2] != "-i" || a[len(a)-1] != "file:"+in {
			t.Fatalf("probe args: %v", a)
		}
		th := thumbArgs(in, "/o/x.part.jpg", 1, 100)
		joined := strings.Join(th, "\x00")
		if !strings.Contains(joined, "-i\x00file:"+in+"\x00") || th[len(th)-1] != "file:/o/x.part.jpg" {
			t.Fatalf("thumb args: %v", th)
		}
	}
}

func TestCacheNameSensitivity(t *testing.T) {
	mt := time.Unix(1700000000, 5)
	base := cacheName("k", mt, 10, 1.5, 320)
	if cacheName("k", mt, 10, 1.5, 320) != base {
		t.Fatal("同参数应稳定")
	}
	for name, other := range map[string]string{
		"key":   cacheName("k2", mt, 10, 1.5, 320),
		"mtime": cacheName("k", mt.Add(time.Second), 10, 1.5, 320),
		"size":  cacheName("k", mt, 11, 1.5, 320),
		"at":    cacheName("k", mt, 10, 1.6, 320),
		"width": cacheName("k", mt, 10, 1.5, 321),
	} {
		if other == base {
			t.Errorf("%s 变化应得到不同缓存名", name)
		}
	}
	if !strings.HasSuffix(base, ".jpg") || strings.ContainsAny(base, `/\ :`) {
		t.Fatalf("缓存名不安全: %s", base)
	}
}

func TestCacheCleanupBounds(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	c := newThumbCache(dir, 10, 1<<20, func() time.Time { return now })
	write := func(name string, size int, age time.Duration) {
		p := filepath.Join(dir, name)
		os.WriteFile(p, make([]byte, size), 0o644)
		os.Chtimes(p, now.Add(-age), now.Add(-age))
	}
	for i := 0; i < 15; i++ { // 15 个，最旧的 i=14
		write(string(rune('a'+i))+".jpg", 10, time.Duration(15-i)*time.Minute)
	}
	write("stale.part.jpg", 5, 2*time.Hour)
	write("fresh.part.jpg", 5, time.Minute)
	write("note.txt", 5, 5*time.Hour) // 不认识的文件不碰
	c.cleanup()
	entries, _ := os.ReadDir(dir)
	var jpgs int
	names := map[string]bool{}
	for _, e := range entries {
		names[e.Name()] = true
		if strings.HasSuffix(e.Name(), ".jpg") && !strings.Contains(e.Name(), ".part.") {
			jpgs++
		}
	}
	if jpgs != 8 { // 降到上限 10 的 80%
		t.Fatalf("jpg 数量 = %d, want 8", jpgs)
	}
	if !names["o.jpg"] || names["a.jpg"] { // a 最旧被删，o 最新保留
		t.Fatalf("应按最后使用时间淘汰: %v", names)
	}
	if names["stale.part.jpg"] || !names["fresh.part.jpg"] || !names["note.txt"] {
		t.Fatalf("part / 杂项处理不对: %v", names)
	}

	// 按大小
	dir2 := t.TempDir()
	c2 := newThumbCache(dir2, 1000, 100, func() time.Time { return now })
	for i := 0; i < 5; i++ {
		p := filepath.Join(dir2, string(rune('a'+i))+".jpg")
		os.WriteFile(p, make([]byte, 40), 0o644)
		os.Chtimes(p, now.Add(-time.Duration(5-i)*time.Minute), now.Add(-time.Duration(5-i)*time.Minute))
	}
	c2.cleanup()
	es, _ := os.ReadDir(dir2)
	if len(es) != 2 { // 80 字节以内：2 个
		t.Fatalf("按大小清理后剩 %d", len(es))
	}
}

func TestDefaultThumbAtAndClampWidth(t *testing.T) {
	for in, want := range map[float64]float64{0: 0, -1: 0, 3: 0.3, 100: 10, 1000: 10} {
		if got := defaultThumbAt(in); got != want {
			t.Errorf("defaultThumbAt(%v) = %v, want %v", in, got, want)
		}
	}
	for in, want := range map[int]int{0: 320, -5: 320, 1: 16, 500: 500, 99999: 1920} {
		if got := clampWidth(in); got != want {
			t.Errorf("clampWidth(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestConcurrentSameThumbnailGeneratesOnce(t *testing.T) {
	e := newEnv(t, nil)
	p := e.video(t, "c.mp4")
	done := make(chan Thumb, 6)
	for i := 0; i < 6; i++ {
		go func() {
			th, err := e.svc.Thumbnail(context.Background(), p, 1, 120)
			if err != nil {
				t.Error(err)
			}
			done <- th
		}()
	}
	var first string
	for i := 0; i < 6; i++ {
		th := <-done
		if first == "" {
			first = th.Path
		}
		if th.Path != first {
			t.Fatalf("路径不一致")
		}
	}
	es, _ := os.ReadDir(e.thumbs)
	if len(es) != 1 {
		t.Fatalf("应只有 1 个缓存文件（无 .part 残留）: %d", len(es))
	}
}

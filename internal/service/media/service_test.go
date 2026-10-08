package media

import (
	"FFmpegFree/internal/localassets"
	"context"
	"math"
	"net/http/httptest"
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

func TestClampWidth(t *testing.T) {
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

// ---------- #9 评审修订 ----------

func TestStatMediaRejectsFIFOWithoutBlocking(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 没有 mkfifo")
	}
	dir := t.TempDir()
	fifo := filepath.Join(dir, "pipe.mp4")
	if err := syscallMkfifo(fifo); err != nil {
		t.Skipf("mkfifo 不可用: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		_, _, _, err := statMedia(fifo)
		done <- err
	}()
	select {
	case err := <-done:
		if !apperr.Is(err, apperr.InvalidArgument) {
			t.Fatalf("FIFO 应返回 INVALID_ARGUMENT: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("statMedia 在 FIFO 上阻塞了")
	}
	// 走完整的 Probe / Thumbnail 也不能卡
	svc := New(Config{ThumbsDir: filepath.Join(dir, "t"), Require: func() (ffmpeg.Binaries, error) {
		return ffmpeg.Binaries{FFmpeg: "/bin/false", FFprobe: "/bin/false"}, nil
	}})
	res, err := svc.Probe(context.Background(), []string{fifo})
	if err != nil || res[0].Error == nil || res[0].Error.Code != apperr.InvalidArgument {
		t.Fatalf("%+v %v", res, err)
	}
	if _, err := svc.Thumbnail(context.Background(), fifo, 0, 100); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("%v", err)
	}
}

func TestProbeSubtitleOnlyIsProbeFailed(t *testing.T) {
	e := newEnv(t, nil)
	srt := filepath.Join(e.dir, "s.srt")
	os.WriteFile(srt, []byte("1\n00:00:00,000 --> 00:00:01,000\nhi\n"), 0o644)
	res, err := e.svc.Probe(context.Background(), []string{srt})
	if err != nil || res[0].Error == nil || res[0].Error.Code != apperr.ProbeFailed {
		t.Fatalf("只有字幕流应 PROBE_FAILED: %+v %v", res, err)
	}
	if recent, _ := e.svc.ListRecent(context.Background(), 10); len(recent) != 0 {
		t.Fatalf("失败不入库: %+v", recent)
	}
}

func TestParseOnlySubtitleDataOrCoverArt(t *testing.T) {
	for name, in := range map[string]string{
		"只有字幕":  `{"streams":[{"index":0,"codec_type":"subtitle","codec_name":"subrip"}],"format":{"duration":"1.0"}}`,
		"只有数据":  `{"streams":[{"index":0,"codec_type":"data","codec_name":"bin_data"}],"format":{}}`,
		"字幕加数据": `{"streams":[{"index":0,"codec_type":"subtitle"},{"index":1,"codec_type":"data"}],"format":{}}`,
		"只有封面图": `{"streams":[{"index":0,"codec_type":"video","codec_name":"mjpeg","disposition":{"attached_pic":1}}],"format":{}}`,
	} {
		if _, err := ParseProbe([]byte(in), "x"); !apperr.Is(err, apperr.ProbeFailed) {
			t.Errorf("%s: 应 PROBE_FAILED, got %v", name, err)
		}
	}
	// 封面图 + 音频仍然是有效的音频文件
	m, err := ParseProbe([]byte(`{"streams":[{"index":0,"codec_type":"audio","codec_name":"mp3"},{"index":1,"codec_type":"video","codec_name":"mjpeg","disposition":{"attached_pic":1}}],"format":{"duration":"2"}}`), "x.mp3")
	if err != nil || m.HasVideo || !m.HasAudio {
		t.Fatalf("%+v %v", m, err)
	}
}

func TestThumbnailAtSecCapAndFallbackToZero(t *testing.T) {
	e := newEnv(t, nil)
	p := e.video(t, "at.mp4", "-t", "1")
	ctx := context.Background()
	for _, at := range []float64{1e300, 1e7, 1e7 + 1, 500} {
		th, err := e.svc.Thumbnail(ctx, p, at, 120)
		if err != nil {
			t.Fatalf("at=%v: %v", at, err)
		}
		if th.AtSec != 0 {
			t.Fatalf("at=%v: 退回第 0 秒时返回的 AtSec 应为 0, got %v", at, th.AtSec)
		}
	}
	// 缓存名按 0 算：所有回退请求 + 显式请求第 0 秒共用同一个缓存文件
	zero, err := e.svc.Thumbnail(ctx, p, 0, 120)
	if err != nil || zero.AtSec != 0 {
		t.Fatalf("%+v %v", zero, err)
	}
	th, _ := e.svc.Thumbnail(ctx, p, 1e300, 120)
	if th.Path != zero.Path {
		t.Fatalf("回退结果应用 atSec=0 的缓存名: %s vs %s", th.Path, zero.Path)
	}
	if es, _ := os.ReadDir(e.thumbs); len(es) != 1 {
		t.Fatalf("只应有 1 个缓存文件: %d", len(es))
	}
	// 正常时间点保持原样
	ok, err := e.svc.Thumbnail(ctx, p, 0.5, 120)
	if err != nil || ok.AtSec != 0.5 {
		t.Fatalf("%+v %v", ok, err)
	}
}

func TestCacheNameHugeAtDoesNotOverflow(t *testing.T) {
	mt := time.Unix(1700000000, 0)
	a := cacheName("k", mt, 1, maxThumbAt, 100)
	b := cacheName("k", mt, 1, maxThumbAt-1, 100)
	if a == b {
		t.Fatal("上限内的不同值应得到不同缓存名")
	}
	if int64(math.Round(maxThumbAt*1000)) <= 0 {
		t.Fatal("上限换算毫秒不应溢出")
	}
}

// 假 ffmpeg：忽略参数，睡很久；用来验证超时不重试、取消会结束进程。
func fakeSleepBin(t *testing.T, counter string) string {
	p := filepath.Join(t.TempDir(), "ffmpeg")
	os.WriteFile(p, []byte("#!/bin/sh\necho x >> '"+counter+"'\nsleep 30\n"), 0o755)
	return p
}

func TestThumbnailTimeoutIsNotRetried(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell 脚本假 ffmpeg")
	}
	dir := t.TempDir()
	counter := filepath.Join(dir, "count")
	f := filepath.Join(dir, "v.mp4")
	os.WriteFile(f, []byte("x"), 0o644)
	bin := fakeSleepBin(t, counter)
	svc := New(Config{ThumbsDir: filepath.Join(dir, "t"), ThumbTimeout: 300 * time.Millisecond,
		Require: func() (ffmpeg.Binaries, error) { return ffmpeg.Binaries{FFmpeg: bin, FFprobe: bin}, nil }})
	start := time.Now()
	_, err := svc.Thumbnail(context.Background(), f, 5, 100)
	if !apperr.Is(err, apperr.ProcessFailed) {
		t.Fatalf("%v", err)
	}
	b, _ := os.ReadFile(counter)
	if n := strings.Count(string(b), "x"); n != 1 {
		t.Fatalf("超时不应重试，ffmpeg 被启动了 %d 次", n)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("超时耗时过长")
	}
}

func TestRootContextCancelStopsProbeAndThumbnail(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell 脚本假 ffmpeg")
	}
	dir := t.TempDir()
	f := filepath.Join(dir, "v.mp4")
	os.WriteFile(f, []byte("x"), 0o644)
	bin := fakeSleepBin(t, filepath.Join(dir, "c"))
	svc := New(Config{ThumbsDir: filepath.Join(dir, "t"), ThumbTimeout: time.Minute, ProbeTimeout: time.Minute,
		Require: func() (ffmpeg.Binaries, error) { return ffmpeg.Binaries{FFmpeg: bin, FFprobe: bin}, nil }})
	ctx, cancel := context.WithCancel(context.Background())
	errs := make(chan error, 2)
	go func() { _, err := svc.Probe(ctx, []string{f}); errs <- err }()
	go func() { _, err := svc.Thumbnail(ctx, f, 1, 100); errs <- err }()
	time.Sleep(300 * time.Millisecond)
	cancel()
	for i := 0; i < 2; i++ {
		select {
		case err := <-errs:
			if err == nil {
				t.Fatal("取消后应返回错误")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("根 ctx 取消后调用没有返回")
		}
	}
}

func TestThumbnailOnCoverArtOnlyAudioIsInvalidArgument(t *testing.T) {
	e := newEnv(t, nil)
	cover := filepath.Join(e.dir, "c.jpg")
	gen(t, e.bin, cover, "-f", "lavfi", "-i", "testsrc=size=64x64:duration=1", "-frames:v", "1")
	mp3 := filepath.Join(e.dir, "cover.mp3")
	full := []string{"-v", "error", "-y", "-i", cover, "-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-map", "1:a", "-map", "0:v", "-c:a", "libmp3lame", "-c:v", "copy", "-disposition:v", "attached_pic", "-id3v2_version", "3", "file:" + mp3}
	if b, err := exec.Command(e.bin.FFmpeg, full...).CombinedOutput(); err != nil {
		t.Skipf("生成带封面的 mp3 失败: %v\n%s", err, b)
	}
	res, err := e.svc.Probe(context.Background(), []string{mp3})
	if err != nil || res[0].Error != nil || res[0].HasVideo || !res[0].HasAudio || res[0].ThumbURL != "" {
		t.Fatalf("封面图不算视频: %+v %v", res, err)
	}
	if _, err := e.svc.Thumbnail(context.Background(), mp3, 0, 100); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("带封面的 mp3 直接 Thumbnail 应 INVALID_ARGUMENT: %v", err)
	}
}

func TestOutputCapsAndOverlong(t *testing.T) {
	h := &headWriter{max: 10}
	n, err := h.Write([]byte("0123456789abc"))
	if n != 13 || err != nil || !h.over || h.buf.String() != "0123456789" {
		t.Fatalf("%d %v %v %q", n, err, h.over, h.buf.String())
	}
	h.Write([]byte("more"))
	if h.buf.Len() != 10 {
		t.Fatal("超出后不再增长")
	}
	tw := newTailWriter(8)
	for i := 0; i < 100; i++ {
		tw.Write([]byte("0123456789"))
	}
	if s := tw.String(); len(s) != 8 || s != "23456789" {
		t.Fatalf("应保留最后 8 字节: %q", s)
	}
	if len(tw.buf) > 3*8 {
		t.Fatalf("缓冲不应无限增长: %d", len(tw.buf))
	}
}

// 假 ffprobe 输出超过上限的 JSON：不能无限占内存，返回 PROBE_FAILED。
func TestProbeStdoutIsCapped(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell 脚本假 ffprobe")
	}
	dir := t.TempDir()
	f := filepath.Join(dir, "v.mp4")
	os.WriteFile(f, []byte("x"), 0o644)
	bin := filepath.Join(dir, "ffprobe")
	os.WriteFile(bin, []byte("#!/bin/sh\nyes '{\"a\":1}' | head -c 20000000\n"), 0o755)
	svc := New(Config{ThumbsDir: filepath.Join(dir, "t"),
		Require: func() (ffmpeg.Binaries, error) { return ffmpeg.Binaries{FFmpeg: bin, FFprobe: bin}, nil }})
	res, err := svc.Probe(context.Background(), []string{f})
	if err != nil || res[0].Error == nil || res[0].Error.Code != apperr.ProbeFailed {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestPatternTypeNoneForPercentImageNames(t *testing.T) {
	a := probeArgs("/x/a%03d.png")
	idx := -1
	for i, v := range a {
		if v == "-pattern_type" {
			idx = i
		}
	}
	i := len(a) - 2
	if idx < 0 || a[idx+1] != "none" || idx > i {
		t.Fatalf("图片文件名带 %% 时 -pattern_type none 必须在 -i 之前: %v", a)
	}
	th := thumbArgs("/x/a%03d.jpg", "/o/x.part.jpg", 1, 100)
	joined := strings.Join(th, " ")
	if !strings.Contains(joined, "-pattern_type none -i file:/x/a%03d.jpg") {
		t.Fatalf("%v", th)
	}
	// 普通文件名不加（mp4 等解封装器不认识这个选项）
	for _, in := range []string{"/x/a.mp4", "/x/a%d.mp4", "/x/a.png", "/x/a%d.gif"} {
		if strings.Contains(strings.Join(probeArgs(in), " "), "pattern_type") || strings.Contains(strings.Join(thumbArgs(in, "/o", 0, 10), " "), "pattern_type") {
			t.Errorf("%s 不应加 -pattern_type", in)
		}
	}
}

func TestIntegrationPercentFilenameImage(t *testing.T) {
	e := newEnv(t, nil)
	p := filepath.Join(e.dir, "img%03d.png")
	gen(t, e.bin, p, "-f", "lavfi", "-i", "testsrc=size=64x48:duration=1", "-frames:v", "1", "-update", "1")
	if _, err := os.Stat(p); err != nil {
		t.Skip("没生成出带 % 的文件名")
	}
	res, err := e.svc.Probe(context.Background(), []string{p})
	if err != nil || res[0].Error != nil || !res[0].HasVideo {
		t.Fatalf("带 %% 的图片文件应能探测: %+v %v", res, err)
	}
	if _, err := e.svc.Thumbnail(context.Background(), p, 0, 64); err != nil {
		t.Fatalf("带 %% 的图片文件应能生成缩略图: %v", err)
	}
}

func TestCacheCleanupRaceIsTreatedAsMiss(t *testing.T) {
	e := newEnv(t, nil)
	p := e.video(t, "race.mp4", "-t", "1")
	ctx := context.Background()
	first, err := e.svc.Thumbnail(ctx, p, 0.5, 120)
	if err != nil {
		t.Fatal(err)
	}
	// 模拟 lookup 通过后、读取前缓存被清理：并发地反复删除缓存文件，同时反复请求，不应返回错误。
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				os.Remove(first.Path)
				time.Sleep(time.Millisecond)
			}
		}
	}()
	defer close(stop)
	for i := 0; i < 30; i++ {
		th, err := e.svc.Thumbnail(ctx, p, 0.5, 120)
		if err != nil || !strings.HasPrefix(th.DataURL, "data:image/jpeg;base64,") {
			t.Fatalf("清理与读取的竞态应视为未命中并重新生成: #%d %v", i, err)
		}
	}
}

func TestRemoveRecentIDsCap(t *testing.T) {
	e := newEnv(t, nil)
	ids := make([]string, maxRemoveIDs+1)
	if err := e.svc.RemoveRecent(context.Background(), ids); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("超过 500 个 id 应 INVALID_ARGUMENT: %v", err)
	}
	if err := e.svc.RemoveRecent(context.Background(), ids[:maxRemoveIDs]); err != nil {
		t.Fatalf("恰好 500 个应可以: %v", err)
	}
}

func TestNewCommandCancelKillsProcessGroup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell 脚本")
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "child-alive")
	script := filepath.Join(dir, "p.sh")
	// 父 shell 启动一个孙进程，孙进程每 0.1 秒 touch 一次文件；取消后文件不应再更新。
	os.WriteFile(script, []byte("#!/bin/sh\n(while true; do touch '"+marker+"'; sleep 0.1; done) &\nsleep 30\n"), 0o755)
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	cmd := ffmpeg.NewCommand(ctx, script)
	cmd.Run()
	time.Sleep(300 * time.Millisecond)
	fi1, err := os.Stat(marker)
	if err != nil {
		t.Fatal("孙进程没有运行过")
	}
	time.Sleep(500 * time.Millisecond)
	fi2, _ := os.Stat(marker)
	if !fi2.ModTime().Equal(fi1.ModTime()) {
		t.Fatal("超时后孙进程仍在运行：NewCommand 应结束整个进程组")
	}
}

// RemoveRecent 联动撤销预览 token：契约 6.13「被 RemoveRecent 撤销后一律 404」。
func TestRemoveRecentRevokesPreview(t *testing.T) {
	reg := localassets.New(localassets.Config{})
	e := newEnv(t, func(c *Config) {
		c.OnRemoved = func(ps []string) {
			for _, p := range ps {
				reg.RevokePath(p)
			}
		}
	})
	p := e.video(t, "pv.mp4")
	p2 := e.video(t, "pv2.mp4")
	res, _ := e.svc.Probe(context.Background(), []string{p, p2})
	en, err := reg.Register(p)
	if err != nil {
		t.Fatal(err)
	}
	en2, _ := reg.Register(p2)
	code := func(url string) int {
		rec := httptest.NewRecorder()
		reg.Handler().ServeHTTP(rec, httptest.NewRequest("HEAD", url, nil))
		return rec.Code
	}
	if code(en.URL) != 200 {
		t.Fatal("撤销前应 200")
	}
	if err := e.svc.RemoveRecent(context.Background(), []string{res[0].ID}); err != nil {
		t.Fatal(err)
	}
	if code(en.URL) != 404 {
		t.Fatal("RemoveRecent 后旧 URL 应 404")
	}
	if code(en2.URL) != 200 {
		t.Fatal("没被移除的文件预览不应受影响")
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatal("文件不应被删除")
	}
}

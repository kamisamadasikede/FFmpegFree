package convert

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/localassets"
	"FFmpegFree/internal/paths"
	"FFmpegFree/internal/service/media"
	"FFmpegFree/internal/store"
	"FFmpegFree/internal/task"
)

// countingInspector 数一数探测了几次（验证“文件没变不重探”）。
type countingInspector struct {
	inner Inspector
	n     atomic.Int64
}

func (c *countingInspector) Inspect(ctx context.Context, p string) (store.MediaInfo, error) {
	c.n.Add(1)
	return c.inner.Inspect(ctx, p)
}

// app 模拟一次“应用运行”：打开数据库、任务管理器和转换服务；stop() 相当于退出应用。
type app struct {
	st  *store.Store
	tm  *task.Manager
	svc *Service
	ins *countingInspector
	med *media.Service
}

func startApp(t *testing.T, bin ffmpeg.Binaries, dir string) *app {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	tm := task.NewManager(task.Config{Store: st, Emitter: &rec{prog: map[string][]float64{}}, LogDir: filepath.Join(dir, "logs"),
		BatchConcurrency: 2, ProgressInterval: -1, Logf: func(string, ...any) {}})
	req := func() (ffmpeg.Binaries, error) { return bin, nil }
	med := media.New(media.Config{Require: req, ThumbsDir: filepath.Join(dir, "thumbs"), Store: st,
		OnProbed: func(ctx context.Context, key string, m store.MediaInfo, fi os.FileInfo) {
			st.SetConvertSourceMediaByKey(ctx, key, store.FileFingerprint(fi), &m)
		}})
	ins := &countingInspector{inner: med}
	svc, err := New(context.Background(), Config{Presets: st, Media: ins, Thumbs: med, Tasks: tm, Require: req,
		Preview: localassets.New(localassets.Config{})})
	if err != nil {
		t.Fatal(err)
	}
	return &app{st: st, tm: tm, svc: svc, ins: ins, med: med}
}

func (a *app) stop() {
	a.tm.Shutdown(3 * time.Second)
	a.st.Close()
}

func sourceByID(t *testing.T, page ConvertSourcePage, id string) ConvertSource {
	t.Helper()
	for _, it := range page.Items {
		if it.Source.SourceID == id {
			return it.Source
		}
	}
	t.Fatalf("没有这一行 %s", id)
	return ConvertSource{}
}

// 走查 G3：AddSources 就把完整的媒体信息落库，重启后音频行仍有采样率和声道，无声视频仍是 hasAudio=false（“没有声音”），
// 而且重启后不需要重新探测。
func TestSourceMediaSurvivesRestart(t *testing.T) {
	e := newEnv(t) // 只用来生成测试文件
	dir := t.TempDir()
	wav := e.gen(t, filepath.Join(dir, "podcast.wav"), "-f", "lavfi", "-i", "sine=frequency=440:duration=1:sample_rate=48000", "-ac", "2", "-c:a", "pcm_s16le")
	silent := e.gen(t, filepath.Join(dir, "silent.mp4"), "-f", "lavfi", "-i", "testsrc=size=160x120:rate=10:duration=1", "-c:v", "mpeg4", "-an")
	ctx := context.Background()

	a := startApp(t, e.bin, dir)
	res, err := a.svc.AddSources(ctx, []string{wav, silent, wav})
	if err != nil {
		t.Fatal(err)
	}
	if a.ins.n.Load() != 2 {
		t.Fatalf("同一次调用里重复的文件只探测一次: %d", a.ins.n.Load())
	}
	for _, r := range res {
		if r.Error != nil || r.Source == nil || r.Source.Media == nil {
			t.Fatalf("AddSources 应带上媒体信息: %+v", r)
		}
	}
	if res[0].Source == res[2].Source {
		t.Fatal("重复项不共享指针")
	}
	wavID, silentID := res[0].Source.SourceID, res[1].Source.SourceID
	a.stop()

	b := startApp(t, e.bin, dir) // 重启
	defer b.stop()
	page, err := b.svc.ListSources(ctx, ConvertSourceFilter{})
	if err != nil {
		t.Fatal(err)
	}
	w := sourceByID(t, page, wavID).Media
	if w == nil || !w.HasAudio || w.HasVideo || w.SampleRate != 48000 || w.Channels != 2 || w.AudioCodec != "pcm_s16le" ||
		w.AudioCodecName != "PCM" || w.Duration <= 0 || len(w.Streams) == 0 {
		t.Fatalf("重启后音频行信息不全: %+v", w)
	}
	sv := sourceByID(t, page, silentID).Media
	if sv == nil || !sv.HasVideo || sv.HasAudio || sv.Width != 160 || sv.Height != 120 || sv.VideoCodecName != "MPEG-4" {
		t.Fatalf("重启后无声视频应仍是 hasAudio=false: %+v", sv)
	}
	g, err := b.svc.GetSource(ctx, wavID)
	if err != nil || g.Source.Media == nil || g.Source.Media.SampleRate != 48000 {
		t.Fatalf("GetSource: %+v %v", g.Source.Media, err)
	}
	if n := b.ins.n.Load(); n != 0 {
		t.Fatalf("文件没变，重启后不应重新探测: %d 次", n)
	}

	// 文件被替换（大小 / 修改时间变了）：下次列表时重探
	e.gen(t, wav, "-f", "lavfi", "-i", "sine=frequency=440:duration=2:sample_rate=22050", "-ac", "1", "-c:a", "pcm_s16le")
	future := time.Now().Add(time.Minute)
	os.Chtimes(wav, future, future)
	page, _ = b.svc.ListSources(ctx, ConvertSourceFilter{})
	if w := sourceByID(t, page, wavID).Media; w == nil || w.SampleRate != 22050 || w.Channels != 1 || b.ins.n.Load() != 1 {
		t.Fatalf("文件变了应重探: %+v (%d 次)", w, b.ins.n.Load())
	}
}

// 旧行（v0.23.4 之前加的、没有持久化结果）在 ListSources / GetSource 时懒探测补上并落库；文件不在时不探测、退回 media 表；
// 探测失败记下标记，文件不变就不再重探；MediaService.Probe 成功也会刷新同一文件的行。
func TestSourceMediaLazyBackfill(t *testing.T) {
	e := newEnv(t)
	dir := t.TempDir()
	ctx := context.Background()
	in := e.genVideo(t, filepath.Join(dir, "old.mp4"), 1)
	gone := filepath.Join(dir, "gone.mp4")
	bad := filepath.Join(dir, "bad.mp4")
	os.WriteFile(bad, []byte("not a video"), 0o644)

	a := startApp(t, e.bin, dir)
	defer a.stop()
	// 模拟升级前的旧行：直接建行，不带媒体信息
	old, _, _ := a.st.UpsertConvertSource(ctx, in, mustKey(t, in), 1)
	gk := mustKey(t, gone)
	goneRow, _, _ := a.st.UpsertConvertSource(ctx, gone, gk, 2)
	a.st.UpsertMedia(ctx, gk, store.MediaInfo{ID: "M1", Path: gone, Name: "gone.mp4", VideoCodec: "h264", AudioCodec: "aac", Width: 64, Height: 48})
	badRow, _, _ := a.st.UpsertConvertSource(ctx, bad, mustKey(t, bad), 3)

	page, err := a.svc.ListSources(ctx, ConvertSourceFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if m := sourceByID(t, page, old.SourceID).Media; m == nil || !m.HasVideo || !m.HasAudio || m.SampleRate == 0 || m.Channels == 0 {
		t.Fatalf("旧行应懒探测补上: %+v", m)
	}
	if got, _ := a.st.GetConvertSource(ctx, old.SourceID); got.Media == nil || got.MediaFP == "" {
		t.Fatalf("补上的结果要落库: %+v", got)
	}
	if m := sourceByID(t, page, goneRow.SourceID).Media; m == nil || m.Width != 64 || !m.HasVideo || !m.HasAudio || m.VideoCodecName != "H.264" {
		t.Fatalf("文件不在：退回 media 表关联: %+v", m)
	}
	if m := sourceByID(t, page, badRow.SourceID).Media; m != nil {
		t.Fatalf("探测失败没有 media: %+v", m)
	}
	n := a.ins.n.Load()
	if n != 2 {
		t.Fatalf("只探测在的两个文件: %d", n)
	}
	a.svc.ListSources(ctx, ConvertSourceFilter{})
	a.svc.GetSource(ctx, badRow.SourceID)
	if a.ins.n.Load() != n {
		t.Fatalf("文件没变不重探（含探测失败的）: %d", a.ins.n.Load())
	}

	// MediaService.Probe 刷新同一文件的行：先把行里的结果清掉，再 Probe
	a.st.DB().Exec(`UPDATE convert_sources SET media = NULL, media_fp = '' WHERE id = ?`, old.SourceID)
	if _, err := a.med.Probe(ctx, []string{in}); err != nil {
		t.Fatal(err)
	}
	if got, _ := a.st.GetConvertSource(ctx, old.SourceID); got.Media == nil || !got.Media.HasAudio || got.MediaFP == "" || got.Media.ThumbURL != "" {
		t.Fatalf("Probe 后源文件行应被刷新（不带 thumbUrl）: %+v", got.Media)
	}
}

func mustKey(t *testing.T, p string) string {
	t.Helper()
	_, k, err := pathsNormalize(p)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// 走查 G4：扩展名在白名单里、但编码 WebView 解不了的文件（ProRes / FFV1 / DNxHD 视频、AC-3 / MP2 纯音频）
// 预览返回 UNSUPPORTED（reason=format）；能播的照常给地址。任务的输入 / 输出同样按编码挡。
func TestPreviewGatedByCodec(t *testing.T) {
	e := newEnv(t)
	dir := t.TempDir()
	ctx := context.Background()
	vid := func(name, codec string, extra ...string) string {
		args := append([]string{"-f", "lavfi", "-i", "testsrc=size=160x120:rate=10:duration=1", "-c:v", codec}, extra...)
		return e.gen(t, filepath.Join(dir, name), args...)
	}
	aud := func(name, codec string, extra ...string) string {
		args := append([]string{"-f", "lavfi", "-i", "sine=frequency=440:duration=1", "-c:a", codec}, extra...)
		return e.gen(t, filepath.Join(dir, name), args...)
	}
	a := startApp(t, e.bin, dir)
	defer a.stop()
	cases := []struct {
		path string
		ok   bool
	}{
		{vid("prores.mov", "prores_ks"), false},
		{vid("ffv1.mkv", "ffv1"), false},
		{vid("dnxhd.mov", "dnxhd", "-s", "1920x1080", "-r", "30000/1001", "-b:v", "145M", "-pix_fmt", "yuv422p", "-t", "0.2"), false},
		{vid("mpeg4.mp4", "mpeg4"), true},
		{aud("ac3.wav", "ac3"), false},
		{aud("mp2.mp3", "mp2", "-f", "mp2"), false},
		{aud("pcm.wav", "pcm_s16le"), true},
		{aud("lame.mp3", "libmp3lame"), true},
	}
	var prores ConvertSource
	for i, c := range cases {
		src := firstSource(t, a.svc, c.path)
		if i == 0 {
			prores = src
		}
		u, err := a.svc.GetSourcePreviewURL(ctx, src.SourceID)
		if c.ok {
			if err != nil || !strings.HasPrefix(u.URL, "/local/") {
				t.Errorf("%s 应可预览: %v", filepath.Base(c.path), err)
			}
			continue
		}
		if !apperr.Is(err, apperr.Unsupported) || detailOf(err) != "reason=format" {
			t.Errorf("%s 应 UNSUPPORTED reason=format: %v (%q)", filepath.Base(c.path), err, detailOf(err))
		}
	}
	// 任务中心：ProRes 输入不能预览，转出来的 MP4 可以
	ts, err := a.svc.SubmitSources(ctx, ConvertSubmitRequest{SourceIDs: []string{prores.SourceID},
		Options: ffmpeg.ConvertOptions{Container: "mp4", VideoCodec: "h264", AudioCodec: "aac"}, OutputDir: filepath.Join(dir, "o")})
	if err != nil {
		t.Fatal(err)
	}
	tk, err := a.tm.Wait(ctx, ts[0].ID)
	if err != nil || tk.Status != task.StatusSucceeded {
		t.Skipf("转换失败（编码器缺失？）: %+v %v", tk.Error, err)
	}
	_, err = a.svc.TaskPreviewURL(ctx, tk.ID, "input")
	wantReason(t, err, apperr.Unsupported, "reason=format")
	if u, err := a.svc.TaskPreviewURL(ctx, tk.ID, "output"); err != nil || !strings.HasPrefix(u.URL, "/local/") {
		t.Fatalf("H.264 输出可以预览: %v", err)
	}
}

func firstSource(t *testing.T, svc *Service, p string) ConvertSource {
	t.Helper()
	res, err := svc.AddSources(context.Background(), []string{p})
	if err != nil || res[0].Source == nil {
		t.Fatalf("%+v %v", res, err)
	}
	return *res[0].Source
}

// 编码门控名单本身（只在 playable.go 一处）。
func TestUnplayableCodec(t *testing.T) {
	v := func(c string) *store.MediaInfo {
		return &store.MediaInfo{HasVideo: true, VideoCodec: c, HasAudio: true, AudioCodec: "ac3"}
	}
	au := func(c string) *store.MediaInfo { return &store.MediaInfo{HasAudio: true, AudioCodec: c} }
	for _, c := range []string{"prores", "PRORES", "dnxhd", "cfhd", "ffv1", "hap", "qtrle", "rawvideo", "utvideo", "mpeg2video",
		"msmpeg4v3", "wmv3", "vc1", "mjpeg", "huffyuv"} {
		if unplayableCodec(v(c)) == "" {
			t.Errorf("视频 %s 应被挡", c)
		}
	}
	for _, c := range []string{"h264", "hevc", "vp8", "vp9", "av1", "mpeg4", "theora", "gif"} {
		if unplayableCodec(v(c)) != "" {
			t.Errorf("视频 %s 不应被挡（有画面时不看音频编码）", c)
		}
	}
	for _, c := range []string{"wmav2", "ape", "amr_nb", "ac3", "eac3", "dts", "mp2", "wavpack"} {
		if unplayableCodec(au(c)) == "" {
			t.Errorf("音频 %s 应被挡", c)
		}
	}
	for _, c := range []string{"aac", "mp3", "flac", "opus", "vorbis", "pcm_s16le", "pcm_f32le"} {
		if unplayableCodec(au(c)) != "" {
			t.Errorf("音频 %s 不应被挡", c)
		}
	}
	if unplayableCodec(nil) != "" {
		t.Error("没探测到时不挡")
	}
}

// 走查 X8：提交时源文件读不了，message 带文件名、不带问号。
func TestSubmitUnreadableSourceMessage(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("需要非 root 的 Unix 权限")
	}
	e := newEnv(t)
	dir := t.TempDir()
	in := e.genVideo(t, filepath.Join(dir, "婚礼-初剪.mp4"), 1)
	a := startApp(t, e.bin, dir)
	defer a.stop()
	src := firstSource(t, a.svc, in)
	os.Chmod(in, 0)
	defer os.Chmod(in, 0o644)
	_, err := a.svc.SubmitSources(context.Background(), ConvertSubmitRequest{SourceIDs: []string{src.SourceID},
		Options: ffmpeg.ConvertOptions{Container: "mkv", VideoCodec: "copy", AudioCodec: "copy"}, OutputDir: filepath.Join(dir, "o")})
	ae := apperr.From(err)
	if !apperr.Is(err, apperr.IOError) || ae.Message != "无法读取文件“婚礼-初剪.mp4”，可能没有读取权限。" {
		t.Fatalf("%v", err)
	}
	if strings.ContainsAny(ae.Message, "?？") || !strings.HasPrefix(ae.Detail, in+"\n") {
		t.Fatalf("message 不带问号；detail 第一行是路径: %+v", ae)
	}
}

var pathsNormalize = paths.Normalize

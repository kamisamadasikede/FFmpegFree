package convert

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/service/media"
	"FFmpegFree/internal/store"
	"FFmpegFree/internal/task"
)

type rec struct {
	mu   sync.Mutex
	prog map[string][]float64
}

func (r *rec) Emit(name string, p any) {
	if pe, ok := p.(task.ProgressEvent); ok {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.prog[pe.ID] = append(r.prog[pe.ID], pe.Progress)
	}
}
func (r *rec) get(id string) []float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]float64(nil), r.prog[id]...)
}

type env struct {
	svc  *Service
	tm   *task.Manager
	st   *store.Store
	bin  ffmpeg.Binaries
	dir  string
	em   *rec
	defA string // DefaultOutputDir 返回值
}

func realBins(t *testing.T) ffmpeg.Binaries {
	t.Helper()
	fm, err1 := exec.LookPath("ffmpeg")
	fp, err2 := exec.LookPath("ffprobe")
	if err1 != nil || err2 != nil {
		t.Skip("没有 ffmpeg / ffprobe，跳过集成测试")
	}
	return ffmpeg.Binaries{FFmpeg: fm, FFprobe: fp}
}

func newEnv(t *testing.T) *env { t.Helper(); return newEnvWith(t, nil) }

// newEnvWith 同 newEnv，mod 可以在 New 之前改 Config（v0.24 的上传目录、启动时中断的重转条数等）。
func newEnvWith(t *testing.T, mod func(*Config)) *env {
	t.Helper()
	bin := realBins(t)
	dir := t.TempDir()
	st, err := store.Open(context.Background(), filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	e := &env{st: st, bin: bin, dir: dir, em: &rec{prog: map[string][]float64{}}}
	e.tm = task.NewManager(task.Config{Store: st, Emitter: e.em, LogDir: filepath.Join(dir, "logs"), BatchConcurrency: 2,
		ProgressInterval: -1, Logf: func(string, ...any) {}})
	t.Cleanup(func() { e.tm.Shutdown(3 * time.Second) })
	req := func() (ffmpeg.Binaries, error) { return bin, nil }
	med := media.New(media.Config{Require: req, ThumbsDir: filepath.Join(dir, "thumbs")})
	cfg := Config{Presets: st, Media: med, Tasks: e.tm, Require: req,
		DefaultOutputDir: func(context.Context) string { return e.defA }}
	if mod != nil {
		mod(&cfg)
	}
	e.svc, err = New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func (e *env) gen(t *testing.T, out string, args ...string) string {
	t.Helper()
	full := append([]string{"-v", "error", "-y"}, args...)
	full = append(full, "file:"+out)
	if b, err := exec.Command(e.bin.FFmpeg, full...).CombinedOutput(); err != nil {
		t.Skipf("生成测试媒体失败（编码器缺失？）: %v\n%s", err, b)
	}
	return out
}

// 生成 secs 秒的测试视频（带音频）。
func (e *env) genVideo(t *testing.T, out string, secs int) string {
	return e.gen(t, out, "-f", "lavfi", "-i", "testsrc=size=320x240:rate=25:duration="+strconv.Itoa(secs),
		"-f", "lavfi", "-i", "sine=frequency=440:duration="+strconv.Itoa(secs), "-c:v", "mpeg4", "-c:a", "aac", "-shortest")
}

func (e *env) wait(t *testing.T, id string) task.Task {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	tk, err := e.tm.Wait(ctx, id)
	if err != nil {
		t.Fatalf("等待任务: %v", err)
	}
	return tk
}

func (e *env) probe(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := exec.Command(e.bin.FFprobe, "-v", "error", "-print_format", "json", "-show_format", "-show_streams", "file:"+path).Output()
	if err != nil {
		t.Fatalf("输出文件无法被 ffprobe 解析: %v", err)
	}
	var m map[string]any
	json.Unmarshal(b, &m)
	return m
}

func codecs(m map[string]any) []string {
	var out []string
	for _, s := range m["streams"].([]any) {
		sm := s.(map[string]any)
		out = append(out, sm["codec_type"].(string)+":"+sm["codec_name"].(string))
	}
	return out
}

func mustSubmit(t *testing.T, e *env, in []string, o ffmpeg.ConvertOptions, dir string) []task.Task {
	t.Helper()
	ts, err := e.svc.Submit(context.Background(), in, o, dir)
	if err != nil {
		t.Fatal(err)
	}
	return ts
}

func TestConvertFormats(t *testing.T) {
	e := newEnv(t)
	in := e.genVideo(t, filepath.Join(e.dir, "src.mp4"), 2)
	out := filepath.Join(e.dir, "out")
	cases := []struct {
		name string
		o    ffmpeg.ConvertOptions
		ext  string
		want []string
	}{
		{"mp4 h264", ffmpeg.ConvertOptions{Container: "mp4", VideoCodec: "h264", AudioCodec: "aac"}, "mp4", []string{"video:h264", "audio:aac"}},
		{"webm vp9", ffmpeg.ConvertOptions{Container: "webm", VideoCodec: "vp9", AudioCodec: "opus"}, "webm", []string{"video:vp9", "audio:opus"}},
		{"mp3", ffmpeg.ConvertOptions{Container: "mp3", AudioCodec: "mp3", AudioBitrate: 128000}, "mp3", []string{"audio:mp3"}},
		{"gif", ffmpeg.ConvertOptions{Container: "gif", Width: 160, Fps: 10}, "gif", []string{"video:gif"}},
		{"mkv copy", ffmpeg.ConvertOptions{Container: "mkv", VideoCodec: "copy", AudioCodec: "copy"}, "mkv", []string{"video:mpeg4", "audio:aac"}},
		{"缩放", ffmpeg.ConvertOptions{Container: "mp4", VideoCodec: "h264", Width: 160}, "mp4", []string{"video:h264", "audio:aac"}},
	}
	for _, c := range cases {
		dir := filepath.Join(out, strings.ReplaceAll(c.name, " ", "_"))
		ts := mustSubmit(t, e, []string{in}, c.o, dir)
		if len(ts) != 1 || ts[0].Type != task.TypeConvert {
			t.Fatalf("%s: %+v", c.name, ts)
		}
		d := e.wait(t, ts[0].ID)
		if d.Status != task.StatusSucceeded {
			t.Fatalf("%s: %+v", c.name, d)
		}
		if want := filepath.Join(dir, "src."+c.ext); d.OutputPath != want {
			t.Fatalf("%s: 输出路径 %s != %s", c.name, d.OutputPath, want)
		}
		got := codecs(e.probe(t, d.OutputPath))
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Fatalf("%s: 流 %v, want %v", c.name, got, c.want)
		}
		if d.Progress != 1 {
			t.Fatalf("%s: 成功后进度应为 1: %v", c.name, d.Progress)
		}
		if ents, _ := filepath.Glob(filepath.Join(dir, "*.part.*")); len(ents) != 0 {
			t.Fatalf("%s: 残留 .part: %v", c.name, ents)
		}
	}
	// 缩放结果确实是 160 宽
	if m := e.probe(t, filepath.Join(out, "缩放", "src.mp4")); m["streams"].([]any)[0].(map[string]any)["width"].(float64) != 160 {
		t.Fatalf("缩放没生效: %v", m["streams"])
	}
}

func TestConvertProgressMonotonicToOne(t *testing.T) {
	e := newEnv(t)
	// 足够大、用 h265 编码，保证运行时间超过 ffmpeg 的进度输出间隔（0.5 秒）
	in := e.gen(t, filepath.Join(e.dir, "p.mp4"), "-f", "lavfi", "-i", "testsrc2=size=1280x720:rate=30:duration=12", "-c:v", "mpeg4", "-q:v", "3")
	ts := mustSubmit(t, e, []string{in}, ffmpeg.ConvertOptions{Container: "mp4", VideoCodec: "h265", AudioCodec: "none"}, "")
	d := e.wait(t, ts[0].ID)
	if d.Status != task.StatusSucceeded {
		t.Fatalf("%+v", d)
	}
	p := e.em.get(ts[0].ID)
	if len(p) < 2 {
		t.Fatalf("应有多次进度: %v", p)
	}
	for i := 1; i < len(p); i++ {
		if p[i] < p[i-1] {
			t.Fatalf("进度回退: %v", p)
		}
	}
	if p[len(p)-1] != 1 {
		t.Fatalf("最后一次进度应为 1: %v", p)
	}
	// 输出目录为空 → 源文件同目录，重名 " (1)"（转换用带空格的序号，契约 6.14.5）
	if d.OutputPath != filepath.Join(e.dir, "p (1).mp4") {
		t.Fatalf("输出到源文件同目录且不覆盖输入: %s", d.OutputPath)
	}
}

func TestConvertNameCollisionsAndSpecialNames(t *testing.T) {
	e := newEnv(t)
	base := e.genVideo(t, filepath.Join(e.dir, "base.mp4"), 1)
	names := []string{"my video.mp4", "-leading dash.mp4", "中文 名字.mp4", "a:b'c&d.mp4", "100%.mp4"}
	var ins []string
	for _, n := range names {
		p := filepath.Join(e.dir, "in", n)
		os.MkdirAll(filepath.Dir(p), 0o755)
		b, _ := os.ReadFile(base)
		os.WriteFile(p, b, 0o644)
		ins = append(ins, p)
	}
	outDir := filepath.Join(e.dir, "o")
	o := ffmpeg.ConvertOptions{Container: "mkv", VideoCodec: "copy", AudioCodec: "copy"}
	first := mustSubmit(t, e, ins, o, outDir)
	for _, tk := range first {
		if d := e.wait(t, tk.ID); d.Status != task.StatusSucceeded {
			t.Fatalf("%+v", d)
		}
	}
	// 再提交一次：全部重名 → " (1)"，第一批输出原样保留
	second := mustSubmit(t, e, ins, o, outDir)
	for i, tk := range second {
		d := e.wait(t, tk.ID)
		stem := strings.TrimSuffix(names[i], ".mp4")
		if d.Status != task.StatusSucceeded || d.OutputPath != filepath.Join(outDir, stem+" (1).mkv") {
			t.Fatalf("%s: %+v", names[i], d)
		}
		if _, err := os.Stat(filepath.Join(outDir, stem+".mkv")); err != nil {
			t.Fatalf("第一批输出被破坏: %v", err)
		}
	}
	// 同批里同名输入（不同目录）→ 不冲突
	a := filepath.Join(e.dir, "d1", "same.mp4")
	b := filepath.Join(e.dir, "d2", "same.mp4")
	for _, p := range []string{a, b} {
		os.MkdirAll(filepath.Dir(p), 0o755)
		data, _ := os.ReadFile(base)
		os.WriteFile(p, data, 0o644)
	}
	ts := mustSubmit(t, e, []string{a, b}, o, filepath.Join(e.dir, "same-out"))
	outs := map[string]bool{}
	for _, tk := range ts {
		d := e.wait(t, tk.ID)
		if d.Status != task.StatusSucceeded {
			t.Fatalf("%+v", d)
		}
		outs[d.OutputPath] = true
	}
	if len(outs) != 2 {
		t.Fatalf("同批同名输入应得到两个不同的输出: %v", outs)
	}
}

func TestConvertCancelLeavesNothing(t *testing.T) {
	e := newEnv(t)
	in := e.gen(t, filepath.Join(e.dir, "long.mp4"), "-f", "lavfi", "-i", "testsrc2=size=1280x720:rate=30:duration=120", "-c:v", "mpeg4", "-q:v", "3")
	outDir := filepath.Join(e.dir, "co")
	ts := mustSubmit(t, e, []string{in}, ffmpeg.ConvertOptions{Container: "mp4", VideoCodec: "h265", AudioCodec: "none"}, outDir)
	id := ts[0].ID
	deadline := time.Now().Add(20 * time.Second)
	for {
		p := e.em.get(id)
		if len(p) > 0 && p[len(p)-1] > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("等不到进度")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := e.tm.Cancel(id); err != nil {
		t.Fatal(err)
	}
	d := e.wait(t, id)
	if d.Status != task.StatusCanceled {
		t.Fatalf("%+v", d)
	}
	ents, _ := os.ReadDir(outDir)
	for _, x := range ents {
		t.Fatalf("取消后输出目录应为空（无最终文件也无 .part）: %s", x.Name())
	}
}

func TestConvertFailureAndRetry(t *testing.T) {
	e := newEnv(t)
	in := e.genVideo(t, filepath.Join(e.dir, "r.mp4"), 1)
	// 输出目录被一个"文件"占用 → 任务失败（IO_ERROR）
	blocker := filepath.Join(e.dir, "blocker")
	os.WriteFile(blocker, []byte("x"), 0o644)
	badDir := filepath.Join(blocker, "sub") // 父级是文件，创建目录必然失败
	ts := mustSubmit(t, e, []string{in}, ffmpeg.ConvertOptions{Container: "mp4", VideoCodec: "h264", AudioCodec: "aac"}, badDir)
	d := e.wait(t, ts[0].ID)
	if d.Status != task.StatusFailed || d.Error == nil {
		t.Fatalf("%+v", d)
	}
	// 修复条件后重试：把 blocker 换成目录
	os.Remove(blocker)
	nt, err := e.tm.Retry(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if nt.ID != d.ID {
		t.Fatal("原地重试应沿用同一个 id")
	}
	d2 := e.wait(t, nt.ID)
	if d2.Status != task.StatusSucceeded || d2.OutputPath != filepath.Join(badDir, "r.mp4") {
		t.Fatalf("%+v", d2)
	}
	e.probe(t, d2.OutputPath)
	// 成功后不能再 Retry
	if _, err := e.tm.Retry(d2.ID); !apperr.Is(err, apperr.TaskConflict) {
		t.Fatalf("succeeded 重试应 TASK_CONFLICT: %v", err)
	}
	// 输入文件被删除后重试：工厂重新探测，失败为 NOT_FOUND，记录不变
	markFailed(t, e.st, d2.ID)
	os.Remove(in)
	if _, err := e.tm.Retry(d2.ID); !apperr.Is(err, apperr.NotFound) {
		t.Fatalf("输入已删除，重试应 NOT_FOUND: %v", err)
	}
}

func TestSubmitValidation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	video := e.genVideo(t, filepath.Join(e.dir, "v.mp4"), 1)
	silent := e.gen(t, filepath.Join(e.dir, "s.mp4"), "-f", "lavfi", "-i", "testsrc=size=160x120:rate=10:duration=1", "-c:v", "mpeg4")
	txt := filepath.Join(e.dir, "n.txt")
	os.WriteFile(txt, []byte("not media"), 0o644)
	ok := ffmpeg.ConvertOptions{Container: "mp4", VideoCodec: "h264", AudioCodec: "aac"}

	code := func(name string, err error, want apperr.Code) {
		t.Helper()
		if !apperr.Is(err, want) {
			t.Errorf("%s: 期望 %s, got %v", name, want, err)
		}
	}
	_, err := e.svc.Submit(ctx, nil, ok, "")
	code("空列表", err, apperr.InvalidArgument)
	many := make([]string, MaxInputsPerSubmit+1)
	for i := range many {
		many[i] = video
	}
	_, err = e.svc.Submit(ctx, many, ok, "")
	code("超过 50 个", err, apperr.InvalidArgument)
	_, err = e.svc.Submit(ctx, []string{video}, ffmpeg.ConvertOptions{Container: "mp4", VideoCodec: "h264", TargetSizeMB: 5}, "")
	code("目标大小暂缓", err, apperr.InvalidArgument)
	_, err = e.svc.Submit(ctx, []string{video}, ffmpeg.ConvertOptions{Container: "exe"}, "")
	code("未知容器", err, apperr.InvalidArgument)
	_, err = e.svc.Submit(ctx, []string{video}, ok, "relative/out")
	code("相对输出目录", err, apperr.InvalidArgument)
	_, err = e.svc.Submit(ctx, []string{video}, ok, video)
	code("输出目录是文件", err, apperr.InvalidArgument)
	_, err = e.svc.Submit(ctx, []string{video, filepath.Join(e.dir, "nope.mp4")}, ok, "")
	code("文件不存在", err, apperr.NotFound)
	_, err = e.svc.Submit(ctx, []string{video, txt}, ok, "")
	code("不是媒体文件", err, apperr.ProbeFailed)
	_, err = e.svc.Submit(ctx, []string{silent}, ffmpeg.ConvertOptions{Container: "mp3", AudioCodec: "mp3"}, "")
	code("无音轨转 mp3", err, apperr.InvalidArgument)
	_, err = e.svc.Submit(ctx, []string{e.dir}, ok, "")
	code("目录", err, apperr.InvalidArgument)
	// 校验失败时一个任务都不该提交
	if p, _ := e.tm.List(task.Filter{}); len(p.Items) != 0 {
		t.Fatalf("校验失败不应产生任务: %d", len(p.Items))
	}
	// 错误 detail 指出是哪个文件
	_, err = e.svc.Submit(ctx, []string{video, txt}, ok, "")
	if ae := apperr.From(err); !strings.Contains(ae.Detail, "n.txt") {
		t.Errorf("detail 应包含出错的文件: %q", ae.Detail)
	}
	// 无音轨输入转视频容器：静默无音频，可以成功
	ts := mustSubmit(t, e, []string{silent}, ok, filepath.Join(e.dir, "silent-out"))
	if d := e.wait(t, ts[0].ID); d.Status != task.StatusSucceeded {
		t.Fatalf("%+v", d)
	}
	// ffmpeg 缺失
	e2 := newEnv(t)
	e2.svc.cfg.Require = func() (ffmpeg.Binaries, error) { return ffmpeg.Binaries{}, apperr.New(apperr.FFmpegNotFound, "x") }
	_, err = e2.svc.Submit(ctx, []string{video}, ok, "")
	code("ffmpeg 缺失", err, apperr.FFmpegNotFound)
}

func TestDefaultOutputDirUsedWhenEmpty(t *testing.T) {
	e := newEnv(t)
	in := e.genVideo(t, filepath.Join(e.dir, "d.mp4"), 1)
	o := ffmpeg.ConvertOptions{Container: "mkv", VideoCodec: "copy", AudioCodec: "copy"}
	e.defA = filepath.Join(e.dir, "default-out")
	ts := mustSubmit(t, e, []string{in}, o, "")
	d := e.wait(t, ts[0].ID)
	if d.OutputPath != filepath.Join(e.defA, "d.mkv") {
		t.Fatalf("outputDir 为空应用默认输出目录: %s", d.OutputPath)
	}
	// 显式 outputDir 优先
	explicit := filepath.Join(e.dir, "explicit")
	ts = mustSubmit(t, e, []string{in}, o, explicit)
	if d := e.wait(t, ts[0].ID); d.OutputPath != filepath.Join(explicit, "d.mkv") {
		t.Fatalf("%s", d.OutputPath)
	}
	// 默认目录也为空 → 源文件同目录
	e.defA = ""
	ts = mustSubmit(t, e, []string{in}, o, "")
	if d := e.wait(t, ts[0].ID); d.OutputPath != filepath.Join(e.dir, "d.mkv") {
		t.Fatalf("%s", d.OutputPath)
	}
	// Params 里保存的是已解析的目录，之后改设置不影响重试
	e.defA = filepath.Join(e.dir, "changed")
	tk := ts[0]
	got, _ := e.tm.Get(tk.ID)
	if !strings.Contains(got.Params, `"outputDir":""`) {
		t.Fatalf("同目录输出应记录为空目录: %s", got.Params)
	}
}

func TestPresetsCRUD(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	list, err := e.svc.ListPresets(ctx)
	if err != nil || len(list) != len(builtinPresets()) {
		t.Fatalf("%d %v", len(list), err)
	}
	for _, p := range list {
		if !p.BuiltIn {
			t.Fatalf("初始只有内置: %+v", p)
		}
		if err := ffmpeg.ValidateConvertOptions(p.Options); err != nil {
			t.Errorf("内置预设 %s 参数不合法: %v", p.ID, err)
		}
	}
	// 重复初始化不产生重复
	if _, err := New(ctx, Config{Presets: e.st}); err != nil {
		t.Fatal(err)
	}
	if l, _ := e.svc.ListPresets(ctx); len(l) != len(list) {
		t.Fatal("重复 seed 不应增加")
	}
	np, err := e.svc.SavePreset(ctx, Preset{Name: "  我的 mkv  ", BuiltIn: true, Options: ffmpeg.ConvertOptions{Container: "mkv", VideoCodec: "h264"}})
	if err != nil || np.ID == "" || np.BuiltIn || np.Name != "我的 mkv" {
		t.Fatalf("%+v %v", np, err)
	}
	np.Name = "改名"
	np.Options.Crf = 20
	up, err := e.svc.SavePreset(ctx, np)
	if err != nil || up.ID != np.ID || up.Name != "改名" || up.Options.Crf != 20 {
		t.Fatalf("%+v %v", up, err)
	}
	l, _ := e.svc.ListPresets(ctx)
	if last := l[len(l)-1]; last.ID != np.ID || last.Options.Crf != 20 {
		t.Fatalf("用户预设在内置之后: %+v", last)
	}
	bad := map[string]Preset{
		"空名":     {Name: " ", Options: ffmpeg.ConvertOptions{Container: "mp4", VideoCodec: "h264"}},
		"名字过长":   {Name: strings.Repeat("长", 61), Options: ffmpeg.ConvertOptions{Container: "mp4", VideoCodec: "h264"}},
		"参数不合法":  {Name: "x", Options: ffmpeg.ConvertOptions{Container: "mp4", VideoCodec: "vp9x"}},
		"目标大小暂缓": {Name: "x", Options: ffmpeg.ConvertOptions{Container: "mp4", VideoCodec: "h264", TargetSizeMB: 5}},
		"改内置":    {ID: "builtin-mp4-h264", Name: "x", Options: ffmpeg.ConvertOptions{Container: "mp4", VideoCodec: "h264"}},
	}
	for n, p := range bad {
		if _, err := e.svc.SavePreset(ctx, p); !apperr.Is(err, apperr.InvalidArgument) {
			t.Errorf("%s: %v", n, err)
		}
	}
	if _, err := e.svc.SavePreset(ctx, Preset{ID: "nope", Name: "x", Options: ffmpeg.ConvertOptions{Container: "mp4", VideoCodec: "h264"}}); !apperr.Is(err, apperr.NotFound) {
		t.Errorf("更新不存在: %v", err)
	}
	if err := e.svc.DeletePreset(ctx, "builtin-mp3"); !apperr.Is(err, apperr.InvalidArgument) {
		t.Errorf("删内置: %v", err)
	}
	if err := e.svc.DeletePreset(ctx, "nope"); !apperr.Is(err, apperr.NotFound) {
		t.Errorf("删不存在: %v", err)
	}
	if err := e.svc.DeletePreset(ctx, np.ID); err != nil {
		t.Fatal(err)
	}
	// 用户预设上限
	for i := 0; i < maxUserPresets; i++ {
		if _, err := e.svc.SavePreset(ctx, Preset{Name: "p" + strconv.Itoa(i), Options: ffmpeg.ConvertOptions{Container: "mp4", VideoCodec: "h264"}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.svc.SavePreset(ctx, Preset{Name: "多一个", Options: ffmpeg.ConvertOptions{Container: "mp4", VideoCodec: "h264"}}); !apperr.Is(err, apperr.InvalidArgument) {
		t.Errorf("超过上限: %v", err)
	}
}

// M5：ctx 被取消（应用退出）时 Submit 返回 CANCELED，不是 INTERNAL。
func TestSubmitCanceledContext(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(context.Background(), filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	tm := task.NewManager(task.Config{Store: st, LogDir: filepath.Join(dir, "logs"), BatchConcurrency: 1,
		ProgressInterval: -1, Logf: func(string, ...any) {}})
	t.Cleanup(func() { tm.Shutdown(2 * time.Second) })
	slow := filepath.Join(dir, "ffprobe")
	os.WriteFile(slow, []byte("#!/bin/sh\nsleep 30\n"), 0o755) // Windows 上文件不可执行，只跑下面的"已取消"用例
	bin := ffmpeg.Binaries{FFmpeg: filepath.Join(dir, "ffmpeg"), FFprobe: slow}
	req := func() (ffmpeg.Binaries, error) { return bin, nil }
	svc, err := New(context.Background(), Config{Presets: st, Media: media.New(media.Config{Require: req, ThumbsDir: filepath.Join(dir, "thumbs")}), Tasks: tm, Require: req})
	if err != nil {
		t.Fatal(err)
	}
	in := filepath.Join(dir, "a.mp4")
	os.WriteFile(in, []byte("x"), 0o644)
	opts := ffmpeg.ConvertOptions{Container: "mp4", VideoCodec: "h264"}

	// 已取消的 ctx：立即返回，不提交任务
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out, err := svc.Submit(ctx, []string{in}, opts, "")
	if !apperr.Is(err, apperr.Canceled) || len(out) != 0 {
		t.Fatalf("已取消应 CANCELED: %v out=%v", err, out)
	}
	if p, _ := tm.List(task.Filter{}); p.Total != 0 {
		t.Fatalf("不应提交任务: %d", p.Total)
	}
	if runtime.GOOS == "windows" {
		return
	}
	// 探测进行中取消（应用退出）：结束 ffprobe 并返回 CANCELED
	ctx2, cancel2 := context.WithCancel(context.Background())
	time.AfterFunc(300*time.Millisecond, cancel2)
	start := time.Now()
	_, err = svc.Submit(ctx2, []string{in}, opts, "")
	if !apperr.Is(err, apperr.Canceled) {
		t.Fatalf("探测中取消应 CANCELED: %v", err)
	}
	if time.Since(start) > 10*time.Second {
		t.Fatal("取消后没有及时返回")
	}
}

// markFailed 把一条已结束的任务直接在库里改成 failed（v0.23 起 succeeded 不能 Retry，
// 测试“重试”路径时先把成功的任务变成失败的）。
func markFailed(t *testing.T, st *store.Store, id string) {
	t.Helper()
	tk, err := st.GetTask(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	tk.Status = task.StatusFailed
	tk.Version++
	if err := st.UpdateTask(context.Background(), tk); err != nil {
		t.Fatal(err)
	}
}

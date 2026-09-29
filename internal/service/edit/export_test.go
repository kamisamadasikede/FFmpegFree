package edit

import (
	"FFmpegFree/internal/fsutil"
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/store"
	"FFmpegFree/internal/task"
)

func (e *env) export(t *testing.T, p EditProject, opts EditExportOptions) task.Task {
	t.Helper()
	tk, err := e.svc.Export(context.Background(), p, opts)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	return tk
}

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestExportSingleTrack(t *testing.T) {
	e := newEnv(t)
	v := e.genVideo(t, "a.mp4", 4, "320x240")
	out := filepath.Join(e.dir, "out")
	p := proj(vclip("c1", v, "V1", 0, 1, 3))
	p.Name = "单轨"
	p.Output = EditOutput{Width: 640, Height: 360, Fps: 25}
	tk := e.export(t, p, EditExportOptions{OutputDir: out})
	if tk.Type != task.TypeEditExport || tk.Title != "单轨.mp4" || len(tk.InputPaths) != 1 || tk.OutputPath != filepath.Join(out, "单轨.mp4") {
		t.Fatalf("%+v", tk)
	}
	done := e.wait(t, tk.ID)
	if done.Status != task.StatusSucceeded {
		t.Fatalf("%+v", done.Error)
	}
	if done.OutputPath != filepath.Join(out, "单轨.mp4") {
		t.Fatal(done.OutputPath)
	}
	r := e.probe(t, done.OutputPath)
	if !near(r.dur(), 2, 0.15) || r.count("video") != 1 || r.count("audio") != 1 {
		t.Fatalf("dur=%v streams=%+v", r.dur(), r.Streams)
	}
	for _, s := range r.Streams {
		if s.CodecType == "video" && (s.Width != 640 || s.Height != 360 || s.CodecName != "h264") {
			t.Fatalf("%+v", s)
		}
	}
	// 没有 .part 残留、临时目录已清理
	ents, _ := os.ReadDir(out)
	for _, en := range ents {
		if strings.Contains(en.Name(), ".part") {
			t.Fatalf(".part 残留: %s", en.Name())
		}
	}
	if ents, _ := os.ReadDir(e.tmp); len(ents) != 0 {
		t.Fatalf("临时目录未清理: %v", ents)
	}
	// 进度：单调，最终 1，outTimeSec 有值
	ev := e.em.get(tk.ID)
	last := -1.0
	for _, pe := range ev {
		if pe.Progress < last {
			t.Fatalf("进度回退 %v < %v", pe.Progress, last)
		}
		last = pe.Progress
	}
	if len(ev) == 0 || last != 1 {
		t.Fatalf("进度事件 %d 个，最后 %v", len(ev), last)
	}
	// 输出的日志里有 deprecated 行也不影响成功；日志不含完整命令行 / 临时脚本路径以外的路径
}

func TestExportMultiTrackOverlayAndAudioMix(t *testing.T) {
	e := newEnv(t)
	v1 := e.genVideo(t, "v1.mp4", 4, "320x240")
	v2 := e.genVideo(t, "v2.mp4", 3, "160x120")
	au := e.genAudio(t, "m.mp3", 5)
	p := proj(
		vclip("base", v1, "V1", 0, 0, 4),
		vclip("top", v2, "V2", 1, 0, 2),
		vclip("top2", v2, "V1", 4, 0, 1), // V1 上 4 秒接着 1 秒
	)
	p.Output = EditOutput{Width: 320, Height: 240, Fps: 25}
	p.AudioTrack = []AudioClip{aclip("a1", au, "A1", 0, 0, 5), aclip("a2", v1, "A2", 1, 0, 2)}
	p.Name = "multi"
	tk := e.export(t, p, EditExportOptions{OutputDir: filepath.Join(e.dir, "o")})
	done := e.wait(t, tk.ID)
	if done.Status != task.StatusSucceeded {
		t.Fatalf("%+v", done.Error)
	}
	r := e.probe(t, done.OutputPath)
	if !near(r.dur(), 5, 0.2) || r.count("video") != 1 || r.count("audio") != 1 {
		t.Fatalf("dur=%v %+v", r.dur(), r.Streams)
	}
}

func TestExportWithTransitionAndEffects(t *testing.T) {
	e := newEnv(t)
	v1 := e.genVideo(t, "v1.mp4", 3, "320x240")
	v2 := e.genVideo(t, "v2.mp4", 3, "320x240")
	a := vclip("c1", v1, "V1", 0, 0, 3)
	a.TransitionToNext, a.TransitionDurationSec, a.EffectPreset, a.Blur = "fade", 0.7, "sepia", 1
	b := vclip("c2", v2, "V1", 3, 0, 3)
	b.Speed, b.OutSec = 2, 3 // 1.5 秒
	p := proj(a, b)
	p.Output = EditOutput{Width: 320, Height: 240, Fps: 25}
	p.Effects = GlobalEffects{Brightness: 0.1, Contrast: 1.2, Saturation: 1.1, Sharpen: 1}
	p.AudioTrack = []AudioClip{{ID: "a1", Path: v1, TrackID: "A1", OutSec: 3, Speed: 4, Volume: 0.5}} // 速度 4 → atempo 链
	tk := e.export(t, p, EditExportOptions{OutputName: "fx", OutputDir: filepath.Join(e.dir, "o")})
	done := e.wait(t, tk.ID)
	if done.Status != task.StatusSucceeded {
		t.Fatalf("%+v", done.Error)
	}
	r := e.probe(t, done.OutputPath)
	// 时长 = 3 + 1.5 - 0.7(转场重叠) = 3.8
	if !near(r.dur(), 3.8, 0.2) {
		t.Fatalf("dur=%v", r.dur())
	}
}

func TestExportNoAudioTrackIsSilentNotFromVideo(t *testing.T) {
	e := newEnv(t)
	v := e.genVideo(t, "v.mp4", 2, "320x240") // 素材自带 440Hz 音频
	p := proj(vclip("c1", v, "V1", 0, 0, 2))
	p.Output = EditOutput{Width: 320, Height: 240, Fps: 25}
	tk := e.export(t, p, EditExportOptions{OutputDir: filepath.Join(e.dir, "o")})
	done := e.wait(t, tk.ID)
	if done.Status != task.StatusSucceeded {
		t.Fatalf("%+v", done.Error)
	}
	r := e.probe(t, done.OutputPath)
	if r.count("audio") != 1 || !near(r.dur(), 2, 0.15) {
		t.Fatalf("应有一条（静音）音轨: %+v", r.Streams)
	}
	// 用 volumedetect 确认确实静音（不是视频自带的正弦波）
	b, _ := execOut(e.bin.FFmpeg, "-hide_banner", "-i", "file:"+done.OutputPath, "-vn", "-af", "volumedetect", "-f", "null", "-")
	if !strings.Contains(b, "max_volume: -91") && !strings.Contains(b, "mean_volume: -91") && !strings.Contains(b, "-inf") {
		t.Fatalf("应为静音，volumedetect: %s", tailLines(b, 6))
	}
}

func TestExportAudioIsAudible(t *testing.T) {
	e := newEnv(t)
	v := e.genVideo(t, "v.mp4", 2, "320x240")
	au := e.genAudio(t, "a.mp3", 2)
	p := proj(vclip("c1", v, "V1", 0, 0, 2))
	p.Output = EditOutput{Width: 320, Height: 240, Fps: 25}
	p.AudioTrack = []AudioClip{aclip("a1", au, "A1", 0, 0, 2)}
	tk := e.export(t, p, EditExportOptions{OutputDir: filepath.Join(e.dir, "o")})
	done := e.wait(t, tk.ID)
	if done.Status != task.StatusSucceeded {
		t.Fatalf("%+v", done.Error)
	}
	b, _ := execOut(e.bin.FFmpeg, "-hide_banner", "-i", "file:"+done.OutputPath, "-vn", "-af", "volumedetect", "-f", "null", "-")
	if strings.Contains(b, "max_volume: -91") || !strings.Contains(b, "max_volume:") {
		t.Fatalf("应有声音: %s", tailLines(b, 6))
	}
}

func TestExportSilentSourceVideoOnly(t *testing.T) {
	e := newEnv(t)
	v := e.genSilentVideo(t, "s.mp4", 2) // 素材没有音轨
	p := proj(vclip("c1", v, "V1", 0, 0, 2))
	p.Output = EditOutput{Width: 320, Height: 240, Fps: 25}
	tk := e.export(t, p, EditExportOptions{OutputDir: filepath.Join(e.dir, "o")})
	done := e.wait(t, tk.ID)
	if done.Status != task.StatusSucceeded {
		t.Fatalf("%+v", done.Error)
	}
	if r := e.probe(t, done.OutputPath); r.count("audio") != 1 || r.count("video") != 1 {
		t.Fatalf("%+v", r.Streams)
	}
}

func TestExportGapFillsBlackAndSilence(t *testing.T) {
	e := newEnv(t)
	v := e.genVideo(t, "v.mp4", 2, "320x240")
	au := e.genAudio(t, "a.mp3", 1)
	// 视频 0~1，空隙 1~2，视频 2~3；音频 2.5 秒开始 1 秒 → 总长 3.5，末尾 3~3.5 无画面
	p := proj(vclip("c1", v, "V1", 0, 0, 1), vclip("c2", v, "V1", 2, 0, 1))
	p.Output = EditOutput{Width: 320, Height: 240, Fps: 25}
	p.AudioTrack = []AudioClip{aclip("a1", au, "A1", 2.5, 0, 1)}
	pl, err := e.svc.ValidateProject(context.Background(), p)
	if err != nil || !near(pl.DurationSec, 3.5, 1e-9) || !hasWarn(pl, WarnClipGap) {
		t.Fatalf("%+v %v", pl, err)
	}
	tk := e.export(t, p, EditExportOptions{OutputDir: filepath.Join(e.dir, "o")})
	done := e.wait(t, tk.ID)
	if done.Status != task.StatusSucceeded {
		t.Fatalf("%+v", done.Error)
	}
	r := e.probe(t, done.OutputPath)
	if !near(r.dur(), 3.5, 0.2) {
		t.Fatalf("dur=%v", r.dur())
	}
	// 1.2 秒处应为黑场：取一帧算平均亮度（signalstats YAVG）
	yavg := func(at string) float64 {
		out, _ := execOut(e.bin.FFmpeg, "-hide_banner", "-ss", at, "-i", "file:"+done.OutputPath, "-frames:v", "1",
			"-vf", "signalstats,metadata=print:file=-", "-f", "null", "-")
		for _, l := range strings.Split(out, "\n") {
			if i := strings.Index(l, "lavfi.signalstats.YAVG="); i >= 0 {
				return atof(l[i+len("lavfi.signalstats.YAVG="):])
			}
		}
		t.Fatalf("没有 YAVG: %s", tailLines(out, 8))
		return 0
	}
	if y := yavg("1.4"); y > 20 {
		t.Fatalf("空隙应为黑场，YAVG=%v", y)
	}
	if y := yavg("0.5"); y < 30 {
		t.Fatalf("有画面处不应是黑场，YAVG=%v", y)
	}
	if y := yavg("3.3"); y > 20 {
		t.Fatalf("末尾应为黑场，YAVG=%v", y)
	}
	// 音频 2.5 秒才开始：0~2.4 秒（含视频有画面的部分）没有任何音频，应为静音；2.6~3.4 有声音
	vol := func(ss, d string) string {
		b, _ := execOut(e.bin.FFmpeg, "-hide_banner", "-ss", ss, "-t", d, "-i", "file:"+done.OutputPath, "-vn", "-af", "volumedetect", "-f", "null", "-")
		return b
	}
	if b := vol("0", "2.3"); !strings.Contains(b, "max_volume: -91") {
		t.Fatalf("空隙 / 无音频处应静音: %s", tailLines(b, 6))
	}
	if b := vol("2.7", "0.6"); strings.Contains(b, "max_volume: -91") || !strings.Contains(b, "max_volume:") {
		t.Fatalf("音频段应有声音: %s", tailLines(b, 6))
	}
}

func TestExportNameCollisionAppendsSuffix(t *testing.T) {
	e := newEnv(t)
	v := e.genVideo(t, "v.mp4", 1, "320x240")
	out := filepath.Join(e.dir, "o")
	os.MkdirAll(out, 0o755)
	os.WriteFile(filepath.Join(out, "same.mp4"), []byte("keep me"), 0o644)
	p := proj(vclip("c1", v, "V1", 0, 0, 1))
	p.Output = EditOutput{Width: 320, Height: 240, Fps: 25}
	var paths []string
	for i := 0; i < 2; i++ {
		tk := e.export(t, p, EditExportOptions{OutputName: "same", OutputDir: out})
		done := e.wait(t, tk.ID)
		if done.Status != task.StatusSucceeded {
			t.Fatalf("%+v", done.Error)
		}
		paths = append(paths, done.OutputPath)
	}
	if paths[0] != filepath.Join(out, "same(1).mp4") || paths[1] != filepath.Join(out, "same(2).mp4") {
		t.Fatalf("%v", paths)
	}
	if b, _ := os.ReadFile(filepath.Join(out, "same.mp4")); string(b) != "keep me" {
		t.Fatal("原文件被覆盖")
	}
}

func TestExportDefaultOutputDirAndFallback(t *testing.T) {
	e := newEnv(t)
	v := e.genVideo(t, "v.mp4", 1, "320x240")
	p := proj(vclip("c1", v, "V1", 0, 0, 1))
	p.Name = "无目录"
	p.Output = EditOutput{Width: 320, Height: 240, Fps: 25}
	// 无 outputDir、无默认目录 → 第一个 clip 所在文件夹
	tk := e.export(t, p, EditExportOptions{})
	if filepath.Dir(tk.OutputPath) != e.dir {
		t.Fatalf("%s", tk.OutputPath)
	}
	e.wait(t, tk.ID)
	// 设置了默认输出目录
	e.defA = filepath.Join(e.dir, "def")
	tk = e.export(t, p, EditExportOptions{})
	if filepath.Dir(tk.OutputPath) != e.defA {
		t.Fatalf("%s", tk.OutputPath)
	}
	e.wait(t, tk.ID)
	// 相对路径 → INVALID_ARGUMENT，不产生任务
	before := len(e.tm.ListActive())
	_, err := e.svc.Export(context.Background(), p, EditExportOptions{OutputDir: "rel/dir"})
	if code(t, err) != apperr.InvalidArgument || len(e.tm.ListActive()) != before {
		t.Fatal(err)
	}
}

func TestExportSubmitTimeFailuresCreateNoTask(t *testing.T) {
	e := newEnv(t)
	v := e.genVideo(t, "v.mp4", 1, "320x240")
	countTasks := func() int64 {
		pg, _ := e.st.ListTasks(context.Background(), storeFilter())
		return pg.Total
	}
	p := proj(vclip("c1", v, "V1", 0, 0, 1))
	p.Output = EditOutput{Width: 320, Height: 240, Fps: 25}
	bad := []struct {
		name string
		p    EditProject
		o    EditExportOptions
		code apperr.Code
	}{
		{"素材不存在", proj(vclip("c1", filepath.Join(e.dir, "none.mp4"), "V1", 0, 0, 1)), EditExportOptions{}, apperr.NotFound},
		{"outSec=0", proj(vclip("c1", v, "V1", 0, 0, 0)), EditExportOptions{}, apperr.InvalidArgument},
		{"输出目录是文件", p, EditExportOptions{OutputDir: v}, apperr.InvalidArgument},
		{"输出目录父级是文件", p, EditExportOptions{OutputDir: filepath.Join(v, "sub")}, apperr.InvalidArgument},
	}
	for _, c := range bad {
		_, err := e.svc.Export(context.Background(), c.p, c.o)
		if err == nil || code(t, err) != c.code {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	// 只读目录（root 下无法测试则跳过）
	ro := filepath.Join(e.dir, "ro")
	os.MkdirAll(ro, 0o755)
	os.Chmod(ro, 0o555)
	defer os.Chmod(ro, 0o755)
	if f, err := os.CreateTemp(ro, "x"); err == nil {
		f.Close()
		t.Log("当前用户可写只读目录（root？），跳过只读目录断言")
	} else if _, err := e.svc.Export(context.Background(), p, EditExportOptions{OutputDir: ro}); code(t, err) != apperr.IOError {
		t.Errorf("只读目录: %v", err)
	}
	if n := countTasks(); n != 0 {
		t.Fatalf("失败的提交不应产生任务，实际 %d 个", n)
	}
}

func TestExportOutputNameSanitized(t *testing.T) {
	e := newEnv(t)
	v := e.genVideo(t, "v.mp4", 1, "320x240")
	p := proj(vclip("c1", v, "V1", 0, 0, 1))
	p.Name = "工程名"
	p.Output = EditOutput{Width: 320, Height: 240, Fps: 25, Format: "mkv"}
	out := filepath.Join(e.dir, "o")
	cases := map[string]string{
		"../../evil/x": "....evilx.mkv", // 分隔符被删掉；中间的点合法，不会逃出输出目录
		"CON":          "_CON.mkv",
		"a:b*c":        "abc.mkv",
		"   ":          "工程名.mkv", // 纯空白 = 未填 → 用工程名
		"":             "工程名.mkv",
		"日本語 name..":   "日本語 name.mkv",
	}
	for in, want := range cases {
		tk, err := e.svc.Export(context.Background(), p, EditExportOptions{OutputName: in, OutputDir: out})
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if got := filepath.Base(tk.OutputPath); got != want {
			t.Errorf("%q → %q，期望 %q", in, got, want)
		}
		if filepath.Dir(tk.OutputPath) != out {
			t.Errorf("%q 逃出了输出目录: %s", in, tk.OutputPath)
		}
	}
	// 净化后为空（不是原本为空）→ edit，不回退工程名
	tk, err := e.svc.Export(context.Background(), p, EditExportOptions{OutputName: "///", OutputDir: out})
	if err != nil || filepath.Base(tk.OutputPath) != "edit.mkv" {
		t.Fatalf("%v %v", tk.OutputPath, err)
	}
}

func TestExportPathTooLongOnWindows(t *testing.T) {
	// 在任意平台上把 GOOS 设成 windows 验证：超长在提交时同步 INVALID_ARGUMENT，不产生任务；用假的探测让它不依赖真实 ffmpeg。
	s := New(Config{Media: fakeMedia{base}, Tasks: &fakeTasks{}, GOOS: "windows",
		Require:        func() (ffmpeg.Binaries, error) { return ffmpeg.Binaries{FFmpeg: "x", FFprobe: "y"}, nil },
		SupportsScript: func(context.Context, string) (string, error) { return OptFilterFile, nil }})
	dir := t.TempDir()
	long := dir
	for len(long) < 240 {
		long = filepath.Join(long, "dddddddddd")
	}
	os.MkdirAll(long, 0o755)
	p := proj(vclip("c1", pV, "V1", 0, 0, 1))
	_, err := s.Export(context.Background(), p, EditExportOptions{OutputDir: long, OutputName: "x"})
	if code(t, err) != apperr.InvalidArgument || !strings.Contains(apperr.From(err).Message, "259") {
		t.Fatalf("%v", err)
	}
	// detail：第一行 project，第二行 path_length=<n> limit=259（n = fsutil.OutputPathLength，含 (99) 与 .part 预留）
	want := fmt.Sprintf("project\npath_length=%d limit=259", fsutil.OutputPathLength(long, "x", ".mp4"))
	if d := apperr.From(err).Detail; d != want || fsutil.OutputPathLength(long, "x", ".mp4") <= 259 {
		t.Fatalf("detail=%q want %q", d, want)
	}
	// \\?\ 与 \\.\ 开头的 outputDir 一律拒绝（不看平台）
	for _, d := range []string{`\\?\C:\out`, `\\.\C:\out`} {
		_, err := s.Export(context.Background(), p, EditExportOptions{OutputDir: d, OutputName: "x"})
		if code(t, err) != apperr.InvalidArgument || firstLine(err) != "project" {
			t.Fatalf("%s: %v", d, err)
		}
	}
	if s.cfg.Tasks.(*fakeTasks).n != 0 {
		t.Fatal("不应产生任务")
	}
	// 短路径通过（Linux 上路径分隔符不影响长度计算）
	if _, err := s.Export(context.Background(), p, EditExportOptions{OutputDir: dir, OutputName: "x"}); err != nil {
		t.Fatal(err)
	}
	if s.cfg.Tasks.(*fakeTasks).n != 1 {
		t.Fatal("应提交 1 个任务")
	}
}

func TestExportCancel(t *testing.T) {
	e := newEnv(t)
	v := e.genVideo(t, "v.mp4", 3, "1280x720")
	// 大画布 + 慢预设的长时间线：足够慢，能在中途取消
	p := EditProject{Name: "slow", Output: EditOutput{Width: 1920, Height: 1080, Fps: 60}}
	for i := 0; i < 6; i++ {
		c := vclip("c"+itoa(i), v, "V1", float64(i*3), 0, 3)
		c.Blur = 4
		p.VideoTrack = append(p.VideoTrack, c)
	}
	out := filepath.Join(e.dir, "o")
	tk := e.export(t, p, EditExportOptions{OutputDir: out})
	// 等到开始运行
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		g, _ := e.tm.Get(tk.ID)
		if g.Status == task.StatusRunning && len(e.em.get(tk.ID)) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err := e.tm.Cancel(tk.ID); err != nil {
		t.Fatal(err)
	}
	done := e.wait(t, tk.ID)
	if done.Status != task.StatusCanceled {
		t.Fatalf("status=%s err=%+v", done.Status, done.Error)
	}
	ents, _ := os.ReadDir(out)
	for _, en := range ents {
		t.Fatalf("取消后不应有任何输出或 .part: %s", en.Name())
	}
	if ents, _ := os.ReadDir(e.tmp); len(ents) != 0 {
		t.Fatalf("临时目录未清理: %v", ents)
	}
}

func TestExportFailureClassification(t *testing.T) {
	e := newEnv(t)
	v := e.genVideo(t, "v.mp4", 2, "320x240")
	p := proj(vclip("c1", v, "V1", 0, 0, 2))
	p.Output = EditOutput{Width: 320, Height: 240, Fps: 25}
	out := filepath.Join(e.dir, "o")

	// ① 提交后素材被换成损坏文件 → PROBE_FAILED（不落 PROCESS_FAILED）
	tk := e.export(t, p, EditExportOptions{OutputName: "bad", OutputDir: out})
	e.wait(t, tk.ID)
	// 换成垃圾文件后 Retry：重新校验探测 → PROBE_FAILED，不产生新任务
	os.WriteFile(v, []byte(strings.Repeat("not a video", 100)), 0o644)
	_, err := e.tm.Retry(tk.ID)
	if err == nil || code(t, err) != apperr.ProbeFailed {
		t.Fatalf("Retry 应 PROBE_FAILED: %v", err)
	}
	if firstLine(err) != "clip=c1 path="+v {
		t.Fatalf("detail 第一行: %q", firstLine(err))
	}
	// 删除后 Retry → NOT_FOUND
	os.Remove(v)
	_, err = e.tm.Retry(tk.ID)
	if err == nil || code(t, err) != apperr.NotFound {
		t.Fatalf("Retry 素材已删应 NOT_FOUND: %v", err)
	}
}

func TestExportRunnerFailureIsProcessFailed(t *testing.T) {
	// 让 ffmpeg 在运行期失败：滤镜里没有的编码器（用不存在的 -c:v 通过伪造 plan 直接构造 runner）。
	e := newEnv(t)
	v := e.genVideo(t, "v.mp4", 2, "320x240")
	p := proj(vclip("c1", v, "V1", 0, 0, 2))
	p.Output = EditOutput{Width: 320, Height: 240, Fps: 25}
	pl, err := e.svc.build(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	pl.format = "mkv"
	// 坏的 filtergraph：引用不存在的滤镜
	r := &badRunner{exportRunner{s: e.svc, bin: e.bin, pl: pl, out: filepath.Join(e.dir, "o", "x.mkv")}}
	tk, err := e.tm.Submit(task.Spec{Type: task.TypeEditExport, Title: "x", OutputPath: r.out}, r)
	if err != nil {
		t.Fatal(err)
	}
	done := e.wait(t, tk.ID)
	if done.Status != task.StatusFailed || done.Error == nil || done.Error.Code != apperr.ProcessFailed {
		t.Fatalf("%s %+v", done.Status, done.Error)
	}
	if !strings.Contains(strings.ToLower(done.Error.Detail), "nosuchfilter") && !strings.Contains(strings.ToLower(done.Error.Detail), "no such filter") {
		t.Fatalf("detail 应含 ffmpeg 尾部日志: %s", done.Error.Detail)
	}
	if _, err := os.Stat(r.out); err == nil {
		t.Fatal("失败不应有输出")
	}
	if _, err := os.Stat(filepath.Join(e.dir, "o", "x.part.mkv")); err == nil {
		t.Fatal(".part 应被清理")
	}
}

func TestExportSubmittedWhileRunningQueues(t *testing.T) {
	e := newEnvWith(t, realBins(t), 1) // 并发 1：第二个必须排队
	v := e.genVideo(t, "v.mp4", 2, "320x240")
	p := proj(vclip("c1", v, "V1", 0, 0, 2))
	p.Output = EditOutput{Width: 320, Height: 240, Fps: 25}
	out := filepath.Join(e.dir, "o")
	t1 := e.export(t, p, EditExportOptions{OutputName: "q", OutputDir: out})
	t2 := e.export(t, p, EditExportOptions{OutputName: "q", OutputDir: out})
	d1, d2 := e.wait(t, t1.ID), e.wait(t, t2.ID)
	if d1.Status != task.StatusSucceeded || d2.Status != task.StatusSucceeded || d1.OutputPath == d2.OutputPath {
		t.Fatalf("%v %v | %s %s", d1.Status, d2.Status, d1.OutputPath, d2.OutputPath)
	}
	if d2.StartedAt < d1.FinishedAt-5 {
		t.Fatalf("第二个应在第一个结束后才开始: %d < %d", d2.StartedAt, d1.FinishedAt)
	}
}

func TestExportRetry(t *testing.T) {
	e := newEnv(t)
	v := e.genVideo(t, "v.mp4", 1, "320x240")
	p := proj(vclip("c1", v, "V1", 0, 0, 1))
	p.Output = EditOutput{Width: 320, Height: 240, Fps: 25}
	out := filepath.Join(e.dir, "o")
	tk := e.export(t, p, EditExportOptions{OutputName: "r", OutputDir: out})
	d := e.wait(t, tk.ID)
	// 之后改默认输出目录不影响 Retry（沿用原来解析好的目录）
	e.defA = filepath.Join(e.dir, "other")
	nt, err := e.tm.Retry(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	d2 := e.wait(t, nt.ID)
	if d2.Status != task.StatusSucceeded || filepath.Dir(d2.OutputPath) != out || d2.OutputPath == d.OutputPath || d2.Type != task.TypeEditExport {
		t.Fatalf("%+v", d2)
	}
}

func TestParamsAreSnapshot(t *testing.T) {
	e := newEnv(t)
	v := e.genVideo(t, "v.mp4", 1, "320x240")
	p := proj(vclip("c1", v, "V1", 0, 0, 1))
	tk := e.export(t, p, EditExportOptions{OutputDir: filepath.Join(e.dir, "o")})
	if !strings.Contains(tk.Params, `"project"`) || !strings.Contains(tk.Params, `"outputDir"`) {
		t.Fatalf("params: %s", tk.Params)
	}
	e.wait(t, tk.ID)
}

// ---------- 辅助 ----------

type fakeTasks struct {
	n    int
	last task.Runner
}

func (f *fakeTasks) Submit(spec task.Spec, r task.Runner) (task.Task, error) {
	f.n++
	f.last = r
	return task.Task{ID: "fake", Type: spec.Type, Title: spec.Title, OutputPath: spec.OutputPath}, nil
}
func (f *fakeTasks) RegisterFactory(task.Type, task.Factory) {}

func storeFilter() store.TaskFilter { return store.TaskFilter{Limit: 200} }

func execOut(exe string, args ...string) (string, error) {
	cmd := exec.Command(exe, args...)
	b, err := cmd.CombinedOutput()
	return string(b), err
}

func atof(s string) float64 {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, " \r\n"); i >= 0 {
		s = s[:i]
	}
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// badRunner 用坏的 filtergraph 触发 ffmpeg 运行期失败。
type badRunner struct{ exportRunner }

func (r *badRunner) Run(ctx context.Context, report func(task.Progress)) (string, error) {
	tmp, _ := os.MkdirTemp(r.s.cfg.TempDir, "edit-")
	defer os.RemoveAll(tmp)
	script := filepath.Join(tmp, "graph.txt")
	os.WriteFile(script, []byte("color=c=black:s=64x64:d=1,nosuchfilter[vout];anullsrc=r=48000:cl=stereo,atrim=end=1[aout]\n"), 0o600)
	fr := &task.FFmpegRunner{Exe: r.bin.FFmpeg, Output: r.out, DurationSec: 1, Classify: classifyExportError,
		BuildArgs: func(part string) []string { return exportArgs(r.pl, script, part) }}
	return fr.Run(ctx, report)
}

// 真 ffmpeg：导出走探测选中的 -/filter_complex（7.0 起；9.x 唯一可用），输出能被 ffprobe 读取。
// 用 EDIT_TEST_FFMPEG_DIR 指向别的 ffmpeg 目录可在 9.x 上重跑。
func TestExportRealUsesFileOption(t *testing.T) {
	e := newEnv(t)
	v := e.genVideo(t, "a.mp4", 3, "320x240")
	au := e.genAudio(t, "m.mp3", 3)
	p := proj(vclip("c1", v, "V1", 0, 0, 2), vclip("c2", v, "V2", 1, 0, 2))
	p.Output = EditOutput{Width: 320, Height: 240, Fps: 25}
	p.AudioTrack = []AudioClip{aclip("a1", au, "A1", 0, 0, 3)}
	r, _, err := e.svc.prepareExport(context.Background(), p, EditExportOptions{OutputDir: filepath.Join(e.dir, "o")}, "")
	if err != nil {
		t.Fatal(err)
	}
	pl := r.(*exportRunner).pl
	if pl.filterOpt != OptFilterFile {
		t.Fatalf("选项 %q", pl.filterOpt)
	}
	tk := e.export(t, p, EditExportOptions{OutputDir: filepath.Join(e.dir, "o")})
	done := e.wait(t, tk.ID)
	if done.Status != task.StatusSucceeded {
		t.Fatalf("%+v", done.Error)
	}
	res := e.probe(t, done.OutputPath)
	t.Logf("ffmpeg=%s 输出 %s：时长 %.2fs，视频流 %d，音频流 %d", e.bin.FFmpeg, filepath.Base(done.OutputPath), res.dur(), res.count("video"), res.count("audio"))
	if !near(res.dur(), 3, 0.2) || res.count("video") != 1 || res.count("audio") != 1 {
		t.Fatalf("%+v", res.Streams)
	}
}

// 真 ffmpeg 6.x/7.x：强制走旧选项 -filter_complex_script 也能导出（9.x 已移除该选项，自动跳过）。
func TestExportRealOldOptionFallback(t *testing.T) {
	e := newEnv(t)
	if opt, err := e.svc.probeFilterScript(context.Background(), e.bin.FFmpeg); err != nil {
		t.Fatal(err)
	} else if ok, _ := e.svc.tryFilterOption(context.Background(), e.bin.FFmpeg, OptFilterScript, writeProbe(t)); !ok {
		t.Skipf("该 ffmpeg 不支持 %s（探测选中 %s）", OptFilterScript, opt)
	}
	e.svc.cfg.SupportsScript = func(context.Context, string) (string, error) { return OptFilterScript, nil }
	v := e.genVideo(t, "a.mp4", 2, "320x240")
	p := proj(vclip("c1", v, "V1", 0, 0, 2))
	p.Output = EditOutput{Width: 320, Height: 240, Fps: 25}
	tk := e.export(t, p, EditExportOptions{OutputDir: filepath.Join(e.dir, "o")})
	done := e.wait(t, tk.ID)
	if done.Status != task.StatusSucceeded {
		t.Fatalf("%+v", done.Error)
	}
	res := e.probe(t, done.OutputPath)
	if !near(res.dur(), 2, 0.2) || res.count("video") != 1 || res.count("audio") != 1 {
		t.Fatalf("%+v", res.Streams)
	}
	t.Logf("旧选项导出 OK：%.2fs", res.dur())
}

func writeProbe(t *testing.T) string {
	p := filepath.Join(t.TempDir(), "p.txt")
	os.WriteFile(p, []byte("[0:v]scale=16:16[v]\n"), 0o600)
	return p
}

package convert

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/store"
	"FFmpegFree/internal/task"
)

// ---------- ConvertService.Reconvert（契约 v0.24 / v0.24.1，6.17）与提交相关的 v0.24 行为（真 ffmpeg） ----------

var mp4Opts = ffmpeg.ConvertOptions{Container: "mp4", VideoCodec: "h264", AudioCodec: "aac"}

func (e *env) waitReconvert(t *testing.T, id string) task.Task {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		tk, err := e.tm.Get(id)
		if err == nil && !tk.Reconverting && tk.Status == task.StatusSucceeded {
			return tk
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("等重转结束超时")
	return task.Task{}
}

func (e *env) doneRecord(t *testing.T, secs int) (ConvertSource, task.Task, string) {
	t.Helper()
	in := e.genVideo(t, filepath.Join(e.dir, "a.mp4"), secs)
	src := e.addSource(t, in)
	d := e.submitSources(t, ConvertSubmitRequest{SourceIDs: []string{src.SourceID}, Options: mp4Opts, OutputDir: filepath.Join(e.dir, "o")})[0]
	if d.Status != task.StatusSucceeded {
		t.Skipf("转换失败（编码器缺失？）: %+v", d.Error)
	}
	return src, d, in
}

func wantR(t *testing.T, err error, code apperr.Code, reasonPrefix string) {
	t.Helper()
	if !apperr.Is(err, code) || !strings.HasPrefix(detailOf(err), reasonPrefix) {
		t.Fatalf("want %s %q, got %v (detail %q)", code, reasonPrefix, err, detailOf(err))
	}
}

func TestReconvertServiceInPlace(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	_, d, _ := e.doneRecord(t, 1)
	tk, err := e.svc.Reconvert(ctx, ReconvertRequest{TaskID: d.ID})
	if err != nil {
		t.Fatal(err)
	}
	if tk.ID != d.ID || !tk.Reconverting || tk.OutputPath != d.OutputPath || tk.HiddenInTaskCenter {
		t.Fatalf("%+v", tk)
	}
	got := e.waitReconvert(t, d.ID)
	if got.OutputPath != d.OutputPath || got.LastReconvertError != nil || got.Version <= d.Version || got.Result == nil {
		t.Fatalf("%+v", got)
	}
	if ms, _ := filepath.Glob(filepath.Join(filepath.Dir(d.OutputPath), "*")); len(ms) != 1 {
		t.Fatalf("同一个文件名，不加 (1)，不留临时文件: %v", ms)
	}
	// 同格式改参数：成功，参数快照换成新的
	h := 120
	o := mp4Opts
	o.Height = h
	if _, err := e.svc.Reconvert(ctx, ReconvertRequest{TaskID: d.ID, Options: &o}); err != nil {
		t.Fatal(err)
	}
	got = e.waitReconvert(t, d.ID)
	if !strings.Contains(got.Params, `"height":120`) || got.Result == nil || got.Result.Height != 120 {
		t.Fatalf("%s %+v", got.Params, got.Result)
	}
	// 换格式：format_change（预设或 options 都一样）
	mkv := ffmpeg.ConvertOptions{Container: "mkv", VideoCodec: "h264", AudioCodec: "aac"}
	_, err = e.svc.Reconvert(ctx, ReconvertRequest{TaskID: d.ID, Options: &mkv})
	wantR(t, err, apperr.InvalidArgument, "reason=format_change")
	_, err = e.svc.Reconvert(ctx, ReconvertRequest{TaskID: d.ID, PresetID: "builtin-mp3"})
	wantR(t, err, apperr.InvalidArgument, "reason=format_change")
	_, err = e.svc.Reconvert(ctx, ReconvertRequest{TaskID: "nope"})
	wantR(t, err, apperr.NotFound, "reason=record")
}

// 校验顺序（v0.24.1）：状态 → 副本没就绪 → 源文件不在 → output_moved → params_locked → format_change。
func TestReconvertServiceValidationOrder(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	_, d, in := e.doneRecord(t, 1)
	check := func() task.TaskPathCheck {
		cs, err := e.tm.CheckPaths([]string{d.ID})
		if err != nil {
			t.Fatal(err)
		}
		return cs[0]
	}
	if c := check(); c.ReconvertMode != task.ReconvertReplace || c.ReconvertBlock != "" {
		t.Fatalf("%+v", c)
	}
	mkv := ffmpeg.ConvertOptions{Container: "mkv"}
	// 源文件不在（输出也不在、还要换格式）：先报源文件
	os.Rename(in, in+".bak")
	os.Rename(d.OutputPath, d.OutputPath+".bak")
	_, err := e.svc.Reconvert(ctx, ReconvertRequest{TaskID: d.ID, Options: &mkv})
	wantR(t, err, apperr.NotFound, "reason=file")
	if c := check(); c.ReconvertMode != "" || c.ReconvertBlock != task.BlockSourceGone {
		t.Fatalf("%+v", c)
	}
	os.Rename(in+".bak", in)
	// PM 15(a)：输出不在 → regenerate，只能用原参数
	if c := check(); c.ReconvertMode != task.ReconvertRegenerate || c.ReconvertBlock != "" || c.OutputExists {
		t.Fatalf("%+v", c)
	}
	o := mp4Opts
	o.Height = 100
	_, err = e.svc.Reconvert(ctx, ReconvertRequest{TaskID: d.ID, Options: &o})
	wantR(t, err, apperr.InvalidArgument, "reason=params_locked")
	_, err = e.svc.Reconvert(ctx, ReconvertRequest{TaskID: d.ID, PresetID: "builtin-mp4"})
	wantR(t, err, apperr.InvalidArgument, "reason=params_locked")
	_, err = e.svc.Reconvert(ctx, ReconvertRequest{TaskID: d.ID, Options: &mkv}) // params_locked 先于 format_change
	wantR(t, err, apperr.InvalidArgument, "reason=params_locked")
	// PM 15(b)：原位置是别的文件 → output_moved（先于参数检查）
	os.WriteFile(d.OutputPath, []byte("not ours"), 0o644)
	old := time.Now().Add(-time.Hour)
	os.Chtimes(d.OutputPath, old, old)
	_, err = e.svc.Reconvert(ctx, ReconvertRequest{TaskID: d.ID, Options: &mkv})
	wantR(t, err, apperr.TaskConflict, "reason=output_moved")
	if c := check(); c.ReconvertBlock != task.BlockOutputMoved {
		t.Fatalf("%+v", c)
	}
	os.Remove(d.OutputPath)
	// regenerate 成功：原路径重新生成，原参数
	if _, err := e.svc.Reconvert(ctx, ReconvertRequest{TaskID: d.ID}); err != nil {
		t.Fatal(err)
	}
	got := e.waitReconvert(t, d.ID)
	if fi, err := os.Stat(d.OutputPath); err != nil || fi.Size() == 0 || got.Params != d.Params || got.LastReconvertError != nil {
		t.Fatalf("%v %+v", err, got)
	}
	// 状态：失败的记录 invalid_state（排在一切之前，即使源文件也不在）
	ft, _ := e.tm.Submit(task.Spec{Type: task.TypeConvert, Params: d.Params, InputPaths: []string{"/nope"}}, task.RunnerFunc(
		func(context.Context, func(task.Progress)) (string, error) {
			return "", apperr.New(apperr.ProcessFailed, "x")
		}))
	e.wait(t, ft.ID)
	_, err = e.svc.Reconvert(ctx, ReconvertRequest{TaskID: ft.ID})
	wantR(t, err, apperr.TaskConflict, "reason=invalid_state")
}

// 副本没就绪（重新添加后正在复制新副本）：TASK_CONFLICT reason=copying，CheckPaths 报 copy_not_ready；重转读副本。
func TestReconvertCopyNotReady(t *testing.T) {
	uploads := filepath.Join(t.TempDir(), "uploads")
	e := newEnvWith(t, func(c *Config) {
		c.UploadsDir = func(context.Context) string { return uploads }
		c.CopyProgressInterval = -1
	})
	ctx := context.Background()
	in := e.genVideo(t, filepath.Join(e.dir, "a.mp4"), 1)
	r, _ := e.svc.AddSources(ctx, []string{in})
	sid := r[0].Source.SourceID
	waitCopy(t, e.svc, sid, store.CopyReady)
	d := e.submitSources(t, ConvertSubmitRequest{SourceIDs: []string{sid}, Options: mp4Opts, OutputDir: filepath.Join(e.dir, "o")})[0]
	if d.Status != task.StatusSucceeded {
		t.Skipf("%+v", d.Error)
	}
	if !strings.HasPrefix(d.InputPaths[0], uploads) {
		t.Fatalf("转换读副本: %v", d.InputPaths)
	}
	started, release := holdCopies(t)
	f, _ := os.OpenFile(in, os.O_APPEND|os.O_WRONLY, 0)
	f.Write(make([]byte, 3<<20)) // 原文件变了 → 再添加时新做副本
	f.Close()
	if r, _ := e.svc.AddSources(ctx, []string{in}); r[0].Error != nil {
		t.Fatal(r[0].Error)
	}
	<-started
	_, err := e.svc.Reconvert(ctx, ReconvertRequest{TaskID: d.ID})
	wantR(t, err, apperr.TaskConflict, "reason=copying")
	if cs, _ := e.tm.CheckPaths([]string{d.ID}); cs[0].ReconvertBlock != task.BlockCopyNotReady {
		t.Fatalf("%+v", cs[0])
	}
	release()
	copyChunkHook = nil
	waitCopy(t, e.svc, sid, store.CopyReady)
	if cs, _ := e.tm.CheckPaths([]string{d.ID}); cs[0].ReconvertMode != task.ReconvertReplace {
		t.Fatalf("%+v", cs[0])
	}
}

func waitCopy(t *testing.T, svc *Service, sid, state string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if ent, err := svc.GetSource(context.Background(), sid); err == nil && ent.Source.CopyState == state {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("等副本 %s 超时", state)
}

// SubmitSources：没就绪的行跳过（skipped 带 reason），只提交就绪的。
func TestSubmitSourcesSkipped(t *testing.T) {
	uploads := filepath.Join(t.TempDir(), "uploads")
	free := int64(1 << 50)
	e := newEnvWith(t, func(c *Config) {
		c.UploadsDir = func(context.Context) string { return uploads }
		c.FreeSpace = func(string) (int64, error) { return free, nil }
	})
	ctx := context.Background()
	a := e.genVideo(t, filepath.Join(e.dir, "a.mp4"), 1)
	ra, _ := e.svc.AddSources(ctx, []string{a})
	waitCopy(t, e.svc, ra[0].Source.SourceID, store.CopyReady)
	// c：空间不足 → failed
	free = 1
	c := e.genVideo(t, filepath.Join(e.dir, "c.mp4"), 1)
	rc, _ := e.svc.AddSources(ctx, []string{c})
	free = 1 << 50
	// b：复制中
	started, release := holdCopies(t)
	b := filepath.Join(e.dir, "b.mov")
	os.WriteFile(b, make([]byte, 3<<20), 0o644)
	rb, _ := e.svc.AddSources(ctx, []string{b})
	<-started
	res, err := e.svc.SubmitSources(ctx, ConvertSubmitRequest{SourceIDs: []string{rb[0].Source.SourceID, ra[0].Source.SourceID, rc[0].Source.SourceID},
		Options: mp4Opts, OutputDir: filepath.Join(e.dir, "o")})
	release()
	if err != nil || len(res.Tasks) != 1 || res.Tasks[0].SourceID != ra[0].Source.SourceID {
		t.Fatalf("%+v %v", res, err)
	}
	if len(res.Skipped) != 2 || res.Skipped[0] != (SkippedSource{SourceID: rb[0].Source.SourceID, Reason: "copying"}) ||
		res.Skipped[1] != (SkippedSource{SourceID: rc[0].Source.SourceID, Reason: "copy_failed"}) {
		t.Fatalf("%+v", res.Skipped)
	}
	e.wait(t, res.Tasks[0].ID)
}

// PM 13：启动时恢复的中断重转条数只返回一次。
func TestTakeInterruptedReconverts(t *testing.T) {
	e := newEnvWith(t, func(c *Config) { c.InterruptedReconverts = 2 })
	if n := e.svc.TakeInterruptedReconverts(); n != 2 {
		t.Fatal(n)
	}
	if n := e.svc.TakeInterruptedReconverts(); n != 0 {
		t.Fatal(n)
	}
}

// 视频转图片：取第 1 秒那一帧，不足 1 秒取第一帧；result 只有大小和宽高。
func TestVideoToImage(t *testing.T) {
	e := newEnv(t)
	for _, c := range []struct {
		name string
		secs string
	}{{"long.mp4", "3"}, {"short.mp4", "0.4"}} {
		in := e.gen(t, filepath.Join(e.dir, c.name), "-f", "lavfi", "-i", "testsrc=size=320x240:rate=25:duration="+c.secs, "-c:v", "mpeg4")
		src := e.addSource(t, in)
		for _, ext := range []string{"jpg", "png"} {
			tk := e.submitSources(t, ConvertSubmitRequest{SourceIDs: []string{src.SourceID}, PresetID: "builtin-" + ext,
				Options: ffmpeg.ConvertOptions{Container: ext}, OutputDir: filepath.Join(e.dir, "img")})[0]
			if tk.Status != task.StatusSucceeded {
				t.Fatalf("%s → %s: %+v", c.name, ext, tk.Error)
			}
			if filepath.Ext(tk.OutputPath) != "."+ext || tk.Result == nil || tk.Result.Width != 320 || tk.Result.Height != 240 ||
				tk.Result.DurationSec != 0 || tk.Result.SizeBytes <= 0 || tk.Result.AudioBitrateKbps != 0 {
				t.Fatalf("%s → %s: %s %+v", c.name, ext, tk.OutputPath, tk.Result)
			}
		}
	}
}

// 时长未知时 -ss 1 越过结尾、没有产出：按第 0 帧重试一次（6.16.5）。
func TestImageFallbackFirstFrame(t *testing.T) {
	e := newEnv(t)
	in := e.gen(t, filepath.Join(e.dir, "s.mp4"), "-f", "lavfi", "-i", "testsrc=size=160x120:rate=25:duration=0.3", "-c:v", "mpeg4")
	out := filepath.Join(e.dir, "f.jpg")
	frame := func(ss string) func(string) []string {
		return func(part string) []string {
			return []string{"-y", "-ss", ss, "-i", "file:" + in, "-frames:v", "1", "-update", "1", "file:" + part}
		}
	}
	r := &task.FFmpegRunner{Exe: e.bin.FFmpeg, Output: out, Classify: ffmpeg.ClassifyConvertError, BuildArgs: frame("1"), BuildFallbackArgs: frame("0")}
	if _, err := r.Run(context.Background(), func(task.Progress) {}); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(out); err != nil || fi.Size() == 0 {
		t.Fatal("应改取第一帧产出图片")
	}
}

// short_output：输出 < 0.9 × 预期 且差 2 秒以上才标；记录仍是成功。
func TestShortOutput(t *testing.T) {
	cases := []struct {
		exp, act float64
		want     bool
	}{{100, 80, true}, {100, 95, false}, {10, 7.9, true}, {10, 8.5, false}, {3, 1.5, false}, {0, 1, false}, {10, 0, false}}
	for _, c := range cases {
		if shortOutput(c.exp, c.act) != c.want {
			t.Errorf("%v", c)
		}
	}
	e := newEnv(t)
	out := filepath.Join(e.dir, "short.mp4")
	r := &resultRunner{FFmpegRunner: &task.FFmpegRunner{Exe: e.bin.FFmpeg, Output: out, DurationSec: 2, Classify: ffmpeg.ClassifyConvertError,
		BuildArgs: func(part string) []string {
			return []string{"-y", "-f", "lavfi", "-i", "testsrc=size=160x120:rate=10:duration=2", "-c:v", "mpeg4", "file:" + part}
		}}, probe: e.svc.probeResult, expected: 10}
	if _, err := r.Run(context.Background(), func(task.Progress) {}); err != nil {
		t.Skipf("%v", err)
	}
	if r.Result() == nil || len(r.Result().Warnings) != 1 || r.Result().Warnings[0] != WarningShortOutput {
		t.Fatalf("%+v", r.Result())
	}
	r2 := &resultRunner{FFmpegRunner: &task.FFmpegRunner{Exe: e.bin.FFmpeg, Output: filepath.Join(e.dir, "ok.mp4"), DurationSec: 2, Classify: ffmpeg.ClassifyConvertError,
		BuildArgs: func(part string) []string {
			return []string{"-y", "-f", "lavfi", "-i", "testsrc=size=160x120:rate=10:duration=2", "-c:v", "mpeg4", "file:" + part}
		}}, probe: e.svc.probeResult, expected: 2}
	r2.Run(context.Background(), func(task.Progress) {})
	if r2.Result() == nil || len(r2.Result().Warnings) != 0 {
		t.Fatalf("%+v", r2.Result())
	}
}

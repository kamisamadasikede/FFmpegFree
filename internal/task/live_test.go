package task

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/store"
)

func TestSubmitRejectsLegacyLiveTypes(t *testing.T) {
	f := newFx(t, 1)
	for _, typ := range []Type{store.TypeLiveRelay, store.TypeLiveRecordPush} {
		if _, err := f.m.Submit(Spec{Type: typ}, ok); !apperr.Is(err, apperr.InvalidArgument) {
			t.Fatalf("旧类型 %s 不能提交: %v", typ, err)
		}
	}
	tk, err := f.m.Submit(Spec{Type: TypeLiveScreenPush}, ok)
	if err != nil || tk.Progress != -1 {
		t.Fatalf("live_screen_push 应可提交且进度 -1: %+v %v", tk, err)
	}
	waitTask(t, f.m, tk.ID)
	if !IsLive(TypeLiveScreenPush) || !IsLive(TypeLiveFilePush) {
		t.Fatal("IsLive 应包含两个新类型")
	}
}

func TestRetryLiveIsUnsupported(t *testing.T) {
	f := newFx(t, 1)
	tk, _ := f.m.Submit(Spec{Type: TypeLiveFilePush}, ok)
	waitTask(t, f.m, tk.ID)
	_, err := f.m.Retry(tk.ID)
	if !apperr.Is(err, apperr.Unsupported) {
		t.Fatalf("直播会话 Retry 应 UNSUPPORTED: %v", err)
	}
	if ae := apperr.From(err); ae.Message != "直播会话不能重试，请重新开始推流" {
		t.Fatalf("message: %q", ae.Message)
	}
}

// 库里的旧类型记录：List / Get / ListTasksByStatus 忽略、不报错，不影响其他记录。
func TestLegacyTypeRowsIgnoredOnRead(t *testing.T) {
	f := newFx(t, 1)
	ctx := context.Background()
	for i, typ := range []store.TaskType{store.TypeLiveRelay, store.TypeLiveRecordPush, store.TypeConvert} {
		tk := store.Task{ID: "L" + string(rune('A'+i)), Type: typ, Status: store.StatusSucceeded, Title: "x", InputPaths: []string{}, Version: 1, CreatedAt: int64(100 + i)}
		if err := f.st.InsertTask(ctx, tk); err != nil {
			t.Fatal(err)
		}
	}
	p, err := f.m.List(Filter{})
	if err != nil || p.Total != 1 || len(p.Items) != 1 || p.Items[0].Type != store.TypeConvert {
		t.Fatalf("List 应只看到 convert: %+v %v", p, err)
	}
	if _, err := f.m.Get("LA"); !apperr.Is(err, apperr.NotFound) {
		t.Fatalf("旧类型 Get 按不存在处理: %v", err)
	}
	if _, err := f.m.Get("LC"); err != nil {
		t.Fatal(err)
	}
	if p, err := f.m.List(Filter{Types: []Type{store.TypeLiveRelay}}); err != nil || p.Total != 0 {
		t.Fatalf("按旧类型过滤也忽略: %+v %v", p, err)
	}
}

func TestRateWindow(t *testing.T) {
	var w rateWindow
	if v := w.add(0, 0); v != 0 {
		t.Fatal(v)
	}
	// 1000 kbit/s：每秒 125000 字节
	var v float64
	for i := 1; i <= 12; i++ {
		v = w.add(float64(i), int64(i)*125000)
	}
	if math.Abs(v-1000) > 1 {
		t.Fatalf("匀速 1000kbps: %v", v)
	}
	// 码率突变为 2000 kbit/s，5 秒后窗口里几乎全是新码率
	size := int64(12 * 125000)
	for i := 13; i <= 20; i++ {
		size += 250000
		v = w.add(float64(i), size)
	}
	if math.Abs(v-2000) > 1 {
		t.Fatalf("窗口应只覆盖最近 5 秒: %v", v)
	}
	// out_time 不增长 / 回退 / size 未知：沿用上一个值，永不 NaN / Inf
	for _, c := range []struct {
		out  float64
		size int64
	}{{20, size + 1}, {19, size + 100}, {21, 0}, {0, 5}} {
		if got := w.add(c.out, c.size); got != v || math.IsNaN(got) || math.IsInf(got, 0) {
			t.Fatalf("应沿用上一个值 %v，得到 %v", v, got)
		}
	}
}

// 直播任务的 task:progress 带 fps / bitrateKbps / droppedFrames；NoBitrate 时不带 bitrateKbps；ReportGate 之前不上报。
func TestFFmpegRunnerLiveMetricsAndGate(t *testing.T) {
	f := newFx(t, 1)
	script := fakeMetricsBin(t)
	run := func(noBitrate bool) (Task, []ProgressEvent) {
		em := len(f.em.all())
		r := &FFmpegRunner{Exe: script, Live: true, NoBitrate: noBitrate,
			BuildArgs:  func(string) []string { return []string{"metrics"} },
			ReportGate: func(u ffmpeg.ProgressUpdate) bool { return !u.End && u.OutTimeSec > 0 }}
		tk, _ := f.m.Submit(Spec{Type: TypeLiveFilePush}, r)
		d := waitTask(t, f.m, tk.ID)
		var evs []ProgressEvent
		for _, e := range f.em.all()[em:] {
			if p, ok := e.payload.(ProgressEvent); ok && p.ID == tk.ID {
				evs = append(evs, p)
			}
		}
		return d, evs
	}
	d, evs := run(false)
	if d.Status != StatusSucceeded || len(evs) == 0 {
		t.Fatalf("%+v %d", d, len(evs))
	}
	if evs[0].OutTimeSec <= 0 {
		t.Fatalf("第一条上报的 progress 必须已经 out_time>0: %+v", evs[0])
	}
	found := false
	for _, e := range evs {
		if e.Progress != -1 {
			t.Fatalf("%+v", e)
		}
		if e.Fps == 25 && e.DroppedFrames == 3 && e.BitrateKbps > 900 && e.BitrateKbps < 1100 {
			found = true
		}
	}
	if !found {
		t.Fatalf("应有一条带 fps=25 / dropped=3 / 约 1000kbps 的 progress: %+v", evs)
	}
	d, evs = run(true)
	if d.Status != StatusSucceeded {
		t.Fatalf("%+v", d)
	}
	for _, e := range evs {
		if e.BitrateKbps != 0 {
			t.Fatalf("有存档时不应有 bitrateKbps: %+v", e)
		}
	}
	// 终态后指标清零（只在运行中有值）
	if d.Fps != 0 || d.BitrateKbps != 0 || d.DroppedFrames != 0 {
		t.Fatalf("终态不带实时指标: %+v", d)
	}
	_ = time.Second
}

// 假 ffmpeg：先给一个 out_time_us=N/A 的块（不应上报），再给匀速 1000kbps 的块，最后 progress=end。
func fakeMetricsBin(t *testing.T) string {
	p := filepath.Join(t.TempDir(), "ffmpeg")
	script := `#!/bin/sh
echo "fps=0.00"; echo "total_size=0"; echo "out_time_us=N/A"; echo "progress=continue"
i=1
while [ $i -le 8 ]; do
  echo "fps=25.00"; echo "drop_frames=3"; echo "total_size=$((i*125000))"; echo "out_time_us=$((i*1000000))"; echo "speed=1.00x"; echo "progress=continue"
  i=$((i+1))
done
echo "total_size=1000000"; echo "out_time_us=8000000"; echo "progress=end"
exit 0
`
	os.WriteFile(p, []byte(script), 0o755)
	return p
}

// 旧类型（保留但不再产生）：库里预置记录 + 日志 + 输出文件，Get / Cancel / Remove / Retry / GetLog 一律 NOT_FOUND，
// Remove 整体失败且同批合法 id 不被删、旧记录的日志和输出文件不被碰；真正不存在的 id 仍被忽略。
func TestLegacyTypeIDsAreNotFoundEverywhere(t *testing.T) {
	f := newFx(t, 1)
	ctx := context.Background()
	if err := os.MkdirAll(filepath.Join(f.dir, "logs"), 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := []store.TaskType{store.TypeLiveRelay, store.TypeLiveRecordPush, store.TypeEditRender}
	type rec struct{ id, log, out string }
	var recs []rec
	for i, typ := range legacy {
		id := fmt.Sprintf("OLD%d", i)
		r := rec{id: id, log: filepath.Join(f.dir, "logs", id+".log"), out: filepath.Join(f.dir, id+".mp4")}
		for _, p := range []string{r.log, r.out} {
			if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		// 状态覆盖已结束（succeeded）与"进行中"（running，遗留的脏数据）两种
		st := store.StatusSucceeded
		if i == 1 {
			st = store.StatusRunning
		}
		tk := store.Task{ID: id, Type: typ, Status: st, Title: "old", InputPaths: []string{}, OutputPath: r.out, LogPath: r.log,
			Version: 1, CreatedAt: int64(100 + i), StartedAt: 1}
		if err := f.st.InsertTask(ctx, tk); err != nil {
			t.Fatal(err)
		}
		recs = append(recs, r)
	}
	// 一条合法的已结束任务，用来验证 Remove 整体失败时不被删
	good := store.Task{ID: "GOOD1", Type: store.TypeConvert, Status: store.StatusFailed, Title: "g", InputPaths: []string{}, Version: 1, CreatedAt: 200}
	if err := f.st.InsertTask(ctx, good); err != nil {
		t.Fatal(err)
	}

	for _, r := range recs {
		if _, err := f.m.Get(r.id); !apperr.Is(err, apperr.NotFound) {
			t.Errorf("Get(%s): %v", r.id, err)
		}
		if err := f.m.Cancel(r.id); !apperr.Is(err, apperr.NotFound) {
			t.Errorf("Cancel(%s) 应 NOT_FOUND（不是 TASK_CONFLICT）: %v", r.id, err)
		}
		if _, err := f.m.Retry(r.id); !apperr.Is(err, apperr.NotFound) {
			t.Errorf("Retry(%s) 应 NOT_FOUND（不是 UNSUPPORTED）: %v", r.id, err)
		}
		if _, err := f.m.GetLog(r.id, 10); !apperr.Is(err, apperr.NotFound) {
			t.Errorf("GetLog(%s): %v", r.id, err)
		}
		for _, del := range []bool{false, true} {
			if err := f.m.Remove([]string{r.id}, del); !apperr.Is(err, apperr.NotFound) {
				t.Errorf("Remove(%s, %v) 应 NOT_FOUND: %v", r.id, del, err)
			}
			if err := f.m.Remove([]string{"GOOD1", "nonexistent", r.id}, del); !apperr.Is(err, apperr.NotFound) {
				t.Errorf("同批含旧类型应整体 NOT_FOUND: %v", err)
			}
		}
	}
	// 旧记录还在库里（直接查表），日志和输出文件没被碰；同批合法 id 没被删
	for _, r := range recs {
		var n int
		if err := f.st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE id = ?`, r.id).Scan(&n); err != nil || n != 1 {
			t.Errorf("旧记录 %s 不应被删: n=%d err=%v", r.id, n, err)
		}
		for _, p := range []string{r.log, r.out} {
			if _, err := os.Stat(p); err != nil {
				t.Errorf("旧记录的文件不应被删: %v", err)
			}
		}
	}
	if _, err := f.m.Get("GOOD1"); err != nil {
		t.Fatalf("同批合法 id 不应被删: %v", err)
	}
	// 真正不存在的 id 仍然忽略；合法 id 正常删除
	if err := f.m.Remove([]string{"nonexistent"}, false); err != nil {
		t.Fatalf("不存在的 id 应忽略: %v", err)
	}
	if err := f.m.Cancel("nonexistent"); !apperr.Is(err, apperr.NotFound) {
		t.Fatalf("%v", err)
	}
	if err := f.m.Remove([]string{"GOOD1"}, false); err != nil {
		t.Fatal(err)
	}
	// ClearFinished 也不动旧记录（任务中心不展示，也没有入口删）
	if err := f.m.ClearFinished(); err != nil {
		t.Fatal(err)
	}
	for _, r := range recs {
		for _, p := range []string{r.log, r.out} {
			if _, err := os.Stat(p); err != nil {
				t.Errorf("ClearFinished 不应删旧记录的文件: %v", err)
			}
		}
	}
	// 没有任何 legacy 类型能被提交
	for _, typ := range legacy {
		if _, err := f.m.Submit(Spec{Type: typ}, RunnerFunc(func(context.Context, func(Progress)) (string, error) { return "", nil })); err == nil {
			t.Errorf("%s 不应能提交", typ)
		}
	}
}

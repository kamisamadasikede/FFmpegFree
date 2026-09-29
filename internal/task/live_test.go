package task

import (
	"context"
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

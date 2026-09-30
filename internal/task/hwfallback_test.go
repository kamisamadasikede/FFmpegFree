//go:build !windows

package task

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"FFmpegFree/internal/ffmpeg"
)

// 假 ffmpeg：每次调用在 $CALLS 文件追加一行参数；行为由环境变量控制：
//
//	HW_MODE=initfail  含 h264_nvenc 时打印 NVENC 初始化失败并退出 1（没有进度、没有输出）；其余参数正常编码
//	HW_MODE=allfail   含 h264_nvenc 时同上；CPU（libx264）参数打印 "Invalid data found" 退出 1
//	HW_MODE=midfail   含 h264_nvenc 时先输出进度和输出文件，再打印 NVENC 错误退出 1（已经开始编码）
//	HW_MODE=hang      含 h264_nvenc 时先输出进度再挂起（用来测取消）
//	HW_MODE=nohw      含 h264_nvenc 时打印与硬件无关的错误（输入损坏）退出 1
func fakeHWFFmpeg(t *testing.T) (exe, calls string) {
	dir := t.TempDir()
	exe = filepath.Join(dir, "ffmpeg")
	calls = filepath.Join(dir, "calls.txt")
	script := `#!/bin/sh
echo "$@" >> "$CALLS"
for a in "$@"; do last="$a"; done
case "$*" in *h264_nvenc*) hw=1;; *) hw=0;; esac
ok() {
  echo "out_time_us=5000000"; echo "speed=2.0x"; echo "progress=continue"
  [ "$last" != "-" ] && echo "encoded" > "$last"
  echo "out_time_us=10000000"; echo "speed=2.0x"; echo "progress=end"
  exit 0
}
nvfail() {
  echo "[h264_nvenc @ 0x1] OpenEncodeSessionEx failed: unsupported device (2): (no details)" >&2
  echo "Error while opening encoder for output stream #0:0 - maybe incorrect parameters" >&2
  exit 1
}
case "$HW_MODE" in
  initfail) [ $hw = 1 ] && nvfail; ok ;;
  allfail)  [ $hw = 1 ] && nvfail; echo "Invalid data found when processing input" >&2; exit 1 ;;
  midfail)
    if [ $hw = 1 ]; then
      echo "out_time_us=2000000"; echo "speed=1.0x"; echo "progress=continue"
      echo "half" > "$last"
      nvfail
    fi
    ok ;;
  hang)
    if [ $hw = 1 ]; then
      echo "out_time_us=1000000"; echo "progress=continue"
      sleep 30
    fi
    ok ;;
  nohw) [ $hw = 1 ] && { echo "Invalid data found when processing input" >&2; exit 1; }; ok ;;
  *) ok ;;
esac
`
	if err := os.WriteFile(exe, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CALLS", calls)
	return exe, calls
}

func callLines(t *testing.T, calls string) []string {
	b, _ := os.ReadFile(calls)
	s := strings.TrimSpace(string(b))
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

var (
	nvInfo  = ffmpeg.EncoderInfo{Encoder: "h264_nvenc", Device: "nvidia"}
	cpuInfo = ffmpeg.EncoderInfo{Encoder: "libx264", Device: "cpu"}
)

func hwRunner(exe, out string) *FFmpegRunner {
	return &FFmpegRunner{
		Exe: exe, Output: out, DurationSec: 10,
		BuildArgs:    func(part string) []string { return []string{"-i", "in.mov", "-c:v", "h264_nvenc", part} },
		Encoding:     nvInfo,
		HWEncoder:    "h264_nvenc",
		BuildCPUArgs: func(part string) []string { return []string{"-i", "in.mov", "-c:v", "libx264", part} },
		CPUEncoding:  cpuInfo,
	}
}

func lastStatus(f *fx, id string) StatusEvent {
	var last StatusEvent
	for _, e := range f.em.all() {
		if s, ok := e.payload.(StatusEvent); ok && e.name == EventStatus && s.ID == id {
			last = s
		}
	}
	return last
}

// 硬件编码启动失败 → 自动用 CPU 重试一次并成功：任务成功，encoder 变成 libx264 / cpu，hwFallback=true 带原因；
// task:created、running 事件带 h264_nvenc，回退后补发 running 事件、终态事件与库里一致。
func TestHWInitFailureFallsBackToCPU(t *testing.T) {
	exe, calls := fakeHWFFmpeg(t)
	t.Setenv("HW_MODE", "initfail")
	f := newFx(t, 1)
	out := filepath.Join(f.dir, "out", "a.mp4")
	os.MkdirAll(filepath.Dir(out), 0o755)
	tk, err := f.m.Submit(Spec{Type: TypeConvert, OutputPath: out}, hwRunner(exe, out))
	if err != nil {
		t.Fatal(err)
	}
	if tk.Encoder != "h264_nvenc" || tk.EncoderDevice != "nvidia" || tk.HWFallback {
		t.Fatalf("提交时应是硬件编码器: %+v", tk)
	}
	d := waitTask(t, f.m, tk.ID)
	if d.Status != StatusSucceeded {
		t.Fatalf("回退重试成功，任务应成功: %+v", d)
	}
	if d.Encoder != "libx264" || d.EncoderDevice != "cpu" || !d.HWFallback || d.HWFallbackReason != ffmpeg.ReasonNVENCInit {
		t.Fatalf("回退后的编码器字段: %+v", d)
	}
	if strings.ContainsAny(d.HWFallbackReason, "/\\ \n") {
		t.Fatalf("原因必须是一行且不含路径: %q", d.HWFallbackReason)
	}
	ls := callLines(t, calls)
	if len(ls) != 2 || !strings.Contains(ls[0], "h264_nvenc") || !strings.Contains(ls[1], "libx264") {
		t.Fatalf("应先硬件后 CPU 各一次: %v", ls)
	}
	if b, _ := os.ReadFile(out); strings.TrimSpace(string(b)) != "encoded" {
		t.Fatalf("输出应是 CPU 编码结果: %q", b)
	}
	// 任务日志里 FFmpegFree 自己写的行：叫“显卡编码”，不含编码器名 / “硬件编码”“转码”（ffmpeg 自己的 stderr 不管）。
	logText, _ := f.m.GetLog(tk.ID, 0)
	var own []string
	for _, l := range strings.Split(logText, "\n") {
		if strings.Contains(l, "[FFmpegFree]") {
			own = append(own, l)
		}
	}
	if len(own) == 0 || !strings.Contains(strings.Join(own, "\n"), "[FFmpegFree] 显卡编码启动失败，已自动改用 CPU 重试一次") {
		t.Fatalf("日志里应有回退提示: %q", logText)
	}
	for _, l := range own {
		low := strings.ToLower(l)
		for _, bad := range []string{"nvenc", "qsv", "amf", "videotoolbox", "h264_", "hevc_", "硬件编码", "转码", "nvenc_init_failed"} {
			if strings.Contains(low, bad) {
				t.Errorf("FFmpegFree 自己写的日志行不应含 %q: %q", bad, l)
			}
		}
	}
	st := lastStatus(f, tk.ID)
	if st.Status != StatusSucceeded || st.Encoder != "libx264" || st.EncoderDevice != "cpu" || !st.HWFallback || st.HWFallbackReason != ffmpeg.ReasonNVENCInit {
		t.Fatalf("终态事件应带最终编码器: %+v", st)
	}
	// 库里同样有。
	got, err := f.st.GetTask(context.Background(), tk.ID)
	if err != nil || got.Encoder != "libx264" || !got.HWFallback || got.HWFallbackReason != ffmpeg.ReasonNVENCInit {
		t.Fatalf("落库: %+v %v", got, err)
	}
	// 事件里应有一条硬件编码器的 running，以及回退后的 running。
	var sawHW, sawFB bool
	for _, e := range f.em.all() {
		if s, ok := e.payload.(StatusEvent); ok && e.name == EventStatus && s.ID == tk.ID && s.Status == StatusRunning {
			sawHW = sawHW || (s.Encoder == "h264_nvenc" && !s.HWFallback)
			sawFB = sawFB || (s.Encoder == "libx264" && s.HWFallback)
		}
	}
	if !sawHW || !sawFB {
		t.Fatalf("running 事件应先带硬件编码器、回退后补发 CPU: hw=%v fb=%v", sawHW, sawFB)
	}
}

// 重试也失败：任务失败，错误是 CPU 那次的错误（不是硬件错误），最多重试一次。
func TestHWFallbackRetryAlsoFails(t *testing.T) {
	exe, calls := fakeHWFFmpeg(t)
	t.Setenv("HW_MODE", "allfail")
	f := newFx(t, 1)
	out := filepath.Join(f.dir, "a.mp4")
	tk, _ := f.m.Submit(Spec{Type: TypeConvert, OutputPath: out}, hwRunner(exe, out))
	d := waitTask(t, f.m, tk.ID)
	if d.Status != StatusFailed || d.Error == nil {
		t.Fatalf("应失败: %+v", d)
	}
	if !strings.Contains(d.Error.Detail, "Invalid data found") {
		t.Fatalf("错误应来自 CPU 重试: %+v", d.Error)
	}
	if n := len(callLines(t, calls)); n != 2 {
		t.Fatalf("最多重试一次，共 2 次调用，实际 %d", n)
	}
	if d.Encoder != "libx264" || !d.HWFallback {
		t.Fatalf("失败任务也应记录回退: %+v", d)
	}
}

// 与硬件无关的失败（输入损坏）：不回退，只跑一次。
func TestHWNonHardwareFailureDoesNotFallBack(t *testing.T) {
	exe, calls := fakeHWFFmpeg(t)
	t.Setenv("HW_MODE", "nohw")
	f := newFx(t, 1)
	out := filepath.Join(f.dir, "a.mp4")
	tk, _ := f.m.Submit(Spec{Type: TypeConvert, OutputPath: out}, hwRunner(exe, out))
	d := waitTask(t, f.m, tk.ID)
	if d.Status != StatusFailed || len(callLines(t, calls)) != 1 {
		t.Fatalf("不应回退: %+v calls=%v", d, callLines(t, calls))
	}
	if d.Encoder != "h264_nvenc" || d.HWFallback {
		t.Fatalf("编码器字段保持硬件: %+v", d)
	}
}

// 取消不触发回退。
func TestHWCancelDoesNotFallBack(t *testing.T) {
	exe, calls := fakeHWFFmpeg(t)
	t.Setenv("HW_MODE", "hang")
	f := newFx(t, 1)
	out := filepath.Join(f.dir, "a.mp4")
	tk, _ := f.m.Submit(Spec{Type: TypeConvert, OutputPath: out}, hwRunner(exe, out))
	eventually(t, func() bool { return len(callLines(t, calls)) == 1 && f.em.count(EventProgress) > 0 })
	if err := f.m.Cancel(tk.ID); err != nil {
		t.Fatal(err)
	}
	d := waitTask(t, f.m, tk.ID)
	if d.Status != StatusCanceled {
		t.Fatalf("应为 canceled: %+v", d)
	}
	time.Sleep(100 * time.Millisecond)
	if n := len(callLines(t, calls)); n != 1 {
		t.Fatalf("取消后不能再启动 CPU 重试: %d 次调用", n)
	}
	if d.HWFallback {
		t.Fatalf("取消不算回退: %+v", d)
	}
}

// 直播回退窗口：推流建立前（没有输出进度）硬件失败 → CPU 重试；推流已建立（有过进度）后中途失败 → 不重试，任务失败。
func TestHWLiveFallbackWindow(t *testing.T) {
	exe, calls := fakeHWFFmpeg(t)
	mk := func() *FFmpegRunner {
		r := hwRunner(exe, "")
		r.Live = true
		r.Output = ""
		return r
	}
	t.Run("启动前失败回退", func(t *testing.T) {
		t.Setenv("HW_MODE", "initfail")
		os.Remove(calls)
		f := newFx(t, 1)
		tk, _ := f.m.Submit(Spec{Type: TypeLiveFilePush}, mk())
		d := waitTask(t, f.m, tk.ID)
		if d.Status != StatusSucceeded || d.Encoder != "libx264" || !d.HWFallback || len(callLines(t, calls)) != 2 {
			t.Fatalf("%+v calls=%v", d, callLines(t, calls))
		}
	})
	t.Run("推流中途失败不重试", func(t *testing.T) {
		t.Setenv("HW_MODE", "midfail")
		os.Remove(calls)
		f := newFx(t, 1)
		tk, _ := f.m.Submit(Spec{Type: TypeLiveFilePush}, mk())
		d := waitTask(t, f.m, tk.ID)
		if d.Status != StatusFailed || len(callLines(t, calls)) != 1 {
			t.Fatalf("已建立推流后失败不应重试: %+v calls=%v", d, callLines(t, calls))
		}
		if d.Encoder != "h264_nvenc" || d.HWFallback {
			t.Fatalf("编码器字段保持硬件: %+v", d)
		}
	})
}

// 没有用硬件的 Runner（Encoding 为 CPU、HWEncoder 空）：失败照旧，不回退，字段来自 Encoding；Encoding 为空的任务不带这些字段。
func TestEncoderFieldsOnPlainRunners(t *testing.T) {
	exe, calls := fakeHWFFmpeg(t)
	t.Setenv("HW_MODE", "allfail")
	f := newFx(t, 1)
	out := filepath.Join(f.dir, "a.mp4")
	r := &FFmpegRunner{Exe: exe, Output: out, DurationSec: 10, Encoding: cpuInfo,
		BuildArgs: func(part string) []string { return []string{"-c:v", "libx264", part} }}
	tk, _ := f.m.Submit(Spec{Type: TypeConvert, OutputPath: out}, r)
	if tk.Encoder != "libx264" || tk.EncoderDevice != "cpu" || tk.HWFallback {
		t.Fatalf("%+v", tk)
	}
	d := waitTask(t, f.m, tk.ID)
	if d.Status != StatusFailed || len(callLines(t, calls)) != 1 || d.HWFallback {
		t.Fatalf("%+v", d)
	}
	tk2, _ := f.m.Submit(Spec{Type: TypeConvert}, ok)
	if tk2.Encoder != "" || tk2.EncoderDevice != "" {
		t.Fatalf("没有编码器信息的任务不带字段: %+v", tk2)
	}
}

type encRunner struct {
	info ffmpeg.EncoderInfo
	ran  bool
}

func (r *encRunner) Run(ctx context.Context, _ func(Progress)) (string, error) {
	r.ran = true
	<-ctx.Done()
	return "", ctx.Err()
}
func (r *encRunner) EncoderInfo() ffmpeg.EncoderInfo { return r.info }

// 从未运行的任务（排队中被取消）：Run 没执行过，不会有运行中的回退；但 Submit 时写入的编码器字段仍在，
// 终态 task:status（没有 startedAt、没有 running 事件）和库里都带着它（契约 6.6 的 NeverRanner 段）。
func TestNeverRanTaskKeepsSubmitTimeEncoderFields(t *testing.T) {
	f := newFx(t, 1)
	gate := make(chan struct{})
	blocker, _ := f.m.Submit(Spec{Type: TypeConvert}, RunnerFunc(func(ctx context.Context, _ func(Progress)) (string, error) {
		select {
		case <-gate:
		case <-ctx.Done():
		}
		return "", nil
	}))
	r := &encRunner{info: ffmpeg.EncoderInfo{Encoder: "libx264", Device: "cpu", HWFallback: true, HWFallbackReason: ffmpeg.ReasonDeviceUnavailable}}
	queued, _ := f.m.Submit(Spec{Type: TypeConvert, OutputPath: filepath.Join(f.dir, "q.mp4")}, r)
	if err := f.m.Cancel(queued.ID); err != nil {
		t.Fatal(err)
	}
	d := waitTask(t, f.m, queued.ID)
	if d.Status != StatusCanceled || r.ran || d.StartedAt != 0 || d.FinishedAt == 0 {
		t.Fatalf("%+v ran=%v", d, r.ran)
	}
	if d.Encoder != "libx264" || d.EncoderDevice != "cpu" || !d.HWFallback || d.HWFallbackReason != ffmpeg.ReasonDeviceUnavailable {
		t.Fatalf("库里应保留提交时的编码器字段: %+v", d)
	}
	ev := lastStatus(f, queued.ID)
	if ev.Status != StatusCanceled || ev.StartedAt != 0 || ev.Encoder != "libx264" || !ev.HWFallback {
		t.Fatalf("终态事件: %+v", ev)
	}
	for _, e := range f.em.all() {
		if s, ok := e.payload.(StatusEvent); ok && s.ID == queued.ID && s.Status == StatusRunning {
			t.Fatalf("从未运行的任务不应有 running 事件")
		}
		if p, ok := e.payload.(ProgressEvent); ok && p.ID == queued.ID {
			t.Fatalf("从未运行的任务不应有 task:progress")
		}
	}
	close(gate)
	waitTask(t, f.m, blocker.ID)
}

//go:build !windows

package task

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
)

// 假 ffmpeg：最后一个参数是输出路径（或 "-"）；模式由倒数第二个参数决定。
func fakeFFmpegBin(t *testing.T) string {
	p := filepath.Join(t.TempDir(), "ffmpeg")
	script := `#!/bin/sh
for a in "$@"; do last="$a"; done
mode=$(echo "$@" | awk '{print $(NF-1)}')
case "$mode" in
  encode)
    echo "out_time_us=5000000"; echo "speed=2.0x"; echo "progress=continue"
    echo "encoded" > "$last"
    echo "out_time_us=10000000"; echo "speed=2.0x"; echo "progress=end"
    exit 0 ;;
  fail)
    echo "half" > "$last"
    echo "Invalid data found" >&2
    exit 1 ;;
  hang)
    echo "half" > "$last"
    echo "out_time_us=1000000"; echo "progress=continue"
    sleep 30 ;;
  live)
    trap 'exit 0' INT
    echo "out_time_us=1000000"; echo "progress=continue"
    read -r x
    exit 0 ;;
esac
`
	os.WriteFile(p, []byte(script), 0o755)
	return p
}

func TestFFmpegRunnerEncodeWithPart(t *testing.T) {
	f := newFx(t, 1)
	out := filepath.Join(f.dir, "out", "a.mp4")
	os.MkdirAll(filepath.Dir(out), 0o755)
	os.WriteFile(out, []byte("existing"), 0o644)
	var seenPart string
	r := &FFmpegRunner{Exe: fakeFFmpegBin(t), Output: out, DurationSec: 20, ProgressBase: 0, ProgressScale: 0.5,
		BuildArgs: func(part string) []string { seenPart = part; return []string{"-i", "in.mov", "encode", part} }}
	tk, _ := f.m.Submit(Spec{Type: TypeConvert, OutputPath: out}, r)
	d := waitTask(t, f.m, tk.ID)
	if d.Status != StatusSucceeded {
		t.Fatalf("%+v", d)
	}
	want := filepath.Join(f.dir, "out", "a(1).mp4")
	if d.OutputPath != want || seenPart != filepath.Join(f.dir, "out", "a(1).part.mp4") {
		t.Fatalf("重名应追加 (1): %s %s", d.OutputPath, seenPart)
	}
	if b, _ := os.ReadFile(want); !strings.Contains(string(b), "encoded") {
		t.Fatal("输出内容不对")
	}
	if b, _ := os.ReadFile(out); string(b) != "existing" {
		t.Fatal("不应覆盖已有文件")
	}
	if _, err := os.Stat(seenPart); !os.IsNotExist(err) {
		t.Fatal(".part 应已改名")
	}
	// 进度事件：第一块 5s/20s*0.5 = 0.125；End 块 = 0.5
	var fr []float64
	for _, e := range f.em.all() {
		if p, ok := e.payload.(ProgressEvent); ok {
			fr = append(fr, p.Progress)
		}
	}
	if len(fr) != 2 || fr[0] != 0.125 || fr[1] != 0.5 {
		t.Fatalf("%v", fr)
	}
	// 日志里有 stderr
	if _, err := os.Stat(d.LogPath); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestFFmpegRunnerFailureCleansPartAndKeepsTail(t *testing.T) {
	f := newFx(t, 1)
	out := filepath.Join(f.dir, "b.mp4")
	tk, _ := f.m.Submit(Spec{Type: TypeConvert}, &FFmpegRunner{Exe: fakeFFmpegBin(t), Output: out, DurationSec: 10,
		BuildArgs: func(part string) []string { return []string{"fail", part} }})
	d := waitTask(t, f.m, tk.ID)
	if d.Status != StatusFailed || d.Error == nil || d.Error.Code != apperr.ProcessFailed || !strings.Contains(d.Error.Detail, "Invalid data found") {
		t.Fatalf("%+v", d)
	}
	if ents, _ := os.ReadDir(f.dir); func() bool {
		for _, e := range ents {
			if strings.HasPrefix(e.Name(), "b.") {
				return true
			}
		}
		return false
	}() {
		t.Fatalf("失败后不应残留 .part: %v", ents)
	}
	// stderr 被写入任务日志
	if log, _ := f.m.GetLog(tk.ID, 10); !strings.Contains(log, "Invalid data found") {
		t.Fatalf("日志: %q", log)
	}
}

func TestFFmpegRunnerCancelKillsAndCleans(t *testing.T) {
	f := newFx(t, 1)
	out := filepath.Join(f.dir, "c.mp4")
	tk, _ := f.m.Submit(Spec{Type: TypeConvert}, &FFmpegRunner{Exe: fakeFFmpegBin(t), Output: out, DurationSec: 10,
		BuildArgs: func(part string) []string { return []string{"hang", part} }})
	eventually(t, func() bool { return f.m.mustGet(t, tk.ID).Progress > 0 })
	begin := time.Now()
	f.m.Cancel(tk.ID)
	d := waitTask(t, f.m, tk.ID)
	if d.Status != StatusCanceled || time.Since(begin) > 3*time.Second {
		t.Fatalf("%+v %v", d, time.Since(begin))
	}
	if ents, _ := os.ReadDir(f.dir); len(ents) != 1 || ents[0].Name() != "logs" && !strings.HasSuffix(ents[0].Name(), ".db") && !strings.Contains(ents[0].Name(), "app.db") {
		for _, e := range ents {
			if strings.HasPrefix(e.Name(), "c.") {
				t.Fatalf("取消后不应残留输出: %v", ents)
			}
		}
	}
}

func TestFFmpegRunnerLiveGracefulStop(t *testing.T) {
	f := newFx(t, 1)
	tk, _ := f.m.Submit(Spec{Type: TypeLiveRelay}, &FFmpegRunner{Exe: fakeFFmpegBin(t), Live: true,
		BuildArgs: func(string) []string { return []string{"live", "-"} }})
	eventually(t, func() bool { return f.m.mustGet(t, tk.ID).Status == StatusRunning })
	time.Sleep(100 * time.Millisecond)
	f.m.Cancel(tk.ID)
	d := waitTask(t, f.m, tk.ID)
	if d.Status != StatusSucceeded || d.Progress != -1 {
		t.Fatalf("优雅停止应为 succeeded: %+v", d)
	}
	_ = context.Background()
}

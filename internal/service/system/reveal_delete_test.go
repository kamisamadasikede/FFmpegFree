package system

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/store"
	"FFmpegFree/internal/task"
)

// 契约 v0.23.3：删除失败留下的文件，删除调用之后 10 分钟内可以 RevealInFolder（即使不在 defaultOutputDir 里）；
// 过期后、以及无关路径照旧拒绝。
func TestRevealAllowsRecentDeleteFailure(t *testing.T) {
	f := newRevealFx(t)
	var mu sync.Mutex
	now := time.Now()
	clock := func() time.Time { mu.Lock(); defer mu.Unlock(); return now }
	advance := func(d time.Duration) { mu.Lock(); now = now.Add(d); mu.Unlock() }
	st, err := store.Open(context.Background(), filepath.Join(f.root, "app2.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	tm := task.NewManager(task.Config{Store: st, LogDir: filepath.Join(f.root, "logs2"), BatchConcurrency: 1,
		ProgressInterval: -1, Logf: func(string, ...any) {}, Now: clock})
	t.Cleanup(func() { tm.Shutdown(2 * time.Second) })
	f.mgr.cfg.Tasks = tm
	f.setOut(t, f.out) // 输出放在默认输出目录之外

	out := filepath.Join(f.outside, "conv.mp4")
	tk, err := tm.Submit(task.Spec{Type: task.TypeConvert, OutputPath: out, ReserveOutput: true}, task.RunnerFunc(func(ctx context.Context, _ func(task.Progress)) (string, error) {
		return task.RunWithPart(ctx, out, func(part string) error { return os.WriteFile(part, []byte("v"), 0o644) })
	}))
	if err != nil {
		t.Fatal(err)
	}
	done, err := tm.Wait(context.Background(), tk.ID)
	if err != nil || done.Status != task.StatusSucceeded {
		t.Fatalf("%+v %v", done, err)
	}
	// 文件被换过（修改时间早于任务开始）→ not_task_output，记录照删、文件留下
	old := time.UnixMilli(done.StartedAt).Add(-time.Hour)
	os.Chtimes(done.OutputPath, old, old)
	res, err := tm.DeleteRecords([]string{tk.ID}, task.TypeConvert, true, nil)
	if err != nil || len(res.Failures) != 1 || res.Failures[0].Path != done.OutputPath || res.Failures[0].Reason != task.DeleteNotTaskOutput {
		t.Fatalf("%+v %v", res, err)
	}
	if tm.IsTaskOutput(done.OutputPath) {
		t.Fatal("记录已删，不再是登记的输出")
	}
	f.launched = nil
	if err := f.mgr.RevealInFolder(res.Failures[0].Path); err != nil || len(f.launched) != 1 {
		t.Fatalf("删除失败的路径应能打开所在文件夹: %v %v", err, f.launched)
	}
	// 同目录的无关文件、所在文件夹本身仍然拒绝
	other := f.file(t, filepath.Join(f.outside, "other.mp4"))
	if c := revealCode(f.mgr.RevealInFolder(other)); c != apperr.InvalidArgument {
		t.Fatalf("无关路径应拒绝: %v", c)
	}
	if c := revealCode(f.mgr.RevealInFolder(f.outside)); c != apperr.InvalidArgument {
		t.Fatalf("所在文件夹本身不在放行表里: %v", c)
	}
	// 9 分钟后仍可以，过了 10 分钟拒绝
	advance(9 * time.Minute)
	if err := f.mgr.RevealInFolder(done.OutputPath); err != nil {
		t.Fatalf("10 分钟内应可以: %v", err)
	}
	advance(2 * time.Minute)
	if c := revealCode(f.mgr.RevealInFolder(done.OutputPath)); c != apperr.InvalidArgument {
		t.Fatalf("过期后应拒绝: %v", c)
	}
}

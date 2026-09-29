package task

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/store"
)

type rec struct {
	mu   sync.Mutex
	evts []recEvt
}
type recEvt struct {
	name    string
	payload any
}

func (r *rec) Emit(name string, p any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.evts = append(r.evts, recEvt{name, p})
}
func (r *rec) all() []recEvt {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recEvt(nil), r.evts...)
}
func (r *rec) statuses(id string) []Status {
	var out []Status
	for _, e := range r.all() {
		if s, ok := e.payload.(StatusEvent); ok && e.name == EventStatus && s.ID == id {
			out = append(out, s.Status)
		}
	}
	return out
}
func (r *rec) count(name string) int {
	n := 0
	for _, e := range r.all() {
		if e.name == name {
			n++
		}
	}
	return n
}

type fx struct {
	m   *Manager
	st  *store.Store
	em  *rec
	dir string
}

func newFx(t *testing.T, batch int) *fx {
	t.Helper()
	dir := t.TempDir()
	if r, err := filepath.EvalSymlinks(dir); err == nil {
		dir = r // macOS 的 /var 是链接：Remove 不信任含符号链接的输出目录
	}
	st, err := store.Open(context.Background(), filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	em := &rec{}
	m := NewManager(Config{Store: st, Emitter: em, LogDir: filepath.Join(dir, "logs"), BatchConcurrency: batch, ProgressInterval: -1,
		Logf: func(string, ...any) {}})
	t.Cleanup(func() { m.Shutdown(2 * time.Second) })
	return &fx{m: m, st: st, em: em, dir: dir}
}

func waitTask(t *testing.T, m *Manager, id string) Task {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tk, err := m.Wait(ctx, id)
	if err != nil {
		t.Fatalf("等待任务 %s: %v", id, err)
	}
	return tk
}

func eventually(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("等待条件超时")
}

var ok = RunnerFunc(func(ctx context.Context, report func(Progress)) (string, error) { return "/out/x.mp4", nil })

func TestStateMachineSuccess(t *testing.T) {
	f := newFx(t, 2)
	tk, err := f.m.Submit(Spec{Type: TypeConvert, Title: "T", InputPaths: []string{"/in.mov"}, Params: `{"a":1}`}, RunnerFunc(
		func(ctx context.Context, report func(Progress)) (string, error) {
			report(Progress{Fraction: 0.5, Speed: "2.0x", EtaSec: 3, OutTimeSec: 10})
			return "/out/x.mp4", nil
		}))
	if err != nil {
		t.Fatal(err)
	}
	if tk.Status != StatusQueued || tk.Version != 1 || tk.ID == "" || tk.Progress != 0 {
		t.Fatalf("%+v", tk)
	}
	done := waitTask(t, f.m, tk.ID)
	if done.Status != StatusSucceeded || done.OutputPath != "/out/x.mp4" || done.Progress != 1 || done.FinishedAt == 0 || done.StartedAt == 0 {
		t.Fatalf("%+v", done)
	}
	eventually(t, func() bool { return len(f.em.statuses(tk.ID)) == 2 })
	if got := f.em.statuses(tk.ID); got[0] != StatusRunning || got[1] != StatusSucceeded {
		t.Fatalf("%v", got)
	}
	if f.em.count(EventCreated) != 1 {
		t.Fatal("task:created 应发一次")
	}
	// 落库
	db, _ := f.st.GetTask(context.Background(), tk.ID)
	if db.Status != StatusSucceeded || db.OutputPath != "/out/x.mp4" || db.Params != `{"a":1}` || db.Version != done.Version || db.Progress != 1 {
		t.Fatalf("%+v", db)
	}
	// 版本单调递增：所有 task:* 事件版本都大于上一个
	var last int64
	for _, e := range f.em.all() {
		var v int64
		switch p := e.payload.(type) {
		case StatusEvent:
			v = p.Version
		case ProgressEvent:
			v = p.Version
		case Task:
			v = p.Version
		}
		if v <= last {
			t.Fatalf("版本应严格递增: %d after %d (%s)", v, last, e.name)
		}
		last = v
	}
}

func TestFailureRecordsError(t *testing.T) {
	f := newFx(t, 1)
	tk, _ := f.m.Submit(Spec{Type: TypeConvert}, RunnerFunc(func(context.Context, func(Progress)) (string, error) {
		return "", apperr.New(apperr.ProcessFailed, "boom").WithDetail("tail")
	}))
	d := waitTask(t, f.m, tk.ID)
	if d.Status != StatusFailed || d.Error == nil || d.Error.Code != apperr.ProcessFailed || d.Error.Detail != "tail" {
		t.Fatalf("%+v", d)
	}
	db, _ := f.st.GetTask(context.Background(), tk.ID)
	if db.Error == nil || db.Error.Detail != "tail" {
		t.Fatalf("错误应落库: %+v", db)
	}
	// 普通 error 归为 INTERNAL；panic 也不应搞垮管理器
	t2, _ := f.m.Submit(Spec{Type: TypeConvert}, RunnerFunc(func(context.Context, func(Progress)) (string, error) { return "", errors.New("plain") }))
	if d := waitTask(t, f.m, t2.ID); d.Error.Code != apperr.Internal {
		t.Fatalf("%+v", d.Error)
	}
	t3, _ := f.m.Submit(Spec{Type: TypeConvert}, RunnerFunc(func(context.Context, func(Progress)) (string, error) { panic("oops") }))
	if d := waitTask(t, f.m, t3.ID); d.Status != StatusFailed || !strings.Contains(d.Error.Message, "panic") {
		t.Fatalf("%+v", d)
	}
}

func TestSubmitValidation(t *testing.T) {
	f := newFx(t, 1)
	if _, err := f.m.Submit(Spec{Type: "bogus"}, ok); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("%v", err)
	}
	if _, err := f.m.Submit(Spec{Type: TypeConvert}, nil); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("%v", err)
	}
	tk, _ := f.m.Submit(Spec{Type: TypeConvert, ID: "FIXED"}, ok)
	if tk.ID != "FIXED" {
		t.Fatal("应使用指定 ID")
	}
	waitTask(t, f.m, "FIXED")
}

func TestBatchPoolConcurrencyLimit(t *testing.T) {
	f := newFx(t, 2)
	var cur, peak int32
	gate := make(chan struct{})
	mk := func() Runner {
		return RunnerFunc(func(ctx context.Context, _ func(Progress)) (string, error) {
			n := atomic.AddInt32(&cur, 1)
			for {
				p := atomic.LoadInt32(&peak)
				if n <= p || atomic.CompareAndSwapInt32(&peak, p, n) {
					break
				}
			}
			select {
			case <-gate:
			case <-ctx.Done():
			}
			atomic.AddInt32(&cur, -1)
			return "", nil
		})
	}
	var ids []string
	for i := 0; i < 6; i++ {
		tk, _ := f.m.Submit(Spec{Type: TypeConvert}, mk())
		ids = append(ids, tk.ID)
	}
	eventually(t, func() bool { return atomic.LoadInt32(&cur) == 2 })
	time.Sleep(50 * time.Millisecond)
	if atomic.LoadInt32(&cur) != 2 {
		t.Fatalf("同时运行应恰好 2 个: %d", cur)
	}
	act := f.m.ListActive()
	var running, queued int
	for _, a := range act {
		switch a.Status {
		case StatusRunning:
			running++
		case StatusQueued:
			queued++
		}
	}
	if running != 2 || queued != 4 {
		t.Fatalf("running=%d queued=%d", running, queued)
	}
	// FIFO：前两个先跑
	if a, b := f.m.mustGet(t, ids[0]), f.m.mustGet(t, ids[5]); a.Status != StatusRunning || b.Status != StatusQueued {
		t.Fatalf("%s %s", a.Status, b.Status)
	}
	close(gate)
	for _, id := range ids {
		waitTask(t, f.m, id)
	}
	if peak != 2 {
		t.Fatalf("峰值并发应为 2: %d", peak)
	}
}

func (m *Manager) mustGet(t *testing.T, id string) Task {
	t.Helper()
	tk, err := m.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	return tk
}

func TestLivePoolNotBlockedByBatch(t *testing.T) {
	f := newFx(t, 1)
	gate := make(chan struct{})
	blocker := RunnerFunc(func(ctx context.Context, _ func(Progress)) (string, error) {
		select {
		case <-gate:
		case <-ctx.Done():
		}
		return "", nil
	})
	b1, _ := f.m.Submit(Spec{Type: TypeConvert}, blocker)
	b2, _ := f.m.Submit(Spec{Type: TypeConvert}, blocker) // 排队
	var liveRan int32
	live := RunnerFunc(func(ctx context.Context, report func(Progress)) (string, error) {
		atomic.AddInt32(&liveRan, 1)
		report(Progress{Fraction: 0.3})
		<-ctx.Done()
		return "", nil
	})
	l1, _ := f.m.Submit(Spec{Type: TypeLiveRelay}, live)
	l2, _ := f.m.Submit(Spec{Type: TypeLiveFilePush}, live)
	l3, _ := f.m.Submit(Spec{Type: TypeLiveRecordPush}, live)
	eventually(t, func() bool { return atomic.LoadInt32(&liveRan) == 3 })
	if f.m.mustGet(t, b2.ID).Status != StatusQueued || f.m.mustGet(t, b1.ID).Status != StatusRunning {
		t.Fatal("batch 池应仍被占满，第二个在排队")
	}
	// 直播任务进度恒为 -1
	for _, id := range []string{l1.ID, l2.ID, l3.ID} {
		if p := f.m.mustGet(t, id).Progress; p != -1 {
			t.Fatalf("直播进度应为 -1: %v", p)
		}
	}
	if l1.Progress != -1 {
		t.Fatal("提交时就应是 -1")
	}
	close(gate)
	for _, id := range []string{l1.ID, l2.ID, l3.ID} {
		f.m.Cancel(id)
	}
	for _, id := range []string{b1.ID, b2.ID, l1.ID, l2.ID, l3.ID} {
		waitTask(t, f.m, id)
	}
}

func TestCancelRunningAndQueued(t *testing.T) {
	f := newFx(t, 1)
	started := make(chan struct{})
	run := RunnerFunc(func(ctx context.Context, _ func(Progress)) (string, error) {
		close(started)
		<-ctx.Done()
		return "", ctx.Err()
	})
	a, _ := f.m.Submit(Spec{Type: TypeConvert}, run)
	var ranB int32
	b, _ := f.m.Submit(Spec{Type: TypeConvert}, RunnerFunc(func(context.Context, func(Progress)) (string, error) {
		atomic.AddInt32(&ranB, 1)
		return "", nil
	}))
	<-started
	// 取消排队中的 b：不会运行，直接 canceled
	if err := f.m.Cancel(b.ID); err != nil {
		t.Fatal(err)
	}
	if got := f.m.mustGet(t, b.ID); got.Status != StatusCanceled {
		t.Fatalf("%+v", got)
	}
	// 取消运行中的 a
	if err := f.m.Cancel(a.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.m.Cancel(a.ID); err != nil { // 重复取消无害（此时可能还在收尾）
		if !apperr.Is(err, apperr.TaskConflict) {
			t.Fatal(err)
		}
	}
	if d := waitTask(t, f.m, a.ID); d.Status != StatusCanceled || d.Error != nil {
		t.Fatalf("%+v", d)
	}
	if atomic.LoadInt32(&ranB) != 0 {
		t.Fatal("被取消的排队任务不应执行")
	}
	// 已结束 → TASK_CONFLICT；不存在 → NOT_FOUND
	if err := f.m.Cancel(a.ID); !apperr.Is(err, apperr.TaskConflict) {
		t.Fatalf("%v", err)
	}
	if err := f.m.Cancel("nope"); !apperr.Is(err, apperr.NotFound) {
		t.Fatalf("%v", err)
	}
	// 落库状态
	for _, id := range []string{a.ID, b.ID} {
		if db, _ := f.st.GetTask(context.Background(), id); db.Status != StatusCanceled {
			t.Fatalf("%s: %+v", id, db)
		}
	}
	// 取消后队列腾出位置，后续任务能正常跑
	c, _ := f.m.Submit(Spec{Type: TypeConvert}, ok)
	if d := waitTask(t, f.m, c.ID); d.Status != StatusSucceeded {
		t.Fatalf("%+v", d)
	}
}

func TestGracefulStopCountsAsSucceeded(t *testing.T) {
	f := newFx(t, 1)
	r := RunnerFunc(func(ctx context.Context, _ func(Progress)) (string, error) {
		<-ctx.Done()
		time.Sleep(20 * time.Millisecond) // 模拟写文件尾
		return "/archive/a.mp4", nil      // 优雅停止：存档完整，返回 nil
	})
	tk, _ := f.m.Submit(Spec{Type: TypeLiveRecordPush}, r)
	eventually(t, func() bool { return f.m.mustGet(t, tk.ID).Status == StatusRunning })
	f.m.Cancel(tk.ID)
	d := waitTask(t, f.m, tk.ID)
	if d.Status != StatusSucceeded || d.OutputPath != "/archive/a.mp4" || d.Progress != -1 {
		t.Fatalf("Runner 优雅返回 nil 时应为 succeeded 且保留 -1 进度: %+v", d)
	}
}

func TestProgressThrottling(t *testing.T) {
	dir := t.TempDir()
	st, _ := store.Open(context.Background(), filepath.Join(dir, "app.db"))
	defer st.Close()
	em := &rec{}
	m := NewManager(Config{Store: st, Emitter: em, BatchConcurrency: 1, ProgressInterval: 100 * time.Millisecond, Logf: func(string, ...any) {}})
	defer m.Shutdown(time.Second)
	tk, _ := m.Submit(Spec{Type: TypeConvert}, RunnerFunc(func(ctx context.Context, report func(Progress)) (string, error) {
		end := time.Now().Add(450 * time.Millisecond)
		i := 0
		for time.Now().Before(end) {
			i++
			report(Progress{Fraction: float64(i) / 1e6, Speed: "1x"})
			time.Sleep(time.Millisecond)
		}
		report(Progress{Fraction: 0.9, Speed: "9x"}) // 最后一次必须最终送达
		time.Sleep(150 * time.Millisecond)
		return "", nil
	}))
	waitTask2 := func() {
		ctx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		m.Wait(ctx, tk.ID)
	}
	waitTask2()
	n := em.count(EventProgress)
	if n < 3 || n > 7 {
		t.Fatalf("450ms 内间隔 100ms 应约 4~5 次，实际 %d", n)
	}
	// 相邻两次推送间隔不小于阈值（允许少量调度误差由计数保证），且最后一次是 0.9
	var last ProgressEvent
	for _, e := range em.all() {
		if p, ok := e.payload.(ProgressEvent); ok {
			last = p
		}
	}
	if last.Progress != 0.9 || last.Speed != "9x" {
		t.Fatalf("被节流抑制的最后一次进度应补发: %+v", last)
	}
	// 进度不落库（契约 6.5）：库里 running 期间 progress 仍是 0，成功后才写 1
	db, _ := st.GetTask(context.Background(), tk.ID)
	if db.Progress != 1 {
		t.Fatalf("终态才落库进度: %v", db.Progress)
	}
}

func TestProgressNotPersistedWhileRunning(t *testing.T) {
	f := newFx(t, 1)
	gate := make(chan struct{})
	tk, _ := f.m.Submit(Spec{Type: TypeConvert}, RunnerFunc(func(ctx context.Context, report func(Progress)) (string, error) {
		report(Progress{Fraction: 0.6, Speed: "3x", EtaSec: 9})
		<-gate
		return "", nil
	}))
	eventually(t, func() bool { return f.m.mustGet(t, tk.ID).Progress == 0.6 })
	live := f.m.mustGet(t, tk.ID)
	if live.Speed != "3x" || live.EtaSec != 9 {
		t.Fatalf("内存里应有实时进度: %+v", live)
	}
	db, _ := f.st.GetTask(context.Background(), tk.ID)
	if db.Progress != 0 || db.Speed != "" {
		t.Fatalf("进度不应落库: %+v", db)
	}
	// List 会叠加实时进度
	p, _ := f.m.List(Filter{})
	if p.Items[0].Progress != 0.6 {
		t.Fatalf("%+v", p.Items[0])
	}
	close(gate)
	waitTask(t, f.m, tk.ID)
}

func TestInterruptedOnStartupAndShutdown(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "app.db")
	st, _ := store.Open(context.Background(), dbPath)
	m := NewManager(Config{Store: st, BatchConcurrency: 1, Logf: func(string, ...any) {}})
	release := make(chan struct{})
	r1, _ := m.Submit(Spec{Type: TypeConvert}, RunnerFunc(func(ctx context.Context, _ func(Progress)) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}))
	r2, _ := m.Submit(Spec{Type: TypeConvert}, ok) // 排队
	eventually(t, func() bool { return m.mustGet(t, r1.ID).Status == StatusRunning })
	_ = release
	// 应用退出：运行中和排队中的都变成 interrupted，而不是 canceled
	m.Shutdown(2 * time.Second)
	for _, id := range []string{r1.ID, r2.ID} {
		db, _ := st.GetTask(context.Background(), id)
		if db.Status != StatusInterrupted {
			t.Fatalf("%s 应为 interrupted: %s", id, db.Status)
		}
	}
	if _, err := m.Submit(Spec{Type: TypeConvert}, ok); err == nil {
		t.Fatal("退出后不应再接受任务")
	}
	// 模拟崩溃：库里残留 queued / running，重启时 MarkInterrupted 处理
	st.InsertTask(context.Background(), store.Task{ID: "CRASH1", Type: TypeConvert, Status: StatusRunning, Version: 4, CreatedAt: 1})
	st.InsertTask(context.Background(), store.Task{ID: "CRASH2", Type: TypeConvert, Status: StatusQueued, Version: 1, CreatedAt: 2})
	st.Close()
	st2, _ := store.Open(context.Background(), dbPath)
	defer st2.Close()
	n, err := st2.MarkInterrupted(context.Background(), time.Now())
	if err != nil || n != 2 {
		t.Fatalf("%d %v", n, err)
	}
	m2 := NewManager(Config{Store: st2, Logf: func(string, ...any) {}})
	got, _ := m2.Get("CRASH1")
	if got.Status != StatusInterrupted || got.Version != 5 {
		t.Fatalf("%+v", got)
	}
	if len(m2.ListActive()) != 0 {
		t.Fatal("重启后不应有活动任务")
	}
}

func TestRetry(t *testing.T) {
	f := newFx(t, 1)
	var calls int32
	f.m.RegisterFactory(TypeConvert, func(old Task) (Runner, error) {
		return RunnerFunc(func(context.Context, func(Progress)) (string, error) {
			atomic.AddInt32(&calls, 1)
			if !strings.Contains(old.Params, `"x":1`) {
				return "", errors.New("params lost")
			}
			return "/out/retry.mp4", nil
		}), nil
	})
	first, _ := f.m.Submit(Spec{Type: TypeConvert, Title: "c", InputPaths: []string{"/a"}, Params: `{"x":1}`},
		RunnerFunc(func(context.Context, func(Progress)) (string, error) { return "", errors.New("fail once") }))
	waitTask(t, f.m, first.ID)
	nt, err := f.m.Retry(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if nt.ID == first.ID || nt.Params != `{"x":1}` || nt.Title != "c" || nt.InputPaths[0] != "/a" {
		t.Fatalf("%+v", nt)
	}
	if d := waitTask(t, f.m, nt.ID); d.Status != StatusSucceeded || d.OutputPath != "/out/retry.mp4" {
		t.Fatalf("%+v", d)
	}
	// 原任务保留
	if o := f.m.mustGet(t, first.ID); o.Status != StatusFailed {
		t.Fatalf("%+v", o)
	}
	// 进行中不能重试；未注册类型不能重试；不存在
	gate := make(chan struct{})
	act, _ := f.m.Submit(Spec{Type: TypeEditExport}, RunnerFunc(func(ctx context.Context, _ func(Progress)) (string, error) { <-gate; return "", nil }))
	eventually(t, func() bool { return f.m.mustGet(t, act.ID).Status == StatusRunning })
	if _, err := f.m.Retry(act.ID); !apperr.Is(err, apperr.TaskConflict) {
		t.Fatalf("%v", err)
	}
	close(gate)
	waitTask(t, f.m, act.ID)
	if _, err := f.m.Retry(act.ID); !apperr.Is(err, apperr.Unsupported) {
		t.Fatalf("未注册工厂应返回 UNSUPPORTED: %v", err)
	}
	if _, err := f.m.Retry("nope"); !apperr.Is(err, apperr.NotFound) {
		t.Fatalf("%v", err)
	}
}

func TestRemoveAndClearFinished(t *testing.T) {
	f := newFx(t, 1)
	out := filepath.Join(f.dir, "out.mp4")
	os.WriteFile(out, []byte("x"), 0o644)
	okT, _ := f.m.Submit(Spec{Type: TypeConvert}, RunnerFunc(func(ctx context.Context, _ func(Progress)) (string, error) {
		io := LogWriter(ctx)
		io.Write([]byte("hello log\nline2\nline3\n"))
		os.WriteFile(out, []byte("x"), 0o644) // 输出文件在任务运行期间生成
		return out, nil
	}))
	waitTask(t, f.m, okT.ID)
	logPath := f.m.mustGet(t, okT.ID).LogPath
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("应生成日志文件: %v", err)
	}
	// 进行中不能删
	gate := make(chan struct{})
	act, _ := f.m.Submit(Spec{Type: TypeConvert}, RunnerFunc(func(ctx context.Context, _ func(Progress)) (string, error) { <-gate; return "", nil }))
	eventually(t, func() bool { return f.m.mustGet(t, act.ID).Status == StatusRunning })
	if err := f.m.Remove([]string{okT.ID, act.ID}, false); !apperr.Is(err, apperr.TaskConflict) {
		t.Fatalf("%v", err)
	}
	if _, err := f.m.Get(okT.ID); err != nil {
		t.Fatal("冲突时不应删除任何任务")
	}
	// 删除成功任务，同时删除输出文件与日志
	if err := f.m.Remove([]string{okT.ID, "ghost"}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("deleteOutput=true 应删除输出文件")
	}
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Fatal("日志应被清理")
	}
	if _, err := f.m.Get(okT.ID); !apperr.Is(err, apperr.NotFound) {
		t.Fatalf("%v", err)
	}
	var removed RemovedEvent
	for _, e := range f.em.all() {
		if r, ok := e.payload.(RemovedEvent); ok {
			removed = r
		}
	}
	if len(removed.IDs) != 1 || removed.IDs[0] != okT.ID {
		t.Fatalf("%+v", removed)
	}
	// ClearFinished：不动进行中的，不删输出文件
	keep := filepath.Join(f.dir, "keep.mp4")
	os.WriteFile(keep, []byte("y"), 0o644)
	fin, _ := f.m.Submit(Spec{Type: TypeConvert}, RunnerFunc(func(context.Context, func(Progress)) (string, error) { return keep, nil }))
	eventually(t, func() bool {
		return f.m.mustGet(t, fin.ID).Status == StatusQueued || f.m.mustGet(t, fin.ID).Status == StatusRunning
	})
	close(gate)
	waitTask(t, f.m, act.ID)
	waitTask(t, f.m, fin.ID)
	before := f.em.count(EventRemoved)
	if err := f.m.ClearFinished(); err != nil {
		t.Fatal(err)
	}
	if p, _ := f.m.List(Filter{}); p.Total != 0 {
		t.Fatalf("%+v", p)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatal("ClearFinished 不应删除输出文件")
	}
	if f.em.count(EventRemoved) != before+1 {
		t.Fatal("应发 task:removed")
	}
}

func TestGetLogTail(t *testing.T) {
	f := newFx(t, 1)
	tk, _ := f.m.Submit(Spec{Type: TypeConvert}, RunnerFunc(func(ctx context.Context, _ func(Progress)) (string, error) {
		for i := 1; i <= 10; i++ {
			fmt.Fprintf(LogWriter(ctx), "line %d\n", i)
		}
		return "", nil
	}))
	waitTask(t, f.m, tk.ID)
	got, err := f.m.GetLog(tk.ID, 3)
	if err != nil || got != "line 8\nline 9\nline 10" {
		t.Fatalf("%q %v", got, err)
	}
	all, _ := f.m.GetLog(tk.ID, 0)
	if !strings.HasPrefix(all, "line 1\n") {
		t.Fatalf("%q", all)
	}
	// 没有日志的任务返回空串
	t2, _ := f.m.Submit(Spec{Type: TypeConvert}, ok)
	waitTask(t, f.m, t2.ID)
	if s, err := f.m.GetLog(t2.ID, 5); err != nil || s != "" {
		t.Fatalf("%q %v", s, err)
	}
	if _, err := f.m.GetLog("nope", 5); !apperr.Is(err, apperr.NotFound) {
		t.Fatal(err)
	}
}

type finRunner struct {
	RunnerFunc
	got chan Task
}

func (f *finRunner) OnFinish(t Task) { f.got <- t }

func TestFinalizerCalledAfterTerminalState(t *testing.T) {
	f := newFx(t, 1)
	fr := &finRunner{got: make(chan Task, 2), RunnerFunc: func(context.Context, func(Progress)) (string, error) { return "o", nil }}
	tk, _ := f.m.Submit(Spec{Type: TypeConvert}, fr)
	select {
	case got := <-fr.got:
		if got.ID != tk.ID || got.Status != StatusSucceeded {
			t.Fatalf("%+v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnFinish 未被调用")
	}
	// 排队中被取消也调用
	gate := make(chan struct{})
	f.m.Submit(Spec{Type: TypeConvert}, RunnerFunc(func(ctx context.Context, _ func(Progress)) (string, error) { <-gate; return "", nil }))
	fr2 := &finRunner{got: make(chan Task, 2), RunnerFunc: ok}
	q, _ := f.m.Submit(Spec{Type: TypeConvert}, fr2)
	f.m.Cancel(q.ID)
	select {
	case got := <-fr2.got:
		if got.Status != StatusCanceled {
			t.Fatalf("%+v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("排队取消也应调用 OnFinish")
	}
	close(gate)
}

func TestSetBatchConcurrency(t *testing.T) {
	f := newFx(t, 1)
	var cur, peak int32
	gate := make(chan struct{})
	r := func() Runner {
		return RunnerFunc(func(ctx context.Context, _ func(Progress)) (string, error) {
			n := atomic.AddInt32(&cur, 1)
			for {
				p := atomic.LoadInt32(&peak)
				if n <= p || atomic.CompareAndSwapInt32(&peak, p, n) {
					break
				}
			}
			<-gate
			atomic.AddInt32(&cur, -1)
			return "", nil
		})
	}
	var ids []string
	for i := 0; i < 4; i++ {
		tk, _ := f.m.Submit(Spec{Type: TypeConvert}, r())
		ids = append(ids, tk.ID)
	}
	eventually(t, func() bool { return atomic.LoadInt32(&cur) == 1 })
	f.m.SetBatchConcurrency(3)
	eventually(t, func() bool { return atomic.LoadInt32(&cur) == 3 })
	if f.m.BatchConcurrency() != 3 {
		t.Fatal()
	}
	close(gate)
	for _, id := range ids {
		waitTask(t, f.m, id)
	}
	if DefaultBatchConcurrency() < 1 || DefaultBatchConcurrency() > 3 {
		t.Fatal("默认并发应在 1~3")
	}
}

func TestRunWithPart(t *testing.T) {
	dir := t.TempDir()
	want := filepath.Join(dir, "a.mp4")
	if PartPath(want) != filepath.Join(dir, "a.part.mp4") {
		t.Fatal(PartPath(want))
	}
	var gotPart string
	out, err := RunWithPart(context.Background(), want, func(part string) error {
		gotPart = part
		if _, err := os.Stat(want); err == nil {
			t.Error("最终文件在完成前不应存在")
		}
		return os.WriteFile(part, []byte("data"), 0o644)
	})
	if err != nil || out != want || gotPart != filepath.Join(dir, "a.part.mp4") {
		t.Fatalf("%q %q %v", out, gotPart, err)
	}
	if _, err := os.Stat(gotPart); !os.IsNotExist(err) {
		t.Fatal(".part 应已改名")
	}
	// 重名追加 (1)、(2)
	out2, _ := RunWithPart(context.Background(), want, func(p string) error { return os.WriteFile(p, []byte("2"), 0o644) })
	out3, _ := RunWithPart(context.Background(), want, func(p string) error { return os.WriteFile(p, []byte("3"), 0o644) })
	if out2 != filepath.Join(dir, "a(1).mp4") || out3 != filepath.Join(dir, "a(2).mp4") {
		t.Fatalf("%s %s", out2, out3)
	}
	// 失败删除 .part，且不产生最终文件
	failTarget := filepath.Join(dir, "sub", "b.mkv")
	_, err = RunWithPart(context.Background(), failTarget, func(p string) error {
		os.WriteFile(p, []byte("half"), 0o644)
		return errors.New("fail")
	})
	if err == nil {
		t.Fatal()
	}
	if ents, _ := os.ReadDir(filepath.Join(dir, "sub")); len(ents) != 0 {
		t.Fatalf("失败后不应留下任何文件: %v", ents)
	}
	// 无扩展名
	if PartPath("/x/noext") != "/x/noext.part" {
		t.Fatal(PartPath("/x/noext"))
	}
}

func TestSetConcurrencyAutoAndShrinkKeepsRunning(t *testing.T) {
	f := newFx(t, 3)
	var cur int32
	gate := make(chan struct{})
	mk := func() Runner {
		return RunnerFunc(func(ctx context.Context, _ func(Progress)) (string, error) {
			atomic.AddInt32(&cur, 1)
			<-gate
			atomic.AddInt32(&cur, -1)
			return "", nil
		})
	}
	var ids []string
	for i := 0; i < 6; i++ {
		tk, _ := f.m.Submit(Spec{Type: TypeConvert}, mk())
		ids = append(ids, tk.ID)
	}
	eventually(t, func() bool { return atomic.LoadInt32(&cur) == 3 })
	// 调小：已在运行的 3 个不被打断，新任务暂不启动
	f.m.SetConcurrency(1)
	time.Sleep(50 * time.Millisecond)
	if atomic.LoadInt32(&cur) != 3 || f.m.BatchConcurrency() != 1 {
		t.Fatalf("调小不应打断运行中的任务: cur=%d limit=%d", cur, f.m.BatchConcurrency())
	}
	// 0 = 自动
	f.m.SetConcurrency(0)
	if f.m.BatchConcurrency() != DefaultBatchConcurrency() {
		t.Fatalf("0 应为自动值: %d", f.m.BatchConcurrency())
	}
	f.m.SetConcurrency(1)
	close(gate)
	for _, id := range ids {
		if d := waitTask(t, f.m, id); d.Status != StatusSucceeded {
			t.Fatalf("%+v", d)
		}
	}
}

func TestSetConcurrencyRaceWithSubmit(t *testing.T) {
	f := newFx(t, 1)
	stop := make(chan struct{})
	go func() {
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
				f.m.SetConcurrency(i % 5)
			}
		}
	}()
	var ids []string
	for i := 0; i < 30; i++ {
		tk, err := f.m.Submit(Spec{Type: TypeConvert}, ok)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, tk.ID)
	}
	for _, id := range ids {
		waitTask(t, f.m, id)
	}
	close(stop)
}

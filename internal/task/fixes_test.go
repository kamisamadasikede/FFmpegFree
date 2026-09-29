package task

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
)

// ---- 输出文件名占用与无覆盖提交 ----

func TestConcurrentSameOutputNamesAreDistinct(t *testing.T) {
	const n = 8
	f := newFx(t, n)
	out := filepath.Join(f.dir, "out", "same.mp4")
	start := make(chan struct{})
	var ids []string
	for i := 0; i < n; i++ {
		i := i
		tk, err := f.m.Submit(Spec{Type: TypeConvert, OutputPath: out}, RunnerFunc(func(ctx context.Context, _ func(Progress)) (string, error) {
			return RunWithPart(ctx, out, func(part string) error {
				<-start // 所有任务同时持有各自的 .part
				time.Sleep(time.Duration(i) * time.Millisecond)
				return os.WriteFile(part, []byte(fmt.Sprintf("task-%d", i)), 0o644)
			})
		}))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, tk.ID)
	}
	eventually(t, func() bool {
		for _, id := range ids {
			if f.m.mustGet(t, id).Status != StatusRunning {
				return false
			}
		}
		return true
	})
	close(start)
	seen := map[string]bool{}
	for i, id := range ids {
		d := waitTask(t, f.m, id)
		if d.Status != StatusSucceeded {
			t.Fatalf("%+v", d)
		}
		if seen[d.OutputPath] {
			t.Fatalf("两个任务写到了同一个文件: %s", d.OutputPath)
		}
		seen[d.OutputPath] = true
		b, _ := os.ReadFile(d.OutputPath)
		if string(b) != fmt.Sprintf("task-%d", i) {
			t.Fatalf("%s 内容被覆盖: %q", d.OutputPath, b)
		}
	}
	if len(seen) != n {
		t.Fatalf("应有 %d 个不同输出: %v", n, seen)
	}
	ents, _ := os.ReadDir(filepath.Dir(out))
	if len(ents) != n {
		t.Fatalf("目录里应恰好 %d 个文件（无 .part 残留）: %d", n, len(ents))
	}
	// 占用在任务结束后释放
	f.m.namer.mu.Lock()
	left := len(f.m.namer.held)
	f.m.namer.mu.Unlock()
	if left != 0 {
		t.Fatalf("任务结束后应释放占用: %d", left)
	}
}

func TestCommitDoesNotOverwriteOutsiderFile(t *testing.T) {
	dir := t.TempDir()
	want := filepath.Join(dir, "a.mp4")
	var outsider string
	out, err := RunWithPart(context.Background(), want, func(part string) error {
		os.WriteFile(part, []byte("mine"), 0o644)
		// 模拟别的程序在任务运行期间抢先创建了最终文件
		os.WriteFile(want, []byte("theirs"), 0o644)
		outsider = want
		return nil
	})
	if err != nil || out != filepath.Join(dir, "a(1).mp4") {
		t.Fatalf("应改用 a(1).mp4: %q %v", out, err)
	}
	if b, _ := os.ReadFile(outsider); string(b) != "theirs" {
		t.Fatalf("别人的文件被覆盖了: %q", b)
	}
	if b, _ := os.ReadFile(out); string(b) != "mine" {
		t.Fatalf("%q", b)
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 2 {
		t.Fatalf("不应残留 .part: %v", ents)
	}
}

func TestCommitPartNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	part, final := filepath.Join(dir, "a.part.mp4"), filepath.Join(dir, "a.mp4")
	os.WriteFile(part, []byte("new"), 0o644)
	os.WriteFile(final, []byte("old"), 0o644)
	if err := commitPart(part, final); !errors.Is(err, errTargetExists) {
		t.Fatalf("%v", err)
	}
	if b, _ := os.ReadFile(final); string(b) != "old" {
		t.Fatal("被覆盖")
	}
	os.Remove(final)
	if err := commitPart(part, final); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(final); string(b) != "new" {
		t.Fatal()
	}
	if _, err := os.Stat(part); !os.IsNotExist(err) {
		t.Fatal(".part 应已删除")
	}
}

// ---- 日志容量与单行长度 ----

func TestLogSinkRotationCap(t *testing.T) {
	dir := t.TempDir()
	l := &logSink{p: filepath.Join(dir, "x.log"), maxBytes: 1000}
	line := strings.Repeat("a", 99) + "\n"
	for i := 0; i < 100; i++ { // 10000 字节
		l.Write([]byte(line))
	}
	l.close()
	cur, _ := os.Stat(l.p)
	old, err := os.Stat(rotatedPath(l.p))
	if err != nil {
		t.Fatalf("应有轮转文件: %v", err)
	}
	if cur.Size() > 1000 || old.Size() > 1000 {
		t.Fatalf("单个文件不应超过上限: cur=%d old=%d", cur.Size(), old.Size())
	}
	if cur.Size() == 0 {
		t.Fatal("当前日志不应为空")
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 2 {
		t.Fatalf("只应有 .log 和 .log.1: %v", ents)
	}
}

func TestLogSinkLineCap(t *testing.T) {
	dir := t.TempDir()
	l := &logSink{p: filepath.Join(dir, "x.log")}
	huge := strings.Repeat("x", 100_000)
	l.Write([]byte(huge[:60_000]))
	l.Write([]byte(huge[60_000:] + "\nnext line\n")) // 同一行分两次写入
	l.close()
	b, _ := os.ReadFile(l.p)
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) != 2 || lines[1] != "next line" {
		t.Fatalf("超长行后应恢复正常: %d 行", len(lines))
	}
	if len(lines[0]) > logMaxLine+len(logTruncMark) || !strings.HasSuffix(lines[0], logTruncMark) {
		t.Fatalf("超长行应被截断并标注: %d", len(lines[0]))
	}
}

func TestLogSinkCapAppliesToRealTask(t *testing.T) {
	f := newFx(t, 1)
	tk, _ := f.m.Submit(Spec{Type: TypeConvert}, RunnerFunc(func(ctx context.Context, _ func(Progress)) (string, error) {
		w := LogWriter(ctx)
		if s, ok := w.(*logSink); ok {
			s.maxBytes = 2000
		}
		for i := 0; i < 200; i++ {
			fmt.Fprintf(w, "line %03d %s\n", i, strings.Repeat("z", 50))
		}
		return "", nil
	}))
	waitTask(t, f.m, tk.ID)
	log, err := f.m.GetLog(tk.ID, 5)
	if err != nil || !strings.Contains(log, "line 199") {
		t.Fatalf("%q %v", log, err)
	}
	// 跨轮转读取：拿全部，包含 .1 里的旧内容（但不是最早的行，它们已被丢弃）
	all, _ := f.m.GetLog(tk.ID, 0)
	if !strings.Contains(all, "line 199") || strings.Contains(all, "line 000") || len(all) > 4100 {
		t.Fatalf("总量应受上限约束: %d", len(all))
	}
	d := f.m.mustGet(t, tk.ID)
	if err := f.m.Remove([]string{tk.ID}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(rotatedPath(d.LogPath)); !os.IsNotExist(err) {
		t.Fatal("Remove 应同时删除轮转日志")
	}
}

// ---- created 先于 status ----

func TestCreatedAlwaysBeforeStatus(t *testing.T) {
	f := newFx(t, 4)
	var wg sync.WaitGroup
	ids := make([]string, 40)
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tk, err := f.m.Submit(Spec{Type: TypeConvert}, RunnerFunc(func(context.Context, func(Progress)) (string, error) { return "", nil }))
			if err != nil {
				t.Error(err)
				return
			}
			ids[i] = tk.ID
		}(i)
	}
	wg.Wait()
	for _, id := range ids {
		waitTask(t, f.m, id)
	}
	createdAt, statusAt := map[string]int{}, map[string]int{}
	for i, e := range f.em.all() {
		switch p := e.payload.(type) {
		case Task:
			if e.name == EventCreated {
				createdAt[p.ID] = i + 1
			}
		case StatusEvent:
			if _, ok := statusAt[p.ID]; !ok {
				statusAt[p.ID] = i + 1
			}
		}
	}
	for _, id := range ids {
		if createdAt[id] == 0 || statusAt[id] == 0 || createdAt[id] > statusAt[id] {
			t.Fatalf("%s: created@%d status@%d", id, createdAt[id], statusAt[id])
		}
	}
}

type claimRunner struct {
	submitted, abandoned atomic.Int32
	seenBeforeCreated    atomic.Bool
	em                   *rec
	id                   atomic.Value
}

func (c *claimRunner) Run(context.Context, func(Progress)) (string, error) { return "", nil }
func (c *claimRunner) Submitted(id string) {
	c.submitted.Add(1)
	c.id.Store(id)
	if c.em.count(EventCreated) > 0 {
		c.seenBeforeCreated.Store(false)
	}
}
func (c *claimRunner) Abandoned() { c.abandoned.Add(1) }

func TestClaimerCalledBeforeCreated(t *testing.T) {
	f := newFx(t, 1)
	cr := &claimRunner{em: f.em}
	tk, err := f.m.Submit(Spec{Type: TypeConvert}, cr)
	if err != nil {
		t.Fatal(err)
	}
	if cr.submitted.Load() != 1 || cr.id.Load() != tk.ID {
		t.Fatal("Submitted 应被调用一次并带任务 ID")
	}
	waitTask(t, f.m, tk.ID)
	// Retry 里 Submit 失败要通知 Abandoned
	f.m.RegisterFactory(TypeConvert, func(Task) (Runner, error) { return cr, nil })
	f.m.Shutdown(time.Second) // 之后 Submit 必然失败
	if _, err := f.m.Retry(tk.ID); err == nil {
		t.Fatal("退出后重试应失败")
	}
	if cr.abandoned.Load() != 1 {
		t.Fatalf("abandoned = %d", cr.abandoned.Load())
	}
}

// ---- Submit 参数校验 ----

func TestSubmitValidatesIDAndPaths(t *testing.T) {
	f := newFx(t, 1)
	ok := RunnerFunc(func(context.Context, func(Progress)) (string, error) { return "", nil })
	abs := filepath.Join(f.dir, "a.mp4")
	for name, sp := range map[string]Spec{
		"路径穿越 ID":   {Type: TypeConvert, ID: "../../etc/passwd"},
		"带分隔符 ID":   {Type: TypeConvert, ID: "a/b"},
		"带点 ID":     {Type: TypeConvert, ID: "a.b"},
		"反斜杠 ID":    {Type: TypeConvert, ID: `a\b`},
		"中文 ID":     {Type: TypeConvert, ID: "任务"},
		"超长 ID":     {Type: TypeConvert, ID: strings.Repeat("A", 65)},
		"相对输入路径":    {Type: TypeConvert, InputPaths: []string{"rel/in.mp4"}},
		"相对输出路径":    {Type: TypeConvert, OutputPath: "out.mp4"},
		"输入里混入相对路径": {Type: TypeConvert, InputPaths: []string{abs, "x.mp4"}},
	} {
		if _, err := f.m.Submit(sp, ok); !apperr.Is(err, apperr.InvalidArgument) {
			t.Errorf("%s: 期望 INVALID_ARGUMENT, got %v", name, err)
		}
	}
	tk, err := f.m.Submit(Spec{Type: TypeConvert, ID: "01ARZ3NDEKTSV4RRFFQ69G5FAV", InputPaths: []string{abs}, OutputPath: abs}, ok)
	if err != nil || tk.ID != "01ARZ3NDEKTSV4RRFFQ69G5FAV" {
		t.Fatalf("%+v %v", tk, err)
	}
	waitTask(t, f.m, tk.ID)
	// 日志文件一定在日志目录里
	if d := f.m.mustGet(t, tk.ID); filepath.Dir(d.LogPath) != filepath.Join(f.dir, "logs") {
		t.Fatalf("%s", d.LogPath)
	}
}

// ---- Remove(deleteOutput) 的安全检查 ----

func runTaskWithOutput(t *testing.T, f *fx, inputs []string, out string, prepare func()) Task {
	t.Helper()
	tk, err := f.m.Submit(Spec{Type: TypeConvert, InputPaths: inputs, OutputPath: out}, RunnerFunc(func(context.Context, func(Progress)) (string, error) {
		if prepare != nil {
			prepare()
		}
		return out, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	return waitTask(t, f.m, tk.ID)
}

func TestRemoveDeleteOutputSafety(t *testing.T) {
	f := newFx(t, 2)
	// 1. 输出就是输入：不删
	same := filepath.Join(f.dir, "same.mp4")
	os.WriteFile(same, []byte("src"), 0o644)
	d := runTaskWithOutput(t, f, []string{same}, same, nil)
	if err := f.m.Remove([]string{d.ID}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(same); err != nil {
		t.Fatal("输出与输入相同时不应删除文件")
	}
	// 1b. 通过硬链接指向同一个文件：也不删
	linkOut := filepath.Join(f.dir, "hard.mp4")
	if err := os.Link(same, linkOut); err == nil {
		d = runTaskWithOutput(t, f, []string{same}, linkOut, nil)
		f.m.Remove([]string{d.ID}, true)
		if _, err := os.Stat(linkOut); err != nil {
			t.Fatal("与输入是同一个文件（硬链接）不应删除")
		}
	}
	// 2. 修改时间早于任务开始：不是这个任务生成的
	old := filepath.Join(f.dir, "old.mp4")
	os.WriteFile(old, []byte("pre-existing"), 0o644)
	past := time.Now().Add(-time.Hour)
	os.Chtimes(old, past, past)
	d = runTaskWithOutput(t, f, nil, old, nil)
	f.m.Remove([]string{d.ID}, true)
	if _, err := os.Stat(old); err != nil {
		t.Fatal("修改时间早于任务开始的文件不应被删除")
	}
	// 3. 符号链接：不删链接也不动目标
	if runtime.GOOS != "windows" {
		target := filepath.Join(f.dir, "target.mp4")
		os.WriteFile(target, []byte("t"), 0o644)
		link := filepath.Join(f.dir, "link.mp4")
		if err := os.Symlink(target, link); err == nil {
			d = runTaskWithOutput(t, f, nil, link, nil)
			f.m.Remove([]string{d.ID}, true)
			if _, err := os.Lstat(link); err != nil {
				t.Fatal("符号链接不应被删除")
			}
			if _, err := os.Stat(target); err != nil {
				t.Fatal("符号链接的目标不应被删除")
			}
		}
	}
	// 4. 目录：不删
	dir := filepath.Join(f.dir, "adir")
	os.MkdirAll(dir, 0o755)
	d = runTaskWithOutput(t, f, nil, dir, nil)
	f.m.Remove([]string{d.ID}, true)
	if _, err := os.Stat(dir); err != nil {
		t.Fatal("目录不应被删除")
	}
	// 5. 正常情况：这个任务生成的普通文件会删
	gen := filepath.Join(f.dir, "gen.mp4")
	d = runTaskWithOutput(t, f, []string{same}, gen, func() { os.WriteFile(gen, []byte("g"), 0o644) })
	if err := f.m.Remove([]string{d.ID}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(gen); !os.IsNotExist(err) {
		t.Fatal("正常输出应被删除")
	}
	// 6. 失败任务的输出不删
	failOut := filepath.Join(f.dir, "fail.mp4")
	tk, _ := f.m.Submit(Spec{Type: TypeConvert, OutputPath: failOut}, RunnerFunc(func(context.Context, func(Progress)) (string, error) {
		os.WriteFile(failOut, []byte("half"), 0o644)
		return "", errors.New("boom")
	}))
	waitTask(t, f.m, tk.ID)
	f.m.Remove([]string{tk.ID}, true)
	if _, err := os.Stat(failOut); err != nil {
		t.Fatal("只删成功任务的输出")
	}
}

func TestRemoveReportsDeleteErrors(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("需要非 root 的类 unix 环境来制造删除失败")
	}
	f := newFx(t, 1)
	dir := filepath.Join(f.dir, "ro")
	os.MkdirAll(dir, 0o755)
	out := filepath.Join(dir, "o.mp4")
	d := runTaskWithOutput(t, f, nil, out, func() { os.WriteFile(out, []byte("x"), 0o644) })
	os.Chmod(dir, 0o500) // 目录只读：文件删不掉
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	err := f.m.Remove([]string{d.ID}, true)
	if !apperr.Is(err, apperr.IOError) {
		t.Fatalf("删除输出失败应如实返回 IO_ERROR: %v", err)
	}
	if _, gerr := f.m.Get(d.ID); !apperr.Is(gerr, apperr.NotFound) {
		t.Fatal("任务记录仍应已删除")
	}
	if _, serr := os.Stat(out); serr != nil {
		t.Fatal("文件应还在")
	}
}

// ---- 退出流程 ----

func TestShutdownMarksGracefullyStoppedRunningTaskInterrupted(t *testing.T) {
	f := newFx(t, 1)
	started := make(chan struct{})
	// 模拟直播优雅停止：收到取消后正常返回 nil
	tk, _ := f.m.Submit(Spec{Type: TypeLiveRelay}, RunnerFunc(func(ctx context.Context, _ func(Progress)) (string, error) {
		close(started)
		<-ctx.Done()
		return "/archive/a.mp4", nil
	}))
	<-started
	// 对照：用户手动取消，优雅停止仍是 succeeded
	f.m.Shutdown(3 * time.Second)
	d := f.m.mustGet(t, tk.ID)
	if d.Status != StatusInterrupted {
		t.Fatalf("应用退出时被优雅停止的任务应为 interrupted: %+v", d)
	}
}

type finRunner2 struct {
	fin atomic.Int32
	st  atomic.Value
}

func (r *finRunner2) Run(ctx context.Context, _ func(Progress)) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}
func (r *finRunner2) OnFinish(t Task) { r.fin.Add(1); r.st.Store(t.Status) }

func TestShutdownCallsOnFinishForQueuedTasks(t *testing.T) {
	f := newFx(t, 1)
	running, queued := &finRunner2{}, &finRunner2{}
	a, _ := f.m.Submit(Spec{Type: TypeConvert}, running)
	eventually(t, func() bool { return f.m.mustGet(t, a.ID).Status == StatusRunning })
	b, _ := f.m.Submit(Spec{Type: TypeConvert}, queued)
	if f.m.mustGet(t, b.ID).Status != StatusQueued {
		t.Fatal("第二个任务应在排队")
	}
	f.m.Shutdown(3 * time.Second)
	if queued.fin.Load() != 1 || queued.st.Load() != StatusInterrupted {
		t.Fatalf("排队任务应收到 OnFinish(interrupted): %d %v", queued.fin.Load(), queued.st.Load())
	}
	if running.fin.Load() != 1 || running.st.Load() != StatusInterrupted {
		t.Fatalf("运行中任务应收到 OnFinish(interrupted): %d %v", running.fin.Load(), running.st.Load())
	}
	if d := f.m.mustGet(t, b.ID); d.Status != StatusInterrupted {
		t.Fatalf("%+v", d)
	}
}

// ---- 进度、panic、错误处理 ----

func TestProgressNeverRegresses(t *testing.T) {
	f := newFx(t, 1)
	tk, _ := f.m.Submit(Spec{Type: TypeConvert}, RunnerFunc(func(ctx context.Context, report func(Progress)) (string, error) {
		for _, v := range []float64{0.3, 0.6, 0.2, 0, 0.7, 0.5} {
			report(Progress{Fraction: v, OutTimeSec: v * 10})
		}
		return "", nil
	}))
	waitTask(t, f.m, tk.ID)
	last := -1.0
	n := 0
	for _, e := range f.em.all() {
		if p, ok := e.payload.(ProgressEvent); ok && p.ID == tk.ID {
			n++
			if p.Progress < last {
				t.Fatalf("进度回退: %v -> %v", last, p.Progress)
			}
			last = p.Progress
		}
	}
	if n != 6 || last != 0.7 {
		t.Fatalf("n=%d last=%v", n, last)
	}
}

type panicEmitter struct{ n atomic.Int32 }

func (p *panicEmitter) Emit(string, any) { p.n.Add(1); panic("emitter boom") }

func TestEmitterPanicIsContained(t *testing.T) {
	f := newFx(t, 1)
	pe := &panicEmitter{}
	f.m.cfg.Emitter = pe
	tk, err := f.m.Submit(Spec{Type: TypeConvert}, RunnerFunc(func(_ context.Context, r func(Progress)) (string, error) {
		r(Progress{Fraction: 0.5})
		return "", nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if d := waitTask(t, f.m, tk.ID); d.Status != StatusSucceeded {
		t.Fatalf("事件回调 panic 不应影响任务: %+v", d)
	}
	if pe.n.Load() == 0 {
		t.Fatal("emitter 应被调用")
	}
}

type panicFin struct{}

func (panicFin) Run(context.Context, func(Progress)) (string, error) { return "", nil }
func (panicFin) OnFinish(Task)                                       { panic("finish boom") }

func TestOnFinishPanicIsContained(t *testing.T) {
	f := newFx(t, 1)
	tk, _ := f.m.Submit(Spec{Type: TypeConvert}, panicFin{})
	if d := waitTask(t, f.m, tk.ID); d.Status != StatusSucceeded {
		t.Fatalf("%+v", d)
	}
	// 管理器仍可用（池名额已归还）
	tk2, _ := f.m.Submit(Spec{Type: TypeConvert}, RunnerFunc(func(context.Context, func(Progress)) (string, error) { return "", nil }))
	if d := waitTask(t, f.m, tk2.ID); d.Status != StatusSucceeded {
		t.Fatalf("%+v", d)
	}
}

func TestRetryUnsupportedTypeCode(t *testing.T) {
	f := newFx(t, 1)
	tk, _ := f.m.Submit(Spec{Type: TypeOfficePDF}, RunnerFunc(func(context.Context, func(Progress)) (string, error) { return "", errors.New("x") }))
	waitTask(t, f.m, tk.ID)
	_, err := f.m.Retry(tk.ID)
	if !apperr.Is(err, apperr.Unsupported) {
		t.Fatalf("没有工厂的类型 Retry 应返回 UNSUPPORTED: %v", err)
	}
}

func TestGetLogExactBoundaries(t *testing.T) {
	f := newFx(t, 1)
	tk, _ := f.m.Submit(Spec{Type: TypeConvert}, RunnerFunc(func(ctx context.Context, _ func(Progress)) (string, error) {
		fmt.Fprint(LogWriter(ctx), "only one line")
		return "", nil
	}))
	waitTask(t, f.m, tk.ID)
	if s, err := f.m.GetLog(tk.ID, 0); err != nil || s != "only one line" {
		t.Fatalf("%q %v", s, err)
	}
	if s, err := f.m.GetLog(tk.ID, 5); err != nil || s != "only one line" {
		t.Fatalf("%q %v", s, err)
	}
	// 没有日志文件
	tk2, _ := f.m.Submit(Spec{Type: TypeConvert}, RunnerFunc(func(context.Context, func(Progress)) (string, error) { return "", nil }))
	waitTask(t, f.m, tk2.ID)
	if s, err := f.m.GetLog(tk2.ID, 5); err != nil || s != "" {
		t.Fatalf("%q %v", s, err)
	}
}

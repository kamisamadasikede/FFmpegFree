package task

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
)

// ---------- 契约 v0.23：提交时占位、原地重试、转换页删除、任务中心隐藏 ----------

func detailOf(err error) string {
	var ae *apperr.AppError
	if errors.As(err, &ae) {
		return ae.Detail
	}
	return ""
}

func writePart(p string) error { return os.WriteFile(p, []byte("v"), 0o644) }

// partRunner 用 RunWithPart 写 out（提交时占的名字优先）。gate 非 nil 时先等它。
func partRunner(out string, gate chan struct{}) Runner {
	return RunnerFunc(func(ctx context.Context, _ func(Progress)) (string, error) {
		if gate != nil {
			select {
			case <-gate:
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
		return RunWithPart(ctx, out, writePart)
	})
}

func statusEvents(f *fx, id string) []StatusEvent {
	var out []StatusEvent
	for _, e := range f.em.all() {
		if s, ok := e.payload.(StatusEvent); ok && e.name == EventStatus && s.ID == id {
			out = append(out, s)
		}
	}
	return out
}

func TestSubmitReservesOutputWithSpacedSuffixForConvert(t *testing.T) {
	f := newFx(t, 1)
	gate := make(chan struct{})
	block, _ := f.m.Submit(Spec{Type: TypeEditExport}, RunnerFunc(func(ctx context.Context, _ func(Progress)) (string, error) {
		<-gate
		return "", nil
	}))
	dir := filepath.Join(f.dir, "o")
	os.MkdirAll(dir, 0o755)
	a := filepath.Join(dir, "a.mp4")
	os.WriteFile(filepath.Join(dir, "b.mp4"), []byte("old"), 0o644) // 磁盘上已有

	sub := func(typ Type, out string) Task {
		t.Helper()
		tk, err := f.m.Submit(Spec{Type: typ, OutputPath: out, ReserveOutput: true}, partRunner(out, nil))
		if err != nil {
			t.Fatal(err)
		}
		return tk
	}
	c1 := sub(TypeConvert, a)
	c2 := sub(TypeConvert, a) // 排队中的 c1 已占 a.mp4
	c3 := sub(TypeConvert, filepath.Join(dir, "b.mp4"))
	e1 := sub(TypeEditExport, a) // 非转换类型保持紧凑格式
	if c1.OutputPath != a || c2.OutputPath != filepath.Join(dir, "a (1).mp4") || c3.OutputPath != filepath.Join(dir, "b (1).mp4") ||
		e1.OutputPath != filepath.Join(dir, "a(1).mp4") {
		t.Fatalf("提交时定名: %s | %s | %s | %s", c1.OutputPath, c2.OutputPath, c3.OutputPath, e1.OutputPath)
	}
	if got, _ := f.st.GetTask(context.Background(), c2.ID); got.OutputPath != c2.OutputPath {
		t.Fatalf("落库的 outputPath 应是占到的名字: %s", got.OutputPath)
	}
	// 大小写不敏感的文件系统（Windows / macOS）上大小写不同也算重名
	if caseInsensitiveFS {
		if c4 := sub(TypeConvert, filepath.Join(dir, "A.MP4")); c4.OutputPath != filepath.Join(dir, "A (2).MP4") {
			t.Fatalf("大小写不同也算重名: %s", c4.OutputPath)
		}
	} else {
		sub(TypeConvert, a) // 占掉 a (2).mp4，与上面的分支结果一致
	}
	if p := f.m.PeekOutputName(a, TypeConvert); p != filepath.Join(dir, "a (3).mp4") {
		t.Fatalf("PeekOutputName: %s", p)
	}
	// 排队中取消 → 释放占位
	if err := f.m.Cancel(c2.ID); err != nil {
		t.Fatal(err)
	}
	waitTask(t, f.m, c2.ID)
	if p := f.m.PeekOutputName(a, TypeConvert); p != filepath.Join(dir, "a (1).mp4") {
		t.Fatalf("取消后应释放: %s", p)
	}
	close(gate)
	waitTask(t, f.m, block.ID)
	for _, tk := range []Task{c1, c3, e1} {
		d := waitTask(t, f.m, tk.ID)
		if d.Status != StatusSucceeded || d.OutputPath != tk.OutputPath {
			t.Fatalf("运行时应直接用占好的名字: %+v", d)
		}
		if _, err := os.Stat(d.OutputPath); err != nil {
			t.Fatal(err)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "b.mp4")); string(b) != "old" {
		t.Fatal("已有文件被覆盖")
	}
}

// 提交后、运行前磁盘上被别的程序建了同名文件：运行时顺延（带空格），并随 running 事件告知新名字。
func TestReservedNameTakenOnDiskBeforeRun(t *testing.T) {
	f := newFx(t, 1)
	gate := make(chan struct{})
	block, _ := f.m.Submit(Spec{Type: TypeEditExport}, RunnerFunc(func(ctx context.Context, _ func(Progress)) (string, error) {
		<-gate
		return "", nil
	}))
	out := filepath.Join(f.dir, "x.mp4")
	tk, _ := f.m.Submit(Spec{Type: TypeConvert, OutputPath: out, ReserveOutput: true}, partRunner(out, nil))
	os.WriteFile(out, []byte("other"), 0o644)
	close(gate)
	waitTask(t, f.m, block.ID)
	d := waitTask(t, f.m, tk.ID)
	want := filepath.Join(f.dir, "x (1).mp4")
	if d.Status != StatusSucceeded || d.OutputPath != want {
		t.Fatalf("%+v", d)
	}
	found := false
	for _, s := range statusEvents(f, tk.ID) {
		if s.Status == StatusRunning && s.OutputPath == want {
			found = true
		}
	}
	if !found {
		t.Fatal("顺延的新名字应随 running 事件发出")
	}
}

func TestRetryInPlaceResetsRun(t *testing.T) {
	f := newFx(t, 1)
	ctx := context.Background()
	out := filepath.Join(f.dir, "r.mp4")
	gate := make(chan struct{})
	f.m.RegisterFactory(TypeConvert, func(old Task) (Runner, error) { return partRunner(old.OutputPath, gate), nil })
	first, err := f.m.Submit(Spec{Type: TypeConvert, Title: "r.mov → MP4", InputPaths: []string{"/in/r.mov"}, Params: `{"presetName":"P"}`,
		OutputPath: out, ReserveOutput: true, SourceID: "SRC"},
		RunnerFunc(func(ctx context.Context, report func(Progress)) (string, error) {
			report(Progress{Fraction: 0.5, Speed: "2x"})
			return "", errors.New("boom")
		}))
	if err != nil {
		t.Fatal(err)
	}
	failed := waitTask(t, f.m, first.ID)
	if failed.Status != StatusFailed || failed.Error == nil || failed.StartedAt == 0 || failed.FinishedAt == 0 {
		t.Fatalf("%+v", failed)
	}
	// 上一次运行留下的 .part（例如崩溃）
	os.WriteFile(PartPath(out), []byte("stale"), 0o644)
	if _, err := f.m.HideFinishedInTaskCenter(); err != nil {
		t.Fatal(err)
	}
	created := f.em.count(EventCreated)

	nt, err := f.m.Retry(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if nt.ID != first.ID || nt.Status != StatusQueued || nt.Progress != 0 || nt.Speed != "" || nt.Error != nil ||
		nt.StartedAt != 0 || nt.FinishedAt != 0 || nt.HiddenInTaskCenter || nt.Result != nil ||
		nt.Version != failed.Version+1 || nt.OutputPath != out ||
		nt.Title != failed.Title || nt.Params != failed.Params || nt.SourceID != "SRC" || nt.CreatedAt != failed.CreatedAt ||
		len(nt.InputPaths) != 1 || nt.InputPaths[0] != "/in/r.mov" {
		t.Fatalf("重置不对: %+v", nt)
	}
	if _, err := os.Stat(PartPath(out)); !os.IsNotExist(err) {
		t.Fatal("上一次的 .part 应在入队前删掉")
	}
	if got, _ := f.st.GetTask(ctx, first.ID); got.Status != StatusQueued || got.Version != nt.Version || got.HiddenInTaskCenter || got.Error != nil {
		t.Fatalf("落库: %+v", got)
	}
	if f.em.count(EventCreated) != created {
		t.Fatal("原地重试不发 task:created")
	}
	var retried []StatusEvent
	for _, s := range statusEvents(f, first.ID) {
		if s.Retried {
			retried = append(retried, s)
		}
	}
	if len(retried) != 1 {
		t.Fatalf("应有且只有一条 retried 事件: %+v", retried)
	}
	r := retried[0]
	if r.Status != StatusQueued || r.Version != nt.Version || r.Progress == nil || *r.Progress != 0 ||
		r.HiddenInTaskCenter == nil || *r.HiddenInTaskCenter || r.OutputPath != out {
		t.Fatalf("retried 事件: %+v", r)
	}
	// 进行中不能再 Retry
	if _, err := f.m.Retry(first.ID); !apperr.Is(err, apperr.TaskConflict) {
		t.Fatalf("%v", err)
	}
	close(gate)
	d := waitTask(t, f.m, first.ID)
	if d.Status != StatusSucceeded || d.OutputPath != out || d.Progress != 1 {
		t.Fatalf("%+v", d)
	}
	log, _ := f.m.GetLog(first.ID, 100)
	if !strings.Contains(log, retryLogLine) {
		t.Fatalf("日志应有重试行: %q", log)
	}
	if _, err := f.m.Retry(first.ID); !apperr.Is(err, apperr.TaskConflict) || !strings.Contains(err.Error(), "不能重试") {
		t.Fatalf("成功后 Retry 应 TASK_CONFLICT: %v", err)
	}
}

func TestRetryOriginalNameTakenFallsBackWithSpacedSuffix(t *testing.T) {
	f := newFx(t, 1)
	f.m.RegisterFactory(TypeConvert, func(old Task) (Runner, error) { return partRunner(old.OutputPath, nil), nil })
	f.m.RegisterFactory(TypeEditExport, func(old Task) (Runner, error) { return partRunner(old.OutputPath, nil), nil })
	fail := RunnerFunc(func(context.Context, func(Progress)) (string, error) { return "", errors.New("x") })
	cases := []struct {
		typ        Type
		submitAs   string
		want       string
		occupyName string
	}{
		{TypeConvert, "s.mp4", "s (1).mp4", "s.mp4"},
		{TypeConvert, "t (1).mp4", "t.mp4", "t (1).mp4"}, // 去掉旧的 " (n)" 再顺延：t.mp4 空着
		{TypeEditExport, "u.mp4", "u(1).mp4", "u.mp4"},
	}
	for _, c := range cases {
		out := filepath.Join(f.dir, c.submitAs)
		tk, _ := f.m.Submit(Spec{Type: c.typ, OutputPath: out}, fail)
		waitTask(t, f.m, tk.ID)
		os.WriteFile(filepath.Join(f.dir, c.occupyName), []byte("other"), 0o644)
		nt, err := f.m.Retry(tk.ID)
		if err != nil {
			t.Fatal(err)
		}
		if nt.OutputPath != filepath.Join(f.dir, c.want) {
			t.Fatalf("%s: %s", c.submitAs, nt.OutputPath)
		}
		if d := waitTask(t, f.m, tk.ID); d.Status != StatusSucceeded || d.OutputPath != nt.OutputPath {
			t.Fatalf("%+v", d)
		}
	}
}

// 排队中取消的任务（从未运行）重试后再次排队中取消：仍按 NeverRanner 处理。
func TestRetryCanceledQueuedTaskAgain(t *testing.T) {
	f := newFx(t, 1)
	gate := make(chan struct{})
	defer close(gate)
	f.m.Submit(Spec{Type: TypeEditExport}, RunnerFunc(func(ctx context.Context, _ func(Progress)) (string, error) {
		<-gate
		return "", nil
	}))
	f.m.RegisterFactory(TypeConvert, func(old Task) (Runner, error) { return partRunner(old.OutputPath, nil), nil })
	out := filepath.Join(f.dir, "q.mp4")
	tk, _ := f.m.Submit(Spec{Type: TypeConvert, OutputPath: out, ReserveOutput: true}, partRunner(out, nil))
	f.m.Cancel(tk.ID)
	c := waitTask(t, f.m, tk.ID)
	if c.Status != StatusCanceled || c.StartedAt != 0 {
		t.Fatalf("%+v", c)
	}
	nt, err := f.m.Retry(tk.ID)
	if err != nil || nt.OutputPath != out {
		t.Fatalf("%+v %v", nt, err)
	}
	f.m.Cancel(tk.ID)
	if c2 := waitTask(t, f.m, tk.ID); c2.Status != StatusCanceled || c2.StartedAt != 0 || c2.Version <= nt.Version {
		t.Fatalf("%+v", c2)
	}
	if p := f.m.PeekOutputName(out, TypeConvert); p != out {
		t.Fatalf("终态后应释放占位: %s", p)
	}
}

func succeededConvert(t *testing.T, f *fx, name string, in string) Task {
	t.Helper()
	out := filepath.Join(f.dir, "out", name)
	tk, err := f.m.Submit(Spec{Type: TypeConvert, OutputPath: out, ReserveOutput: true, InputPaths: []string{in}}, partRunner(out, nil))
	if err != nil {
		t.Fatal(err)
	}
	d := waitTask(t, f.m, tk.ID)
	if d.Status != StatusSucceeded {
		t.Fatalf("%+v", d)
	}
	return d
}

func TestDeleteRecords(t *testing.T) {
	f := newFx(t, 4)
	f.m.cfg.DeleteWait = 300 * time.Millisecond
	in := filepath.Join(f.dir, "src.mov")
	os.WriteFile(in, []byte("src"), 0o644)

	// 成功记录 + 删输出：撤销 token → 删文件 → 删记录，输入文件不碰
	ok1 := succeededConvert(t, f, "a.mp4", in)
	var revoked []string
	removed := f.em.count(EventRemoved)
	res, err := f.m.DeleteRecords([]string{ok1.ID, ok1.ID, "nope"}, TypeConvert, true, func(p string) { revoked = append(revoked, p) })
	if err != nil || len(res.DeletedTaskIDs) != 1 || res.DeletedFiles != 1 || len(res.Failures) != 0 {
		t.Fatalf("%+v %v", res, err)
	}
	if len(revoked) != 1 || revoked[0] != ok1.OutputPath {
		t.Fatalf("删除前应撤销预览 token: %v", revoked)
	}
	if _, err := os.Stat(ok1.OutputPath); !os.IsNotExist(err) {
		t.Fatal("输出应已删除")
	}
	if _, err := os.Stat(in); err != nil {
		t.Fatal("源文件永远不删")
	}
	if _, err := f.m.Get(ok1.ID); !apperr.Is(err, apperr.NotFound) {
		t.Fatal("记录应已删除")
	}
	if f.em.count(EventRemoved) != removed+1 {
		t.Fatal("应发一次 task:removed")
	}

	// 不删输出：文件保留
	ok2 := succeededConvert(t, f, "b.mp4", in)
	if res, _ := f.m.DeleteRecords([]string{ok2.ID}, TypeConvert, false, nil); res.DeletedFiles != 0 || len(res.DeletedTaskIDs) != 1 {
		t.Fatalf("%+v", res)
	}
	if _, err := os.Stat(ok2.OutputPath); err != nil {
		t.Fatal("deleteOutputs=false 不删文件")
	}

	// not_task_output：文件修改时间早于任务开始（被替换过）
	ok3 := succeededConvert(t, f, "c.mp4", in)
	old := time.UnixMilli(ok3.StartedAt).Add(-time.Hour)
	os.Chtimes(ok3.OutputPath, old, old)
	res, _ = f.m.DeleteRecords([]string{ok3.ID}, TypeConvert, true, nil)
	if len(res.DeletedTaskIDs) != 1 || len(res.Failures) != 1 || res.Failures[0].Reason != DeleteNotTaskOutput ||
		res.Failures[0].Message != "文件已被替换或移动，没有删除" || res.Failures[0].Path != ok3.OutputPath {
		t.Fatalf("%+v", res)
	}
	if _, err := os.Stat(ok3.OutputPath); err != nil {
		t.Fatal("不是任务输出的文件不删")
	}

	// in_use：删除失败时记录照删、文件留着
	ok4 := succeededConvert(t, f, "d.mp4", in)
	removeFile = func(p string) error { return &os.PathError{Op: "remove", Path: p, Err: inUseErrno} }
	res, _ = f.m.DeleteRecords([]string{ok4.ID}, TypeConvert, true, nil)
	removeFile = os.Remove
	if len(res.DeletedTaskIDs) != 1 || len(res.Failures) != 1 || res.Failures[0].Reason != DeleteInUse || res.Failures[0].Message != "文件正在被使用，没有删除" {
		t.Fatalf("%+v", res)
	}

	// 运行中：先取消再删
	running, _ := f.m.Submit(Spec{Type: TypeConvert, OutputPath: filepath.Join(f.dir, "out", "e.mp4"), ReserveOutput: true},
		partRunner(filepath.Join(f.dir, "out", "e.mp4"), make(chan struct{})))
	eventually(t, func() bool { return f.m.mustGet(t, running.ID).Status == StatusRunning })
	res, err = f.m.DeleteRecords([]string{running.ID}, TypeConvert, true, nil)
	if err != nil || len(res.DeletedTaskIDs) != 1 || len(res.Failures) != 0 {
		t.Fatalf("运行中的应先取消再删: %+v %v", res, err)
	}
	if _, err := f.m.Get(running.ID); !apperr.Is(err, apperr.NotFound) {
		t.Fatal("记录应已删除")
	}

	// still_running：不响应取消的任务等不到终态 → 不删
	stuck := make(chan struct{})
	hard, _ := f.m.Submit(Spec{Type: TypeConvert}, RunnerFunc(func(ctx context.Context, _ func(Progress)) (string, error) {
		<-stuck
		return "", ctx.Err()
	}))
	eventually(t, func() bool { return f.m.mustGet(t, hard.ID).Status == StatusRunning })
	res, _ = f.m.DeleteRecords([]string{hard.ID}, TypeConvert, false, nil)
	if len(res.DeletedTaskIDs) != 0 || len(res.Failures) != 1 || res.Failures[0].Reason != DeleteStillRunning ||
		res.Failures[0].Message != "任务还没停下来，没有删除这条记录" {
		t.Fatalf("%+v", res)
	}
	// still_running 没有文件，path 为空且 JSON 里省略（契约 6.14.2 DeleteFailure）
	if res.Failures[0].Path != "" {
		t.Fatalf("%+v", res.Failures[0])
	}
	if b, _ := json.Marshal(res.Failures[0]); strings.Contains(string(b), `"path"`) {
		t.Fatalf("%s", b)
	}
	close(stuck)
	waitTask(t, f.m, hard.ID)
	if _, err := f.m.Get(hard.ID); err != nil {
		t.Fatal("没停下来的记录应保留")
	}

	// 失败记录的 .part 残留随删除清掉
	pout := filepath.Join(f.dir, "out", "f.mp4")
	ft, _ := f.m.Submit(Spec{Type: TypeConvert, OutputPath: pout}, RunnerFunc(func(context.Context, func(Progress)) (string, error) {
		return "", errors.New("x")
	}))
	waitTask(t, f.m, ft.ID)
	os.WriteFile(PartPath(pout), []byte("p"), 0o644)
	if res, _ := f.m.DeleteRecords([]string{ft.ID}, TypeConvert, true, nil); len(res.DeletedTaskIDs) != 1 || res.DeletedFiles != 0 {
		t.Fatalf("%+v", res)
	}
	if _, err := os.Stat(PartPath(pout)); !os.IsNotExist(err) {
		t.Fatal(".part 残留应删掉")
	}

	// 不是转换记录：整体 INVALID_ARGUMENT
	ed, _ := f.m.Submit(Spec{Type: TypeEditExport}, ok)
	waitTask(t, f.m, ed.ID)
	if _, err := f.m.DeleteRecords([]string{ed.ID}, TypeConvert, false, nil); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("%v", err)
	}
	if _, err := f.m.Get(ed.ID); err != nil {
		t.Fatal("不应删除")
	}
}

func TestHideAndUnhideInTaskCenter(t *testing.T) {
	f := newFx(t, 2)
	a, _ := f.m.Submit(Spec{Type: TypeConvert}, ok)
	b, _ := f.m.Submit(Spec{Type: TypeEditExport}, RunnerFunc(func(context.Context, func(Progress)) (string, error) { return "", errors.New("x") }))
	da, db := waitTask(t, f.m, a.ID), waitTask(t, f.m, b.ID)
	gate := make(chan struct{})
	defer close(gate)
	run, _ := f.m.Submit(Spec{Type: TypeEditExport}, RunnerFunc(func(context.Context, func(Progress)) (string, error) { <-gate; return "", nil }))
	eventually(t, func() bool { return f.m.mustGet(t, run.ID).Status == StatusRunning })

	evts := len(f.em.all())
	n, err := f.m.HideFinishedInTaskCenter()
	if err != nil || n != 2 {
		t.Fatalf("所有类型的已结束任务都隐藏: %d %v", n, err)
	}
	if len(f.em.all()) != evts {
		t.Fatal("HideFinishedInTaskCenter 不发事件")
	}
	if got := f.m.mustGet(t, a.ID); !got.HiddenInTaskCenter || got.Version != da.Version {
		t.Fatalf("隐藏不改 version: %+v", got)
	}
	if p, _ := f.m.List(Filter{}); p.Total != 1 || p.Items[0].ID != run.ID {
		t.Fatalf("默认只列未隐藏的: %+v", p)
	}
	if p, _ := f.m.List(Filter{IncludeHidden: true}); p.Total != 3 {
		t.Fatalf("includeHidden: %d", p.Total)
	}
	if err := f.m.UnhideInTaskCenter([]string{b.ID, b.ID, run.ID}); err != nil {
		t.Fatal(err)
	}
	got := f.m.mustGet(t, b.ID)
	if got.HiddenInTaskCenter || got.Version != db.Version+1 {
		t.Fatalf("%+v", got)
	}
	ev := statusEvents(f, b.ID)
	last := ev[len(ev)-1]
	if last.HiddenInTaskCenter == nil || *last.HiddenInTaskCenter || last.Version != got.Version || last.Status != StatusFailed || last.Retried {
		t.Fatalf("取消隐藏事件: %+v", last)
	}
	before := len(f.em.all())
	if err := f.m.UnhideInTaskCenter([]string{b.ID}); err != nil || len(f.em.all()) != before {
		t.Fatalf("幂等：已经没隐藏的不发事件: %v", err)
	}
	if err := f.m.UnhideInTaskCenter([]string{a.ID, "nope"}); !apperr.Is(err, apperr.NotFound) || detailOf(err) != "reason=record" {
		t.Fatalf("%v", err)
	}
	if f.m.mustGet(t, a.ID).HiddenInTaskCenter != true {
		t.Fatal("整体校验失败时什么都不改")
	}
	if err := f.m.UnhideInTaskCenter(nil); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("%v", err)
	}
}

func TestRemoveRejectsConvertTasks(t *testing.T) {
	f := newFx(t, 2)
	c, _ := f.m.Submit(Spec{Type: TypeConvert}, ok)
	e, _ := f.m.Submit(Spec{Type: TypeEditExport}, ok)
	waitTask(t, f.m, c.ID)
	waitTask(t, f.m, e.ID)
	err := f.m.Remove([]string{e.ID, c.ID}, false)
	if !apperr.Is(err, apperr.InvalidArgument) || !strings.Contains(err.Error(), "转换记录请在格式转换页删除") {
		t.Fatalf("%v", err)
	}
	if _, err := f.m.Get(e.ID); err != nil {
		t.Fatal("整体拒绝：一个都不删")
	}
	if err := f.m.Remove([]string{e.ID}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.m.Get(e.ID); !apperr.Is(err, apperr.NotFound) {
		t.Fatal("非转换任务照常删除")
	}
}

func TestTaskFile(t *testing.T) {
	f := newFx(t, 2)
	in := filepath.Join(f.dir, "in.mov")
	os.WriteFile(in, []byte("x"), 0o644)
	d := succeededConvert(t, f, "a.mp4", in)
	if p, tk, err := f.m.TaskFile(d.ID, "output"); err != nil || p != d.OutputPath || tk.ID != d.ID {
		t.Fatalf("%s %v", p, err)
	}
	if p, _, err := f.m.TaskFile(d.ID, "input"); err != nil || p != in {
		t.Fatalf("%s %v", p, err)
	}
	if _, _, err := f.m.TaskFile(d.ID, "log"); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("%v", err)
	}
	if _, _, err := f.m.TaskFile("nope", "output"); detailOf(err) != "reason=record" {
		t.Fatalf("%v", err)
	}
	os.Remove(d.OutputPath)
	if err := os.Symlink(in, d.OutputPath); err == nil {
		if _, _, err := f.m.TaskFile(d.ID, "output"); detailOf(err) != "reason=file" {
			t.Fatalf("符号链接不算: %v", err)
		}
	}
	os.Remove(in)
	if _, _, err := f.m.TaskFile(d.ID, "input"); detailOf(err) != "reason=file" {
		t.Fatalf("%v", err)
	}
	failed, _ := f.m.Submit(Spec{Type: TypeConvert, OutputPath: d.OutputPath}, RunnerFunc(func(context.Context, func(Progress)) (string, error) { return "", errors.New("x") }))
	waitTask(t, f.m, failed.ID)
	if _, _, err := f.m.TaskFile(failed.ID, "output"); detailOf(err) != "reason=file" {
		t.Fatalf("没成功就没有输出: %v", err)
	}
}

func TestCheckPaths(t *testing.T) {
	f := newFx(t, 2)
	in := filepath.Join(f.dir, "in.mov")
	os.WriteFile(in, []byte("x"), 0o644)
	d := succeededConvert(t, f, "a.mp4", in)
	got, err := f.m.CheckPaths([]string{d.ID, "nope"})
	if err != nil || len(got) != 2 {
		t.Fatalf("%+v %v", got, err)
	}
	if !got[0].Found || !got[0].InputExists || !got[0].OutputExists || got[1].Found || got[1].TaskID != "nope" {
		t.Fatalf("%+v", got)
	}
	os.Remove(in)
	if got, _ := f.m.CheckPaths([]string{d.ID}); got[0].InputExists || !got[0].OutputExists {
		t.Fatalf("%+v", got)
	}
	if _, err := f.m.CheckPaths(nil); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("%v", err)
	}
}

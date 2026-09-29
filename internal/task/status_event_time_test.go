package task

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"FFmpegFree/internal/store"
)

// statusEvents 返回某任务收到的全部 task:status 事件（按发出顺序）。
func (r *rec) statusEvents(id string) []StatusEvent {
	var out []StatusEvent
	for _, e := range r.all() {
		if s, ok := e.payload.(StatusEvent); ok && e.name == EventStatus && s.ID == id {
			out = append(out, s)
		}
	}
	return out
}

// terminalEvent 等待并返回任务的终态 task:status 事件（事件在 Wait 返回之后才可能被观察到，所以轮询）。
func terminalEvent(t *testing.T, f *fx, id string) StatusEvent {
	t.Helper()
	var ev StatusEvent
	eventually(t, func() bool {
		evs := f.em.statusEvents(id)
		if len(evs) == 0 || evs[len(evs)-1].Status.Active() {
			return false
		}
		ev = evs[len(evs)-1]
		return true
	})
	return ev
}

// checkTimes 断言 Task（内存/Get）、落库记录和终态事件三者的 startedAt / finishedAt 一致。
func checkTimes(t *testing.T, f *fx, tk Task, ev StatusEvent, wantStarted bool) {
	t.Helper()
	db, err := f.st.GetTask(context.Background(), tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if tk.FinishedAt == 0 || db.FinishedAt != tk.FinishedAt || ev.FinishedAt != tk.FinishedAt {
		t.Fatalf("finishedAt 不一致: task=%d db=%d event=%d", tk.FinishedAt, db.FinishedAt, ev.FinishedAt)
	}
	if db.StartedAt != tk.StartedAt || ev.StartedAt != tk.StartedAt {
		t.Fatalf("startedAt 不一致: task=%d db=%d event=%d", tk.StartedAt, db.StartedAt, ev.StartedAt)
	}
	if wantStarted {
		if tk.StartedAt == 0 || tk.FinishedAt < tk.StartedAt {
			t.Fatalf("应有 startedAt 且 finishedAt >= startedAt: %d %d", tk.StartedAt, tk.FinishedAt)
		}
	} else if tk.StartedAt != 0 {
		t.Fatalf("从未 running 的任务 startedAt 应为 0: %d", tk.StartedAt)
	}
	if db.Status != tk.Status || ev.Status != tk.Status {
		t.Fatalf("状态不一致: task=%s db=%s event=%s", tk.Status, db.Status, ev.Status)
	}
}

func TestStatusEventRunningHasStartedAtOnly(t *testing.T) {
	f := newFx(t, 1)
	gate := make(chan struct{})
	tk, _ := f.m.Submit(Spec{Type: TypeConvert}, RunnerFunc(func(ctx context.Context, _ func(Progress)) (string, error) {
		<-gate
		return "/out/x.mp4", nil
	}))
	eventually(t, func() bool { return len(f.em.statusEvents(tk.ID)) == 1 })
	run := f.em.statusEvents(tk.ID)[0]
	if run.Status != StatusRunning || run.StartedAt == 0 || run.FinishedAt != 0 {
		t.Fatalf("running 事件应带 startedAt、不带 finishedAt: %+v", run)
	}
	cur := f.m.mustGet(t, tk.ID)
	if run.StartedAt != cur.StartedAt {
		t.Fatalf("事件与 Task 的 startedAt 不一致: %d vs %d", run.StartedAt, cur.StartedAt)
	}
	db, _ := f.st.GetTask(context.Background(), tk.ID)
	if db.StartedAt != run.StartedAt || db.FinishedAt != 0 {
		t.Fatalf("running 落库应有 startedAt、无 finishedAt: %+v", db)
	}
	close(gate)
	waitTask(t, f.m, tk.ID)
}

func TestStatusEventTerminalTimesSucceededAndFailed(t *testing.T) {
	f := newFx(t, 2)
	okT, _ := f.m.Submit(Spec{Type: TypeConvert}, ok)
	badT, _ := f.m.Submit(Spec{Type: TypeConvert}, RunnerFunc(func(context.Context, func(Progress)) (string, error) {
		return "", errors.New("boom")
	}))
	for _, c := range []struct {
		id string
		st Status
	}{{okT.ID, StatusSucceeded}, {badT.ID, StatusFailed}} {
		done := waitTask(t, f.m, c.id)
		if done.Status != c.st {
			t.Fatalf("%+v", done)
		}
		ev := terminalEvent(t, f, c.id)
		checkTimes(t, f, done, ev, true)
		// running 事件的 startedAt 与终态事件一致
		if evs := f.em.statusEvents(c.id); evs[0].Status != StatusRunning || evs[0].StartedAt != ev.StartedAt {
			t.Fatalf("running 与终态事件的 startedAt 应一致: %+v", evs)
		}
	}
}

func TestStatusEventTerminalTimesCanceledWhileRunning(t *testing.T) {
	f := newFx(t, 1)
	tk, _ := f.m.Submit(Spec{Type: TypeConvert}, RunnerFunc(func(ctx context.Context, _ func(Progress)) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}))
	eventually(t, func() bool { return f.m.mustGet(t, tk.ID).Status == StatusRunning })
	if err := f.m.Cancel(tk.ID); err != nil {
		t.Fatal(err)
	}
	done := waitTask(t, f.m, tk.ID)
	if done.Status != StatusCanceled {
		t.Fatalf("%+v", done)
	}
	checkTimes(t, f, done, terminalEvent(t, f, tk.ID), true)
}

func TestStatusEventTerminalTimesCanceledWhileQueued(t *testing.T) {
	f := newFx(t, 1)
	gate := make(chan struct{})
	blocker, _ := f.m.Submit(Spec{Type: TypeConvert}, RunnerFunc(func(ctx context.Context, _ func(Progress)) (string, error) {
		<-gate
		return "", nil
	}))
	eventually(t, func() bool { return f.m.mustGet(t, blocker.ID).Status == StatusRunning })
	queued, _ := f.m.Submit(Spec{Type: TypeConvert}, ok)
	if err := f.m.Cancel(queued.ID); err != nil {
		t.Fatal(err)
	}
	done := waitTask(t, f.m, queued.ID)
	if done.Status != StatusCanceled {
		t.Fatalf("%+v", done)
	}
	ev := terminalEvent(t, f, queued.ID)
	// 从未 running：startedAt 为 0（JSON 里省略），finishedAt 有值
	checkTimes(t, f, done, ev, false)
	if evs := f.em.statusEvents(queued.ID); len(evs) != 1 {
		t.Fatalf("排队中取消只应发一个终态事件: %+v", evs)
	}
	close(gate)
	waitTask(t, f.m, blocker.ID)
}

func TestStatusEventTerminalTimesInterruptedOnShutdown(t *testing.T) {
	f := newFx(t, 1)
	running, _ := f.m.Submit(Spec{Type: TypeConvert}, RunnerFunc(func(ctx context.Context, _ func(Progress)) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}))
	queued, _ := f.m.Submit(Spec{Type: TypeConvert}, ok)
	eventually(t, func() bool { return f.m.mustGet(t, running.ID).Status == StatusRunning })
	f.m.Shutdown(2 * time.Second)

	r := f.m.mustGet(t, running.ID)
	if r.Status != StatusInterrupted {
		t.Fatalf("%+v", r)
	}
	checkTimes(t, f, r, terminalEvent(t, f, running.ID), true)
	q := f.m.mustGet(t, queued.ID)
	if q.Status != StatusInterrupted {
		t.Fatalf("%+v", q)
	}
	checkTimes(t, f, q, terminalEvent(t, f, queued.ID), false)
}

// 崩溃恢复：启动时 MarkInterrupted 把残留的 queued / running 置为 interrupted，
// 必须写入 finishedAt 并保留已有的 startedAt（该路径在启动阶段执行，前端尚未订阅，所以不发事件，只落库）。
func TestCrashRecoveryInterruptedTimes(t *testing.T) {
	f := newFx(t, 1)
	ctx := context.Background()
	f.st.InsertTask(ctx, store.Task{ID: "CRASHRUN", Type: TypeConvert, Status: StatusRunning, Version: 4, CreatedAt: 1000, StartedAt: 2000})
	f.st.InsertTask(ctx, store.Task{ID: "CRASHQ", Type: TypeConvert, Status: StatusQueued, Version: 1, CreatedAt: 1500})
	f.st.InsertTask(ctx, store.Task{ID: "DONE", Type: TypeConvert, Status: StatusSucceeded, Version: 3, CreatedAt: 1, StartedAt: 2, FinishedAt: 3})
	now := time.UnixMilli(9000)
	if n, err := f.st.MarkInterrupted(ctx, now); err != nil || n != 2 {
		t.Fatalf("%d %v", n, err)
	}
	run := f.m.mustGet(t, "CRASHRUN")
	if run.Status != StatusInterrupted || run.StartedAt != 2000 || run.FinishedAt != 9000 || run.Version != 5 {
		t.Fatalf("%+v", run)
	}
	q := f.m.mustGet(t, "CRASHQ")
	if q.Status != StatusInterrupted || q.StartedAt != 0 || q.FinishedAt != 9000 {
		t.Fatalf("%+v", q)
	}
	// 已结束的任务不受影响
	if d := f.m.mustGet(t, "DONE"); d.StartedAt != 2 || d.FinishedAt != 3 || d.Version != 3 {
		t.Fatalf("%+v", d)
	}
	// List 返回的历史任务同样带时间
	page, err := f.m.List(Filter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range page.Items {
		if !it.Status.Active() && it.FinishedAt == 0 {
			t.Fatalf("List 返回的终态任务缺 finishedAt: %+v", it)
		}
	}
}

// Retry 生成的是新任务：时间从零开始，原任务的 startedAt / finishedAt 不被改动。
func TestRetryTimes(t *testing.T) {
	f := newFx(t, 1)
	f.m.RegisterFactory(TypeConvert, func(Task) (Runner, error) { return ok, nil })
	first, _ := f.m.Submit(Spec{Type: TypeConvert, Params: `{}`}, RunnerFunc(func(context.Context, func(Progress)) (string, error) {
		return "", errors.New("fail")
	}))
	old := waitTask(t, f.m, first.ID)
	nt, err := f.m.Retry(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if nt.StartedAt != 0 || nt.FinishedAt != 0 {
		t.Fatalf("新任务刚创建时不应有 startedAt / finishedAt: %+v", nt)
	}
	done := waitTask(t, f.m, nt.ID)
	checkTimes(t, f, done, terminalEvent(t, f, nt.ID), true)
	after := f.m.mustGet(t, first.ID)
	if after.StartedAt != old.StartedAt || after.FinishedAt != old.FinishedAt {
		t.Fatalf("原任务的时间被改动: %+v -> %+v", old, after)
	}
}

// JSON 契约：startedAt / finishedAt 为 0 时省略，非 0 时输出。
func TestStatusEventJSONOmitEmpty(t *testing.T) {
	raw := func(e StatusEvent) map[string]any {
		b, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		json.Unmarshal(b, &m)
		return m
	}
	m := raw(StatusEvent{ID: "a", Version: 2, Status: StatusRunning, StartedAt: 10})
	if m["startedAt"] != float64(10) {
		t.Fatalf("%v", m)
	}
	if _, has := m["finishedAt"]; has {
		t.Fatalf("running 事件不应有 finishedAt: %v", m)
	}
	m = raw(StatusEvent{ID: "a", Version: 2, Status: StatusCanceled, FinishedAt: 20})
	if _, has := m["startedAt"]; has {
		t.Fatalf("从未 running 的终态事件应省略 startedAt: %v", m)
	}
	if m["finishedAt"] != float64(20) {
		t.Fatalf("%v", m)
	}
}

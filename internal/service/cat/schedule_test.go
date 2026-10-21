package cat_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"FFmpegFree/internal/agentexec"
	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/service/cat"
	"FFmpegFree/internal/store"
)

var cst = time.FixedZone("CST", 8*3600)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time  { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *fakeClock) Set(t time.Time) { c.mu.Lock(); c.t = t; c.mu.Unlock() }

// gate 是可控的假助手：每一轮记下参数，等 release 或被取消。
type gate struct {
	mu      sync.Mutex
	opts    []catagent.TurnOptions
	running atomic.Int32
	release chan struct{}
	entered chan struct{} // 每进入一轮 run 发一个信号（running 已 +1 之后）
	fail    bool
}

func newGate(block bool) *gate {
	g := &gate{release: make(chan struct{}), entered: make(chan struct{}, 64)}
	if !block {
		close(g.release)
	}
	return g
}

func (g *gate) adapter() *fakeAdapter {
	return &fakeAdapter{run: func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		g.mu.Lock()
		g.opts = append(g.opts, o)
		g.mu.Unlock()
		g.running.Add(1)
		defer g.running.Add(-1)
		select {
		case g.entered <- struct{}{}:
		default:
		}
		select {
		case <-g.release:
		case <-o.Ctx.Done():
			return catagent.TurnResponse{}, apperr.New(apperr.Canceled, "已取消")
		}
		if g.fail {
			return catagent.TurnResponse{}, apperr.New(apperr.CatReplyFailed, "出错了")
		}
		return catagent.TurnResponse{Message: catagent.WireMessage{Role: "assistant", Content: "好"}}, nil
	}}
}

type notes struct {
	mu   sync.Mutex
	list []cat.Notification
}

func (n *notes) add(x cat.Notification) { n.mu.Lock(); n.list = append(n.list, x); n.mu.Unlock() }
func (n *notes) all() []cat.Notification {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]cat.Notification(nil), n.list...)
}

type schedEnv struct {
	svc   *cat.Service
	st    *store.Store
	clock *fakeClock
	gate  *gate
	notes *notes
	proj  cat.Project
	dir   string
}

func newSchedEnv(t *testing.T, block bool, mut func(*cat.Config)) *schedEnv {
	t.Helper()
	e := &schedEnv{st: openStore(t), clock: &fakeClock{t: time.Date(2026, 10, 14, 8, 59, 0, 0, cst)}, gate: newGate(block), notes: &notes{}}
	cfg := cat.Config{Now: e.clock.Now, Location: cst, Notify: e.notes.add, DataRoot: t.TempDir(), ScheduleTick: time.Hour}
	if mut != nil {
		mut(&cfg)
	}
	e.svc, _ = newProjSvc(t, e.st, e.gate.adapter(), cfg)
	e.dir = filepath.Join(t.TempDir(), "proj")
	_ = os.MkdirAll(e.dir, 0o755)
	pr, err := e.svc.CreateCatProject(context.Background(), cat.CreateProjectRequest{Path: e.dir, Name: "演示"})
	if err != nil {
		t.Fatal(err)
	}
	e.proj = pr.Project
	t.Cleanup(func() {
		select {
		case <-e.gate.release:
		default:
			close(e.gate.release)
		}
		e.svc.CloseScheduler()
		e.svc.Wait()
	})
	return e
}

func (e *schedEnv) create(t *testing.T, in cat.ScheduleInput) cat.Schedule {
	t.Helper()
	if in.Name == "" {
		in.Name = "每日检查"
	}
	if in.Prompt == "" {
		in.Prompt = "检查一下"
	}
	in.ProjectID = e.proj.ID
	sc, err := e.svc.CreateCatSchedule(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	return sc
}

func waitRuns(t *testing.T, svc *cat.Service, schedID string, ok func([]cat.ScheduleRun) bool) []cat.ScheduleRun {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		runs, err := svc.ListCatScheduleRuns(context.Background(), cat.ListScheduleRunsRequest{ScheduleID: schedID})
		if err == nil && ok(runs) {
			return runs
		}
		if time.Now().After(deadline) {
			t.Fatalf("runs not as expected: %+v %v", runs, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func lastStatus(want string) func([]cat.ScheduleRun) bool {
	return func(r []cat.ScheduleRun) bool { return len(r) > 0 && r[0].Status == want }
}

func TestNextRunAfter(t *testing.T) {
	at := func(d, h, m int) time.Time { return time.Date(2026, 10, d, h, m, 0, 0, cst) } // 10-16 是周五
	ms := func(x time.Time) int64 { return x.UnixMilli() }
	cases := []struct {
		row   store.CatScheduleRow
		after time.Time
		want  time.Time
	}{
		{store.CatScheduleRow{Frequency: "daily", Time: "09:00"}, at(14, 8, 0), at(14, 9, 0)},
		{store.CatScheduleRow{Frequency: "daily", Time: "09:00"}, at(14, 9, 0), at(15, 9, 0)},
		{store.CatScheduleRow{Frequency: "weekdays", Time: "09:00"}, at(16, 10, 0), at(19, 9, 0)},
		{store.CatScheduleRow{Frequency: "weekly", Time: "18:30", Weekdays: []int{3, 7}}, at(15, 19, 0), at(18, 18, 30)},
	}
	for i, c := range cases {
		if got := cat.NextRunAfter(c.row, c.after, cst); got != ms(c.want) {
			t.Fatalf("case %d: got %v want %v", i, time.UnixMilli(got).In(cst), c.want)
		}
	}
	once := store.CatScheduleRow{Frequency: "once", RunAt: ms(at(20, 1, 0))}
	if cat.NextRunAfter(once, at(19, 0, 0), cst) != once.RunAt || cat.NextRunAfter(once, at(21, 0, 0), cst) != 0 {
		t.Fatal("once")
	}
}

func TestScheduleValidation(t *testing.T) {
	e := newSchedEnv(t, false, nil)
	ctx := context.Background()
	base := cat.ScheduleInput{Name: "a", Prompt: "p", ProjectID: e.proj.ID, Frequency: "daily", Time: "09:00"}
	bad := []func(*cat.ScheduleInput){
		func(i *cat.ScheduleInput) { i.Name = strings.Repeat("名", 41) },
		func(i *cat.ScheduleInput) { i.Prompt = "" },
		func(i *cat.ScheduleInput) { i.Time = "9:00" },
		func(i *cat.ScheduleInput) { i.Frequency = "cron" },
		func(i *cat.ScheduleInput) { i.Frequency = "weekly"; i.Weekdays = nil },
		func(i *cat.ScheduleInput) { i.Frequency = "weekly"; i.Weekdays = []int{8} },
		func(i *cat.ScheduleInput) { i.Frequency = "once"; i.RunAt = e.clock.Now().UnixMilli() },
		func(i *cat.ScheduleInput) { i.Effort = "MAX!" },                        // 格式不对
		func(i *cat.ScheduleInput) { i.Model = "grok-4.5"; i.Effort = "xhigh" }, // v0.44：该模型没有 xhigh
		func(i *cat.ScheduleInput) { i.ApprovalMode = "yolo" },
	}
	for i, f := range bad {
		in := base
		f(&in)
		if _, err := e.svc.CreateCatSchedule(ctx, in); !apperr.Is(err, apperr.InvalidArgument) {
			t.Fatalf("case %d: %v", i, err)
		}
	}
	in := base
	in.ProjectID = "nope"
	if _, err := e.svc.CreateCatSchedule(ctx, in); !apperr.Is(err, apperr.NotFound) {
		t.Fatal(err)
	}
	in = base
	in.Name = strings.Repeat("名", 40)
	in.Frequency, in.Weekdays = "weekly", []int{5, 1, 5}
	sc, err := e.svc.CreateCatSchedule(ctx, in)
	if err != nil || sc.ApprovalMode != "default" || !sc.Enabled || len(sc.Weekdays) != 2 || sc.Weekdays[0] != 1 || sc.Timezone == "" || sc.NextRunAt == 0 {
		t.Fatalf("%+v %v", sc, err)
	}
	if _, err := e.svc.PauseCatSchedule(ctx, cat.ScheduleIDRequest{ID: "nope"}); !apperr.Is(err, apperr.NotFound) {
		t.Fatal(err)
	}
}

func TestScheduleRunsAtTimeWithRulesAndNotifies(t *testing.T) {
	e := newSchedEnv(t, false, nil)
	ctx := context.Background()
	if _, err := e.svc.SaveCatSettings(ctx, cat.Settings{CustomInstruction: "用中文"}); err != nil {
		t.Fatal(err)
	}
	sc := e.create(t, cat.ScheduleInput{Frequency: "daily", Time: "09:00", Model: "grok-4.6", Effort: "high"})
	e.svc.StartScheduler(ctx) // 08:59：没到点
	if runs, _ := e.svc.ListCatScheduleRuns(ctx, cat.ListScheduleRunsRequest{ScheduleID: sc.ID}); len(runs) != 0 {
		t.Fatalf("%+v", runs)
	}
	e.clock.Set(time.Date(2026, 10, 14, 9, 0, 30, 0, cst))
	e.svc.SchedulerTick(ctx)
	runs := waitRuns(t, e.svc, sc.ID, lastStatus(cat.RunSucceeded))
	r := runs[0]
	if r.ConversationID == "" || r.Manual || r.ScheduledFor != time.Date(2026, 10, 14, 9, 0, 0, 0, cst).UnixMilli() {
		t.Fatalf("%+v", r)
	}
	conv, err := e.svc.GetCatConversation(ctx, r.ConversationID)
	if err != nil || conv.ProjectID != e.proj.ID || conv.Title != "每日检查 · 10-14 09:00" {
		t.Fatalf("%+v %v", conv.CatConversation, err)
	}
	e.gate.mu.Lock()
	o := e.gate.opts[0]
	e.gate.mu.Unlock()
	physical, err := agentexec.CanonicalRoot(e.dir)
	if err != nil {
		t.Fatal(err)
	}
	if o.Rules != "用中文" || o.PermissionMode != "default" || o.ModelID != "grok-4.6" || o.ThinkLevelID != "high" || o.ProjectPath != physical {
		t.Fatalf("%+v", o)
	}
	list, _ := e.svc.ListCatSchedules(ctx)
	if len(list) != 1 || list[0].NextRunAt != time.Date(2026, 10, 15, 9, 0, 0, 0, cst).UnixMilli() || list[0].LastRun == nil {
		t.Fatalf("%+v", list)
	}
	deadline := time.Now().Add(2 * time.Second)
	for len(e.notes.all()) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	ns := e.notes.all()
	if len(ns) != 1 || ns[0].Title != "定时任务已完成：每日检查" || !strings.HasPrefix(ns[0].Body, "演示 · 用时 0 分 0 秒。") || ns[0].ConvID != r.ConversationID {
		t.Fatalf("%+v", ns)
	}
}

func TestScheduleMissedOnceAndAsleep(t *testing.T) {
	e := newSchedEnv(t, false, nil)
	ctx := context.Background()
	sc := e.create(t, cat.ScheduleInput{Frequency: "daily", Time: "09:00"})
	// 应用三天没开：只记一条 closed，不补跑。
	e.clock.Set(time.Date(2026, 10, 17, 12, 0, 0, 0, cst))
	e.svc.StartScheduler(ctx)
	runs := waitRuns(t, e.svc, sc.ID, lastStatus(cat.RunMissed))
	if len(runs) != 1 || runs[0].MissedReason != cat.MissedClosed || runs[0].ConversationID != "" {
		t.Fatalf("%+v", runs)
	}
	list, _ := e.svc.ListCatSchedules(ctx)
	if list[0].NextRunAt != time.Date(2026, 10, 18, 9, 0, 0, 0, cst).UnixMilli() {
		t.Fatalf("next %v", time.UnixMilli(list[0].NextRunAt))
	}
	// 睡过了第二天 9 点：asleep。
	e.clock.Set(time.Date(2026, 10, 18, 11, 0, 0, 0, cst))
	e.svc.SchedulerTick(ctx)
	runs = waitRuns(t, e.svc, sc.ID, func(r []cat.ScheduleRun) bool { return len(r) == 2 })
	if runs[0].Status != cat.RunMissed || runs[0].MissedReason != cat.MissedAsleep {
		t.Fatalf("%+v", runs)
	}
	if len(e.gate.opts) != 0 || len(e.notes.all()) != 0 {
		t.Fatal("missed must not run or notify")
	}
	// 仅一次：错过后停用。
	once := e.create(t, cat.ScheduleInput{Name: "一次", Frequency: "once", RunAt: time.Date(2026, 10, 18, 11, 30, 0, 0, cst).UnixMilli()})
	e.clock.Set(time.Date(2026, 10, 18, 12, 0, 0, 0, cst))
	e.svc.SchedulerTick(ctx)
	waitRuns(t, e.svc, once.ID, lastStatus(cat.RunMissed))
	list, _ = e.svc.ListCatSchedules(ctx)
	for _, x := range list {
		if x.ID == once.ID && (x.Enabled || x.NextRunAt != 0) {
			t.Fatalf("%+v", x)
		}
	}
	if _, err := e.svc.ResumeCatSchedule(ctx, cat.ScheduleIDRequest{ID: once.ID}); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatal(err)
	}
}

func TestScheduleSkipConcurrencyAndQueue(t *testing.T) {
	e := newSchedEnv(t, true, nil)
	ctx := context.Background()
	a := e.create(t, cat.ScheduleInput{Name: "A", Frequency: "daily", Time: "09:00"})
	b := e.create(t, cat.ScheduleInput{Name: "B", Frequency: "daily", Time: "09:00"})
	c := e.create(t, cat.ScheduleInput{Name: "C", Frequency: "daily", Time: "09:00"})
	e.clock.Set(time.Date(2026, 10, 14, 9, 0, 10, 0, cst))
	e.svc.SchedulerTick(ctx)
	waitRuns(t, e.svc, a.ID, lastStatus(cat.RunRunning))
	waitRuns(t, e.svc, b.ID, lastStatus(cat.RunRunning))
	waitRuns(t, e.svc, c.ID, lastStatus(cat.RunQueued)) // 第 3 个排队（按 createdAt）
	// A 还没结束：立即运行 → skipped；C 排队中再立即运行 → skipped。
	r, err := e.svc.RunNowCatSchedule(ctx, cat.ScheduleIDRequest{ID: a.ID})
	if err != nil || r.Status != cat.RunSkipped || !r.Manual {
		t.Fatalf("%+v %v", r, err)
	}
	if r, _ := e.svc.RunNowCatSchedule(ctx, cat.ScheduleIDRequest{ID: c.ID}); r.Status != cat.RunSkipped {
		t.Fatalf("%+v", r)
	}
	// The scheduler still reserves two logical slots, but these schedules share
	// one worktree and therefore only one native writer may enter at a time.
	select {
	case <-e.gate.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first native turn did not start")
	}
	select {
	case <-e.gate.entered:
		t.Fatal("two scheduled writers entered the shared worktree")
	case <-time.After(150 * time.Millisecond):
	}
	if n := e.gate.running.Load(); n != 1 {
		t.Fatalf("native writers = %d, want 1", n)
	}
	close(e.gate.release)
	waitRuns(t, e.svc, c.ID, func(r []cat.ScheduleRun) bool {
		for _, x := range r {
			if x.Status == cat.RunSucceeded {
				return true
			}
		}
		return false
	})
}

func TestScheduleStoppedAndTimeout(t *testing.T) {
	e := newSchedEnv(t, true, func(c *cat.Config) { c.ScheduleRunTimeout = 300 * time.Millisecond })
	ctx := context.Background()
	sc := e.create(t, cat.ScheduleInput{Frequency: "daily", Time: "09:00"})
	r, err := e.svc.RunNowCatSchedule(ctx, cat.ScheduleIDRequest{ID: sc.ID})
	if err != nil {
		t.Fatal(err)
	}
	runs := waitRuns(t, e.svc, sc.ID, func(r []cat.ScheduleRun) bool {
		return len(r) > 0 && r[0].ConversationID != "" && r[0].Status == cat.RunRunning
	})
	if runs[0].ID != r.ID {
		t.Fatal("id")
	}
	// 用户在对话里按停止 → stopped，不通知。
	deadline := time.Now().Add(2 * time.Second)
	for e.gate.running.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	_ = e.svc.CancelCatTurn(ctx, catagent.CancelCatTurnRequest{ConvID: runs[0].ConversationID})
	waitRuns(t, e.svc, sc.ID, lastStatus(cat.RunStopped))
	// 超时 → failed + 说明，发失败通知。
	if _, err := e.svc.RunNowCatSchedule(ctx, cat.ScheduleIDRequest{ID: sc.ID}); err != nil {
		t.Fatal(err)
	}
	runs = waitRuns(t, e.svc, sc.ID, lastStatus(cat.RunFailed))
	if runs[0].Error == nil || runs[0].Error.Message != cat.MsgScheduleTimeout {
		t.Fatalf("%+v", runs[0])
	}
	deadline = time.Now().Add(2 * time.Second)
	for len(e.notes.all()) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	ns := e.notes.all()
	if len(ns) != 1 || ns[0].Title != "定时任务运行失败：每日检查" {
		t.Fatalf("%+v", ns)
	}
}

func TestScheduleNotifyRespectsSettingAndForeground(t *testing.T) {
	fg := atomic.Bool{}
	foregroundChecked := make(chan bool, 2)
	e := newSchedEnv(t, false, func(c *cat.Config) {
		c.Foreground = func() bool {
			value := fg.Load()
			foregroundChecked <- value
			return value
		}
	})
	ctx := context.Background()
	sc := e.create(t, cat.ScheduleInput{Frequency: "daily", Time: "09:00"})
	fg.Store(true)
	_, _ = e.svc.RunNowCatSchedule(ctx, cat.ScheduleIDRequest{ID: sc.ID})
	waitRuns(t, e.svc, sc.ID, lastStatus(cat.RunSucceeded))
	// A durable terminal status precedes notifyRun. Do not change foreground
	// state until that run has actually made its notification decision.
	select {
	case foreground := <-foregroundChecked:
		if !foreground {
			t.Fatal("first run did not observe the foreground window")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first notification decision did not finish")
	}
	fg.Store(false)
	off := false
	_, _ = e.svc.SaveCatSettings(ctx, cat.Settings{NotifyOnFinish: &off})
	_, _ = e.svc.RunNowCatSchedule(ctx, cat.ScheduleIDRequest{ID: sc.ID})
	waitRuns(t, e.svc, sc.ID, func(r []cat.ScheduleRun) bool { return len(r) == 2 && r[0].Status == cat.RunSucceeded })
	time.Sleep(50 * time.Millisecond)
	if n := len(e.notes.all()); n != 0 {
		t.Fatalf("notified %d", n)
	}
}

func TestScheduleProjectMissingDeletedAndClear(t *testing.T) {
	e := newSchedEnv(t, false, nil)
	ctx := context.Background()
	sc := e.create(t, cat.ScheduleInput{Frequency: "daily", Time: "09:00"})
	_, _ = e.svc.RunNowCatSchedule(ctx, cat.ScheduleIDRequest{ID: sc.ID})
	runs := waitRuns(t, e.svc, sc.ID, lastStatus(cat.RunSucceeded))
	convID := runs[0].ConversationID
	// 清空全部对话：任务和记录还在。
	if err := e.svc.ClearAllCatConversations(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.GetCatConversation(ctx, convID); !apperr.Is(err, apperr.NotFound) {
		t.Fatal(err)
	}
	if runs2, _ := e.svc.ListCatScheduleRuns(ctx, cat.ListScheduleRunsRequest{ScheduleID: sc.ID}); len(runs2) != 1 || runs2[0].ConversationID != convID {
		t.Fatalf("%+v", runs2)
	}
	// 项目文件夹不见了：失败，不建对话。
	_ = os.Rename(e.dir, e.dir+"-moved")
	_, _ = e.svc.RunNowCatSchedule(ctx, cat.ScheduleIDRequest{ID: sc.ID})
	runs = waitRuns(t, e.svc, sc.ID, lastStatus(cat.RunFailed))
	if runs[0].Error == nil || runs[0].Error.Code != apperr.CatProjectMissing || runs[0].ConversationID != "" {
		t.Fatalf("%+v", runs[0])
	}
	_ = os.Rename(e.dir+"-moved", e.dir)
	// 删项目：任务暂停并标 projectDeleted；不能直接恢复。
	if err := e.svc.DeleteCatProject(ctx, cat.DeleteProjectRequest{ID: e.proj.ID}); err != nil {
		t.Fatal(err)
	}
	list, _ := e.svc.ListCatSchedules(ctx)
	if len(list) != 1 || list[0].Enabled || !list[0].ProjectDeleted || list[0].NextRunAt != 0 {
		t.Fatalf("%+v", list)
	}
	if _, err := e.svc.ResumeCatSchedule(ctx, cat.ScheduleIDRequest{ID: sc.ID}); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatal(err)
	}
	// 删任务：记录一起删。
	if err := e.svc.DeleteCatSchedule(ctx, cat.ScheduleIDRequest{ID: sc.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.ListCatScheduleRuns(ctx, cat.ListScheduleRunsRequest{ScheduleID: sc.ID}); !apperr.Is(err, apperr.NotFound) {
		t.Fatal(err)
	}
	var n int
	_ = e.st.DB().QueryRow(`SELECT COUNT(*) FROM cat_schedule_runs`).Scan(&n)
	if n != 0 {
		t.Fatalf("runs left %d", n)
	}
}

func TestScheduleRunsKeep100(t *testing.T) {
	e := newSchedEnv(t, false, nil)
	ctx := context.Background()
	sc := e.create(t, cat.ScheduleInput{Frequency: "daily", Time: "09:00"})
	// 105 天每天都错过：每次检查记一条，只留最近 100 条。
	day := time.Date(2026, 10, 14, 12, 0, 0, 0, cst)
	for i := 0; i < 105; i++ {
		e.clock.Set(day.AddDate(0, 0, i))
		e.svc.SchedulerTick(ctx)
	}
	runs, _ := e.svc.ListCatScheduleRuns(ctx, cat.ListScheduleRunsRequest{ScheduleID: sc.ID, Limit: 500})
	var n int
	_ = e.st.DB().QueryRow(`SELECT COUNT(*) FROM cat_schedule_runs WHERE schedule_id=?`, sc.ID).Scan(&n)
	if len(runs) != 100 || n != 100 || runs[0].ScheduledFor != time.Date(2026, 10, 14+104, 9, 0, 0, 0, cst).UnixMilli() {
		t.Fatalf("len %d n %d first %v", len(runs), n, time.UnixMilli(runs[0].ScheduledFor))
	}
}

func TestSchedulerRecoversQueuedAndRunning(t *testing.T) {
	e := newSchedEnv(t, false, nil)
	ctx := context.Background()
	sc := e.create(t, cat.ScheduleInput{Frequency: "daily", Time: "09:00"})
	_ = e.st.UpsertCatScheduleRun(ctx, store.CatScheduleRunRow{ID: "q", ScheduleID: sc.ID, Status: "queued", ScheduledFor: 1})
	_ = e.st.UpsertCatScheduleRun(ctx, store.CatScheduleRunRow{ID: "r", ScheduleID: sc.ID, Status: "running", ScheduledFor: 2})
	e.svc.StartScheduler(ctx)
	runs, _ := e.svc.ListCatScheduleRuns(ctx, cat.ListScheduleRunsRequest{ScheduleID: sc.ID})
	got := map[string]cat.ScheduleRun{}
	for _, r := range runs {
		got[r.ID] = r
	}
	if got["q"].Status != cat.RunMissed || got["q"].MissedReason != cat.MissedClosed || got["r"].Status != cat.RunFailed || got["r"].Error == nil {
		t.Fatalf("%+v", got)
	}
}

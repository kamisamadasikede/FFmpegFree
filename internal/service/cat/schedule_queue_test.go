package cat_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/service/cat"
	"FFmpegFree/internal/store"
)

type evLog struct {
	mu  sync.Mutex
	evs []cat.ScheduleEvent
}

func (l *evLog) emit(name string, p any) {
	if ev, ok := p.(cat.ScheduleEvent); ok && name == cat.EventSchedule {
		l.mu.Lock()
		l.evs = append(l.evs, ev)
		l.mu.Unlock()
	}
}

// lastPos 某条运行最近一次推送里的状态和排队位置（-1 = 没有）。
func (l *evLog) last(runID string) (string, int, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i := len(l.evs) - 1; i >= 0; i-- {
		if r := l.evs[i].Run; r != nil && r.ID == runID {
			p := -1
			if r.QueuePosition != nil {
				p = *r.QueuePosition
			}
			return r.Status, p, true
		}
	}
	return "", -1, false
}

func pos(r cat.ScheduleRun) int {
	if r.QueuePosition == nil {
		return -1
	}
	return *r.QueuePosition
}

// v0.35.4 queuePosition + v0.35.3 排队在应用开着时一直等。
func TestScheduleQueuePositionAndWaitsWhileOpen(t *testing.T) {
	log := &evLog{}
	e := newSchedEnv(t, true, func(c *cat.Config) { c.Emit = log.emit })
	ctx := context.Background()
	ids := map[string]string{}
	for _, n := range []string{"A", "B", "C", "D"} {
		ids[n] = e.create(t, cat.ScheduleInput{Name: n, Frequency: "daily", Time: "09:00"}).ID
	}
	e.clock.Set(time.Date(2026, 10, 14, 9, 0, 10, 0, cst))
	e.svc.SchedulerTick(ctx)
	a := waitRuns(t, e.svc, ids["A"], func(r []cat.ScheduleRun) bool {
		return len(r) > 0 && r[0].Status == cat.RunRunning && r[0].ConversationID != ""
	})[0]
	b := waitRuns(t, e.svc, ids["B"], lastStatus(cat.RunRunning))[0]
	c := waitRuns(t, e.svc, ids["C"], lastStatus(cat.RunQueued))[0]
	d := waitRuns(t, e.svc, ids["D"], lastStatus(cat.RunQueued))[0]
	if pos(a) != -1 || pos(b) != -1 || pos(c) != 0 || pos(d) != 1 {
		t.Fatalf("positions a=%d b=%d c=%d d=%d", pos(a), pos(b), pos(c), pos(d))
	}
	// 列表的 lastRun 也带。
	list, _ := e.svc.ListCatSchedules(ctx)
	for _, sc := range list {
		want := map[string]int{"A": -1, "B": -1, "C": 0, "D": 1}[sc.Name]
		if sc.LastRun == nil || pos(*sc.LastRun) != want {
			t.Fatalf("%s lastRun %+v", sc.Name, sc.LastRun)
		}
	}
	// cat:schedule 推送里也带。
	if st, p, _ := log.last(d.ID); st != cat.RunQueued || p != 1 {
		t.Fatalf("event d %s %d", st, p)
	}
	// 不落库。
	var n int
	_ = e.st.DB().QueryRow(`SELECT COUNT(*) FROM pragma_table_info('cat_schedule_runs') WHERE name LIKE '%queue%'`).Scan(&n)
	if n != 0 {
		t.Fatal("queue position column in db")
	}
	// 应用一直开着，时钟走过 3 小时（含一次大跳）：排队的不变成已错过。
	e.clock.Set(time.Date(2026, 10, 14, 12, 0, 0, 0, cst))
	e.svc.SchedulerTick(ctx)
	if r := waitRuns(t, e.svc, ids["D"], lastStatus(cat.RunQueued))[0]; r.ID != d.ID || pos(r) != 1 {
		t.Fatalf("%+v", r)
	}
	// A 停掉 → C 开始，D 前移到 0，并重新推送。
	deadline := time.Now().Add(2 * time.Second)
	for e.gate.running.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	_ = e.svc.CancelCatTurn(ctx, catagent.CancelCatTurnRequest{ConvID: a.ConversationID})
	waitRuns(t, e.svc, ids["C"], lastStatus(cat.RunRunning))
	if r := waitRuns(t, e.svc, ids["D"], lastStatus(cat.RunQueued))[0]; pos(r) != 0 {
		t.Fatalf("d after %+v", r)
	}
	deadline = time.Now().Add(2 * time.Second)
	for {
		if st, p, _ := log.last(d.ID); st == cat.RunQueued && p == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("d not re-pushed with position 0")
		}
		time.Sleep(5 * time.Millisecond)
	}
	for {
		if st, p, _ := log.last(c.ID); st == cat.RunRunning && p == -1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("c last event not running without position")
		}
		time.Sleep(5 * time.Millisecond)
	}
	// 手动停止不发通知。
	time.Sleep(30 * time.Millisecond)
	if ns := e.notes.all(); len(ns) != 0 {
		t.Fatalf("notified %+v", ns)
	}
}

// 退出时还在跑的运行：下次启动记失败、说明按产品定稿、不发通知（v0.35.5 ③）。
func TestSchedulerInterruptedRunMessageNoNotify(t *testing.T) {
	e := newSchedEnv(t, false, nil)
	ctx := context.Background()
	sc := e.create(t, cat.ScheduleInput{Frequency: "daily", Time: "09:00"})
	_ = e.st.UpsertCatScheduleRun(ctx, store.CatScheduleRunRow{ID: "r", ScheduleID: sc.ID, Status: "running", ScheduledFor: 2})
	e.svc.StartScheduler(ctx)
	runs, _ := e.svc.ListCatScheduleRuns(ctx, cat.ListScheduleRunsRequest{ScheduleID: sc.ID})
	if len(runs) != 1 || runs[0].Status != cat.RunFailed || runs[0].Error == nil || runs[0].Error.Message != "FFmpegFree 关了，这次运行没跑完。" {
		t.Fatalf("%+v", runs)
	}
	time.Sleep(30 * time.Millisecond)
	if ns := e.notes.all(); len(ns) != 0 {
		t.Fatalf("notified %+v", ns)
	}
}

func TestBuildNotificationTitles(t *testing.T) {
	s := time.Unix(0, 0)
	ok := cat.BuildNotification("每日检查", "演示", cat.ScheduleRun{Status: cat.RunSucceeded, ConversationID: "c"}, s, s.Add(65*time.Second))
	bad := cat.BuildNotification("每日检查", "", cat.ScheduleRun{Status: cat.RunFailed}, s, s)
	if ok.Title != "定时任务已完成：每日检查" || ok.Body != "演示 · 用时 1 分 5 秒。点这里打开对话。" || bad.Title != "定时任务运行失败：每日检查" {
		t.Fatalf("%+v %+v", ok, bad)
	}
}

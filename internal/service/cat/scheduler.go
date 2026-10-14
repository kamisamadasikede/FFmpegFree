package cat

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/i18n"
	"FFmpegFree/internal/id"
	"FFmpegFree/internal/store"
)

// 调度参数（契约 6.19.13.3 / v0.35.1 / v0.35.3）。
const (
	DefaultScheduleTick       = 30 * time.Second
	ScheduleGrace             = 2 * time.Minute
	MaxConcurrentScheduleRuns = 2
	DefaultScheduleRunTimeout = 30 * time.Minute
)

// 运行失败的说明。
const (
	MsgScheduleTimeout     = "超过 30 分钟没跑完。"
	MsgScheduleInterrupted = "FFmpegFree 关了，这次运行没跑完。"
)

// Notification 是一条系统通知。ConvID 用于点通知后打开对话。
type Notification struct {
	ID         string
	ScheduleID string
	Title      string
	Body       string
	ConvID     string
}

// queuedRun 是排队中的一次（内存队列；落库 status=queued）。
type queuedRun struct {
	run       ScheduleRun
	createdAt int64 // 任务的 createdAt，同一触发时间再按它排
}

// scheduler 是应用内调度器状态。
type scheduler struct {
	mu       sync.Mutex
	started  bool
	closing  bool
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	active   map[string]string // scheduleId → 进行中（排队或运行）的 runId
	queue    []*queuedRun
	running  int
	lastTick time.Time
	tickMu   sync.Mutex // 一次只跑一个 tick
	emitMu   sync.Mutex // 运行记录的 cat:schedule 按顺序推，排队位置不会被旧快照盖掉
}

// StartScheduler 启动调度器（应用启动后调一次）：先收尾上次进程留下的记录，立刻检查一次，然后每 30 秒一次。
func (s *Service) StartScheduler(ctx context.Context) {
	if s.cfg.Store == nil {
		return
	}
	s.sched.mu.Lock()
	if s.sched.started {
		s.sched.mu.Unlock()
		return
	}
	s.sched.started = true
	s.sched.active = map[string]string{}
	loopCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	s.sched.cancel = cancel
	s.sched.mu.Unlock()

	errJSON := runToRow(ScheduleRun{Error: apperr.New(apperr.CatReplyFailed, MsgScheduleInterrupted)}).ErrorJSON
	if err := s.cfg.Store.RecoverCatScheduleRuns(loopCtx, s.now().UnixMilli(), errJSON); err != nil {
		s.cfg.Logf("cat schedule recover: %v", err)
	}
	tick := s.cfg.ScheduleTick
	if tick <= 0 {
		tick = DefaultScheduleTick
	}
	s.SchedulerTick(loopCtx)
	s.sched.wg.Add(1)
	go func() {
		defer s.sched.wg.Done()
		t := time.NewTicker(tick)
		defer t.Stop()
		for {
			select {
			case <-loopCtx.Done():
				return
			case <-t.C:
				s.SchedulerTick(loopCtx)
			}
		}
	}()
}

// CloseScheduler 停调度器（应用退出时）。排队中的记 missed（closed）；运行中的留给下次启动收尾。
func (s *Service) CloseScheduler() {
	s.sched.mu.Lock()
	if !s.sched.started || s.sched.closing {
		s.sched.mu.Unlock()
		return
	}
	s.sched.closing = true
	q := s.sched.queue
	s.sched.queue = nil
	if s.sched.cancel != nil {
		s.sched.cancel()
	}
	s.sched.mu.Unlock()
	ctx := context.Background()
	for _, it := range q {
		r := it.run
		r.Status, r.MissedReason, r.FinishedAt = RunMissed, MissedClosed, s.now().UnixMilli()
		_ = s.cfg.Store.UpsertCatScheduleRun(ctx, runToRow(r))
	}
	// 运行中的一轮可能还要很久；最多等 3 秒，没结束的下次启动记失败。
	waited := make(chan struct{})
	go func() { s.sched.wg.Wait(); close(waited) }()
	select {
	case <-waited:
	case <-time.After(3 * time.Second):
	}
}

// SchedulerTick 检查一次到点的任务（测试可直接调，配合注入的时钟）。
func (s *Service) SchedulerTick(ctx context.Context) {
	s.sched.tickMu.Lock()
	defer s.sched.tickMu.Unlock()
	now := s.now()
	s.sched.mu.Lock()
	last := s.sched.lastTick
	s.sched.lastTick = now
	closing := s.sched.closing
	s.sched.mu.Unlock()
	if closing {
		return
	}
	// 第一次检查 = 刚打开应用：错过的是应用没开；之后两次检查之间隔太久（时钟一次跳过 > 2 分钟）= 电脑睡眠。
	reason := MissedClosed
	if !last.IsZero() {
		reason = MissedAsleep
	}
	rows, err := s.cfg.Store.ListCatSchedules(ctx)
	if err != nil {
		s.cfg.Logf("cat schedule list: %v", err)
		return
	}
	// 按计划时间、再按 createdAt 触发，排队顺序就是这个顺序。
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].NextRunAt != rows[j].NextRunAt {
			return rows[i].NextRunAt < rows[j].NextRunAt
		}
		if rows[i].CreatedAt != rows[j].CreatedAt {
			return rows[i].CreatedAt < rows[j].CreatedAt
		}
		return rows[i].ID < rows[j].ID
	})
	nowMs := now.UnixMilli()
	for _, x := range rows {
		if !x.Enabled || x.ProjectDeleted || x.NextRunAt <= 0 || x.NextRunAt > nowMs {
			continue
		}
		due := x.NextRunAt
		late := now.Sub(time.UnixMilli(due)) > ScheduleGrace
		// 下一次：按现在重算（错过多个时间点也只记一条）。
		x.NextRunAt = NextRunAfter(x, now, s.loc())
		if x.Frequency == FreqOnce {
			x.Enabled, x.NextRunAt = false, 0
		}
		x.UpdatedAt = nowMs
		if _, err := s.saveSchedule(ctx, x); err != nil {
			s.cfg.Logf("cat schedule advance %s: %v", x.ID, err)
			continue
		}
		if late {
			s.recordRun(ctx, ScheduleRun{ID: id.New(), ScheduleID: x.ID, Status: RunMissed, ScheduledFor: due,
				FinishedAt: nowMs, MissedReason: reason})
			continue
		}
		s.trigger(ctx, x, due, false)
	}
}

// trigger 触发一次：上一次（排队或运行）还没结束 → skipped；否则排队，有空位就开始。
func (s *Service) trigger(ctx context.Context, x store.CatScheduleRow, scheduledFor int64, manual bool) ScheduleRun {
	run := ScheduleRun{ID: id.New(), ScheduleID: x.ID, ScheduledFor: scheduledFor, Manual: manual}
	s.sched.mu.Lock()
	if s.sched.active == nil {
		s.sched.active = map[string]string{}
	}
	if _, busy := s.sched.active[x.ID]; busy || s.sched.closing {
		s.sched.mu.Unlock()
		run.Status, run.FinishedAt = RunSkipped, s.now().UnixMilli()
		s.recordRun(ctx, run)
		return run
	}
	run.Status = RunQueued
	s.sched.active[x.ID] = run.ID
	s.sched.queue = append(s.sched.queue, &queuedRun{run: run, createdAt: x.CreatedAt})
	sort.SliceStable(s.sched.queue, func(i, j int) bool {
		a, b := s.sched.queue[i], s.sched.queue[j]
		if a.run.ScheduledFor != b.run.ScheduledFor {
			return a.run.ScheduledFor < b.run.ScheduledFor
		}
		if a.createdAt != b.createdAt {
			return a.createdAt < b.createdAt
		}
		return a.run.ScheduleID < b.run.ScheduleID
	})
	s.sched.mu.Unlock()
	s.recordRun(ctx, run)
	s.pump()
	s.emitQueued(run.ID)
	// 返回当前状态（可能已开始）。
	if cur, err := s.cfg.Store.ListCatScheduleRuns(ctx, x.ID, 1); err == nil && len(cur) == 1 && cur[0].ID == run.ID {
		return s.withQueuePosition(runFromRow(cur[0]))
	}
	return s.withQueuePosition(run)
}

// dropQueued 把这个任务排队中的一次拿出队列（删任务 / 删项目时）。记 stopped。
func (s *Service) dropQueued(scheduleID string) {
	s.sched.mu.Lock()
	var dropped *queuedRun
	keep := s.sched.queue[:0]
	for _, it := range s.sched.queue {
		if it.run.ScheduleID == scheduleID && dropped == nil {
			dropped = it
			continue
		}
		keep = append(keep, it)
	}
	s.sched.queue = keep
	if dropped != nil && s.sched.active[scheduleID] == dropped.run.ID {
		delete(s.sched.active, scheduleID)
	}
	s.sched.mu.Unlock()
	if dropped != nil {
		r := dropped.run
		r.Status, r.FinishedAt = RunStopped, s.now().UnixMilli()
		s.recordRun(context.Background(), r)
		s.emitQueued("")
	}
}

// pump 有空位就从队头开始运行（同时最多 2 个）。
func (s *Service) pump() {
	for {
		s.sched.mu.Lock()
		if s.sched.closing || s.sched.running >= MaxConcurrentScheduleRuns || len(s.sched.queue) == 0 {
			s.sched.mu.Unlock()
			return
		}
		it := s.sched.queue[0]
		s.sched.queue = s.sched.queue[1:]
		s.sched.running++
		s.sched.wg.Add(1)
		s.sched.mu.Unlock()
		go func(r ScheduleRun) {
			defer s.sched.wg.Done()
			s.execute(r)
		}(it.run)
	}
}

// release 一次运行结束：腾出名额并接着排队的。
func (s *Service) release(r ScheduleRun) {
	s.sched.mu.Lock()
	s.sched.running--
	if s.sched.active[r.ScheduleID] == r.ID {
		delete(s.sched.active, r.ScheduleID)
	}
	s.sched.mu.Unlock()
	s.pump()
	s.emitQueued("")
}

// withQueuePosition 给排队中的运行填上当前排队位置（v0.35.4）；不是排队中的清掉。
func (s *Service) withQueuePosition(r ScheduleRun) ScheduleRun {
	r.QueuePosition = nil
	if r.Status != RunQueued {
		return r
	}
	s.sched.mu.Lock()
	defer s.sched.mu.Unlock()
	for i, it := range s.sched.queue {
		if it.run.ID == r.ID {
			n := i
			r.QueuePosition = &n
			break
		}
	}
	return r
}

// emitQueued 队列变化后，把所有仍在排队的运行按新位置重新推一次 cat:schedule（v0.35.4）。skipID 是刚推过的那条。
func (s *Service) emitQueued(skipID string) {
	s.sched.emitMu.Lock()
	defer s.sched.emitMu.Unlock()
	s.sched.mu.Lock()
	if s.sched.closing {
		s.sched.mu.Unlock()
		return
	}
	snap := make([]ScheduleRun, 0, len(s.sched.queue))
	for i, it := range s.sched.queue {
		r := it.run
		n := i
		r.QueuePosition = &n
		snap = append(snap, r)
	}
	s.sched.mu.Unlock()
	ctx := context.Background()
	for i := range snap {
		r := snap[i]
		if r.ID == skipID && r.QueuePosition != nil && *r.QueuePosition == len(snap)-1 {
			continue // 刚入队、排在最后：recordRun 已推过同样的位置
		}
		ev := ScheduleEvent{ScheduleID: r.ScheduleID, Run: &r}
		if x, err := s.cfg.Store.GetCatSchedule(ctx, r.ScheduleID); err == nil {
			sc := s.scheduleFromRow(ctx, x)
			ev.Schedule = &sc
		}
		s.emit(EventSchedule, ev)
	}
}

// recordRun 落库、剪到 100 条、推 cat:schedule。
func (s *Service) recordRun(ctx context.Context, r ScheduleRun) {
	if err := s.cfg.Store.UpsertCatScheduleRun(ctx, runToRow(r)); err != nil {
		if !errors.Is(err, sql.ErrNoRows) { // 任务已删：不写
			s.cfg.Logf("cat schedule run write: %v", err)
		}
		return
	}
	if err := s.cfg.Store.PruneCatScheduleRuns(ctx, r.ScheduleID, MaxRunsPerSchedule); err != nil {
		s.cfg.Logf("cat schedule prune: %v", err)
	}
	s.sched.emitMu.Lock()
	defer s.sched.emitMu.Unlock()
	r = s.withQueuePosition(r)
	ev := ScheduleEvent{ScheduleID: r.ScheduleID, Run: &r}
	if x, err := s.cfg.Store.GetCatSchedule(ctx, r.ScheduleID); err == nil {
		sc := s.scheduleFromRow(ctx, x)
		ev.Schedule = &sc
	}
	s.emit(EventSchedule, ev)
}

// execute 跑一次：校验项目 → 新建普通对话 → 发指令 → 等这一轮结束（最长 30 分钟）→ 记结果 → 通知。
func (s *Service) execute(r ScheduleRun) {
	ctx := context.Background()
	released := false
	rel := func() { // 先腾出名额再写终态，界面看到结束后立刻「立即运行」不会被当成上一次没结束
		if !released {
			released = true
			s.release(r)
		}
	}
	defer rel()
	x, err := s.cfg.Store.GetCatSchedule(ctx, r.ScheduleID)
	if err != nil {
		return // 任务已删
	}
	start := s.now()
	r.Status, r.StartedAt = RunRunning, start.UnixMilli()
	s.recordRun(ctx, r)

	fail := func(e *apperr.AppError) {
		r.Status, r.Error, r.FinishedAt = RunFailed, e, s.now().UnixMilli()
		rel()
		s.recordRun(ctx, r)
		s.notifyRun(ctx, x, r, start)
	}
	row, perr := s.cfg.Store.GetCatProject(ctx, x.ProjectID)
	if perr != nil || s.checkProject(row).Missing {
		fail(apperr.From(errProjectMissing()))
		return
	}
	kind := strings.TrimSpace(x.AgentKind)
	if !catagent.Phase1Creatable(kind) {
		kind = catagent.KindCatBuild
	}
	if a, aerr := s.adapter(kind); aerr != nil || a.Status().State != catagent.StateReady {
		fail(apperr.New(apperr.CatNotReady, catagent.MsgNotReady))
		return
	}
	title := x.Name + " · " + time.UnixMilli(r.ScheduledFor).In(s.loc()).Format("01-02 15:04")
	conv, err := s.CreateCatConversation(ctx, CreateConversationRequest{AgentKind: kind, Title: title, ProjectID: x.ProjectID})
	if err != nil {
		fail(apperr.From(err))
		return
	}
	r.ConversationID = conv.ID
	s.recordRun(ctx, r)

	limit := s.cfg.ScheduleRunTimeout
	if limit <= 0 {
		limit = DefaultScheduleRunTimeout
	}
	type outcome struct {
		status string
		err    *apperr.AppError
	}
	done := make(chan outcome, 1)
	hook := &turnHook{
		timeout: limit + time.Minute, // 适配器自己的超时放宽，以这里的 30 分钟为准
		onFinish: func(status string, e *apperr.AppError) {
			select {
			case done <- outcome{status, e}:
			default:
			}
		},
	}
	res, err := s.sendMessage(ctx, SendMessageRequest{
		ConversationID: conv.ID, Content: x.Prompt, ModelID: x.Model, ThinkLevelID: x.Effort, PermissionMode: x.ApprovalMode,
	}, hook)
	if err != nil {
		fail(apperr.From(err))
		return
	}
	timer := time.NewTimer(limit)
	defer timer.Stop()
	var out outcome
	timedOut := false
	select {
	case out = <-done:
	case <-timer.C:
		timedOut = true
		s.cancelTurn(conv.ID, res.TurnID)
		out = <-done
	}
	s.sched.mu.Lock()
	closing := s.sched.closing
	s.sched.mu.Unlock()
	if closing {
		return // 应用在退出：留给下次启动收尾
	}
	r.FinishedAt = s.now().UnixMilli()
	switch {
	case timedOut:
		r.Status, r.Error = RunFailed, apperr.New(apperr.CatReplyFailed, MsgScheduleTimeout)
	case out.status == catagent.TurnCompleted:
		r.Status = RunSucceeded
	case out.status == catagent.TurnCancelled:
		r.Status = RunStopped
	default:
		r.Status, r.Error = RunFailed, out.err
		if r.Error == nil {
			r.Error = apperr.New(apperr.CatReplyFailed, catagent.MsgReplyFailed)
		}
	}
	rel()
	s.recordRun(ctx, r)
	s.notifyRun(ctx, x, r, start)
}

// notifyRun 只对成功 / 失败发系统通知；窗口在前台或设置里关了通知时不发（6.19.13.10-6、v0.35.3）。
func (s *Service) notifyRun(ctx context.Context, x store.CatScheduleRow, r ScheduleRun, start time.Time) {
	if s.cfg.Notify == nil || (r.Status != RunSucceeded && r.Status != RunFailed) {
		return
	}
	if s.cfg.Foreground != nil && s.cfg.Foreground() {
		return
	}
	if st, err := s.GetCatSettings(ctx); err == nil && !st.NotifyEnabled() {
		return
	}
	s.cfg.Notify(BuildNotification(x.Name, s.projectName(ctx, x.ProjectID), r, start, s.now()))
}

func (s *Service) projectName(ctx context.Context, pid string) string {
	if row, err := s.cfg.Store.GetCatProject(ctx, pid); err == nil {
		return row.Name
	}
	return ""
}

// BuildNotification 拼通知文案：标题「定时任务已完成：<名称>」/「定时任务运行失败：<名称>」，
// 正文「<项目> · 用时 X 分 X 秒。点这里打开对话。」（v0.35.1 / v0.35.3；v0.54 走 i18n）。
func BuildNotification(name, project string, r ScheduleRun, start, end time.Time) Notification {
	d := end.Sub(start)
	if d < 0 {
		d = 0
	}
	sec := int(d.Round(time.Second) / time.Second)
	minS := fmt.Sprintf("%d", sec/60)
	secS := fmt.Sprintf("%d", sec%60)
	params := map[string]string{"min": minS, "sec": secS}
	var body string
	if strings.TrimSpace(project) != "" {
		params["project"] = project
		body = i18n.T(i18n.KeyNotifyScheduleBodyProject, params)
	} else {
		body = i18n.T(i18n.KeyNotifyScheduleBody, params)
	}
	if r.ConversationID != "" {
		body += i18n.T(i18n.KeyNotifyScheduleOpenConv, nil)
	}
	titleKey := i18n.KeyNotifyScheduleDoneTitle
	if r.Status == RunFailed {
		titleKey = i18n.KeyNotifyScheduleFailedTitle
	}
	title := i18n.T(titleKey, map[string]string{"name": name})
	return Notification{ID: r.ID, ScheduleID: r.ScheduleID, Title: title, Body: body, ConvID: r.ConversationID}
}

// OpenFromNotification 用户点了通知：定时仍走 cat:schedule；普通 turn 走 cat:notify（契约 6.19.12.10）。
func (s *Service) OpenFromNotification(scheduleID, convID string) {
	if strings.TrimSpace(convID) == "" {
		return
	}
	if strings.TrimSpace(scheduleID) != "" {
		s.emit(EventSchedule, ScheduleEvent{ScheduleID: scheduleID, OpenConvID: convID})
		return
	}
	s.emit(EventNotify, NotifyEvent{OpenConvID: convID})
}

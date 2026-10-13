package cat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/id"
	"FFmpegFree/internal/store"
)

// EventSchedule 是定时任务事件名（契约 6.19.13.5）。
const EventSchedule = "cat:schedule"

// EventNotify 是点普通 turn 系统通知后打开对话（契约 6.19.12.10）。
const EventNotify = "cat:notify"

// 定时任务频率。
const (
	FreqOnce     = "once"
	FreqDaily    = "daily"
	FreqWeekdays = "weekdays"
	FreqWeekly   = "weekly"
)

// 运行状态（契约 6.19.13.1 + v0.35.1 queued）。
const (
	RunQueued    = "queued"
	RunRunning   = "running"
	RunSucceeded = "succeeded"
	RunFailed    = "failed"
	RunMissed    = "missed"
	RunSkipped   = "skipped"
	RunStopped   = "stopped"
)

// 已错过的原因（v0.35.1 6.19.13.10-3）。
const (
	MissedClosed = "closed"
	MissedAsleep = "asleep"
)

// 校验上限。
const (
	MaxScheduleNameRunes   = 40
	MaxSchedulePromptRunes = 4000
	MaxRunsPerSchedule     = 100
	DefaultRunsLimit       = 50
)

// ScheduleRun 是 CatScheduleRun。
type ScheduleRun struct {
	ID             string           `json:"id"`
	ScheduleID     string           `json:"scheduleId"`
	ConversationID string           `json:"conversationId,omitempty"`
	Status         string           `json:"status"`
	Error          *apperr.AppError `json:"error,omitempty"`
	ScheduledFor   int64            `json:"scheduledFor"`
	StartedAt      int64            `json:"startedAt,omitempty"`
	FinishedAt     int64            `json:"finishedAt,omitempty"`
	Manual         bool             `json:"manual"`
	MissedReason   string           `json:"missedReason,omitempty"`
	// QueuePosition 仅 status=queued 时有值：排在它前面的运行数，0 = 下一个就轮到它；不落库（v0.35.4）。
	QueuePosition *int `json:"queuePosition,omitempty"`
}

// ScheduleInput 是 CatScheduleInput（新建 / 编辑共用）。
type ScheduleInput struct {
	Name         string `json:"name"`
	Prompt       string `json:"prompt"`
	ProjectID    string `json:"projectId"`
	Frequency    string `json:"frequency"`
	Time         string `json:"time"`
	Weekdays     []int  `json:"weekdays"`
	RunAt        int64  `json:"runAt"`
	Model        string `json:"model"`
	Effort       string `json:"effort"`
	ApprovalMode string `json:"approvalMode"`
	AgentKind    string `json:"agentKind"` // v0.40：cat_build|claude_cli|codex_cli；空=默认
}

// Schedule 是 CatSchedule。
type Schedule struct {
	ID             string       `json:"id"`
	Name           string       `json:"name"`
	Prompt         string       `json:"prompt"`
	ProjectID      string       `json:"projectId"`
	Frequency      string       `json:"frequency"`
	Time           string       `json:"time"`
	Weekdays       []int        `json:"weekdays"`
	RunAt          int64        `json:"runAt"`
	Timezone       string       `json:"timezone"`
	Model          string       `json:"model"`
	Effort         string       `json:"effort"`
	ApprovalMode   string       `json:"approvalMode"`
	AgentKind      string       `json:"agentKind"` // v0.40
	Enabled        bool         `json:"enabled"`
	ProjectDeleted bool         `json:"projectDeleted"`
	NextRunAt      int64        `json:"nextRunAt,omitempty"`
	LastRun        *ScheduleRun `json:"lastRun,omitempty"`
	CreatedAt      int64        `json:"createdAt"`
	UpdatedAt      int64        `json:"updatedAt"`
}

// UpdateScheduleRequest 是 UpdateCatSchedule 的参数：{ id, ...CatScheduleInput }。
type UpdateScheduleRequest struct {
	ID string `json:"id"`
	ScheduleInput
}

// ScheduleIDRequest 是只带 id 的请求。
type ScheduleIDRequest struct {
	ID string `json:"id"`
}

// ListScheduleRunsRequest 是 ListCatScheduleRuns 的参数。
type ListScheduleRunsRequest struct {
	ScheduleID string `json:"scheduleId"`
	Limit      int    `json:"limit"`
}

// ScheduleEvent 是 cat:schedule 的载荷。OpenConvID 只在用户点了系统通知时带（要求界面打开这条对话）。
type ScheduleEvent struct {
	ScheduleID string       `json:"scheduleId"`
	Schedule   *Schedule    `json:"schedule,omitempty"`
	Run        *ScheduleRun `json:"run,omitempty"`
	Deleted    bool         `json:"deleted,omitempty"`
	OpenConvID string       `json:"openConvId,omitempty"`
}

// NotifyEvent 是 cat:notify 的载荷（点普通 turn 系统通知）。
type NotifyEvent struct {
	OpenConvID string `json:"openConvId"`
}

func (s *Service) now() time.Time {
	if s.cfg.Now != nil {
		return s.cfg.Now()
	}
	return time.Now()
}

func (s *Service) loc() *time.Location {
	if s.cfg.Location != nil {
		return s.cfg.Location
	}
	return time.Local
}

func errScheduleNotFound() error { return apperr.New(apperr.NotFound, "找不到这个定时任务") }

func invalid(msg, reason string) error {
	return apperr.New(apperr.InvalidArgument, msg).WithDetail("reason=" + reason)
}

// parseHHMM 解析 "HH:MM"。
func parseHHMM(v string) (int, int, bool) {
	h, m, ok := strings.Cut(strings.TrimSpace(v), ":")
	if !ok || len(h) != 2 || len(m) != 2 {
		return 0, 0, false
	}
	hh, e1 := strconv.Atoi(h)
	mm, e2 := strconv.Atoi(m)
	if e1 != nil || e2 != nil || hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return 0, 0, false
	}
	return hh, mm, true
}

// normalizeInput 校验并规整输入（契约 6.19.13.2）。项目存在性由调用方查。
func (s *Service) normalizeInput(in ScheduleInput) (ScheduleInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Prompt = strings.TrimSpace(in.Prompt)
	in.ProjectID = strings.TrimSpace(in.ProjectID)
	in.Frequency = strings.TrimSpace(in.Frequency)
	in.Time = strings.TrimSpace(in.Time)
	in.Model = strings.TrimSpace(in.Model)
	in.Effort = strings.TrimSpace(in.Effort)
	in.ApprovalMode = strings.TrimSpace(in.ApprovalMode)
	in.AgentKind = strings.TrimSpace(in.AgentKind)
	if in.AgentKind != "" && !catagent.Phase1Creatable(in.AgentKind) {
		return in, invalid("请选择服务商。", "agent_kind")
	}
	switch n := utf8.RuneCountInString(in.Name); {
	case n == 0:
		return in, invalid("请填写名称。", "name")
	case n > MaxScheduleNameRunes:
		return in, invalid("最多 40 个字。", "name")
	}
	switch n := utf8.RuneCountInString(in.Prompt); {
	case n == 0:
		return in, invalid("请填写指令。", "prompt")
	case n > MaxSchedulePromptRunes:
		return in, invalid("最多 4000 字。", "prompt")
	}
	if in.ProjectID == "" {
		return in, invalid("请选择项目。", "project")
	}
	if !ValidEffort(in.Effort) {
		return in, invalid("思考强度不对", "effort")
	}
	if in.ApprovalMode == "" {
		in.ApprovalMode = "plan"
	}
	if !ValidApprovalMode(in.ApprovalMode) {
		return in, invalid("批准方式不对", "approval_mode")
	}
	switch in.Frequency {
	case FreqOnce:
		if in.RunAt <= s.now().UnixMilli() {
			return in, invalid("这个时间已经过了。", "run_at")
		}
		in.Time, in.Weekdays = "", []int{}
	case FreqDaily, FreqWeekdays, FreqWeekly:
		if _, _, ok := parseHHMM(in.Time); !ok {
			return in, invalid("时间不对", "time")
		}
		in.RunAt = 0
		if in.Frequency != FreqWeekly {
			in.Weekdays = []int{}
			break
		}
		seen := map[int]bool{}
		days := []int{}
		for _, d := range in.Weekdays {
			if d < 1 || d > 7 {
				return in, invalid("星期不对", "weekdays")
			}
			if !seen[d] {
				seen[d] = true
				days = append(days, d)
			}
		}
		if len(days) == 0 {
			return in, invalid("请至少选一天。", "weekdays")
		}
		sort.Ints(days)
		in.Weekdays = days
	default:
		return in, invalid("频率不对", "frequency")
	}
	return in, nil
}

// isoWeekday 把 time.Weekday 转成 1~7（周一=1）。
func isoWeekday(d time.Weekday) int {
	if d == time.Sunday {
		return 7
	}
	return int(d)
}

// NextRunAfter 算 after 之后（严格大于）的下一个计划时刻，毫秒；没有返回 0。
// 本机时区；夏令时跳过的时刻由 time.Date 顺延到下一个有效时刻。
func NextRunAfter(x store.CatScheduleRow, after time.Time, loc *time.Location) int64 {
	if x.Frequency == FreqOnce {
		if x.RunAt > after.UnixMilli() {
			return x.RunAt
		}
		return 0
	}
	hh, mm, ok := parseHHMM(x.Time)
	if !ok {
		return 0
	}
	a := after.In(loc)
	days := map[int]bool{}
	for _, d := range x.Weekdays {
		days[d] = true
	}
	for i := 0; i <= 8; i++ {
		day := time.Date(a.Year(), a.Month(), a.Day()+i, 0, 0, 0, 0, loc)
		wd := isoWeekday(day.Weekday())
		switch x.Frequency {
		case FreqWeekdays:
			if wd > 5 {
				continue
			}
		case FreqWeekly:
			if !days[wd] {
				continue
			}
		}
		t := time.Date(day.Year(), day.Month(), day.Day(), hh, mm, 0, 0, loc)
		if t.After(after) {
			return t.UnixMilli()
		}
	}
	return 0
}

// LocalTimezoneName 是本机时区名（只用于显示）：TZ 环境变量、/etc/localtime 链接，最后退回 UTC+08:00 这样的偏移。
func LocalTimezoneName(loc *time.Location) string {
	if loc != nil && loc != time.Local && loc.String() != "Local" {
		return loc.String()
	}
	if tz := strings.TrimPrefix(os.Getenv("TZ"), ":"); tz != "" && !strings.HasPrefix(tz, "/") {
		return tz
	}
	if p, err := filepath.EvalSymlinks("/etc/localtime"); err == nil {
		if i := strings.Index(p, "zoneinfo/"); i >= 0 {
			return p[i+len("zoneinfo/"):]
		}
	}
	if n := time.Local.String(); n != "" && n != "Local" {
		return n
	}
	_, off := time.Now().Zone()
	sign := "+"
	if off < 0 {
		sign, off = "-", -off
	}
	return "UTC" + sign + pad2(off/3600) + ":" + pad2(off%3600/60)
}

func pad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

func runFromRow(r store.CatScheduleRunRow) ScheduleRun {
	out := ScheduleRun{ID: r.ID, ScheduleID: r.ScheduleID, ConversationID: r.ConversationID, Status: r.Status,
		ScheduledFor: r.ScheduledFor, StartedAt: r.StartedAt, FinishedAt: r.FinishedAt, Manual: r.Manual, MissedReason: r.MissedReason}
	if r.ErrorJSON != "" {
		var e apperr.AppError
		if json.Unmarshal([]byte(r.ErrorJSON), &e) == nil && e.Code != "" {
			out.Error = &e
		}
	}
	return out
}

func runToRow(r ScheduleRun) store.CatScheduleRunRow {
	row := store.CatScheduleRunRow{ID: r.ID, ScheduleID: r.ScheduleID, ConversationID: r.ConversationID, Status: r.Status,
		ScheduledFor: r.ScheduledFor, StartedAt: r.StartedAt, FinishedAt: r.FinishedAt, Manual: r.Manual, MissedReason: r.MissedReason}
	if r.Error != nil {
		b, _ := json.Marshal(r.Error)
		row.ErrorJSON = string(b)
	}
	return row
}

func (s *Service) scheduleFromRow(ctx context.Context, x store.CatScheduleRow) Schedule {
	kind := x.AgentKind
	if kind == "" {
		kind = catagent.KindCatBuild
	}
	out := Schedule{ID: x.ID, Name: x.Name, Prompt: x.Prompt, ProjectID: x.ProjectID, Frequency: x.Frequency, Time: x.Time,
		Weekdays: x.Weekdays, RunAt: x.RunAt, Timezone: LocalTimezoneName(s.cfg.Location), Model: x.Model, Effort: x.Effort,
		ApprovalMode: x.ApprovalMode, AgentKind: kind, Enabled: x.Enabled, ProjectDeleted: x.ProjectDeleted, NextRunAt: x.NextRunAt,
		CreatedAt: x.CreatedAt, UpdatedAt: x.UpdatedAt}
	if out.Weekdays == nil {
		out.Weekdays = []int{}
	}
	if r, err := s.cfg.Store.LatestCatScheduleRun(ctx, x.ID); err == nil {
		lr := s.withQueuePosition(runFromRow(r))
		out.LastRun = &lr
	}
	return out
}

func (s *Service) requireStore() error {
	if s.cfg.Store == nil {
		return apperr.New(apperr.Internal, "本地存储尚未初始化")
	}
	return nil
}

func (s *Service) getScheduleRow(ctx context.Context, rawID string) (store.CatScheduleRow, error) {
	if err := s.requireStore(); err != nil {
		return store.CatScheduleRow{}, err
	}
	sid := strings.TrimSpace(rawID)
	if sid == "" {
		return store.CatScheduleRow{}, invalid("定时任务编号不能为空", "id")
	}
	x, err := s.cfg.Store.GetCatSchedule(ctx, sid)
	if errors.Is(err, sql.ErrNoRows) {
		return x, errScheduleNotFound()
	}
	if err != nil {
		return x, apperr.Wrap(apperr.IOError, "读取定时任务失败", err)
	}
	return x, nil
}

func (s *Service) requireProject(ctx context.Context, pid string) error {
	_, err := s.cfg.Store.GetCatProject(ctx, pid)
	if errors.Is(err, sql.ErrNoRows) {
		return errProjectNotFound()
	}
	if err != nil {
		return apperr.Wrap(apperr.IOError, "读取项目失败", err)
	}
	return nil
}

func (s *Service) saveSchedule(ctx context.Context, x store.CatScheduleRow) (Schedule, error) {
	if err := s.cfg.Store.UpsertCatSchedule(ctx, x); err != nil {
		return Schedule{}, apperr.Wrap(apperr.IOError, "保存失败，请重试。", err)
	}
	out := s.scheduleFromRow(ctx, x)
	s.emit(EventSchedule, ScheduleEvent{ScheduleID: x.ID, Schedule: &out})
	return out, nil
}

// ListCatSchedules 按 createdAt 倒序、再按 id 倒序。
func (s *Service) ListCatSchedules(ctx context.Context) ([]Schedule, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	rows, err := s.cfg.Store.ListCatSchedules(ctx)
	if err != nil {
		return nil, apperr.Wrap(apperr.IOError, "读取定时任务失败", err)
	}
	out := make([]Schedule, 0, len(rows))
	for _, x := range rows {
		out = append(out, s.scheduleFromRow(ctx, x))
	}
	return out, nil
}

// CreateCatSchedule 新建（默认启用）。
func (s *Service) CreateCatSchedule(ctx context.Context, req ScheduleInput) (Schedule, error) {
	if err := s.requireStore(); err != nil {
		return Schedule{}, err
	}
	in, err := s.normalizeInput(req)
	if err != nil {
		return Schedule{}, err
	}
	in.AgentKind = s.resolveScheduleAgentKind(ctx, in.AgentKind)
	if err := s.requireProject(ctx, in.ProjectID); err != nil {
		return Schedule{}, err
	}
	now := s.now()
	x := store.CatScheduleRow{ID: id.New(), CreatedAt: now.UnixMilli(), UpdatedAt: now.UnixMilli(), Enabled: true}
	applyInput(&x, in)
	x.NextRunAt = NextRunAfter(x, now, s.loc())
	return s.saveSchedule(ctx, x)
}

func applyInput(x *store.CatScheduleRow, in ScheduleInput) {
	x.Name, x.Prompt, x.ProjectID, x.Frequency, x.Time = in.Name, in.Prompt, in.ProjectID, in.Frequency, in.Time
	x.Weekdays, x.RunAt, x.Model, x.Effort, x.ApprovalMode = in.Weekdays, in.RunAt, in.Model, in.Effort, in.ApprovalMode
	x.AgentKind = in.AgentKind
	if x.AgentKind == "" {
		x.AgentKind = catagent.KindCatBuild
	}
}

// resolveScheduleAgentKind：空 → CatSettings.defaultAgentKind 或 cat_build（契约 v0.40）。
func (s *Service) resolveScheduleAgentKind(ctx context.Context, raw string) string {
	if catagent.Phase1Creatable(raw) {
		return raw
	}
	if st, err := s.GetCatSettings(ctx); err == nil && catagent.Phase1Creatable(st.DefaultAgentKind) {
		return st.DefaultAgentKind
	}
	return catagent.KindCatBuild
}

// UpdateCatSchedule 整体改。换成有效项目会清掉 projectDeleted（但不自动恢复，需 Resume）；
// 「仅一次」改成新的未来时间后重新启用。
func (s *Service) UpdateCatSchedule(ctx context.Context, req UpdateScheduleRequest) (Schedule, error) {
	x, err := s.getScheduleRow(ctx, req.ID)
	if err != nil {
		return Schedule{}, err
	}
	in, err := s.normalizeInput(req.ScheduleInput)
	if err != nil {
		return Schedule{}, err
	}
	in.AgentKind = s.resolveScheduleAgentKind(ctx, in.AgentKind)
	if err := s.requireProject(ctx, in.ProjectID); err != nil {
		return Schedule{}, err
	}
	reenableOnce := in.Frequency == FreqOnce && (x.Frequency != FreqOnce || x.RunAt != in.RunAt) && !x.ProjectDeleted
	if x.ProjectDeleted && in.ProjectID != x.ProjectID {
		x.ProjectDeleted = false
	}
	applyInput(&x, in)
	if reenableOnce {
		x.Enabled = true
	}
	now := s.now()
	x.UpdatedAt = now.UnixMilli()
	x.NextRunAt = 0
	if x.Enabled && !x.ProjectDeleted {
		x.NextRunAt = NextRunAfter(x, now, s.loc())
	}
	return s.saveSchedule(ctx, x)
}

// DeleteCatSchedule 删任务和它的运行记录；已产生的对话保留。不存在 NOT_FOUND。
func (s *Service) DeleteCatSchedule(ctx context.Context, req ScheduleIDRequest) error {
	x, err := s.getScheduleRow(ctx, req.ID)
	if err != nil {
		return err
	}
	s.dropQueued(x.ID)
	if err := s.cfg.Store.DeleteCatSchedule(ctx, x.ID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return apperr.Wrap(apperr.IOError, "删除失败，请重试。", err)
	}
	s.emit(EventSchedule, ScheduleEvent{ScheduleID: x.ID, Deleted: true})
	return nil
}

// PauseCatSchedule 暂停（nextRunAt=0）。正在跑的一次不停。
func (s *Service) PauseCatSchedule(ctx context.Context, req ScheduleIDRequest) (Schedule, error) {
	x, err := s.getScheduleRow(ctx, req.ID)
	if err != nil {
		return Schedule{}, err
	}
	x.Enabled, x.NextRunAt, x.UpdatedAt = false, 0, s.now().UnixMilli()
	return s.saveSchedule(ctx, x)
}

// ResumeCatSchedule 恢复并重算下次时间。项目已删 / 「仅一次」时间已过 → INVALID_ARGUMENT。
func (s *Service) ResumeCatSchedule(ctx context.Context, req ScheduleIDRequest) (Schedule, error) {
	x, err := s.getScheduleRow(ctx, req.ID)
	if err != nil {
		return Schedule{}, err
	}
	if x.ProjectDeleted {
		return Schedule{}, invalid("项目已删除，请编辑后换一个项目。", "project_deleted")
	}
	now := s.now()
	next := NextRunAfter(x, now, s.loc())
	if next == 0 {
		return Schedule{}, invalid("这个时间已经过了。", "run_at")
	}
	x.Enabled, x.NextRunAt, x.UpdatedAt = true, next, now.UnixMilli()
	return s.saveSchedule(ctx, x)
}

// RunNowCatSchedule 立即运行一次（暂停的任务也可以）。上一次还没结束 → 返回一条 skipped，不报错。
func (s *Service) RunNowCatSchedule(ctx context.Context, req ScheduleIDRequest) (ScheduleRun, error) {
	x, err := s.getScheduleRow(ctx, req.ID)
	if err != nil {
		return ScheduleRun{}, err
	}
	return s.trigger(ctx, x, s.now().UnixMilli(), true), nil
}

// ListCatScheduleRuns 倒序；limit 默认 50，最多 100。
func (s *Service) ListCatScheduleRuns(ctx context.Context, req ListScheduleRunsRequest) ([]ScheduleRun, error) {
	x, err := s.getScheduleRow(ctx, req.ScheduleID)
	if err != nil {
		return nil, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = DefaultRunsLimit
	}
	if limit > MaxRunsPerSchedule {
		limit = MaxRunsPerSchedule
	}
	rows, err := s.cfg.Store.ListCatScheduleRuns(ctx, x.ID, limit)
	if err != nil {
		return nil, apperr.Wrap(apperr.IOError, "读取运行记录失败", err)
	}
	out := make([]ScheduleRun, 0, len(rows))
	for _, r := range rows {
		out = append(out, s.withQueuePosition(runFromRow(r)))
	}
	return out, nil
}

// pauseSchedulesForProject 删除项目后：任务暂停并标 projectDeleted（6.19.13.7-1）；排队中的这次记 stopped。
func (s *Service) pauseSchedulesForProject(ctx context.Context, projectID string) {
	if s.cfg.Store == nil {
		return
	}
	rows, err := s.cfg.Store.CatSchedulesByProject(ctx, projectID)
	if err != nil {
		s.cfg.Logf("cat schedules by project: %v", err)
		return
	}
	for _, x := range rows {
		s.dropQueued(x.ID)
		x.Enabled, x.ProjectDeleted, x.NextRunAt, x.UpdatedAt = false, true, 0, s.now().UnixMilli()
		if _, err := s.saveSchedule(ctx, x); err != nil {
			s.cfg.Logf("cat schedule pause %s: %v", x.ID, err)
		}
	}
}

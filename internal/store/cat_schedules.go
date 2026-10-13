package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// CatScheduleRow 是 cat_schedules 的一行（契约 6.19.13.6；v0.40 含 agent_kind）。
type CatScheduleRow struct {
	ID             string
	Name           string
	Prompt         string
	ProjectID      string
	Frequency      string
	Time           string
	Weekdays       []int
	RunAt          int64
	Model          string
	Effort         string
	ApprovalMode   string
	AgentKind      string // v0.40：cat_build | claude_cli | codex_cli
	Enabled        bool
	ProjectDeleted bool
	NextRunAt      int64
	CreatedAt      int64
	UpdatedAt      int64
}

// CatScheduleRunRow 是 cat_schedule_runs 的一行。ErrorJSON 是 AppError 的 JSON，空 = 没有错误。
type CatScheduleRunRow struct {
	ID             string
	ScheduleID     string
	ConversationID string
	Status         string
	ErrorJSON      string
	ScheduledFor   int64
	StartedAt      int64
	FinishedAt     int64
	Manual         bool
	MissedReason   string
}

const catScheduleColumns = `id, name, prompt, project_id, frequency, time, weekdays, run_at, model, effort, approval_mode, agent_kind,
	enabled, project_deleted, next_run_at, created_at, updated_at`

func scanCatSchedule(r interface{ Scan(...any) error }) (CatScheduleRow, error) {
	var x CatScheduleRow
	var wd string
	var en, pd int
	err := r.Scan(&x.ID, &x.Name, &x.Prompt, &x.ProjectID, &x.Frequency, &x.Time, &wd, &x.RunAt, &x.Model, &x.Effort,
		&x.ApprovalMode, &x.AgentKind, &en, &pd, &x.NextRunAt, &x.CreatedAt, &x.UpdatedAt)
	if err != nil {
		return x, err
	}
	x.Enabled, x.ProjectDeleted = en != 0, pd != 0
	x.Weekdays = []int{}
	_ = json.Unmarshal([]byte(wd), &x.Weekdays)
	if x.Weekdays == nil {
		x.Weekdays = []int{}
	}
	if x.AgentKind == "" {
		x.AgentKind = "cat_build"
	}
	return x, nil
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// UpsertCatSchedule 写入或整行覆盖一个定时任务。
func (s *Store) UpsertCatSchedule(ctx context.Context, x CatScheduleRow) error {
	wd, _ := json.Marshal(x.Weekdays)
	if x.Weekdays == nil {
		wd = []byte("[]")
	}
	if x.AgentKind == "" {
		x.AgentKind = "cat_build"
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO cat_schedules (`+catScheduleColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, prompt=excluded.prompt, project_id=excluded.project_id,
			frequency=excluded.frequency, time=excluded.time, weekdays=excluded.weekdays, run_at=excluded.run_at,
			model=excluded.model, effort=excluded.effort, approval_mode=excluded.approval_mode, agent_kind=excluded.agent_kind,
			enabled=excluded.enabled, project_deleted=excluded.project_deleted, next_run_at=excluded.next_run_at, updated_at=excluded.updated_at`,
		x.ID, x.Name, x.Prompt, x.ProjectID, x.Frequency, x.Time, string(wd), x.RunAt, x.Model, x.Effort, x.ApprovalMode, x.AgentKind,
		b2i(x.Enabled), b2i(x.ProjectDeleted), x.NextRunAt, x.CreatedAt, x.UpdatedAt)
	if err != nil {
		return fmt.Errorf("写入定时任务失败: %w", err)
	}
	return nil
}

// GetCatSchedule 按 id 取；不存在返回 sql.ErrNoRows。
func (s *Store) GetCatSchedule(ctx context.Context, id string) (CatScheduleRow, error) {
	return scanCatSchedule(s.db.QueryRowContext(ctx, `SELECT `+catScheduleColumns+` FROM cat_schedules WHERE id = ?`, id))
}

// ListCatSchedules 按 createdAt 倒序、再按 id 倒序。
func (s *Store) ListCatSchedules(ctx context.Context) ([]CatScheduleRow, error) {
	return s.queryCatSchedules(ctx, `SELECT `+catScheduleColumns+` FROM cat_schedules ORDER BY created_at DESC, id DESC`)
}

// CatSchedulesByProject 列出绑定这个项目的任务。
func (s *Store) CatSchedulesByProject(ctx context.Context, projectID string) ([]CatScheduleRow, error) {
	return s.queryCatSchedules(ctx, `SELECT `+catScheduleColumns+` FROM cat_schedules WHERE project_id = ? ORDER BY created_at DESC, id DESC`, projectID)
}

func (s *Store) queryCatSchedules(ctx context.Context, q string, args ...any) ([]CatScheduleRow, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("读取定时任务失败: %w", err)
	}
	defer rows.Close()
	out := []CatScheduleRow{}
	for rows.Next() {
		x, err := scanCatSchedule(rows)
		if err != nil {
			return nil, fmt.Errorf("读取定时任务失败: %w", err)
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// DeleteCatSchedule 删除任务及其全部运行记录（一个事务）；对话不动。不存在返回 sql.ErrNoRows。
func (s *Store) DeleteCatSchedule(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM cat_schedule_runs WHERE schedule_id = ?`, id); err != nil {
		return fmt.Errorf("删除运行记录失败: %w", err)
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM cat_schedules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("删除定时任务失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return tx.Commit()
}

const catRunColumns = `id, schedule_id, conversation_id, status, error, scheduled_for, started_at, finished_at, manual, missed_reason`

func scanCatRun(r interface{ Scan(...any) error }) (CatScheduleRunRow, error) {
	var x CatScheduleRunRow
	var m int
	err := r.Scan(&x.ID, &x.ScheduleID, &x.ConversationID, &x.Status, &x.ErrorJSON, &x.ScheduledFor, &x.StartedAt, &x.FinishedAt, &m, &x.MissedReason)
	x.Manual = m != 0
	return x, err
}

// UpsertCatScheduleRun 写入或覆盖一条运行记录。任务已不在时不写（返回 sql.ErrNoRows）。
func (s *Store) UpsertCatScheduleRun(ctx context.Context, x CatScheduleRunRow) error {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO cat_schedule_runs (`+catRunColumns+`)
		SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, ? WHERE EXISTS (SELECT 1 FROM cat_schedules WHERE id = ?)
		ON CONFLICT(id) DO UPDATE SET conversation_id=excluded.conversation_id, status=excluded.status, error=excluded.error,
			started_at=excluded.started_at, finished_at=excluded.finished_at, missed_reason=excluded.missed_reason`,
		x.ID, x.ScheduleID, x.ConversationID, x.Status, x.ErrorJSON, x.ScheduledFor, x.StartedAt, x.FinishedAt, b2i(x.Manual), x.MissedReason, x.ScheduleID)
	if err != nil {
		return fmt.Errorf("写入运行记录失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// PruneCatScheduleRuns 每个任务只留最近 keep 条（按 scheduled_for 倒序，同一时刻再按 id 倒序）。
func (s *Store) PruneCatScheduleRuns(ctx context.Context, scheduleID string, keep int) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM cat_schedule_runs WHERE schedule_id = ? AND id NOT IN (
			SELECT id FROM cat_schedule_runs WHERE schedule_id = ? ORDER BY scheduled_for DESC, id DESC LIMIT ?)`,
		scheduleID, scheduleID, keep)
	if err != nil {
		return fmt.Errorf("清理运行记录失败: %w", err)
	}
	return nil
}

// ListCatScheduleRuns 倒序列出一个任务的运行记录。
func (s *Store) ListCatScheduleRuns(ctx context.Context, scheduleID string, limit int) ([]CatScheduleRunRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+catRunColumns+` FROM cat_schedule_runs WHERE schedule_id = ?
		ORDER BY scheduled_for DESC, id DESC LIMIT ?`, scheduleID, limit)
	if err != nil {
		return nil, fmt.Errorf("读取运行记录失败: %w", err)
	}
	defer rows.Close()
	out := []CatScheduleRunRow{}
	for rows.Next() {
		x, err := scanCatRun(rows)
		if err != nil {
			return nil, fmt.Errorf("读取运行记录失败: %w", err)
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// LatestCatScheduleRun 取该任务最近一条运行记录；没有返回 sql.ErrNoRows。
func (s *Store) LatestCatScheduleRun(ctx context.Context, scheduleID string) (CatScheduleRunRow, error) {
	list, err := s.ListCatScheduleRuns(ctx, scheduleID, 1)
	if err != nil {
		return CatScheduleRunRow{}, err
	}
	if len(list) == 0 {
		return CatScheduleRunRow{}, sql.ErrNoRows
	}
	return list[0], nil
}

// RecoverCatScheduleRuns 在启动时收尾上次进程留下的记录：排队中 → missed（closed）；运行中 → failed（带 errJSON）。
func (s *Store) RecoverCatScheduleRuns(ctx context.Context, finishedAt int64, errJSON string) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE cat_schedule_runs SET status='missed', missed_reason='closed', finished_at=?
		WHERE status='queued'`, finishedAt); err != nil {
		return fmt.Errorf("收尾运行记录失败: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE cat_schedule_runs SET status='failed', error=?, finished_at=?
		WHERE status='running'`, errJSON, finishedAt); err != nil {
		return fmt.Errorf("收尾运行记录失败: %w", err)
	}
	return nil
}

// IsNoRows 报告 err 是否是「没有这一行」。
func IsNoRows(err error) bool { return errors.Is(err, sql.ErrNoRows) }

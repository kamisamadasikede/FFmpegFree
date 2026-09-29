package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"FFmpegFree/internal/apperr"
)

// TaskType 和 TaskStatus 见契约第 3 节。
type TaskType string
type TaskStatus string

const (
	TypeConvert    TaskType = "convert"
	TypeEditExport TaskType = "edit_export"
	// TypeEditRender 是 v0.11 之前的旧名，只为读取旧数据保留，不再产生任务。
	//
	// Deprecated: 用 TypeEditExport。
	TypeEditRender     TaskType = "edit_render"
	TypeOfficePDF      TaskType = "office_pdf"
	TypeLiveFilePush   TaskType = "live_file_push"
	TypeLiveRelay      TaskType = "live_relay"
	TypeLiveRecordPush TaskType = "live_record_push"
	TypeFFmpegInstall  TaskType = "ffmpeg_install"
)

const (
	StatusQueued      TaskStatus = "queued"
	StatusRunning     TaskStatus = "running"
	StatusSucceeded   TaskStatus = "succeeded"
	StatusFailed      TaskStatus = "failed"
	StatusCanceled    TaskStatus = "canceled"
	StatusInterrupted TaskStatus = "interrupted"
)

// Active 表示任务还没结束（排队或运行中）。
func (s TaskStatus) Active() bool { return s == StatusQueued || s == StatusRunning }

// Task 是契约第 3 节的 Task。Progress、Speed、EtaSec 只在内存里实时更新，
// 落库只在状态变化时发生（契约 6.5），所以从库里读出来的 Speed、EtaSec 恒为空。
type Task struct {
	ID         string           `json:"id"`
	Type       TaskType         `json:"type"`
	Status     TaskStatus       `json:"status"`
	Title      string           `json:"title"`
	InputPaths []string         `json:"inputPaths"`
	OutputPath string           `json:"outputPath"`
	Progress   float64          `json:"progress"` // 0~1，直播类任务恒为 -1
	Speed      string           `json:"speed"`    // 如 "2.3x"
	EtaSec     float64          `json:"etaSec"`
	Params     string           `json:"params"`  // 原始参数 JSON，用于重试
	Version    int64            `json:"version"` // 每次变更 +1，前端据此丢弃旧事件
	Error      *apperr.AppError `json:"error,omitempty"`
	CreatedAt  int64            `json:"createdAt"`
	StartedAt  int64            `json:"startedAt"`
	FinishedAt int64            `json:"finishedAt"`

	// LogPath 不暴露给前端，前端通过 TaskService.GetLog 读取。
	LogPath string `json:"-"`
}

// TaskFilter 是 TaskService.List 的过滤条件。Types、Statuses 为空表示不过滤。
// Limit 默认 50，最大 200；结果按创建时间倒序。
type TaskFilter struct {
	Types    []TaskType   `json:"types"`
	Statuses []TaskStatus `json:"statuses"`
	Limit    int          `json:"limit"`
	Offset   int          `json:"offset"`
}

// TaskPage 是 List 的分页结果，Total 是符合过滤条件的总数。
type TaskPage struct {
	Items []Task `json:"items"`
	Total int64  `json:"total"`
}

const (
	defaultTaskLimit = 50
	maxTaskLimit     = 200
)

const taskColumns = `id, type, status, title, input_paths, output_path, params, progress, error,
	log_path, version, created_at, started_at, finished_at`

// InsertTask 新建任务记录。
func (s *Store) InsertTask(ctx context.Context, t Task) error {
	inputs, err := json.Marshal(nonNil(t.InputPaths))
	if err != nil {
		return err
	}
	errJSON, err := encodeAppError(t.Error)
	if err != nil {
		return err
	}
	params := t.Params
	if params == "" {
		params = "{}"
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO tasks (`+taskColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.ID, string(t.Type), string(t.Status), t.Title, string(inputs), t.OutputPath, params, t.Progress,
		errJSON, t.LogPath, t.Version, t.CreatedAt, t.StartedAt, t.FinishedAt)
	if err != nil {
		return fmt.Errorf("写入任务失败: %w", err)
	}
	return nil
}

// UpdateTask 保存任务的可变字段（状态、进度、输出、错误、时间、版本）。任务不存在时返回 sql.ErrNoRows。
func (s *Store) UpdateTask(ctx context.Context, t Task) error {
	errJSON, err := encodeAppError(t.Error)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE tasks SET status=?, title=?, output_path=?, progress=?, error=?, log_path=?,
		       version=?, started_at=?, finished_at=?
		WHERE id=?`,
		string(t.Status), t.Title, t.OutputPath, t.Progress, errJSON, t.LogPath,
		t.Version, t.StartedAt, t.FinishedAt, t.ID)
	if err != nil {
		return fmt.Errorf("更新任务失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// GetTask 按 ID 读取任务，不存在返回 sql.ErrNoRows。
func (s *Store) GetTask(ctx context.Context, id string) (Task, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+taskColumns+` FROM tasks WHERE id = ?`, id)
	return scanTask(row)
}

// ListTasks 按过滤条件分页查询。
func (s *Store) ListTasks(ctx context.Context, f TaskFilter) (TaskPage, error) {
	where, args := taskWhere(f)
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks`+where, args...).Scan(&total); err != nil {
		return TaskPage{}, fmt.Errorf("统计任务失败: %w", err)
	}
	limit := f.Limit
	if limit <= 0 {
		limit = defaultTaskLimit
	}
	if limit > maxTaskLimit {
		limit = maxTaskLimit
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+taskColumns+` FROM tasks`+where+` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`,
		append(args, limit, offset)...)
	if err != nil {
		return TaskPage{}, fmt.Errorf("查询任务失败: %w", err)
	}
	defer rows.Close()
	page := TaskPage{Items: []Task{}, Total: total}
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return TaskPage{}, err
		}
		page.Items = append(page.Items, t)
	}
	return page, rows.Err()
}

// ListTasksByStatus 返回指定状态的全部任务（按创建时间升序），用于启动时恢复排队。
func (s *Store) ListTasksByStatus(ctx context.Context, statuses ...TaskStatus) ([]Task, error) {
	where, args := taskWhere(TaskFilter{Statuses: statuses})
	rows, err := s.db.QueryContext(ctx, `SELECT `+taskColumns+` FROM tasks`+where+` ORDER BY created_at ASC, id ASC`, args...)
	if err != nil {
		return nil, fmt.Errorf("查询任务失败: %w", err)
	}
	defer rows.Close()
	var out []Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// DeleteTasks 删除指定任务（只删已结束的，进行中的任务不会被删），返回实际删除的 ID。
func (s *Store) DeleteTasks(ctx context.Context, ids []string) ([]string, error) {
	var deleted []string
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	for _, id := range ids {
		res, err := tx.ExecContext(ctx, `DELETE FROM tasks WHERE id = ? AND status NOT IN ('queued','running')`, id)
		if err != nil {
			return nil, fmt.Errorf("删除任务失败: %w", err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			deleted = append(deleted, id)
		}
	}
	return deleted, tx.Commit()
}

// TaskOutputsByBase 返回 output_path 的文件名（最后一段）与 base 相同（SQLite LIKE 对 ASCII 大小写不敏感的粗筛）的任务输出路径，
// 最多 500 条。调用方要自己做精确比较；用来判断某个路径是不是任务表里登记的输出。
func (s *Store) TaskOutputsByBase(ctx context.Context, base string) ([]string, error) {
	if base == "" {
		return nil, nil
	}
	esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(base)
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT output_path FROM tasks WHERE output_path <> '' AND output_path LIKE ? ESCAPE '\' LIMIT 500`,
		"%"+esc)
	if err != nil {
		return nil, fmt.Errorf("查询任务输出失败: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeleteFinishedTasks 删除所有已结束（成功、失败、取消、中断）的任务，返回被删任务（用于清理日志、发事件）。
func (s *Store) DeleteFinishedTasks(ctx context.Context) ([]Task, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT `+taskColumns+` FROM tasks WHERE status NOT IN ('queued','running')`)
	if err != nil {
		return nil, err
	}
	var gone []Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		gone = append(gone, t)
	}
	rows.Close()
	if _, err := tx.ExecContext(ctx, `DELETE FROM tasks WHERE status NOT IN ('queued','running')`); err != nil {
		return nil, fmt.Errorf("清理任务失败: %w", err)
	}
	return gone, tx.Commit()
}

func taskWhere(f TaskFilter) (string, []any) {
	var conds []string
	var args []any
	if len(f.Types) > 0 {
		conds = append(conds, "type IN ("+placeholders(len(f.Types))+")")
		for _, t := range f.Types {
			args = append(args, string(t))
		}
	}
	if len(f.Statuses) > 0 {
		conds = append(conds, "status IN ("+placeholders(len(f.Statuses))+")")
		for _, s := range f.Statuses {
			args = append(args, string(s))
		}
	}
	if len(conds) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

type rowScanner interface{ Scan(dest ...any) error }

func scanTask(r rowScanner) (Task, error) {
	var t Task
	var typ, status, inputs string
	var errJSON sql.NullString
	if err := r.Scan(&t.ID, &typ, &status, &t.Title, &inputs, &t.OutputPath, &t.Params, &t.Progress, &errJSON,
		&t.LogPath, &t.Version, &t.CreatedAt, &t.StartedAt, &t.FinishedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Task{}, err
		}
		return Task{}, fmt.Errorf("读取任务失败: %w", err)
	}
	t.Type, t.Status = TaskType(typ), TaskStatus(status)
	if err := json.Unmarshal([]byte(inputs), &t.InputPaths); err != nil || t.InputPaths == nil {
		t.InputPaths = []string{}
	}
	if errJSON.Valid && errJSON.String != "" && errJSON.String != "null" {
		var ae apperr.AppError
		if err := json.Unmarshal([]byte(errJSON.String), &ae); err == nil {
			t.Error = &ae
		}
	}
	return t, nil
}

func encodeAppError(e *apperr.AppError) (any, error) {
	if e == nil {
		return nil, nil
	}
	b, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

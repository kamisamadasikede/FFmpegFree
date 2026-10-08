package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
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
	TypeLiveScreenPush TaskType = "live_screen_push"
	TypeFFmpegInstall  TaskType = "ffmpeg_install"

	// Deprecated: TypeLiveRelay 只为读旧数据保留（契约 v0.10），不再产生，Submit 不接受；
	// 库里这类旧记录在所有读取路径上按未知类型忽略（不报错）。
	TypeLiveRelay TaskType = "live_relay"
	// Deprecated: TypeLiveRecordPush 同 TypeLiveRelay，只为读旧数据保留。
	TypeLiveRecordPush TaskType = "live_record_push"
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
	ID         string     `json:"id"`
	Type       TaskType   `json:"type"`
	Status     TaskStatus `json:"status"`
	Title      string     `json:"title"`
	InputPaths []string   `json:"inputPaths"`
	OutputPath string     `json:"outputPath"`
	Progress   float64    `json:"progress"` // 0~1，直播类任务恒为 -1
	Speed      string     `json:"speed"`    // 如 "2.3x"
	EtaSec     float64    `json:"etaSec"`
	// 以下三项只有直播任务在运行中才有值（契约 v0.10），只在内存里、不落库，和 Speed / EtaSec 一样。
	Fps           float64 `json:"fps,omitempty"`           // 当前输出帧率
	BitrateKbps   float64 `json:"bitrateKbps,omitempty"`   // 近 5 秒的输出码率（kbit/s）
	DroppedFrames int64   `json:"droppedFrames,omitempty"` // ffmpeg 累计丢帧数（不是网络丢包）
	// 以下四项是硬件编码接入（契约 v0.18，9.7）：任务实际使用的视频编码器与设备。没有视频编码（纯音频转换、非重编码任务）或还没确定时省略。
	// 会落库（迁移 0004），刷新 / 重启后仍能看到；Retry 生成的新任务重新解析。
	Encoder          string           `json:"encoder,omitempty"`          // 如 h264_nvenc、libx264、libx265、libvpx-vp9、gif、copy
	EncoderDevice    string           `json:"encoderDevice,omitempty"`    // 设备 id（nvidia / intel / amd / apple 等）；CPU 编码为 "cpu"；copy 时省略
	HWFallback       bool             `json:"hwFallback,omitempty"`       // 想用硬件但实际用了 CPU（设备不可用，或硬件编码启动失败后自动用 CPU 重试）
	HWFallbackReason string           `json:"hwFallbackReason,omitempty"` // 一行短原因（固定枚举，不含路径），见契约 9.7
	Params           string           `json:"params"`                     // 原始参数 JSON，用于重试
	Version          int64            `json:"version"`                    // 每次变更 +1，前端据此丢弃旧事件
	Error            *apperr.AppError `json:"error,omitempty"`
	CreatedAt        int64            `json:"createdAt"`
	StartedAt        int64            `json:"startedAt"`
	FinishedAt       int64            `json:"finishedAt"`
	// 以下三项是转换记录（契约 v0.23，6.14.2），落库（迁移 0005）。
	SourceID           string      `json:"sourceId,omitempty"` // 只有 convert 任务有；指向 convert_sources.id
	HiddenInTaskCenter bool        `json:"hiddenInTaskCenter"` // 始终输出；true = 在任务中心隐藏（转换页照常显示）
	Result             *TaskResult `json:"result,omitempty"`   // 只有成功的 convert 任务有；探测失败也可能没有

	// LogPath 不暴露给前端，前端通过 TaskService.GetLog 读取。
	LogPath string `json:"-"`
}

// TaskResult 是成功的 convert 任务完成时对最终输出的探测结果（契约 v0.23，6.14.2 / 6.14.6），落库在 tasks.result（JSON）。
type TaskResult struct {
	SizeBytes        int64   `json:"sizeBytes"`                  // os.Stat 的大小
	DurationSec      float64 `json:"durationSec,omitempty"`      // ffprobe format.duration
	Width            int     `json:"width,omitempty"`            // 显示尺寸；纯音频省略
	Height           int     `json:"height,omitempty"`           //
	AudioBitrateKbps int     `json:"audioBitrateKbps,omitempty"` // 第一条音频流的码率（kbit/s，四舍五入）
}

// TaskFilter 是 TaskService.List 的过滤条件。Types、Statuses 为空表示不过滤。
// Limit 默认 50，最大 200；结果按创建时间倒序。IncludeHidden=false（默认）时不返回 hiddenInTaskCenter=true 的任务（契约 v0.23）。
type TaskFilter struct {
	Types         []TaskType   `json:"types"`
	Statuses      []TaskStatus `json:"statuses"`
	Limit         int          `json:"limit"`
	Offset        int          `json:"offset"`
	IncludeHidden bool         `json:"includeHidden,omitempty"`
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
	log_path, version, created_at, started_at, finished_at, encoder, encoder_device, hw_fallback, hw_fallback_reason,
	source_id, hidden_in_task_center, result`

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
	resJSON, err := encodeResult(t.Result)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO tasks (`+taskColumns+`, output_name_key) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.ID, string(t.Type), string(t.Status), t.Title, string(inputs), t.OutputPath, params, t.Progress,
		errJSON, t.LogPath, t.Version, t.CreatedAt, t.StartedAt, t.FinishedAt,
		t.Encoder, t.EncoderDevice, boolInt(t.HWFallback), t.HWFallbackReason,
		nullStr(t.SourceID), boolInt(t.HiddenInTaskCenter), resJSON, NameKey(t.OutputPath))
	if err != nil {
		return fmt.Errorf("写入任务失败: %w", err)
	}
	return nil
}

// UpdateTask 保存任务的可变字段（状态、进度、输出、错误、时间、版本、隐藏标记、结果）。任务不存在时返回 sql.ErrNoRows。
// output_name_key 随 output_path 同步更新（契约 6.14.9）；source_id、params、created_at 不变。
func (s *Store) UpdateTask(ctx context.Context, t Task) error {
	errJSON, err := encodeAppError(t.Error)
	if err != nil {
		return err
	}
	resJSON, err := encodeResult(t.Result)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE tasks SET status=?, title=?, output_path=?, output_name_key=?, progress=?, error=?, log_path=?,
		       version=?, started_at=?, finished_at=?,
		       encoder=?, encoder_device=?, hw_fallback=?, hw_fallback_reason=?,
		       hidden_in_task_center=?, result=?
		WHERE id=?`,
		string(t.Status), t.Title, t.OutputPath, NameKey(t.OutputPath), t.Progress, errJSON, t.LogPath,
		t.Version, t.StartedAt, t.FinishedAt,
		t.Encoder, t.EncoderDevice, boolInt(t.HWFallback), t.HWFallbackReason,
		boolInt(t.HiddenInTaskCenter), resJSON, t.ID)
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
	row := s.db.QueryRowContext(ctx, `SELECT `+taskColumns+` FROM tasks WHERE id = ? AND type NOT IN `+legacyTypesSQL, id)
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
	where, args := taskWhere(TaskFilter{Statuses: statuses, IncludeHidden: true})
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
		res, err := tx.ExecContext(ctx, `DELETE FROM tasks WHERE id = ? AND status NOT IN ('queued','running') AND type NOT IN `+legacyTypesSQL, id)
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
	rows, err := tx.QueryContext(ctx, `SELECT `+taskColumns+` FROM tasks WHERE status NOT IN ('queued','running') AND type NOT IN `+legacyTypesSQL)
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
	if _, err := tx.ExecContext(ctx, `DELETE FROM tasks WHERE status NOT IN ('queued','running') AND type NOT IN `+legacyTypesSQL); err != nil {
		return nil, fmt.Errorf("清理任务失败: %w", err)
	}
	return gone, tx.Commit()
}

// legacyTypes 是"保留但不再产生"的旧任务类型（契约 v0.10 确认项 ⑧）：库里这类记录在所有读取 / 按 id 操作的入口按不存在处理。
// 新增旧类型只改这一处。edit_render 是 edit_export 的旧名，同样只读旧数据、不再产生。
var legacyTypes = []TaskType{TypeLiveRelay, TypeLiveRecordPush, TypeEditRender}

// legacyTypesSQL 是 legacyTypes 的 SQL 列表，如 ('live_relay','live_record_push','edit_render')。
var legacyTypesSQL = func() string {
	q := make([]string, len(legacyTypes))
	for i, t := range legacyTypes {
		q[i] = "'" + string(t) + "'"
	}
	return "(" + strings.Join(q, ",") + ")"
}()

// LegacyTaskIDs 返回 ids 中"库里有这条记录、但类型是旧类型"的 id（真正不存在的 id 不在其中）。
// Manager.Remove 用它实现"旧类型按不存在处理并整体失败"（契约 v0.10 确认项 ⑧）。
func (s *Store) LegacyTaskIDs(ctx context.Context, ids []string) ([]string, error) {
	var out []string
	for _, id := range ids {
		var found string
		err := s.db.QueryRowContext(ctx, `SELECT id FROM tasks WHERE id = ? AND type IN `+legacyTypesSQL, id).Scan(&found)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("查询旧类型任务失败: %w", err)
		}
		out = append(out, found)
	}
	return out, nil
}

// IsLegacyType 判断任务类型是否是"保留但不再产生"的旧类型；库里这类记录在 List / ListActive / Get 里按不存在处理，不报错。
func IsLegacyType(t TaskType) bool {
	for _, l := range legacyTypes {
		if t == l {
			return true
		}
	}
	return false
}

func taskWhere(f TaskFilter) (string, []any) {
	conds := []string{"type NOT IN " + legacyTypesSQL}
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
	if !f.IncludeHidden {
		conds = append(conds, "hidden_in_task_center = 0")
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
	var errJSON, sourceID, resJSON sql.NullString
	var hwFallback, hidden int
	if err := r.Scan(&t.ID, &typ, &status, &t.Title, &inputs, &t.OutputPath, &t.Params, &t.Progress, &errJSON,
		&t.LogPath, &t.Version, &t.CreatedAt, &t.StartedAt, &t.FinishedAt,
		&t.Encoder, &t.EncoderDevice, &hwFallback, &t.HWFallbackReason,
		&sourceID, &hidden, &resJSON); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Task{}, err
		}
		return Task{}, fmt.Errorf("读取任务失败: %w", err)
	}
	t.Type, t.Status = TaskType(typ), TaskStatus(status)
	t.HWFallback = hwFallback != 0
	t.HiddenInTaskCenter = hidden != 0
	t.SourceID = sourceID.String
	if resJSON.Valid && resJSON.String != "" && resJSON.String != "null" {
		var r TaskResult
		if err := json.Unmarshal([]byte(resJSON.String), &r); err == nil {
			t.Result = &r
		}
	}
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

func encodeResult(r *TaskResult) (any, error) {
	if r == nil {
		return nil, nil
	}
	b, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// NameKey 是 name_key / output_name_key 的规范化（契约 6.14.2）：Go 的 Unicode 小写的文件名（basename）；空路径为 ""。
func NameKey(p string) string {
	if p == "" {
		return ""
	}
	return strings.ToLower(filepath.Base(p))
}

// HideFinishedTasks 把所有已结束且未隐藏的任务设为在任务中心隐藏（契约 v0.23），返回本次隐藏的条数；
// 不删任何东西、version 不变（这是任务中心的显示开关，不是任务状态）。
func (s *Store) HideFinishedTasks(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE tasks SET hidden_in_task_center = 1
		WHERE hidden_in_task_center = 0 AND status NOT IN ('queued','running') AND type NOT IN `+legacyTypesSQL)
	if err != nil {
		return 0, fmt.Errorf("隐藏任务失败: %w", err)
	}
	return res.RowsAffected()
}

// UnhideTasks 在一个事务里把 ids 中已隐藏的任务清成未隐藏、version +1，返回真正被改的任务（改后的值）。
// 本来就没隐藏的 id 什么都不做；不存在 / 旧类型的 id 忽略（调用方先校验）。
func (s *Store) UnhideTasks(ctx context.Context, ids []string) ([]Task, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var changed []string
	for _, id := range ids {
		res, err := tx.ExecContext(ctx, `UPDATE tasks SET hidden_in_task_center = 0, version = version + 1
			WHERE id = ? AND hidden_in_task_center = 1 AND type NOT IN `+legacyTypesSQL, id)
		if err != nil {
			return nil, fmt.Errorf("取消隐藏失败: %w", err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			changed = append(changed, id)
		}
	}
	out := make([]Task, 0, len(changed))
	for _, id := range changed {
		t, err := scanTask(tx.QueryRowContext(ctx, `SELECT `+taskColumns+` FROM tasks WHERE id = ?`, id))
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, tx.Commit()
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

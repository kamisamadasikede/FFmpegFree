package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/id"
	"FFmpegFree/internal/paths"
)

// ConvertSource 是转换页上的一行（一个源文件），见契约 6.14.2。
type ConvertSource struct {
	SourceID       string `json:"sourceId"`
	Path           string `json:"path"`
	Name           string `json:"name"`
	AddedAt        int64  `json:"addedAt"`
	LastActivityAt int64  `json:"lastActivityAt"`
	// Media 是这一行持久化的探测结果（契约 v0.23.4：convert_sources.media），hasVideo / hasAudio / sampleRate / channels
	// 都可靠；没有持久化结果时退回按 path_key 关联 media 表（hasVideo / hasAudio 按编码是否为空推出）；都没有时省略。
	Media *MediaInfo `json:"media,omitempty"`

	// MediaFP 是持久化探测结果对应的文件指纹（"<大小>:<修改时间纳秒>"），'' = 从没探测过。只在后端用，不给前端。
	MediaFP string `json:"-"`

	// 以下是 v0.24 的副本字段（契约 6.15.3），来自 copy_id 指向的 convert_copies 行。
	OriginalPath string           `json:"originalPath"`        // 原文件的绝对路径（= path）
	StoredPath   string           `json:"storedPath"`          // 副本的绝对路径；copyState=none 时为 ""
	CopyState    string           `json:"copyState"`           // none | copying | ready | failed | canceled
	CopiedBytes  int64            `json:"copiedBytes"`         // 已复制的字节数
	TotalBytes   int64            `json:"totalBytes"`          // 副本应有的大小
	CopyError    *apperr.AppError `json:"copyError,omitempty"` // 只有 failed 有
	// CopyID 是当前引用的副本（只在后端用）。
	CopyID string `json:"-"`
}

// 副本状态（契约 6.15.3）。
const (
	CopyNone     = "none"
	CopyCopying  = "copying"
	CopyReady    = "ready"
	CopyFailed   = "failed"
	CopyCanceled = "canceled"
)

// convertSourceFrom 是读源文件行时的 FROM（带副本的 LEFT JOIN）。
const convertSourceFrom = ` FROM convert_sources s LEFT JOIN convert_copies c ON c.id = s.copy_id`

const convertSourceColumns = `s.id, s.path, s.name, s.added_at, s.last_activity_at, s.media, s.media_fp,
	s.copy_id, c.stored_path, c.state, c.copied_bytes, c.total_bytes, c.error`

func scanConvertSource(r rowScanner) (ConvertSource, error) {
	var s ConvertSource
	var media sql.NullString
	var copyID, stored, state, errJSON sql.NullString
	var copied, total sql.NullInt64
	if err := r.Scan(&s.SourceID, &s.Path, &s.Name, &s.AddedAt, &s.LastActivityAt, &media, &s.MediaFP,
		&copyID, &stored, &state, &copied, &total, &errJSON); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ConvertSource{}, err
		}
		return ConvertSource{}, fmt.Errorf("读取源文件行失败: %w", err)
	}
	if media.Valid && media.String != "" {
		var m MediaInfo
		if json.Unmarshal([]byte(media.String), &m) == nil {
			m.FillCodecNames()
			s.Media = &m
		} else {
			s.MediaFP = "" // JSON 损坏：当作没探测过，下次重探
		}
	}
	s.OriginalPath = s.Path
	s.CopyState = CopyNone
	if copyID.Valid && state.Valid {
		s.CopyID = copyID.String
		s.StoredPath, s.CopyState = stored.String, state.String
		s.CopiedBytes, s.TotalBytes = copied.Int64, total.Int64
		if s.CopyState == CopyReady {
			s.CopiedBytes = s.TotalBytes
		}
		if s.CopyState == CopyFailed && errJSON.Valid && errJSON.String != "" {
			var ae apperr.AppError
			if json.Unmarshal([]byte(errJSON.String), &ae) == nil {
				s.CopyError = &ae
			}
		}
	}
	return s, nil
}

// SetConvertSourceMedia 写入一行的探测结果和文件指纹（契约 v0.23.4）。m 为 nil 表示这个指纹的文件探测失败（media 置 NULL，
// 文件不变就不再重探）。thumbUrl、error、id 不入库。行不存在时什么都不做。
func (s *Store) SetConvertSourceMedia(ctx context.Context, id, fp string, m *MediaInfo) error {
	v, err := convertSourceMediaJSON(m)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE convert_sources SET media = ?, media_fp = ? WHERE id = ?`, v, fp, id); err != nil {
		return fmt.Errorf("保存源文件媒体信息失败: %w", err)
	}
	return nil
}

// SetConvertSourceMediaByKey 同 SetConvertSourceMedia，按 path_key 定位（MediaService.Probe 成功后顺带刷新同一文件的行）。
// 没有这一行时什么都不做。
func (s *Store) SetConvertSourceMediaByKey(ctx context.Context, key, fp string, m *MediaInfo) error {
	v, err := convertSourceMediaJSON(m)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE convert_sources SET media = ?, media_fp = ? WHERE path_key = ?`, v, fp, key); err != nil {
		return fmt.Errorf("保存源文件媒体信息失败: %w", err)
	}
	return nil
}

// FileFingerprint 是探测结果对应的文件指纹（convert_sources.media_fp）："<大小>:<修改时间纳秒>"。
// 文件被覆盖或替换后指纹变化，下次列表 / 预览时会重探。
func FileFingerprint(fi os.FileInfo) string {
	return strconv.FormatInt(fi.Size(), 10) + ":" + strconv.FormatInt(fi.ModTime().UnixNano(), 10)
}

func convertSourceMediaJSON(m *MediaInfo) (any, error) {
	if m == nil {
		return nil, nil
	}
	c := *m
	c.ID, c.ThumbURL, c.Error = "", "", nil
	c.FillCodecNames()
	b, err := json.Marshal(c)
	if err != nil {
		return nil, fmt.Errorf("编码媒体信息失败: %w", err)
	}
	return string(b), nil
}

// UpsertConvertSource 按 path_key 找到或新建源文件行（AddSources / Submit 用）：已有 → 只把 last_activity_at 设为 now，existed=true；
// 否则新建（added_at = last_activity_at = now）。path / key 由调用方用 paths.Normalize 生成。
func (s *Store) UpsertConvertSource(ctx context.Context, path, key string, now int64) (ConvertSource, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ConvertSource{}, false, err
	}
	defer tx.Rollback()
	src, err := scanConvertSource(tx.QueryRowContext(ctx, `SELECT `+convertSourceColumns+convertSourceFrom+` WHERE s.path_key = ?`, key))
	existed := err == nil
	switch {
	case existed:
		if _, err := tx.ExecContext(ctx, `UPDATE convert_sources SET last_activity_at = ? WHERE id = ?`, now, src.SourceID); err != nil {
			return ConvertSource{}, false, fmt.Errorf("更新源文件行失败: %w", err)
		}
		src.LastActivityAt = now
	case errors.Is(err, sql.ErrNoRows):
		name := filepath.Base(path)
		src = ConvertSource{SourceID: id.New(), Path: path, OriginalPath: path, Name: name, AddedAt: now, LastActivityAt: now, CopyState: CopyNone}
		if _, err := tx.ExecContext(ctx, `INSERT INTO convert_sources (id, path, path_key, name, name_key, added_at, last_activity_at)
			VALUES (?,?,?,?,?,?,?)`, src.SourceID, path, key, name, NameKey(path), now, now); err != nil {
			return ConvertSource{}, false, fmt.Errorf("新建源文件行失败: %w", err)
		}
	default:
		return ConvertSource{}, false, err
	}
	return src, existed, tx.Commit()
}

// GetConvertSource 按 id 读取源文件行，不存在返回 sql.ErrNoRows。
func (s *Store) GetConvertSource(ctx context.Context, id string) (ConvertSource, error) {
	return scanConvertSource(s.db.QueryRowContext(ctx, `SELECT `+convertSourceColumns+convertSourceFrom+` WHERE s.id = ?`, id))
}

// TouchConvertSources 把这些行的 last_activity_at 设为 now（提交转换 / 再转一次时）。
func (s *Store) TouchConvertSources(ctx context.Context, ids []string, now int64) error {
	for _, id := range ids {
		if _, err := s.db.ExecContext(ctx, `UPDATE convert_sources SET last_activity_at = ? WHERE id = ?`, now, id); err != nil {
			return fmt.Errorf("更新源文件行失败: %w", err)
		}
	}
	return nil
}

// DeleteConvertSource 删除源文件行（只删行，不碰文件）；同一个事务里把它引用的副本 ref_count − 1（契约 6.15.7 第 3 条），
// 返回减过之后的副本行（没有副本时 nil），ref_count 为 0 时由调用方删副本文件和副本行。
func (s *Store) DeleteConvertSource(ctx context.Context, id string) (*ConvertCopy, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var copyID sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT copy_id FROM convert_sources WHERE id = ?`, id).Scan(&copyID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取源文件行失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM convert_sources WHERE id = ?`, id); err != nil {
		return nil, fmt.Errorf("删除源文件行失败: %w", err)
	}
	var released *ConvertCopy
	if copyID.Valid && copyID.String != "" {
		if released, err = derefCopyTx(ctx, tx, copyID.String); err != nil {
			return nil, err
		}
	}
	return released, tx.Commit()
}

// 源文件行的状态筛选（契约 v0.23.1，ConvertSourceFilter.status）。
const (
	SourceStatusAll    = ""
	SourceStatusActive = "active" // 至少有一条 queued / running 的记录
	SourceStatusFailed = "failed" // 至少有一条 failed / interrupted 的记录（canceled 不算）
)

// sourceStatusSQL 是各筛选值对应的 EXISTS 子查询（走 idx_tasks_source_status）。
// v0.24（6.15.6）：active 另算副本正在复制的行，failed 另算副本复制失败的行。
var sourceStatusSQL = map[string]string{
	SourceStatusActive: `(EXISTS (SELECT 1 FROM tasks t WHERE t.source_id = s.id AND t.type = 'convert' AND t.status IN ('queued','running'))
		OR EXISTS (SELECT 1 FROM convert_copies cc WHERE cc.id = s.copy_id AND cc.state = 'copying'))`,
	SourceStatusFailed: `(EXISTS (SELECT 1 FROM tasks t WHERE t.source_id = s.id AND t.type = 'convert' AND t.status IN ('failed','interrupted'))
		OR EXISTS (SELECT 1 FROM convert_copies cc WHERE cc.id = s.copy_id AND cc.state = 'failed'))`,
}

// ValidSourceStatus 判断 status 是不是 ConvertSourceFilter.status 允许的值。
func ValidSourceStatus(status string) bool {
	_, ok := sourceStatusSQL[status]
	return ok || status == SourceStatusAll
}

// ListConvertSources 按 last_activity_at 倒序、id 倒序分页列出全部源文件行。keyword 非空时只列出源文件名命中、
// 或任一 convert 记录的输出文件名命中的行（keyword 已由调用方 strings.ToLower，契约 6.14.9：instr 子串匹配）；
// status 非空时只列出有对应状态记录的行（契约 v0.23.1，值由调用方校验）。
func (s *Store) ListConvertSources(ctx context.Context, keyword, status string, limit, offset int) ([]ConvertSource, int64, error) {
	var conds []string
	args := []any{}
	if keyword != "" {
		conds = append(conds, `(instr(s.name_key, ?) > 0 OR s.id IN (
			SELECT t.source_id FROM tasks t WHERE t.type = 'convert' AND t.source_id IS NOT NULL AND instr(t.output_name_key, ?) > 0))`)
		args = append(args, keyword, keyword)
	}
	if status != SourceStatusAll {
		q, ok := sourceStatusSQL[status]
		if !ok {
			return nil, 0, fmt.Errorf("未知的状态筛选: %q", status)
		}
		conds = append(conds, q)
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM convert_sources s`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("统计源文件行失败: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+convertSourceColumns+convertSourceFrom+where+
		` ORDER BY s.last_activity_at DESC, s.id DESC LIMIT ? OFFSET ?`, append(args, limit, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("查询源文件行失败: %w", err)
	}
	defer rows.Close()
	out := []ConvertSource{}
	for rows.Next() {
		src, err := scanConvertSource(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, src)
	}
	return out, total, rows.Err()
}

// MatchedSourceTaskIDs 返回该行里输出文件名（output_name_key）包含 keyword 的 convert 记录 id（创建时间倒序，最多 limit 个）。
func (s *Store) MatchedSourceTaskIDs(ctx context.Context, sourceID, keyword string, limit int) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM tasks WHERE source_id = ? AND type = 'convert' AND instr(output_name_key, ?) > 0
		ORDER BY created_at DESC, id DESC LIMIT ?`, sourceID, keyword, limit)
	if err != nil {
		return nil, fmt.Errorf("查询记录失败: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ListSourceTasks 列出某个源文件行的转换记录（created_at 倒序、id 倒序），不受 hidden_in_task_center 影响。
func (s *Store) ListSourceTasks(ctx context.Context, sourceID string, limit, offset int) (TaskPage, error) {
	const where = ` WHERE source_id = ? AND type = 'convert'`
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks`+where, sourceID).Scan(&total); err != nil {
		return TaskPage{}, fmt.Errorf("统计记录失败: %w", err)
	}
	page := TaskPage{Items: []Task{}, Total: total}
	if limit <= 0 {
		return page, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+taskColumns+` FROM tasks`+where+` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`,
		sourceID, limit, offset)
	if err != nil {
		return TaskPage{}, fmt.Errorf("查询记录失败: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return TaskPage{}, err
		}
		page.Items = append(page.Items, t)
	}
	return page, rows.Err()
}

// SourceTaskIDs 返回某个源文件行的全部转换记录 id（DeleteSource 用）。
func (s *Store) SourceTaskIDs(ctx context.Context, sourceID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM tasks WHERE source_id = ? ORDER BY created_at DESC, id DESC`, sourceID)
	if err != nil {
		return nil, fmt.Errorf("查询记录失败: %w", err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// MediaByPathKey 按 path_key 读 media 表（ConvertSource.media 没有持久化结果时的退回，契约 6.14.2 / v0.23.4），没有返回 nil。
// 和 ListRecent 一样 hasVideo / hasAudio 按编码是否为空推出；没有采样率、声道、流信息，thumbUrl 为空。
func (s *Store) MediaByPathKey(ctx context.Context, key string) (*MediaInfo, error) {
	var m MediaInfo
	err := s.db.QueryRowContext(ctx, `SELECT id, path, name, size, duration, width, height, video_codec, audio_codec, bitrate, probed_at
		FROM media WHERE path_key = ?`, key).Scan(&m.ID, &m.Path, &m.Name, &m.Size, &m.Duration, &m.Width, &m.Height,
		&m.VideoCodec, &m.AudioCodec, &m.Bitrate, &m.ProbedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取媒体信息失败: %w", err)
	}
	m.HasVideo = m.VideoCodec != ""
	m.HasAudio = m.AudioCodec != ""
	m.FillCodecNames()
	return &m, nil
}

const backfillBatch = 500

// BackfillConvertSources 回填 v0.23 之前的 convert 任务（契约 6.14.8）：按 inputPaths[0] 规范化后的 path_key 建源文件行并写 source_id，
// 同时补 output_name_key。幂等（只处理 type='convert' AND source_id IS NULL 的行和 output_path <> ” AND output_name_key = ” 的行），
// 每批 500 条一个事务。返回本次写了 source_id 的任务数和补了 output_name_key 的任务数。
func (s *Store) BackfillConvertSources(ctx context.Context) (sources int, names int, err error) {
	for {
		n, err := s.backfillSourceBatch(ctx)
		sources += n
		if err != nil {
			return sources, names, err
		}
		if n < backfillBatch {
			break
		}
	}
	for {
		n, err := s.backfillNameKeyBatch(ctx)
		names += n
		if err != nil {
			return sources, names, err
		}
		if n < backfillBatch {
			break
		}
	}
	return sources, names, nil
}

func (s *Store) backfillSourceBatch(ctx context.Context) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	type row struct {
		id, inputs, title string
		created           int64
	}
	rows, err := tx.QueryContext(ctx, `SELECT id, input_paths, title, created_at FROM tasks
		WHERE type = 'convert' AND source_id IS NULL ORDER BY created_at ASC, id ASC LIMIT ?`, backfillBatch)
	if err != nil {
		return 0, fmt.Errorf("查询待回填任务失败: %w", err)
	}
	var batch []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.inputs, &r.title, &r.created); err != nil {
			rows.Close()
			return 0, err
		}
		batch = append(batch, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, r := range batch {
		var inputs []string
		path, key, name := "", "invalid:"+r.id, r.title
		if json.Unmarshal([]byte(r.inputs), &inputs) == nil && len(inputs) > 0 && inputs[0] != "" {
			if p, k, err := paths.Normalize(inputs[0]); err == nil {
				path, key, name = p, k, filepath.Base(p)
			}
		}
		var srcID string
		var added, last int64
		err := tx.QueryRowContext(ctx, `SELECT id, added_at, last_activity_at FROM convert_sources WHERE path_key = ?`, key).Scan(&srcID, &added, &last)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			srcID = id.New()
			if _, err := tx.ExecContext(ctx, `INSERT INTO convert_sources (id, path, path_key, name, name_key, added_at, last_activity_at)
				VALUES (?,?,?,?,?,?,?)`, srcID, path, key, name, strings.ToLower(name), r.created, r.created); err != nil {
				return 0, fmt.Errorf("回填源文件行失败: %w", err)
			}
		case err != nil:
			return 0, err
		default:
			if r.created < added || r.created > last {
				if _, err := tx.ExecContext(ctx, `UPDATE convert_sources SET added_at = MIN(added_at, ?), last_activity_at = MAX(last_activity_at, ?)
					WHERE id = ?`, r.created, r.created, srcID); err != nil {
					return 0, fmt.Errorf("回填源文件行失败: %w", err)
				}
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE tasks SET source_id = ? WHERE id = ?`, srcID, r.id); err != nil {
			return 0, fmt.Errorf("回填 source_id 失败: %w", err)
		}
	}
	return len(batch), tx.Commit()
}

func (s *Store) backfillNameKeyBatch(ctx context.Context) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id, output_path FROM tasks WHERE output_path <> '' AND output_name_key = '' LIMIT ?`, backfillBatch)
	if err != nil {
		return 0, fmt.Errorf("查询待回填任务失败: %w", err)
	}
	type row struct{ id, out string }
	var batch []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.out); err != nil {
			rows.Close()
			return 0, err
		}
		batch = append(batch, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, r := range batch {
		// NameKey 对非空路径永远非空（filepath.Base 不会返回 ""），所以回填不会原地打转。
		if _, err := tx.ExecContext(ctx, `UPDATE tasks SET output_name_key = ? WHERE id = ?`, NameKey(r.out), r.id); err != nil {
			return 0, fmt.Errorf("回填 output_name_key 失败: %w", err)
		}
	}
	return len(batch), tx.Commit()
}

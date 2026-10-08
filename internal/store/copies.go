package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"FFmpegFree/internal/apperr"
)

// ConvertCopy 是 convert_copies 的一行：一份源文件副本（契约 v0.24，6.15.3）。
type ConvertCopy struct {
	ID              string
	OwnerSourceID   string
	OriginalPath    string
	OriginalPathKey string
	OriginalSize    int64
	OriginalMtimeNs int64
	StoredPath      string
	State           string
	TotalBytes      int64
	CopiedBytes     int64
	Error           *apperr.AppError
	RefCount        int
	PendingDelete   bool
	CreatedAt       int64
	FinishedAt      int64
}

const copyColumns = `id, owner_source_id, original_path, original_path_key, original_size, original_mtime_ns,
	stored_path, state, total_bytes, copied_bytes, error, ref_count, pending_delete, created_at, finished_at`

func scanCopy(r rowScanner) (ConvertCopy, error) {
	var c ConvertCopy
	var errJSON sql.NullString
	var finished sql.NullInt64
	var pending int
	if err := r.Scan(&c.ID, &c.OwnerSourceID, &c.OriginalPath, &c.OriginalPathKey, &c.OriginalSize, &c.OriginalMtimeNs,
		&c.StoredPath, &c.State, &c.TotalBytes, &c.CopiedBytes, &errJSON, &c.RefCount, &pending, &c.CreatedAt, &finished); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ConvertCopy{}, err
		}
		return ConvertCopy{}, fmt.Errorf("读取副本失败: %w", err)
	}
	c.PendingDelete = pending != 0
	c.FinishedAt = finished.Int64
	if errJSON.Valid && errJSON.String != "" {
		var ae apperr.AppError
		if json.Unmarshal([]byte(errJSON.String), &ae) == nil {
			c.Error = &ae
		}
	}
	return c, nil
}

type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func getCopy(ctx context.Context, q queryer, id string) (ConvertCopy, error) {
	return scanCopy(q.QueryRowContext(ctx, `SELECT `+copyColumns+` FROM convert_copies WHERE id = ?`, id))
}

// GetCopy 按 id 读副本，不存在返回 sql.ErrNoRows。
func (s *Store) GetCopy(ctx context.Context, id string) (ConvertCopy, error) {
	return getCopy(ctx, s.db, id)
}

// derefCopyTx 把副本的 ref_count − 1（不低于 0），返回减过之后的行。
func derefCopyTx(ctx context.Context, tx *sql.Tx, copyID string) (*ConvertCopy, error) {
	if _, err := tx.ExecContext(ctx, `UPDATE convert_copies SET ref_count = MAX(ref_count - 1, 0) WHERE id = ?`, copyID); err != nil {
		return nil, fmt.Errorf("更新副本引用失败: %w", err)
	}
	c, err := getCopy(ctx, tx, copyID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// switchCopyTx 让源文件行改为引用 newCopyID（ref_count + 1），解除对旧副本的引用（ref_count − 1）。
// 返回旧副本减过之后的行（没有旧副本或新旧相同时 nil）。
func switchCopyTx(ctx context.Context, tx *sql.Tx, sourceID, newCopyID string) (*ConvertCopy, error) {
	var old sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT copy_id FROM convert_sources WHERE id = ?`, sourceID).Scan(&old); err != nil {
		return nil, err
	}
	if old.Valid && old.String == newCopyID {
		return nil, nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE convert_sources SET copy_id = ? WHERE id = ?`, newCopyID, sourceID); err != nil {
		return nil, fmt.Errorf("更新源文件行失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE convert_copies SET ref_count = ref_count + 1 WHERE id = ?`, newCopyID); err != nil {
		return nil, fmt.Errorf("更新副本引用失败: %w", err)
	}
	if old.Valid && old.String != "" {
		return derefCopyTx(ctx, tx, old.String)
	}
	return nil, nil
}

// InsertCopyForSource 新建一份副本并让 sourceID 这一行引用它（ref_count = 1），同一事务里解除对旧副本的引用。
// 返回旧副本减过之后的行（可能 nil）；ref_count 为 0 时调用方删旧副本（契约 6.15.4 第 2.4 条）。
func (s *Store) InsertCopyForSource(ctx context.Context, c ConvertCopy, sourceID string) (*ConvertCopy, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	errJSON, err := encodeAppError(c.Error)
	if err != nil {
		return nil, err
	}
	var finished any
	if c.FinishedAt > 0 {
		finished = c.FinishedAt
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO convert_copies (`+copyColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,0,0,?,?)`,
		c.ID, c.OwnerSourceID, c.OriginalPath, c.OriginalPathKey, c.OriginalSize, c.OriginalMtimeNs,
		c.StoredPath, c.State, c.TotalBytes, c.CopiedBytes, errJSON, c.CreatedAt, finished); err != nil {
		return nil, fmt.Errorf("新建副本失败: %w", err)
	}
	old, err := switchCopyTx(ctx, tx, sourceID, c.ID)
	if err != nil {
		return nil, err
	}
	return old, tx.Commit()
}

// AttachCopy 让 sourceID 这一行引用已有的副本（ref_count + 1），解除对旧副本的引用；返回旧副本减过之后的行。
func (s *Store) AttachCopy(ctx context.Context, sourceID, copyID string) (*ConvertCopy, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	old, err := switchCopyTx(ctx, tx, sourceID, copyID)
	if err != nil {
		return nil, err
	}
	return old, tx.Commit()
}

func (s *Store) listCopies(ctx context.Context, where string, args ...any) ([]ConvertCopy, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+copyColumns+` FROM convert_copies WHERE `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("查询副本失败: %w", err)
	}
	defer rows.Close()
	var out []ConvertCopy
	for rows.Next() {
		c, err := scanCopy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// FindCopiesByIdentity 返回与原文件“是同一个文件”（path_key、大小、修改时间都相同）的 copying / ready 副本（6.15.4 第 2.3 条）。
func (s *Store) FindCopiesByIdentity(ctx context.Context, key string, size, mtimeNs int64) ([]ConvertCopy, error) {
	return s.listCopies(ctx, `original_path_key = ? AND original_size = ? AND original_mtime_ns = ? AND state IN ('copying','ready') AND pending_delete = 0
		ORDER BY created_at ASC`, key, size, mtimeNs)
}

// FindReadyCopiesByStoredPath 返回 stored_path 与 p 相同（SQLite lower 只对 ASCII 不区分大小写的粗筛，调用方再按 path_key 精确比较）的 ready 副本。
func (s *Store) FindReadyCopiesByStoredPath(ctx context.Context, p string) ([]ConvertCopy, error) {
	return s.listCopies(ctx, `state = 'ready' AND pending_delete = 0 AND (stored_path = ? OR lower(stored_path) = lower(?))`, p, p)
}

// ListCopiesByState 返回某个状态的全部副本。
func (s *Store) ListCopiesByState(ctx context.Context, state string) ([]ConvertCopy, error) {
	return s.listCopies(ctx, `state = ? ORDER BY created_at ASC`, state)
}

// ListPendingDeleteCopies 返回等下次启动再删的副本（pending_delete=1，6.15.7 第 4 条）。
func (s *Store) ListPendingDeleteCopies(ctx context.Context) ([]ConvertCopy, error) {
	return s.listCopies(ctx, `pending_delete = 1 OR ref_count = 0 ORDER BY created_at ASC`)
}

// UpdateCopyState 更新副本的状态、已复制字节、错误；finishedAt 为 0 时不写结束时间。
func (s *Store) UpdateCopyState(ctx context.Context, id, state string, copied int64, e *apperr.AppError, finishedAt int64) error {
	errJSON, err := encodeAppError(e)
	if err != nil {
		return err
	}
	var finished any
	if finishedAt > 0 {
		finished = finishedAt
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE convert_copies SET state = ?, copied_bytes = ?, error = ?, finished_at = ? WHERE id = ?`,
		state, copied, errJSON, finished, id); err != nil {
		return fmt.Errorf("更新副本失败: %w", err)
	}
	return nil
}

// DeleteCopyRow 删除副本行（文件由调用方处理）。
func (s *Store) DeleteCopyRow(ctx context.Context, id string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM convert_copies WHERE id = ?`, id); err != nil {
		return fmt.Errorf("删除副本失败: %w", err)
	}
	return nil
}

// MarkCopyPendingDelete 标记副本等下次启动再删（文件删不掉时，6.15.7 第 4 条）。
func (s *Store) MarkCopyPendingDelete(ctx context.Context, id string) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE convert_copies SET pending_delete = 1, ref_count = 0 WHERE id = ?`, id); err != nil {
		return fmt.Errorf("更新副本失败: %w", err)
	}
	return nil
}

// SourceIDsByCopy 返回引用这份副本的源文件行 id（convert:copy 事件每行各发一条）。
func (s *Store) SourceIDsByCopy(ctx context.Context, copyID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM convert_sources WHERE copy_id = ? ORDER BY id`, copyID)
	if err != nil {
		return nil, fmt.Errorf("查询源文件行失败: %w", err)
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

// RecountCopyRefs 按 convert_sources.copy_id 重算全部副本的 ref_count（每次启动做一次自愈，6.15.5）。
func (s *Store) RecountCopyRefs(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE convert_copies SET ref_count = (SELECT COUNT(*) FROM convert_sources s WHERE s.copy_id = convert_copies.id)`)
	if err != nil {
		return fmt.Errorf("重算副本引用失败: %w", err)
	}
	return nil
}

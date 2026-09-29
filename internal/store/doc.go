package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// DocRecent 是 doc_recent 表的一行（最近打开的 PDF）。
type DocRecent struct {
	ID       string `json:"id"`
	Path     string `json:"path"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	OpenedAt int64  `json:"openedAt"`
}

const (
	// DefaultDocRecentKeep 是 doc_recent 表保留的最近记录数。
	DefaultDocRecentKeep = 1000
	defaultDocListLimit  = 20
	maxDocListLimit      = 200
)

// UpsertDocRecent 按 path_key 写入：同一个文件再次打开保留原来的 id，只更新名称、大小和打开时间。
// pathKey 由调用方用 paths.Normalize 生成。写入与清理在同一事务里，只保留最近 DefaultDocRecentKeep 条。
func (s *Store) UpsertDocRecent(ctx context.Context, pathKey string, r DocRecent) (DocRecent, error) {
	if r.OpenedAt == 0 {
		r.OpenedAt = time.Now().UnixMilli()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return DocRecent{}, err
	}
	defer tx.Rollback()
	err = tx.QueryRowContext(ctx, `
		INSERT INTO doc_recent (id, path, path_key, name, size, opened_at) VALUES (?,?,?,?,?,?)
		ON CONFLICT(path_key) DO UPDATE SET
			path=excluded.path, name=excluded.name, size=excluded.size, opened_at=excluded.opened_at
		RETURNING id`,
		r.ID, r.Path, pathKey, r.Name, r.Size, r.OpenedAt).Scan(&r.ID)
	if err != nil {
		return DocRecent{}, fmt.Errorf("写入最近 PDF 记录失败: %w", err)
	}
	keep := s.docKeep.Load()
	if keep <= 0 {
		keep = DefaultDocRecentKeep
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM doc_recent WHERE id NOT IN (
			SELECT id FROM doc_recent ORDER BY opened_at DESC, id DESC LIMIT ?)`, keep); err != nil {
		return DocRecent{}, fmt.Errorf("清理旧的最近 PDF 记录失败: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return DocRecent{}, err
	}
	return r, nil
}

// ListDocRecent 按打开时间倒序返回最近的 PDF 记录。limit 默认 20，最大 200。
func (s *Store) ListDocRecent(ctx context.Context, limit int) ([]DocRecent, error) {
	if limit <= 0 {
		limit = defaultDocListLimit
	}
	if limit > maxDocListLimit {
		limit = maxDocListLimit
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, path, name, size, opened_at FROM doc_recent ORDER BY opened_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("查询最近 PDF 记录失败: %w", err)
	}
	defer rows.Close()
	out := []DocRecent{}
	for rows.Next() {
		var r DocRecent
		if err := rows.Scan(&r.ID, &r.Path, &r.Name, &r.Size, &r.OpenedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// DeleteDocRecent 删除记录（不碰文件），不存在的 id 忽略；返回被删记录的 path_key，供调用方撤销对应的句柄。
func (s *Store) DeleteDocRecent(ctx context.Context, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var keys []string
	for _, id := range ids {
		var k string
		err := tx.QueryRowContext(ctx, `DELETE FROM doc_recent WHERE id = ? RETURNING path_key`, id).Scan(&k)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return nil, fmt.Errorf("删除最近 PDF 记录失败: %w", err)
		}
		keys = append(keys, k)
	}
	return keys, tx.Commit()
}

// SetDocRecentKeep 修改 doc_recent 保留条数（<=0 恢复默认）。测试用。
func (s *Store) SetDocRecentKeep(n int) { s.docKeep.Store(int64(n)) }

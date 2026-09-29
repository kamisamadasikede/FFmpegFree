package store

import (
	"context"
	"database/sql"
	"fmt"
)

// EditProjectRow 是 edit_projects 表的一行。Project 是工程 JSON，store 不解析它（由 edit 服务负责）。
type EditProjectRow struct {
	ID          string
	Name        string
	Project     string
	DurationSec float64
	ClipCount   int
	UpdatedAt   int64
}

// EditProjectMetaRow 是列表用的摘要（不含工程 JSON）。
type EditProjectMetaRow struct {
	ID          string
	Name        string
	DurationSec float64
	ClipCount   int
	UpdatedAt   int64
}

const (
	defaultEditProjectLimit = 50
	maxEditProjectLimit     = 200
)

// SaveEditProject 插入（insert=true）或更新工程；更新不存在的 id 返回 sql.ErrNoRows。
func (s *Store) SaveEditProject(ctx context.Context, r EditProjectRow, insert bool) error {
	if insert {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO edit_projects (id, name, project, duration_sec, clip_count, updated_at) VALUES (?,?,?,?,?,?)`,
			r.ID, r.Name, r.Project, r.DurationSec, r.ClipCount, r.UpdatedAt)
		if err != nil {
			return fmt.Errorf("保存工程失败: %w", err)
		}
		return nil
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE edit_projects SET name=?, project=?, duration_sec=?, clip_count=?, updated_at=? WHERE id=?`,
		r.Name, r.Project, r.DurationSec, r.ClipCount, r.UpdatedAt, r.ID)
	if err != nil {
		return fmt.Errorf("保存工程失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// GetEditProject 读取一个工程，不存在返回 sql.ErrNoRows。
func (s *Store) GetEditProject(ctx context.Context, id string) (EditProjectRow, error) {
	var r EditProjectRow
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, project, duration_sec, clip_count, updated_at FROM edit_projects WHERE id = ?`, id).
		Scan(&r.ID, &r.Name, &r.Project, &r.DurationSec, &r.ClipCount, &r.UpdatedAt)
	return r, err
}

// ListEditProjects 按 updated_at 倒序返回摘要。limit 默认 50，最大 200。
func (s *Store) ListEditProjects(ctx context.Context, limit int) ([]EditProjectMetaRow, error) {
	if limit <= 0 {
		limit = defaultEditProjectLimit
	}
	if limit > maxEditProjectLimit {
		limit = maxEditProjectLimit
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, duration_sec, clip_count, updated_at FROM edit_projects
		ORDER BY updated_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("查询工程失败: %w", err)
	}
	defer rows.Close()
	out := []EditProjectMetaRow{}
	for rows.Next() {
		var m EditProjectMetaRow
		if err := rows.Scan(&m.ID, &m.Name, &m.DurationSec, &m.ClipCount, &m.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// DeleteEditProject 删除工程，不存在返回 sql.ErrNoRows。
func (s *Store) DeleteEditProject(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM edit_projects WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("删除工程失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

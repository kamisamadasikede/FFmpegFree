package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"FFmpegFree/internal/id"
)

// CatProjectRow 是 cat_projects 的一行（契约 6.19.10.7）。missing 不落库，由服务层实时计算。
type CatProjectRow struct {
	ID        string
	Name      string
	Path      string
	PathKey   string
	CreatedAt int64
	UpdatedAt int64
}

const catProjectColumns = `id, name, path, path_key, created_at, updated_at`

func scanCatProject(r interface{ Scan(...any) error }) (CatProjectRow, error) {
	var p CatProjectRow
	err := r.Scan(&p.ID, &p.Name, &p.Path, &p.PathKey, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

// InsertOrGetCatProject 按 path_key 去重插入（6.19.10.2 第 2 条 / 6.19.10.7）：
// 撞唯一键时不新建、不改名，读出已有行并返回 existed=true；并发同时创建同一路径也只会有一行。
func (s *Store) InsertOrGetCatProject(ctx context.Context, p CatProjectRow) (CatProjectRow, bool, error) {
	if p.PathKey == "" {
		return CatProjectRow{}, false, fmt.Errorf("项目 path_key 为空")
	}
	if p.ID == "" {
		p.ID = id.New()
	}
	if p.CreatedAt == 0 {
		p.CreatedAt = time.Now().UnixMilli()
	}
	if p.UpdatedAt == 0 {
		p.UpdatedAt = p.CreatedAt
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO cat_projects (id, name, path, path_key, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(path_key) DO NOTHING`, p.ID, p.Name, p.Path, p.PathKey, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return CatProjectRow{}, false, fmt.Errorf("写入 Cat 项目失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 1 {
		return p, false, nil
	}
	got, err := scanCatProject(s.db.QueryRowContext(ctx, `SELECT `+catProjectColumns+` FROM cat_projects WHERE path_key = ?`, p.PathKey))
	if err != nil {
		return CatProjectRow{}, false, fmt.Errorf("读取已有 Cat 项目失败: %w", err)
	}
	return got, true, nil
}

// GetCatProject 按 id 取项目；不存在返回 sql.ErrNoRows。
func (s *Store) GetCatProject(ctx context.Context, projectID string) (CatProjectRow, error) {
	p, err := scanCatProject(s.db.QueryRowContext(ctx, `SELECT `+catProjectColumns+` FROM cat_projects WHERE id = ?`, projectID))
	if errors.Is(err, sql.ErrNoRows) {
		return CatProjectRow{}, err
	}
	if err != nil {
		return CatProjectRow{}, fmt.Errorf("读取 Cat 项目失败: %w", err)
	}
	return p, nil
}

// ListCatProjects 按“最近对话活动”倒序（契约 6.19.10.6）：活动时间 = 其下对话 updated_at 最大值，
// 没有对话用项目 created_at；相同按 created_at 倒序，再相同按 id 倒序。项目 updated_at 不参与排序。
func (s *Store) ListCatProjects(ctx context.Context) ([]CatProjectRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id, p.name, p.path, p.path_key, p.created_at, p.updated_at
		FROM cat_projects p
		LEFT JOIN (
			SELECT project_id, MAX(updated_at) AS act FROM cat_conversations
			WHERE project_id IS NOT NULL GROUP BY project_id
		) a ON a.project_id = p.id
		ORDER BY COALESCE(a.act, p.created_at) DESC, p.created_at DESC, p.id DESC`)
	if err != nil {
		return nil, fmt.Errorf("列出 Cat 项目失败: %w", err)
	}
	defer rows.Close()
	out := []CatProjectRow{}
	for rows.Next() {
		p, err := scanCatProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// RenameCatProject 只改名字和 updated_at；不存在返回 sql.ErrNoRows。
func (s *Store) RenameCatProject(ctx context.Context, projectID, name string) (CatProjectRow, error) {
	now := time.Now().UnixMilli()
	res, err := s.db.ExecContext(ctx, `UPDATE cat_projects SET name = ?, updated_at = ? WHERE id = ?`, name, now, projectID)
	if err != nil {
		return CatProjectRow{}, fmt.Errorf("项目改名失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return CatProjectRow{}, sql.ErrNoRows
	}
	return s.GetCatProject(ctx, projectID)
}

// CatConversationIDsByProject 列出项目下所有对话 id（删除项目前取消进行中的一轮用）。
func (s *Store) CatConversationIDsByProject(ctx context.Context, projectID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM cat_conversations WHERE project_id = ?`, projectID)
	if err != nil {
		return nil, fmt.Errorf("列出项目对话失败: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// DeleteCatProject 在一个显式事务里删除：项目下所有对话的消息 → 这些对话 → 项目这一行（6.19.10.2 第 4 条）。
// 任何一步失败整体回滚。不依赖外键级联；只动库，绝不访问用户文件夹。项目不存在返回 deleted=false、nil。
func (s *Store) DeleteCatProject(ctx context.Context, projectID string) (deleted bool, err error) {
	return s.deleteCatProject(ctx, projectID, nil)
}

// deleteCatProject 的 hook 只给测试注入中途失败用（验证回滚）。
func (s *Store) deleteCatProject(ctx context.Context, projectID string, hook func(step string) error) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("开始删除项目事务失败: %w", err)
	}
	defer tx.Rollback()
	step := func(name string) error {
		if hook != nil {
			return hook(name)
		}
		return nil
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM cat_messages WHERE conversation_id IN (SELECT id FROM cat_conversations WHERE project_id = ?)`, projectID); err != nil {
		return false, fmt.Errorf("删除项目消息失败: %w", err)
	}
	if err := step("messages"); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM cat_conversations WHERE project_id = ?`, projectID); err != nil {
		return false, fmt.Errorf("删除项目对话失败: %w", err)
	}
	if err := step("conversations"); err != nil {
		return false, err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM cat_projects WHERE id = ?`, projectID)
	if err != nil {
		return false, fmt.Errorf("删除项目失败: %w", err)
	}
	if err := step("project"); err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("提交删除项目事务失败: %w", err)
	}
	return n > 0, nil
}

// GetCatProjectByKey 按 path_key 取项目；不存在返回 sql.ErrNoRows。
func (s *Store) GetCatProjectByKey(ctx context.Context, key string) (CatProjectRow, error) {
	p, err := scanCatProject(s.db.QueryRowContext(ctx, `SELECT `+catProjectColumns+` FROM cat_projects WHERE path_key = ?`, key))
	if errors.Is(err, sql.ErrNoRows) {
		return CatProjectRow{}, err
	}
	if err != nil {
		return CatProjectRow{}, fmt.Errorf("读取 Cat 项目失败: %w", err)
	}
	return p, nil
}

// ErrCatProjectDuplicate 表示新 path_key 已属于另一个项目（RelocateCatProject 撞 UNIQUE）。
var ErrCatProjectDuplicate = errors.New("cat project path_key duplicate")

// RelocateCatProject 只改这一行的 path、path_key、updated_at（契约 v0.31.1，6.19.10.2 第 8 条第 2 款）；
// name 不变，不碰 cat_conversations / cat_messages。不存在返回 sql.ErrNoRows；撞唯一键返回 ErrCatProjectDuplicate。
func (s *Store) RelocateCatProject(ctx context.Context, projectID, path, key string) (CatProjectRow, error) {
	now := time.Now().UnixMilli()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CatProjectRow{}, fmt.Errorf("开始换项目文件夹事务失败: %w", err)
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE cat_projects SET path = ?, path_key = ?, updated_at = ? WHERE id = ?`, path, key, now, projectID)
	if err != nil {
		if strings.Contains(strings.ToUpper(err.Error()), "UNIQUE") {
			return CatProjectRow{}, ErrCatProjectDuplicate
		}
		return CatProjectRow{}, fmt.Errorf("换项目文件夹失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return CatProjectRow{}, sql.ErrNoRows
	}
	if err := tx.Commit(); err != nil {
		return CatProjectRow{}, fmt.Errorf("提交换项目文件夹失败: %w", err)
	}
	return s.GetCatProject(ctx, projectID)
}

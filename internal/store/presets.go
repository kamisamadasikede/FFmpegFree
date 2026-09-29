package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// PresetRow 是 presets 表的一行。Options 是 ConvertOptions 的 JSON，store 不解析它（由 convert 服务负责）。
type PresetRow struct {
	ID      string
	Name    string
	BuiltIn bool
	Options string
	Sort    int
}

// ErrPresetBuiltIn 表示试图修改或删除内置预设。
var ErrPresetBuiltIn = errors.New("内置预设不能修改或删除")

// SeedPresets 写入内置预设：不存在则插入，已存在（内置的）则更新名称、参数和排序，
// 这样升级版本时调整内置预设能生效；同 id 的用户预设不会被覆盖。
func (s *Store) SeedPresets(ctx context.Context, rows []PresetRow) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, r := range rows {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO presets (id, name, built_in, options, sort) VALUES (?,?,1,?,?)
			ON CONFLICT(id) DO UPDATE SET name=excluded.name, options=excluded.options, sort=excluded.sort
			WHERE presets.built_in = 1`, r.ID, r.Name, r.Options, r.Sort); err != nil {
			return fmt.Errorf("写入内置预设失败: %w", err)
		}
	}
	return tx.Commit()
}

// ListPresets 按 sort、name 排序返回全部预设（内置在前）。
func (s *Store) ListPresets(ctx context.Context) ([]PresetRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, built_in, options, sort FROM presets ORDER BY built_in DESC, sort, name, id`)
	if err != nil {
		return nil, fmt.Errorf("查询预设失败: %w", err)
	}
	defer rows.Close()
	out := []PresetRow{}
	for rows.Next() {
		var r PresetRow
		var b int
		if err := rows.Scan(&r.ID, &r.Name, &b, &r.Options, &r.Sort); err != nil {
			return nil, err
		}
		r.BuiltIn = b != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetPreset 读取一个预设，不存在返回 sql.ErrNoRows。
func (s *Store) GetPreset(ctx context.Context, id string) (PresetRow, error) {
	var r PresetRow
	var b int
	err := s.db.QueryRowContext(ctx, `SELECT id, name, built_in, options, sort FROM presets WHERE id = ?`, id).
		Scan(&r.ID, &r.Name, &b, &r.Options, &r.Sort)
	r.BuiltIn = b != 0
	return r, err
}

// CountUserPresets 返回用户自建预设的数量。
func (s *Store) CountUserPresets(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM presets WHERE built_in = 0`).Scan(&n)
	return n, err
}

// SaveUserPreset 插入或更新用户预设。id 对应内置预设时返回 ErrPresetBuiltIn；更新不存在的 id 返回 sql.ErrNoRows。
// insert 为 true 时是新建（id 已存在则失败）。sort 新建时取当前最大值 + 1。
func (s *Store) SaveUserPreset(ctx context.Context, r PresetRow, insert bool) (PresetRow, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PresetRow{}, err
	}
	defer tx.Rollback()
	if insert {
		var max sql.NullInt64
		if err := tx.QueryRowContext(ctx, `SELECT MAX(sort) FROM presets WHERE built_in = 0`).Scan(&max); err != nil {
			return PresetRow{}, err
		}
		r.Sort, r.BuiltIn = int(max.Int64)+1, false
		if _, err := tx.ExecContext(ctx, `INSERT INTO presets (id, name, built_in, options, sort) VALUES (?,?,0,?,?)`,
			r.ID, r.Name, r.Options, r.Sort); err != nil {
			return PresetRow{}, fmt.Errorf("保存预设失败: %w", err)
		}
		return r, tx.Commit()
	}
	var builtIn int
	var sort int
	if err := tx.QueryRowContext(ctx, `SELECT built_in, sort FROM presets WHERE id = ?`, r.ID).Scan(&builtIn, &sort); err != nil {
		return PresetRow{}, err
	}
	if builtIn != 0 {
		return PresetRow{}, ErrPresetBuiltIn
	}
	r.Sort, r.BuiltIn = sort, false
	if _, err := tx.ExecContext(ctx, `UPDATE presets SET name = ?, options = ? WHERE id = ?`, r.Name, r.Options, r.ID); err != nil {
		return PresetRow{}, fmt.Errorf("保存预设失败: %w", err)
	}
	return r, tx.Commit()
}

// DeleteUserPreset 删除用户预设。内置预设返回 ErrPresetBuiltIn，不存在返回 sql.ErrNoRows。
func (s *Store) DeleteUserPreset(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM presets WHERE id = ? AND built_in = 0`, id)
	if err != nil {
		return fmt.Errorf("删除预设失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 1 {
		return nil
	}
	var b int
	if err := s.db.QueryRowContext(ctx, `SELECT built_in FROM presets WHERE id = ?`, id).Scan(&b); err != nil {
		return err
	}
	return ErrPresetBuiltIn
}

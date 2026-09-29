package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// GetSetting 读取 settings 表里的一项，value 按 JSON 解码进 dst。
// 键不存在时返回 found=false，dst 保持不变。
func (s *Store) GetSetting(ctx context.Context, key string, dst any) (found bool, err error) {
	var raw string
	err = s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("读取设置 %s 失败: %w", key, err)
	}
	if err := json.Unmarshal([]byte(raw), dst); err != nil {
		return false, fmt.Errorf("解析设置 %s 失败: %w", key, err)
	}
	return true, nil
}

// SetSetting 把 v 按 JSON 编码后写入 settings 表（存在则覆盖）。
func (s *Store) SetSetting(ctx context.Context, key string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("编码设置 %s 失败: %w", key, err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, string(b))
	if err != nil {
		return fmt.Errorf("写入设置 %s 失败: %w", key, err)
	}
	return nil
}

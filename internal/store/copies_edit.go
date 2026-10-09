package store

import (
	"context"
	"fmt"
)

// UpdateCopyIdentity 更新副本登记的原文件大小和修改时间（文档编辑保存成功后，契约 6.12.41）。
func (s *Store) UpdateCopyIdentity(ctx context.Context, id string, size, mtimeNs int64) error {
	res, err := s.db.ExecContext(ctx, `UPDATE convert_copies SET original_size = ?, original_mtime_ns = ?, total_bytes = ?, copied_bytes = ? WHERE id = ?`,
		size, mtimeNs, size, size, id)
	if err != nil {
		return fmt.Errorf("更新副本指纹失败: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("副本不存在: %s", id)
	}
	return nil
}

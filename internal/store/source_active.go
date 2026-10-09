package store

import (
	"context"
	"fmt"
)

// SourceHasActiveTasks 判断 sourceID 是否有排队中 / 运行中的任务（文档编辑用，契约 6.12.41）。
func (s *Store) SourceHasActiveTasks(ctx context.Context, sourceID string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM tasks WHERE source_id = ? AND status IN ('queued','running')`, sourceID).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("查询进行中任务失败: %w", err)
	}
	return n > 0, nil
}

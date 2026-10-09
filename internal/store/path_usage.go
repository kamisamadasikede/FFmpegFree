package store

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"FFmpegFree/internal/paths"
)

// PathUsage 是某个路径被记录占用的情况（文档另存为目标检查，契约 6.12.42 / 6.12.49）。
type PathUsage struct {
	Converting bool // 有排队中 / 运行中的任务以它为输入或输出，或它是正在重转的记录的输出
	InUse      bool // 是某个源文件行的原文件、某条记录的输出或某个副本
}

func pathKeyOf(p string) string {
	_, k, err := paths.Normalize(p)
	if err != nil {
		return ""
	}
	return k
}

func likeSuffix(base string) string {
	return "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(base) + "%"
}

// TargetPathUsage 查 target 是否已被源文件行 / 记录（输入或输出）/ 副本用着。
// 用文件名做 LIKE 粗筛，再在 Go 里按 paths.Normalize 的 key 精确比较。
func (s *Store) TargetPathUsage(ctx context.Context, target string) (PathUsage, error) {
	return s.TargetPathUsageExcept(ctx, target, "", "")
}

// TargetPathUsageExcept 同 TargetPathUsage，但 ownSourceID 那一行的原文件、ownTaskID 那条记录的输出不算 InUse
// （另存为写回正在编辑的这一行自己，按覆盖保存处理，架构师定 6.12.42）。Converting 不排除：自己这一行正在转换也要拒绝。
func (s *Store) TargetPathUsageExcept(ctx context.Context, target, ownSourceID, ownTaskID string) (PathUsage, error) {
	var u PathUsage
	key := pathKeyOf(target)
	if key == "" {
		return u, nil
	}
	base := filepath.Base(target)

	// 1) 源文件行的原文件
	srows, err := s.db.QueryContext(ctx, `SELECT id FROM convert_sources WHERE path_key = ?`, key)
	if err != nil {
		return u, fmt.Errorf("查询源文件行失败: %w", err)
	}
	var srcIDs []string
	for srows.Next() {
		var id string
		if err := srows.Scan(&id); err != nil {
			srows.Close()
			return u, err
		}
		srcIDs = append(srcIDs, id)
	}
	srows.Close()
	if err := srows.Err(); err != nil {
		return u, err
	}
	for _, srcID := range srcIDs {
		if ownSourceID == "" || srcID != ownSourceID {
			u.InUse = true
		}
		if has, e := s.SourceHasActiveTasks(ctx, srcID); e == nil && has {
			u.Converting = true
		}
	}

	// 2) 任务的输入 / 输出
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, status, reconverting, output_path, input_paths FROM tasks
		 WHERE (output_path <> '' AND output_path LIKE ? ESCAPE '\') OR input_paths LIKE ? ESCAPE '\' LIMIT 1000`,
		likeSuffix(base), likeSuffix(base))
	if err != nil {
		return u, fmt.Errorf("查询任务路径失败: %w", err)
	}
	for rows.Next() {
		var id, status, out, inputs string
		var reconv int
		if err := rows.Scan(&id, &status, &reconv, &out, &inputs); err != nil {
			rows.Close()
			return u, err
		}
		active := status == "queued" || status == "running"
		if out != "" && pathKeyOf(out) == key {
			if ownTaskID == "" || id != ownTaskID {
				u.InUse = true
			}
			if active || reconv != 0 {
				u.Converting = true
			}
		}
		var ins []string
		_ = json.Unmarshal([]byte(inputs), &ins)
		for _, in := range ins {
			if pathKeyOf(in) == key && (active || reconv != 0) {
				u.Converting = true
			}
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return u, err
	}

	// 3) 副本文件
	cps, err := s.listCopies(ctx, `pending_delete = 0 AND (stored_path = ? OR lower(stored_path) = lower(?))`, target, target)
	if err != nil {
		return u, err
	}
	for _, c := range cps {
		if pathKeyOf(c.StoredPath) == key {
			u.InUse = true
		}
	}
	return u, nil
}

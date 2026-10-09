package convert

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/store"
	"FFmpegFree/internal/task"
)

// GetDocSource 返回文档页的一行（kind=doc）；不存在 reason=record。
func (s *Service) GetDocSource(ctx context.Context, id string) (store.ConvertSource, error) {
	_, ss, err := s.records()
	if err != nil {
		return store.ConvertSource{}, err
	}
	src, err := ss.GetConvertSource(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return store.ConvertSource{}, apperr.New(apperr.NotFound, "找不到这条记录").WithDetail("reason=record")
	}
	if err != nil {
		return store.ConvertSource{}, apperr.Wrap(apperr.Internal, "读取源文件行失败", err)
	}
	if src.Kind != store.SourceKindDoc {
		return store.ConvertSource{}, apperr.New(apperr.Unsupported, "不支持这种记录").WithDetail("reason=format")
	}
	return src, nil
}

// SourceHasActiveTasks 文档编辑用。
func (s *Service) SourceHasActiveTasks(ctx context.Context, sourceID string) (bool, error) {
	st, ok := s.cfg.Sources.(*store.Store)
	if !ok {
		return false, nil
	}
	return st.SourceHasActiveTasks(ctx, sourceID)
}

// UpdateDocCopyIdentity 更新副本指纹。
func (s *Service) UpdateDocCopyIdentity(ctx context.Context, copyID string, size, mtimeNs int64) error {
	st, ok := s.cfg.Sources.(*store.Store)
	if !ok || copyID == "" {
		return nil
	}
	return st.UpdateCopyIdentity(ctx, copyID, size, mtimeNs)
}

// MarkDocCopyFailed 副本写失败。
func (s *Service) MarkDocCopyFailed(ctx context.Context, copyID string, e *apperr.AppError) error {
	st, ok := s.cfg.Sources.(*store.Store)
	if !ok || copyID == "" {
		return nil
	}
	return st.UpdateCopyState(ctx, copyID, store.CopyFailed, 0, e, time.Now().UnixMilli())
}

// UpdateDocTaskResultSize 更新成功记录的 result.sizeBytes 并通知前端。
func (s *Service) UpdateDocTaskResultSize(ctx context.Context, taskID string, size int64) error {
	if s.cfg.Tasks == nil {
		return nil
	}
	if m, ok := s.cfg.Tasks.(*task.Manager); ok {
		return m.NotifyResultSize(taskID, size)
	}
	return nil
}

// DocTargetPathUsage 文档另存为目标检查（6.12.42）：目标是否正在转换、是否被别的记录用着
// （ownSourceID / ownTaskID 是正在编辑的这一行，它自己的原文件 / 输出不算被别的记录用着）。
func (s *Service) DocTargetPathUsage(ctx context.Context, target, ownSourceID, ownTaskID string) (converting, inUse bool, err error) {
	st, ok := s.cfg.Sources.(*store.Store)
	if !ok {
		return false, false, nil
	}
	u, err := st.TargetPathUsageExcept(ctx, target, ownSourceID, ownTaskID)
	return u.Converting, u.InUse, err
}

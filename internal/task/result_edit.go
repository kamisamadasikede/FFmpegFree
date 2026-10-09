package task

import (
	"context"

	"FFmpegFree/internal/apperr"
)

// NotifyResultSize 文档编辑保存成功后更新成功任务的 result.sizeBytes：version+1、落库、发 task:status。
func (m *Manager) NotifyResultSize(taskID string, size int64) error {
	t, err := m.cfg.Store.GetTask(context.Background(), taskID)
	if err != nil {
		return apperr.New(apperr.NotFound, "找不到这条记录").WithDetail("reason=record")
	}
	if t.Result == nil {
		t.Result = &TaskResult{}
	}
	t.Result.SizeBytes = size
	t.Version++
	prog := t.Progress
	if err := m.cfg.Store.UpdateTask(context.Background(), t); err != nil {
		return apperr.Wrap(apperr.Internal, "更新记录失败", err)
	}
	m.emit(EventStatus, StatusEvent{
		ID: t.ID, Version: t.Version, Status: t.Status,
		OutputPath: t.OutputPath, Progress: &prog, Result: t.Result,
		Reconverting: rcFlag(t),
	})
	return nil
}

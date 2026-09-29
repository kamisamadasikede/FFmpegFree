package task

import (
	"context"
	"os"
	"strings"

	"FFmpegFree/internal/apperr"
)

// Remove 删除已结束任务的记录（同时清理日志文件），发 task:removed。deleteOutput 为 true 时
// 还会删除成功任务的输出文件。只要 ids 里有一个仍在排队或运行，整个调用失败（TASK_CONFLICT），什么也不删。
// 不存在的 ID 静默忽略。
func (m *Manager) Remove(ids []string, deleteOutput bool) error {
	if len(ids) == 0 {
		return nil
	}
	ctx := context.Background()
	m.mu.Lock()
	for _, id := range ids {
		if _, active := m.entries[id]; active {
			m.mu.Unlock()
			return apperr.New(apperr.TaskConflict, "任务仍在进行，请先取消")
		}
	}
	m.mu.Unlock()

	// 先取出记录，用于清理日志和输出文件。
	var recs []Task
	for _, id := range ids {
		if t, err := m.cfg.Store.GetTask(ctx, id); err == nil {
			recs = append(recs, t)
		}
	}
	deleted, err := m.cfg.Store.DeleteTasks(ctx, ids)
	if err != nil {
		return apperr.Wrap(apperr.IOError, "删除任务失败", err)
	}
	gone := map[string]bool{}
	for _, id := range deleted {
		gone[id] = true
	}
	for _, t := range recs {
		if !gone[t.ID] {
			continue
		}
		m.cleanupFiles(t, deleteOutput)
	}
	if len(deleted) > 0 {
		m.emit(EventRemoved, RemovedEvent{IDs: deleted})
	}
	return nil
}

// ClearFinished 删除所有已结束（成功、失败、取消、中断）的任务记录和日志，发 task:removed。不删除输出文件。
func (m *Manager) ClearFinished() error {
	gone, err := m.cfg.Store.DeleteFinishedTasks(context.Background())
	if err != nil {
		return apperr.Wrap(apperr.IOError, "清理任务失败", err)
	}
	if len(gone) == 0 {
		return nil
	}
	ids := make([]string, 0, len(gone))
	for _, t := range gone {
		ids = append(ids, t.ID)
		m.cleanupFiles(t, false)
	}
	m.emit(EventRemoved, RemovedEvent{IDs: ids})
	return nil
}

func (m *Manager) cleanupFiles(t Task, deleteOutput bool) {
	if t.LogPath != "" {
		os.Remove(t.LogPath)
	}
	if deleteOutput && t.Status == StatusSucceeded && t.OutputPath != "" {
		if fi, err := os.Stat(t.OutputPath); err == nil && fi.Mode().IsRegular() {
			os.Remove(t.OutputPath)
		}
	}
}

// GetLog 返回任务日志的最后 tailLines 行（<=0 表示全部，单次最多读取末尾 1 MB）。日志文件不存在返回空串。
func (m *Manager) GetLog(taskID string, tailLines int) (string, error) {
	t, err := m.Get(taskID)
	if err != nil {
		return "", err
	}
	if t.LogPath == "" {
		return "", nil
	}
	f, err := os.Open(t.LogPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", apperr.Wrap(apperr.IOError, "读取日志失败", err)
	}
	defer f.Close()
	const maxRead = 1 << 20
	fi, err := f.Stat()
	if err != nil {
		return "", apperr.Wrap(apperr.IOError, "读取日志失败", err)
	}
	off := int64(0)
	if fi.Size() > maxRead {
		off = fi.Size() - maxRead
	}
	buf := make([]byte, fi.Size()-off)
	if _, err := f.ReadAt(buf, off); err != nil && err.Error() != "EOF" {
		return "", apperr.Wrap(apperr.IOError, "读取日志失败", err)
	}
	s := string(buf)
	if off > 0 { // 丢掉被截断的第一行
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			s = s[i+1:]
		}
	}
	if tailLines <= 0 {
		return s, nil
	}
	s = strings.TrimRight(s, "\n")
	lines := strings.Split(s, "\n")
	if len(lines) > tailLines {
		lines = lines[len(lines)-tailLines:]
	}
	return strings.Join(lines, "\n"), nil
}

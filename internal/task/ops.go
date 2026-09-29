package task

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	var problems []string
	for _, t := range recs {
		if !gone[t.ID] {
			continue
		}
		problems = append(problems, m.cleanupFiles(t, deleteOutput)...)
	}
	if len(deleted) > 0 {
		m.emit(EventRemoved, RemovedEvent{IDs: deleted})
	}
	if len(problems) > 0 {
		// 任务记录已经删除；只是部分文件没能删掉，如实告诉调用方。
		return apperr.New(apperr.IOError, "任务记录已删除，但有文件没能删除").WithDetail(strings.Join(problems, "\n"))
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
		for _, p := range m.cleanupFiles(t, false) {
			m.logf("清理任务 %s 的日志失败: %s", t.ID, p)
		}
	}
	m.emit(EventRemoved, RemovedEvent{IDs: ids})
	return nil
}

// cleanupFiles 删除任务日志（含轮转的 .1），以及（可选）任务的输出文件；返回没能删除的文件说明。
func (m *Manager) cleanupFiles(t Task, deleteOutput bool) (problems []string) {
	rm := func(p string) {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			problems = append(problems, fmt.Sprintf("%s: %v", p, err))
		}
	}
	// 日志只删日志目录里的文件（记录里的路径来自数据库，不盲目信任）。
	if t.LogPath != "" && m.insideLogDir(t.LogPath) {
		rm(t.LogPath)
		rm(rotatedPath(t.LogPath))
	}
	if deleteOutput {
		if reason := unsafeToDeleteOutput(t); reason != "" {
			if reason != "skip" {
				m.logf("不删除任务 %s 的输出 %s：%s", t.ID, t.OutputPath, reason)
			}
		} else {
			rm(t.OutputPath)
		}
	}
	return problems
}

func (m *Manager) insideLogDir(p string) bool {
	if m.cfg.LogDir == "" {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(m.cfg.LogDir), filepath.Clean(p))
	return err == nil && rel != "." && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)
}

// unsafeToDeleteOutput 判断为什么不能删除该任务的输出文件（返回空串表示可以删）：
//   - 只删成功任务的输出（"skip" = 静默跳过）；
//   - 输出路径等于某个输入路径（原地处理）不删，避免删掉用户的源文件；
//   - 符号链接不删（也不跟随）；只删普通文件；
//   - 文件的修改时间早于任务开始时间：不是这个任务写出来的（例如后来被用户换成了别的文件），不删。
func unsafeToDeleteOutput(t Task) string {
	if t.Status != StatusSucceeded || t.OutputPath == "" {
		return "skip"
	}
	out := filepath.Clean(t.OutputPath)
	for _, in := range t.InputPaths {
		if sameFilePath(out, filepath.Clean(in)) {
			return "输出与输入是同一个文件"
		}
	}
	fi, err := os.Lstat(out)
	if err != nil {
		return "skip"
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return "输出是符号链接"
	}
	if !fi.Mode().IsRegular() {
		return "输出不是普通文件"
	}
	// 文件系统时间戳精度比毫秒粗（内核时钟缓存最多差几毫秒，FAT 是 2 秒），留 3 秒容差。
	if t.StartedAt > 0 && fi.ModTime().UnixMilli() < t.StartedAt-mtimeSlackMs {
		return "文件修改时间早于任务开始，不是该任务生成的"
	}
	return ""
}

// mtimeSlackMs 是比较文件修改时间与任务开始时间时的容差。
const mtimeSlackMs = 3000

func sameFilePath(a, b string) bool {
	if nameKey(a) == nameKey(b) {
		return true
	}
	ia, e1 := os.Stat(a)
	ib, e2 := os.Stat(b)
	return e1 == nil && e2 == nil && os.SameFile(ia, ib)
}

// GetLog 返回任务日志的最后 tailLines 行（<=0 表示全部，最多读取末尾 1 MB，必要时接上轮转出去的旧日志）。
// 日志文件不存在返回空串。
func (m *Manager) GetLog(taskID string, tailLines int) (string, error) {
	t, err := m.Get(taskID)
	if err != nil {
		return "", err
	}
	if t.LogPath == "" || !m.insideLogDir(t.LogPath) {
		return "", nil
	}
	const maxRead = 1 << 20
	cur, curTrunc, err := readTail(t.LogPath, maxRead)
	if err != nil {
		return "", apperr.Wrap(apperr.IOError, "读取日志失败", err)
	}
	s := cur
	if !curTrunc && len(cur) < maxRead {
		if old, oldTrunc, err := readTail(rotatedPath(t.LogPath), int64(maxRead-len(cur))); err == nil && old != "" {
			s = old + cur
			curTrunc = oldTrunc
		}
	}
	if curTrunc { // 丢掉被截断的第一行
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

// readTail 读文件末尾最多 max 字节；truncated 表示文件比读到的更长。文件不存在返回空串。
func readTail(path string, max int64) (s string, truncated bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return "", false, err
	}
	off := int64(0)
	if fi.Size() > max {
		off = fi.Size() - max
		truncated = true
	}
	buf := make([]byte, fi.Size()-off)
	if _, err := f.ReadAt(buf, off); err != nil && !errors.Is(err, io.EOF) {
		return "", false, err
	}
	return string(buf), truncated, nil
}

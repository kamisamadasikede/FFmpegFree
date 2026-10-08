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
	"FFmpegFree/internal/store"
)

// LegacyFinder 由能识别旧类型记录的 Store 实现（*store.Store）。
type LegacyFinder interface {
	LegacyTaskIDs(ctx context.Context, ids []string) ([]string, error)
}

var _ LegacyFinder = (*store.Store)(nil)

// Remove 删除已结束任务的记录（同时清理日志文件），发 task:removed。deleteOutput 为 true 时
// 还会删除成功任务的输出文件。只要 ids 里有一个仍在排队或运行，整个调用失败（TASK_CONFLICT），什么也不删。
// 不存在的 ID 静默忽略。
func (m *Manager) Remove(ids []string, deleteOutput bool) error {
	if len(ids) == 0 {
		return nil
	}
	ctx := context.Background()
	// 旧类型（"保留但不再产生"）的记录按不存在处理：整体 NOT_FOUND，什么也不删（记录、日志、输出都不碰）。
	// 真正不存在的 id 仍然忽略，这里只拦库里有记录、类型是旧类型的 id。
	if lf, ok := m.cfg.Store.(LegacyFinder); ok {
		legacy, err := lf.LegacyTaskIDs(ctx, ids)
		if err != nil {
			return apperr.Wrap(apperr.IOError, "读取任务失败", err)
		}
		if len(legacy) > 0 {
			return apperr.New(apperr.NotFound, "任务不存在")
		}
	}
	// 契约 v0.23：转换记录的真删只在转换页（ConvertService.DeleteRecords / DeleteSource），ids 里有 convert 任务整体拒绝。
	for _, id := range ids {
		if t, err := m.Get(id); err == nil && t.Type == TypeConvert {
			return apperr.New(apperr.InvalidArgument, "转换记录请在格式转换页删除")
		}
	}
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

// ClearFinished 已废弃（契约 v0.23）：等同 HideFinishedInTaskCenter，不再删除任何记录、日志或文件。
func (m *Manager) ClearFinished() error {
	_, err := m.HideFinishedInTaskCenter()
	return err
}

// HideFinishedInTaskCenter 把所有类型、所有已结束且未隐藏的任务设为在任务中心隐藏，返回本次隐藏的条数（契约 v0.23）。
// 不删记录、不删日志、不删文件，version 不变，不发事件。
func (m *Manager) HideFinishedInTaskCenter() (int64, error) {
	n, err := m.cfg.Store.HideFinishedTasks(context.Background())
	if err != nil {
		return 0, apperr.Wrap(apperr.Internal, "隐藏任务失败", err)
	}
	return n, nil
}

// maxBatchIDs 是 v0.23 批量 id 接口（UnhideInTaskCenter、CheckPaths、DeleteRecords 等）一次最多的 id 数。
const maxBatchIDs = 500

// reasonRecord / reasonFile 是 6.14 接口 NOT_FOUND 的 detail（契约 2.2）。
const (
	reasonRecord = "reason=record"
	reasonFile   = "reason=file"
)

// RecordNotFound 是 6.14 接口“记录不存在 / 旧类型”的错误（NOT_FOUND，reason=record）。
func RecordNotFound() error {
	return apperr.New(apperr.NotFound, "记录不存在").WithDetail(reasonRecord)
}

// FileNotFound 是 6.14 接口“记录在，但登记的文件已不存在或不是普通文件”的错误（NOT_FOUND，reason=file）。
func FileNotFound() error {
	return apperr.New(apperr.NotFound, "文件不存在").WithDetail(reasonFile)
}

// UnhideInTaskCenter 取消隐藏（契约 v0.23）：1~500 个 id，先整体校验（任一不存在 / 旧类型 NOT_FOUND reason=record，什么都不改），
// 再把其中已隐藏的清成未隐藏、version +1、各发一次 task:status（status 不变，带 hiddenInTaskCenter: false）。幂等。
func (m *Manager) UnhideInTaskCenter(ids []string) error {
	if len(ids) == 0 || len(ids) > maxBatchIDs {
		return apperr.New(apperr.InvalidArgument, fmt.Sprintf("一次取消隐藏 1~%d 个任务", maxBatchIDs))
	}
	ids = dedupe(ids)
	for _, id := range ids {
		if _, err := m.Get(id); err != nil {
			if apperr.Is(err, apperr.NotFound) {
				return RecordNotFound()
			}
			return apperr.Wrap(apperr.Internal, "读取任务失败", err)
		}
	}
	changed, err := m.cfg.Store.UnhideTasks(context.Background(), ids)
	if err != nil {
		return apperr.Wrap(apperr.Internal, "取消隐藏失败", err)
	}
	notHidden := false
	for _, t := range changed {
		m.emit(EventStatus, StatusEvent{ID: t.ID, Version: t.Version, Status: t.Status, HiddenInTaskCenter: &notHidden})
	}
	return nil
}

func dedupe(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// TaskPathCheck 是 CheckPaths 的一项（契约 6.14.2）。
type TaskPathCheck struct {
	TaskID       string `json:"taskId"`
	Found        bool   `json:"found"`
	InputExists  bool   `json:"inputExists"`
	OutputExists bool   `json:"outputExists"`
}

// CheckPaths 检查任务登记的输入 / 输出文件现在是否还在（契约 v0.23）：1~500 个，结果与入参一一对应；
// 不存在 / 旧类型的 id 不报错（found=false）。只有“不存在”算 false，其他 stat 错误算 true。
func (m *Manager) CheckPaths(ids []string) ([]TaskPathCheck, error) {
	if len(ids) == 0 || len(ids) > maxBatchIDs {
		return nil, apperr.New(apperr.InvalidArgument, fmt.Sprintf("一次检查 1~%d 个任务", maxBatchIDs))
	}
	out := make([]TaskPathCheck, len(ids))
	for i, id := range ids {
		out[i].TaskID = id
		t, err := m.Get(id)
		if err != nil {
			if apperr.Is(err, apperr.NotFound) {
				continue
			}
			return nil, apperr.Wrap(apperr.Internal, "读取任务失败", err)
		}
		out[i].Found = true
		if len(t.InputPaths) > 0 && t.InputPaths[0] != "" {
			out[i].InputExists = RegularExists(t.InputPaths[0], true)
		}
		if t.Status == StatusSucceeded && t.OutputPath != "" && filepath.IsAbs(t.OutputPath) {
			out[i].OutputExists = RegularExists(t.OutputPath, false)
		}
	}
	return out, nil
}

// RegularExists 判断 p 现在是不是普通文件：只有“不存在”（或存在但不是普通文件 / 是符号链接而 follow=false）算 false，
// 其他 stat 错误（如无权限）算 true，交给后续操作报错（契约 6.14.3 CheckSources / CheckPaths）。
func RegularExists(p string, follow bool) bool {
	stat := os.Lstat
	if follow {
		stat = os.Stat
	}
	fi, err := stat(p)
	if err != nil {
		return !errors.Is(err, os.ErrNotExist)
	}
	return fi.Mode().IsRegular()
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
//   - Runner 返回的输出路径必须是绝对路径，且所在目录（含上级）不能含符号链接（EvalSymlinks 后必须不变）；
//   - 输出路径等于某个输入路径（原地处理）不删，避免删掉用户的源文件；
//   - 符号链接不删（也不跟随）；只删普通文件；
//   - 文件的修改时间早于任务开始时间：不是这个任务写出来的（例如后来被用户换成了别的文件），不删。
func unsafeToDeleteOutput(t Task) string {
	if t.Status != StatusSucceeded || t.OutputPath == "" {
		return "skip"
	}
	// Runner 返回的路径不可信：必须是绝对路径（相对路径会按进程工作目录解析，可能删到别处）。
	if !filepath.IsAbs(t.OutputPath) {
		return "输出路径不是绝对路径"
	}
	out := filepath.Clean(t.OutputPath)
	// 输出所在目录（含上级）里有符号链接时不信任：链接可能是后来换上的，删除会落到链接另一端。
	// 做法：解析真实目录，必须与记录的目录逐字一致；叶子文件仍用下面的 Lstat（符号链接本身不删）。
	// 代价：输出目录本身经过符号链接（如 ~/Videos → /mnt/data）时不会自动删除文件（只记日志，记录照常删除）。
	realDir, err := filepath.EvalSymlinks(filepath.Dir(out))
	if err != nil {
		return "skip" // 目录已不存在：文件也不在了
	}
	if nameKey(realDir) != nameKey(filepath.Dir(out)) {
		return "输出所在目录含符号链接，路径不可信"
	}
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

// OutputFinder 是 Store 的可选能力：按文件名粗筛任务表里登记的输出路径（*store.Store 实现）。
type OutputFinder interface {
	TaskOutputsByBase(ctx context.Context, base string) ([]string, error)
}

var _ OutputFinder = (*store.Store)(nil)

// IsTaskOutput 判断 path 是不是任务表里登记的输出路径。比较的是 EvalSymlinks 之后的真实路径
// （Windows / macOS 不区分大小写），所以用任务里登记的路径或它的真实路径都能匹配；
// path 本身是符号链接时一律返回 false（不信任被换成链接的输出）。path 必须是绝对路径。
func (m *Manager) IsTaskOutput(path string) bool {
	if path == "" || !filepath.IsAbs(path) {
		return false
	}
	path = filepath.Clean(path)
	if fi, err := os.Lstat(path); err != nil || fi.Mode()&os.ModeSymlink != 0 {
		return false
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	key := nameKey(real)
	var cands []string
	if f, ok := m.cfg.Store.(OutputFinder); ok {
		list, err := f.TaskOutputsByBase(context.Background(), filepath.Base(path))
		if err != nil {
			m.logf("查询任务输出失败: %v", err)
			return false
		}
		cands = list
	}
	m.mu.Lock()
	for _, e := range m.entries {
		if e.task.OutputPath != "" {
			cands = append(cands, e.task.OutputPath)
		}
	}
	m.mu.Unlock()
	for _, c := range cands {
		if !filepath.IsAbs(c) {
			continue
		}
		if nameKey(c) == nameKey(path) {
			return true
		}
		if rc, err := filepath.EvalSymlinks(c); err == nil && nameKey(rc) == key {
			return true
		}
	}
	return false
}

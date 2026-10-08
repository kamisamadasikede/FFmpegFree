package convert

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/id"
	"FFmpegFree/internal/paths"
	"FFmpegFree/internal/store"
	"FFmpegFree/internal/task"
)

// ---------- 源文件副本（契约 v0.24，6.15.3 ~ 6.15.7） ----------

// EventCopy 是副本复制进度 / 状态事件（契约第 5 节、6.15.4 第 5 条）。
const EventCopy = "convert:copy"

// CopyEvent 是 convert:copy 的 payload。seq 进程内全局递增，前端按 sourceId 丢弃更小的。
type CopyEvent struct {
	SourceID    string           `json:"sourceId"`
	Seq         int64            `json:"seq"`
	CopyState   string           `json:"copyState"`
	CopiedBytes int64            `json:"copiedBytes"`
	TotalBytes  int64            `json:"totalBytes"`
	StoredPath  string           `json:"storedPath"`
	Error       *apperr.AppError `json:"error,omitempty"`
}

// CopyStore 是副本的持久化能力（*store.Store 实现）。
type CopyStore interface {
	GetCopy(ctx context.Context, id string) (store.ConvertCopy, error)
	InsertCopyForSource(ctx context.Context, c store.ConvertCopy, sourceID string) (*store.ConvertCopy, error)
	AttachCopy(ctx context.Context, sourceID, copyID string) (*store.ConvertCopy, error)
	FindCopiesByIdentity(ctx context.Context, key string, size, mtimeNs int64) ([]store.ConvertCopy, error)
	FindReadyCopiesByStoredPath(ctx context.Context, p string) ([]store.ConvertCopy, error)
	ListCopiesByState(ctx context.Context, state string) ([]store.ConvertCopy, error)
	ListPendingDeleteCopies(ctx context.Context) ([]store.ConvertCopy, error)
	UpdateCopyState(ctx context.Context, id, state string, copied int64, e *apperr.AppError, finishedAt int64) error
	DeleteCopyRow(ctx context.Context, id string) error
	MarkCopyPendingDelete(ctx context.Context, id string) error
	SourceIDsByCopy(ctx context.Context, copyID string) ([]string, error)
	RecountCopyRefs(ctx context.Context) error
}

const (
	copyReserve     = 64 << 20 // 磁盘空间检查的预留（64 MiB）
	copyBufSize     = 1 << 20  // 1 MiB 缓冲
	copyConcurrency = 2        // 同时最多 2 个
	// copyMtimeSlack 是比较副本修改时间与原文件修改时间的容差：Chtimes 写回的时间受上传目录所在文件系统的精度限制
	// （HFS+ 1 秒、FAT 2 秒），逐纳秒比较会把我们自己写出的副本误判为“不是我们的文件”（v0.24.1 实现取舍）。
	copyMtimeSlack = 2 * time.Second
)

// humanBytes 按 1024 进位、最多 1 位小数、去掉 .0（契约 6.15.4 的 PM 文案）。
func humanBytes(n int64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	f, i := float64(n), 0
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d B", n)
	}
	s := strconv.FormatFloat(math.Round(f*10)/10, 'f', 1, 64)
	return strings.TrimSuffix(s, ".0") + " " + units[i]
}

// diskFullError 是 CONVERT_DISK_FULL（reason=no_space，detail 三行）。
func diskFullError(need, free int64) *apperr.AppError {
	return apperr.New(apperr.ConvertDiskFull, fmt.Sprintf("磁盘空间不足，需要 %s，剩余 %s。", humanBytes(need), humanBytes(free))).
		WithDetail(fmt.Sprintf("reason=no_space\nneedBytes=%d\nfreeBytes=%d", need, free))
}

func copyIOError() *apperr.AppError { return apperr.New(apperr.IOError, "复制文件失败") }
func copyGoneError() *apperr.AppError {
	return apperr.New(apperr.NotFound, "原文件不存在").WithDetail("reason=file")
}
func sourceChangedError() *apperr.AppError {
	return apperr.New(apperr.IOError, "复制期间原文件被修改了，请重试").WithDetail("reason=source_changed")
}
func interruptedCopyError() *apperr.AppError {
	return apperr.New(apperr.IOError, "复制被中断，请重试").WithDetail("reason=interrupted")
}

// copyPartPath 是副本的临时文件：<原文件名去扩展名>.part.<原扩展名>；没有扩展名时 <原文件名>.part（同 6.5）。
func copyPartPath(stored string) string { return task.PartPath(stored) }

// existingAncestor 返回 dir 或它最近的已存在的上级（查可用空间用：上传目录可能还没建）。
func existingAncestor(dir string) string {
	for d := dir; ; {
		if _, err := os.Stat(d); err == nil {
			return d
		}
		p := filepath.Dir(d)
		if p == d {
			return d
		}
		d = p
	}
}

type copyJob struct {
	c        store.ConvertCopy
	ctx      context.Context
	cancel   context.CancelFunc
	copied   atomic.Int64
	done     chan struct{}
	canceled bool // copier.mu

	emitMu   sync.Mutex
	lastEmit time.Time
	pending  bool
	gen      uint64
}

// copier 是后台复制队列（契约 6.15.4 第 3 条）：同时最多 2 个、先进先出；不是任务。
type copier struct {
	s       *Service
	mu      sync.Mutex
	queue   []*copyJob
	jobs    map[string]*copyJob // copyID → 排队中 / 复制中
	running int
	closing bool
	wg      sync.WaitGroup
	seq     atomic.Int64
}

func newCopier(s *Service) *copier { return &copier{s: s, jobs: map[string]*copyJob{}} }

func (q *copier) enqueue(c store.ConvertCopy) {
	ctx, cancel := context.WithCancel(context.Background())
	j := &copyJob{c: c, ctx: ctx, cancel: cancel, done: make(chan struct{})}
	q.mu.Lock()
	if q.closing {
		q.mu.Unlock()
		cancel()
		return // 退出中：库里仍是 copying，下次启动标成 interrupted
	}
	q.jobs[c.ID] = j
	q.queue = append(q.queue, j)
	q.mu.Unlock()
	q.pump()
}

func (q *copier) pump() {
	for {
		q.mu.Lock()
		if q.closing || q.running >= copyConcurrency || len(q.queue) == 0 {
			q.mu.Unlock()
			return
		}
		j := q.queue[0]
		q.queue = q.queue[1:]
		q.running++
		q.wg.Add(1)
		q.mu.Unlock()
		go func() {
			defer q.wg.Done()
			q.s.runCopy(j)
			q.mu.Lock()
			q.running--
			if q.jobs[j.c.ID] == j {
				delete(q.jobs, j.c.ID)
			}
			q.mu.Unlock()
			close(j.done)
			q.pump()
		}()
	}
}

// pendingBytes 是排队 / 复制中的副本还没写完的字节数之和。
func (q *copier) pendingBytes() int64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	var n int64
	for _, j := range q.jobs {
		if r := j.c.TotalBytes - j.copied.Load(); r > 0 {
			n += r
		}
	}
	return n
}

// live 返回正在复制的副本的实时字节数。
func (q *copier) live(copyID string) (int64, bool) {
	q.mu.Lock()
	j := q.jobs[copyID]
	q.mu.Unlock()
	if j == nil {
		return 0, false
	}
	return j.copied.Load(), true
}

// cancel 停止一份副本的复制：返回 (done, copied, ok)；ok=false 表示它已经不在队列里（刚结束）。
func (q *copier) cancel(copyID string) (<-chan struct{}, int64, bool) {
	q.mu.Lock()
	j := q.jobs[copyID]
	if j == nil {
		q.mu.Unlock()
		return nil, 0, false
	}
	j.canceled = true
	delete(q.jobs, copyID)
	queued := false
	for i, x := range q.queue {
		if x == j {
			q.queue = append(q.queue[:i], q.queue[i+1:]...)
			queued = true
			break
		}
	}
	q.mu.Unlock()
	j.cancel()
	if queued {
		close(j.done)
	}
	return j.done, j.copied.Load(), true
}

// isCanceled 在 copier.mu 下读 canceled；没被取消的从 jobs 里摘掉（之后的 cancel 不再作用于它）。
func (q *copier) settle(j *copyJob) (canceled, closing bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !j.canceled && q.jobs[j.c.ID] == j {
		delete(q.jobs, j.c.ID)
	}
	return j.canceled, q.closing
}

// close 在应用退出时停止全部复制：库里保持 copying，下次启动标成 failed（reason=interrupted），不续传。
func (q *copier) close(timeout time.Duration) {
	q.mu.Lock()
	q.closing = true
	q.queue = nil
	for _, j := range q.jobs {
		j.cancel()
	}
	q.mu.Unlock()
	done := make(chan struct{})
	go func() { q.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(timeout):
	}
}

func (s *Service) copyStore() CopyStore {
	cs, _ := s.cfg.Sources.(CopyStore)
	return cs
}

func (s *Service) copyInterval() time.Duration {
	switch {
	case s.cfg.CopyProgressInterval < 0:
		return 0
	case s.cfg.CopyProgressInterval == 0:
		return 250 * time.Millisecond
	}
	return s.cfg.CopyProgressInterval
}

// emitCopy 给引用这份副本的每一行各发一条 convert:copy。
func (s *Service) emitCopy(c store.ConvertCopy, state string, copied int64, e *apperr.AppError) {
	if s.cfg.Emitter == nil {
		return
	}
	cs := s.copyStore()
	if cs == nil {
		return
	}
	ids, err := cs.SourceIDsByCopy(context.Background(), c.ID)
	if err != nil {
		return
	}
	for _, sid := range ids {
		s.cfg.Emitter.Emit(EventCopy, CopyEvent{SourceID: sid, Seq: s.copier.seq.Add(1), CopyState: state,
			CopiedBytes: copied, TotalBytes: c.TotalBytes, StoredPath: c.StoredPath, Error: e})
	}
}

// progress 是复制中的节流推送（每份副本最多每秒 4 次，被抑制的最后一次在间隔到期后补发）。
func (s *Service) copyProgress(j *copyJob) {
	j.emitMu.Lock()
	defer j.emitMu.Unlock()
	iv := s.copyInterval()
	since := time.Since(j.lastEmit)
	if iv == 0 || since >= iv {
		j.pending = false
		j.gen++
		j.lastEmit = time.Now()
		s.emitCopy(j.c, store.CopyCopying, j.copied.Load(), nil)
		return
	}
	if j.pending {
		return
	}
	j.pending = true
	gen := j.gen
	time.AfterFunc(iv-since, func() {
		j.emitMu.Lock()
		defer j.emitMu.Unlock()
		if !j.pending || j.gen != gen || j.ctx.Err() != nil {
			return
		}
		j.pending = false
		j.gen++
		j.lastEmit = time.Now()
		s.emitCopy(j.c, store.CopyCopying, j.copied.Load(), nil)
	})
}

func (j *copyJob) stopProgress() {
	j.emitMu.Lock()
	j.pending = false
	j.gen++
	j.emitMu.Unlock()
}

// runCopy 复制一份副本（后台）。
func (s *Service) runCopy(j *copyJob) {
	aerr := s.doCopy(j)
	j.stopProgress()
	canceled, closing := s.copier.settle(j)
	part := copyPartPath(j.c.StoredPath)
	if canceled {
		os.Remove(part) // 状态与事件已由 CancelCopy 处理
		return
	}
	if aerr == nil && j.ctx.Err() == nil {
		if err := os.Rename(part, j.c.StoredPath); err != nil {
			s.logf("副本改名失败 %s: %v", j.c.StoredPath, err)
			aerr = copyIOError()
		} else {
			mt := time.Unix(0, j.c.OriginalMtimeNs)
			if err := os.Chtimes(j.c.StoredPath, mt, mt); err != nil {
				s.logf("设置副本修改时间失败 %s: %v", j.c.StoredPath, err)
			}
		}
	}
	if aerr == nil && j.ctx.Err() != nil {
		os.Remove(part)
		if closing {
			return // 应用退出：库里保持 copying，下次启动标成 interrupted
		}
		aerr = interruptedCopyError()
	}
	cs := s.copyStore()
	now := s.cfg.Now()
	if aerr != nil {
		os.Remove(part)
		if closing {
			return
		}
		_ = cs.UpdateCopyState(context.Background(), j.c.ID, store.CopyFailed, j.copied.Load(), aerr, now)
		s.emitCopy(j.c, store.CopyFailed, j.copied.Load(), aerr)
		return
	}
	_ = cs.UpdateCopyState(context.Background(), j.c.ID, store.CopyReady, j.c.TotalBytes, nil, now)
	s.emitCopy(j.c, store.CopyReady, j.c.TotalBytes, nil)
}

// doCopy 把原文件复制到 .part（1 MiB 缓冲、Sync），再核对原文件没被改过。返回 nil 且 ctx 未取消表示 .part 完整。
func (s *Service) doCopy(j *copyJob) *apperr.AppError {
	c := j.c
	if err := os.MkdirAll(filepath.Dir(c.StoredPath), 0o755); err != nil {
		s.logf("创建上传目录失败 %s: %v", filepath.Dir(c.StoredPath), err)
		return s.writeError(err, c)
	}
	in, err := os.Open(c.OriginalPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return copyGoneError()
		}
		s.logf("打开原文件失败 %s: %v", c.OriginalPath, err)
		return copyIOError()
	}
	defer in.Close()
	part := copyPartPath(c.StoredPath)
	out, err := os.OpenFile(part, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		s.logf("创建副本临时文件失败 %s: %v", part, err)
		return s.writeError(err, c)
	}
	closed := false
	defer func() {
		if !closed {
			out.Close()
		}
	}()
	buf := make([]byte, copyBufSize)
	for {
		if j.ctx.Err() != nil {
			return nil
		}
		n, rerr := in.Read(buf)
		if n > 0 {
			w, werr := out.Write(buf[:n])
			j.copied.Add(int64(w))
			if werr != nil {
				s.logf("写入副本失败 %s: %v", part, werr)
				return s.writeError(werr, c)
			}
			s.copyProgress(j)
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			s.logf("读取原文件失败 %s: %v", c.OriginalPath, rerr)
			return copyIOError()
		}
	}
	if err := out.Sync(); err != nil {
		return s.writeError(err, c)
	}
	closed = true
	if err := out.Close(); err != nil {
		return s.writeError(err, c)
	}
	fi, err := os.Stat(c.OriginalPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return copyGoneError()
		}
		return copyIOError()
	}
	if fi.Size() != c.OriginalSize || fi.ModTime().UnixNano() != c.OriginalMtimeNs || j.copied.Load() != c.TotalBytes {
		return sourceChangedError()
	}
	return nil
}

// writeError：写满（ENOSPC / EDQUOT，Windows 112 / 39）→ CONVERT_DISK_FULL；其余 IO_ERROR。
func (s *Service) writeError(err error, c store.ConvertCopy) *apperr.AppError {
	if task.IsDiskFull(err) {
		var free int64
		if f, ferr := s.freeSpace(existingAncestor(filepath.Dir(c.StoredPath))); ferr == nil {
			free = f
		}
		rest := c.TotalBytes
		if j, ok := s.copier.live(c.ID); ok {
			rest -= j
		}
		if rest < 0 {
			rest = 0
		}
		return diskFullError(rest+copyReserve, free)
	}
	return copyIOError()
}

func (s *Service) freeSpace(dir string) (int64, error) {
	if s.cfg.FreeSpace != nil {
		return s.cfg.FreeSpace(dir)
	}
	return diskFree(dir)
}

func (s *Service) logf(format string, args ...any) {
	if s.cfg.Logf != nil {
		s.cfg.Logf(format, args...)
	}
}

func (s *Service) uploadsDir(ctx context.Context) string {
	if s.cfg.UploadsDir == nil {
		return ""
	}
	return s.cfg.UploadsDir(ctx)
}

// copyEnabled 表示添加时要做副本（配置了上传目录、存储支持副本）。
func (s *Service) copyEnabled() bool { return s.cfg.UploadsDir != nil && s.copyStore() != nil }

// storedOK：ready 副本的文件现在是普通文件、大小等于 total_bytes（6.15.4 第 2.2 条）。
func storedOK(c store.ConvertCopy) bool {
	fi, err := os.Lstat(c.StoredPath)
	return err == nil && fi.Mode().IsRegular() && fi.Size() == c.TotalBytes
}

// startCopy 为这一行新做一份副本（6.15.4 第 2.4、3 条）：先查空间，插入副本行并改引用（同一事务解除旧引用），
// 旧副本归零就删，然后排进复制队列并发开始事件；空间不足 / 原文件不在时副本直接是 failed（不是调用错误）。
func (s *Service) startCopy(ctx context.Context, src ConvertSource, fi os.FileInfo, statErr error) error {
	cs := s.copyStore()
	uploads := s.uploadsDir(ctx)
	if cs == nil || uploads == "" || !filepath.IsAbs(uploads) {
		return apperr.New(apperr.IOError, "上传位置不可用，请在设置里重新选择")
	}
	_, key, _ := paths.Normalize(src.Path)
	now := s.cfg.Now()
	c := store.ConvertCopy{ID: id.New(), OwnerSourceID: src.SourceID, OriginalPath: src.Path, OriginalPathKey: key,
		StoredPath: filepath.Join(uploads, src.SourceID, src.Name), CreatedAt: now}
	switch {
	case statErr != nil:
		c.State, c.FinishedAt = store.CopyFailed, now
		if errors.Is(statErr, os.ErrNotExist) {
			c.Error = copyGoneError()
		} else {
			c.Error = copyIOError()
		}
	default:
		c.OriginalSize, c.OriginalMtimeNs, c.TotalBytes = fi.Size(), fi.ModTime().UnixNano(), fi.Size()
		c.State = store.CopyCopying
		need := c.TotalBytes + s.copier.pendingBytes() + copyReserve
		if free, err := s.freeSpace(existingAncestor(uploads)); err == nil && free < need {
			c.State, c.FinishedAt, c.Error = store.CopyFailed, now, diskFullError(need, free)
		}
	}
	old, err := cs.InsertCopyForSource(ctx, c, src.SourceID)
	if err != nil {
		return apperr.Wrap(apperr.Internal, "保存副本失败", err)
	}
	s.releaseCopy(ctx, old, src.SourceID)
	s.emitCopy(c, c.State, 0, c.Error)
	if c.State == store.CopyCopying {
		s.copier.enqueue(c)
	}
	return nil
}

// releaseCopy：解除引用后归零的副本按 6.15.7 删掉（失败只记日志，副本行标 pending_delete 等下次启动）。
func (s *Service) releaseCopy(ctx context.Context, old *store.ConvertCopy, sourceID string) *task.DeleteFailure {
	if old == nil || old.RefCount > 0 {
		return nil
	}
	return s.disposeCopy(ctx, *old, sourceID)
}

// disposeCopy 删一份已没有行引用的副本（6.15.7 第 3、4 条）：只删我们写出的那个文件（普通文件、不是符号链接、
// 大小等于 total_bytes、修改时间等于原文件的），再删 .part 和空的 uploads/<owner>/ 目录，最后删副本行。
// 文件被替换过：不碰文件，返回 not_task_output，照删副本行；删不掉：标 pending_delete，返回 in_use / permission / io。
func (s *Service) disposeCopy(ctx context.Context, c store.ConvertCopy, sourceID string) *task.DeleteFailure {
	cs := s.copyStore()
	if cs == nil {
		return nil
	}
	if done, _, ok := s.copier.cancel(c.ID); ok {
		select {
		case <-done:
		case <-time.After(10 * time.Second):
		}
	}
	st := c.StoredPath
	cleanup := func() {
		os.Remove(copyPartPath(st))
		if dir := filepath.Dir(st); filepath.Base(dir) == c.OwnerSourceID {
			os.Remove(dir) // 只在已经空了时成功
		}
	}
	var fail *task.DeleteFailure
	fi, err := os.Lstat(st)
	switch {
	case st == "" || errors.Is(err, os.ErrNotExist):
	case err != nil:
		f := task.NewCopyDeleteFailure(sourceID, st, task.ClassifyRemoveErr(err))
		fail = &f
	case !fi.Mode().IsRegular() || fi.Size() != c.TotalBytes || absDur(fi.ModTime().UnixNano()-c.OriginalMtimeNs) > copyMtimeSlack:
		f := task.NewCopyDeleteFailure(sourceID, st, task.DeleteNotTaskOutput)
		fail = &f
		s.logf("不删除副本 %s：文件已被替换或移动", st)
		cleanup()
		_ = cs.DeleteCopyRow(ctx, c.ID)
		s.allowReveal(st)
		return fail
	default:
		if s.cfg.Preview != nil {
			s.cfg.Preview.RevokePath(st)
		}
		if err := removeCopyFile(st); err != nil && !errors.Is(err, os.ErrNotExist) {
			s.logf("删除副本 %s 失败: %v", st, err)
			f := task.NewCopyDeleteFailure(sourceID, st, task.ClassifyRemoveErr(err))
			fail = &f
		}
	}
	if fail != nil {
		_ = cs.MarkCopyPendingDelete(ctx, c.ID)
		s.allowReveal(st)
		return fail
	}
	cleanup()
	_ = cs.DeleteCopyRow(ctx, c.ID)
	return nil
}

// removeCopyFile 是删副本文件的入口（测试里替换以模拟“被占用”）。
var removeCopyFile = os.Remove

func absDur(ns int64) time.Duration {
	if ns < 0 {
		ns = -ns
	}
	return time.Duration(ns)
}

func (s *Service) allowReveal(p string) {
	if a, ok := s.cfg.Tasks.(interface{ AllowReveal(string) }); ok {
		a.AllowReveal(p)
	}
}

// recoverCopies 是启动时的副本收尾（6.15.4 第 3 条、6.15.5、6.15.7 第 4 条）：copying → failed（reason=interrupted）并删 .part；
// 重算引用计数；删掉 pending_delete / 已没有行引用的副本。失败只记日志。
func (s *Service) recoverCopies(ctx context.Context) {
	cs := s.copyStore()
	if cs == nil {
		return
	}
	if list, err := cs.ListCopiesByState(ctx, store.CopyCopying); err == nil {
		for _, c := range list {
			os.Remove(copyPartPath(c.StoredPath))
			if err := cs.UpdateCopyState(ctx, c.ID, store.CopyFailed, 0, interruptedCopyError(), s.cfg.Now()); err != nil {
				s.logf("标记中断的复制失败 %s: %v", c.ID, err)
			}
		}
	} else {
		s.logf("读取副本失败: %v", err)
	}
	if err := cs.RecountCopyRefs(ctx); err != nil {
		s.logf("%v", err)
	}
	if list, err := cs.ListPendingDeleteCopies(ctx); err == nil {
		for _, c := range list {
			if c.RefCount == 0 {
				s.disposeCopy(ctx, c, "")
			}
		}
	}
}

// Close 在应用退出时停止复制队列（最多等 timeout）。
func (s *Service) Close(timeout time.Duration) {
	if s.copier != nil {
		s.copier.close(timeout)
	}
}

// liveSource 给正在复制的行填上实时 copiedBytes（6.15.3：进度不落库）。
func (s *Service) liveSource(src ConvertSource) ConvertSource {
	if src.CopyState == store.CopyCopying && src.CopyID != "" {
		if n, ok := s.copier.live(src.CopyID); ok {
			src.CopiedBytes = n
		}
	}
	return src
}

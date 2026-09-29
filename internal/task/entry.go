package task

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"FFmpegFree/internal/apperr"
)

// entry 是一个未结束任务在内存中的状态。
type entry struct {
	m      *Manager
	runner Runner
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	log    *logSink

	mu         sync.Mutex
	task       Task
	cancelReq  bool
	terminal   bool
	lastEmit   time.Time
	pendingP   bool
	flushGen   uint64 // 补发定时器的代数，过期的定时器回调直接忽略
	flushTimer *time.Timer
	outTime    float64
}

func newEntry(m *Manager, t Task, r Runner) *entry {
	ctx, cancel := context.WithCancel(context.Background())
	e := &entry{m: m, runner: r, ctx: ctx, cancel: cancel, done: make(chan struct{}), task: t}
	if m.cfg.LogDir != "" {
		e.log = &logSink{p: filepath.Join(m.cfg.LogDir, t.ID+".log")}
	} else {
		e.log = &logSink{}
	}
	return e
}

func (e *entry) snapshot() Task {
	e.mu.Lock()
	defer e.mu.Unlock()
	t := e.task
	t.InputPaths = append([]string{}, t.InputPaths...)
	return t
}

func (e *entry) markCancelRequested() {
	e.mu.Lock()
	e.cancelReq = true
	e.mu.Unlock()
}

func (e *entry) cancelRequested() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.cancelReq
}

// persist 把当前任务记录写库；失败只记日志，不影响任务本身。调用方持有 e.mu。
func (e *entry) persistLocked() {
	if err := e.m.cfg.Store.UpdateTask(context.Background(), e.task); err != nil {
		e.m.logf("保存任务 %s 状态失败: %v", e.task.ID, err)
	}
}

// start：queued → running。
func (e *entry) start() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.terminal {
		return
	}
	e.task.Status = StatusRunning
	e.task.StartedAt = time.Now().UnixMilli()
	e.task.Version++
	e.persistLocked()
	e.m.emit(EventStatus, StatusEvent{ID: e.task.ID, Version: e.task.Version, Status: StatusRunning})
}

// finish 进入终态：落库并发 task:status。重复调用无效。
func (e *entry) finish(m *Manager, st Status, aerr *apperr.AppError, output string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.terminal {
		return
	}
	e.terminal = true
	if e.flushTimer != nil {
		e.flushTimer.Stop()
	}
	e.task.Status = st
	e.task.Error = aerr
	e.task.FinishedAt = time.Now().UnixMilli()
	e.task.Speed, e.task.EtaSec = "", 0
	if output != "" {
		if filepath.IsAbs(output) {
			e.task.OutputPath = output
		} else {
			// Runner 返回的路径不可信：相对路径会按进程工作目录解析，不采信（保留提交时的预期路径）。
			m.logf("任务 %s 的 Runner 返回了相对路径 %q，已忽略", e.task.ID, output)
		}
	}
	if st == StatusSucceeded && !IsLive(e.task.Type) {
		e.task.Progress = 1
	}
	e.task.Version++
	e.persistLocked()
	m.emit(EventStatus, StatusEvent{
		ID: e.task.ID, Version: e.task.Version, Status: st, Error: aerr,
		OutputPath: e.task.OutputPath, FinishedAt: e.task.FinishedAt,
	})
	e.log.close()
}

// report 是传给 Runner 的进度回调：内存里总是更新，推送按最小间隔节流，被抑制的最后一次会在间隔到期后补发。
func (e *entry) report(p Progress) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.terminal || e.task.Status != StatusRunning {
		return
	}
	if IsLive(e.task.Type) {
		e.task.Progress = -1
	} else {
		f := p.Fraction
		if f < 0 {
			f = 0
		}
		if f > 1 {
			f = 1
		}
		if f < e.task.Progress { // 进度只增不减（out_time=N/A、两遍编码切换时可能给出更小的值）
			f = e.task.Progress
		}
		e.task.Progress = f
	}
	e.task.Speed, e.task.EtaSec = p.Speed, p.EtaSec
	if p.OutTimeSec > e.outTime {
		e.outTime = p.OutTimeSec
	}

	interval := e.m.progressInterval()
	since := time.Since(e.lastEmit)
	if interval == 0 || since >= interval {
		if e.pendingP { // 定时补发还没触发就已经过了间隔：直接发，并作废补发，避免重复
			e.flushTimer.Stop()
			e.pendingP = false
			e.flushGen++
		}
		e.emitProgressLocked()
		return
	}
	if !e.pendingP {
		e.pendingP = true
		gen := e.flushGen
		e.flushTimer = time.AfterFunc(interval-since, func() { e.flush(gen) })
	}
}

func (e *entry) flush(gen uint64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if gen != e.flushGen || !e.pendingP {
		return
	}
	e.pendingP = false
	e.flushGen++
	if e.terminal || e.task.Status != StatusRunning {
		return
	}
	e.emitProgressLocked()
}

func (e *entry) emitProgressLocked() {
	e.task.Version++
	e.lastEmit = time.Now()
	e.m.emit(EventProgress, ProgressEvent{
		ID: e.task.ID, Version: e.task.Version, Progress: e.task.Progress,
		Speed: e.task.Speed, EtaSec: e.task.EtaSec, OutTimeSec: e.outTime,
	})
}

const (
	// 单个任务日志的容量上限：当前文件写满 logRotateBytes 就改名为 <id>.log.1（覆盖旧的 .1）再重新开始，
	// 所以一个任务最多占用 2*logRotateBytes = 16 MB。
	logRotateBytes = 8 << 20
	// 单行最大字节数，超出的部分丢弃并标注（ffmpeg 偶尔会输出没有换行的超长内容）。
	logMaxLine   = 8 << 10
	logTruncMark = "…[行过长，已截断]"
)

// logSink 是任务日志文件，首次写入时才创建。带容量上限（轮转）和单行长度上限。
type logSink struct {
	mu sync.Mutex
	p  string
	f  *os.File
	// size 是当前文件已写字节数；lineLen 是当前行已写字节数；cut 表示当前行已经被截断。
	size    int64
	lineLen int
	cut     bool
	// closed 之后的写入被丢弃。
	closed bool
	// maxBytes 可在测试里调小，0 用 logRotateBytes。
	maxBytes int64
}

func (l *logSink) path() string {
	if l == nil {
		return ""
	}
	return l.p
}

// rotatedPath 是轮转后的旧日志路径。
func rotatedPath(p string) string { return p + ".1" }

func (l *logSink) limit() int64 {
	if l.maxBytes > 0 {
		return l.maxBytes
	}
	return logRotateBytes
}

func (l *logSink) openLocked() bool {
	if l.f != nil {
		return true
	}
	if err := os.MkdirAll(filepath.Dir(l.p), 0o755); err != nil {
		return false
	}
	f, err := os.OpenFile(l.p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return false
	}
	if fi, err := f.Stat(); err == nil {
		l.size = fi.Size()
	}
	l.f = f
	return true
}

func (l *logSink) rotateLocked() {
	if l.f != nil {
		l.f.Close()
		l.f = nil
	}
	os.Remove(rotatedPath(l.p))
	if err := os.Rename(l.p, rotatedPath(l.p)); err != nil {
		os.Remove(l.p)
	}
	l.size = 0
}

func (l *logSink) Write(b []byte) (int, error) {
	if l == nil || l.p == "" {
		return len(b), nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return len(b), nil
	}
	// 逐行处理：每行最多 logMaxLine 字节。
	var out []byte
	for _, c := range b {
		if c == '\n' {
			out = append(out, '\n')
			l.lineLen, l.cut = 0, false
			continue
		}
		if l.cut {
			continue
		}
		if l.lineLen >= logMaxLine {
			out = append(out, logTruncMark...)
			l.cut = true
			continue
		}
		out = append(out, c)
		l.lineLen++
	}
	// 一次写入可能是很大的多行块：按剩余容量拆开写，写满就轮转，保证单个文件永远不超过上限
	// （轮转可能发生在一行中间，GetLog 读取时会丢掉被截断的第一行）。
	for len(out) > 0 {
		if !l.openLocked() {
			return len(b), nil
		}
		limit := l.limit()
		room := limit - l.size
		if room <= 0 {
			if l.size == 0 {
				return len(b), nil // 上限小于 1 字节（不可能），防死循环
			}
			l.rotateLocked()
			continue
		}
		chunk := out
		if int64(len(chunk)) > room {
			chunk = chunk[:room]
		}
		n, err := l.f.Write(chunk)
		l.size += int64(n)
		if err != nil || n == 0 {
			return len(b), nil
		}
		out = out[n:]
		if len(out) > 0 { // 这个文件已满
			l.rotateLocked()
		}
	}
	return len(b), nil
}

func (l *logSink) close() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	if l.f != nil {
		l.f.Close()
		l.f = nil
	}
}

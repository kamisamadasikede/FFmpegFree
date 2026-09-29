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
		e.task.OutputPath = output
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
		e.task.Progress = f
	}
	e.task.Speed, e.task.EtaSec = p.Speed, p.EtaSec
	e.outTime = p.OutTimeSec

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

// logSink 是任务日志文件，首次写入时才创建。
type logSink struct {
	mu sync.Mutex
	p  string
	f  *os.File
	// closed 之后的写入被丢弃。
	closed bool
}

func (l *logSink) path() string {
	if l == nil {
		return ""
	}
	return l.p
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
	if l.f == nil {
		if err := os.MkdirAll(filepath.Dir(l.p), 0o755); err != nil {
			return len(b), nil
		}
		f, err := os.OpenFile(l.p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return len(b), nil
		}
		l.f = f
	}
	return l.f.Write(b)
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

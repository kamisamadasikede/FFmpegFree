package task

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"runtime"
	"sort"
	"sync"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/id"
)

// DefaultBatchConcurrency 是 batch 池默认并发数：CPU 核数的一半，限制在 1~3。
func DefaultBatchConcurrency() int {
	n := runtime.NumCPU() / 2
	if n < 1 {
		return 1
	}
	if n > 3 {
		return 3
	}
	return n
}

// Config 是 Manager 的依赖与参数。
type Config struct {
	Store   Store
	Emitter Emitter // 可为 nil
	// LogDir 是任务日志目录（<数据目录>/logs）。为空则不写日志文件。
	LogDir string
	// BatchConcurrency 是 batch 池并发数，<=0 用 DefaultBatchConcurrency()。
	BatchConcurrency int
	// ProgressInterval 是同一任务 task:progress 的最小间隔，0 表示默认 250ms（每秒最多 4 次），负数表示不限制（测试用）。
	ProgressInterval time.Duration
	// Logf 记录内部错误（落库失败等）；为空用标准 log。
	Logf func(format string, args ...any)
}

// Factory 根据已有任务记录重新构造 Runner，供 Retry 使用（用 Params 重建）。
type Factory func(t Task) (Runner, error)

// Manager 是任务管理器。所有方法并发安全。
type Manager struct {
	cfg Config

	mu        sync.Mutex
	entries   map[string]*entry // 未结束的任务（排队或运行中）
	queue     []*entry          // batch 池排队队列（FIFO）
	running   int               // batch 池正在运行的数量
	limit     int
	factories map[Type]Factory
	closing   bool
	wg        sync.WaitGroup
}

// NewManager 创建任务管理器。
func NewManager(cfg Config) *Manager {
	limit := cfg.BatchConcurrency
	if limit <= 0 {
		limit = DefaultBatchConcurrency()
	}
	return &Manager{cfg: cfg, entries: map[string]*entry{}, limit: limit, factories: map[Type]Factory{}}
}

func (m *Manager) logf(format string, args ...any) {
	if m.cfg.Logf != nil {
		m.cfg.Logf(format, args...)
		return
	}
	log.Printf(format, args...)
}

func (m *Manager) emit(event string, payload any) {
	if m.cfg.Emitter != nil {
		m.cfg.Emitter.Emit(event, payload)
	}
}

func (m *Manager) progressInterval() time.Duration {
	switch {
	case m.cfg.ProgressInterval < 0:
		return 0
	case m.cfg.ProgressInterval == 0:
		return 250 * time.Millisecond
	}
	return m.cfg.ProgressInterval
}

// RegisterFactory 注册某类任务的 Runner 工厂，Retry 依赖它。同一类型重复注册以最后一次为准。
func (m *Manager) RegisterFactory(t Type, f Factory) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.factories[t] = f
}

// BatchConcurrency 返回 batch 池当前并发数。
func (m *Manager) BatchConcurrency() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.limit
}

// SetBatchConcurrency 调整 batch 池并发数（设置里的并发数变化时调用）。调大立即生效，
// 调小不会打断已在运行的任务，只是暂停从队列取新任务直到数量降到新上限以下。
func (m *Manager) SetBatchConcurrency(n int) {
	if n <= 0 {
		n = DefaultBatchConcurrency()
	}
	m.mu.Lock()
	m.limit = n
	m.mu.Unlock()
	m.pump()
}

// Submit 提交任务：落库为 queued、发 task:created，然后交给调度池。
//   - batch 类任务在池里有空位时立即开始，否则排队；
//   - live 类任务不排队，立即开始，不占 batch 名额。
func (m *Manager) Submit(spec Spec, r Runner) (Task, error) {
	if r == nil {
		return Task{}, apperr.New(apperr.InvalidArgument, "任务缺少执行体")
	}
	if !validType(spec.Type) {
		return Task{}, apperr.New(apperr.InvalidArgument, fmt.Sprintf("未知的任务类型 %q", spec.Type))
	}
	m.mu.Lock()
	if m.closing {
		m.mu.Unlock()
		return Task{}, apperr.New(apperr.Internal, "应用正在退出，无法提交新任务")
	}
	m.mu.Unlock()

	taskID := spec.ID
	if taskID == "" {
		taskID = id.New()
	}
	now := time.Now().UnixMilli()
	t := Task{
		ID: taskID, Type: spec.Type, Status: StatusQueued, Title: spec.Title,
		InputPaths: nonNil(spec.InputPaths), OutputPath: spec.OutputPath, Params: spec.Params,
		Version: 1, CreatedAt: now,
	}
	if IsLive(spec.Type) {
		t.Progress = -1 // 契约：直播类任务进度恒为 -1
	}
	e := newEntry(m, t, r)
	t.LogPath = e.log.path()
	e.task = t
	if err := m.cfg.Store.InsertTask(context.Background(), t); err != nil {
		e.log.close()
		return Task{}, apperr.Wrap(apperr.IOError, "保存任务失败", err)
	}

	m.mu.Lock()
	if m.closing {
		m.mu.Unlock()
		// 极端竞态：刚落库就进入退出流程，直接标记中断。
		e.finish(m, StatusInterrupted, nil, "")
		return e.snapshot(), nil
	}
	if _, dup := m.entries[t.ID]; dup {
		m.mu.Unlock()
		return Task{}, apperr.New(apperr.InvalidArgument, "任务 ID 重复")
	}
	m.entries[t.ID] = e
	live := IsLive(spec.Type)
	if !live {
		m.queue = append(m.queue, e)
	}
	m.mu.Unlock()

	m.emit(EventCreated, e.snapshot())
	if live {
		m.launch(e, false)
	} else {
		m.pump()
	}
	return e.snapshot(), nil
}

func validType(t Type) bool {
	switch t {
	case TypeConvert, TypeEditRender, TypeOfficePDF, TypeLiveFilePush, TypeLiveRelay, TypeLiveRecordPush, TypeFFmpegInstall:
		return true
	}
	return false
}

// pump 在 batch 池有空位时从队列取任务启动。
func (m *Manager) pump() {
	for {
		m.mu.Lock()
		if m.closing || m.running >= m.limit || len(m.queue) == 0 {
			m.mu.Unlock()
			return
		}
		e := m.queue[0]
		m.queue = m.queue[1:]
		m.running++
		m.mu.Unlock()
		m.launch(e, true)
	}
}

// launch 在新 goroutine 里执行任务。countBatch 表示占用了 batch 名额，结束时要归还。
func (m *Manager) launch(e *entry, countBatch bool) {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.execute(e)
		m.mu.Lock()
		delete(m.entries, e.task.ID)
		if countBatch {
			m.running--
		}
		m.mu.Unlock()
		close(e.done)
		if countBatch {
			m.pump()
		}
		if f, ok := e.runner.(Finalizer); ok {
			f.OnFinish(e.snapshot())
		}
	}()
}

func (m *Manager) execute(e *entry) {
	if e.ctx.Err() != nil { // 出队前后被取消
		m.finishAfterRun(e, e.ctx.Err(), "")
		return
	}
	e.start()
	ctx := context.WithValue(e.ctx, ctxKey{}, Info{ID: e.task.ID, Type: e.task.Type, log: e.log})
	out, err := safeRun(ctx, e.runner, e.report)
	m.finishAfterRun(e, err, out)
}

func safeRun(ctx context.Context, r Runner, report func(Progress)) (out string, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = apperr.New(apperr.Internal, fmt.Sprintf("任务执行时发生 panic: %v", p))
		}
	}()
	return r.Run(ctx, report)
}

// finishAfterRun 根据 Run 的结果决定终态：
//   - 成功 → succeeded（即使期间收到过取消请求：直播优雅停止就是这种情况，存档已完整）；
//   - 应用退出中 → interrupted；
//   - 被取消 → canceled；
//   - 其他 → failed。
func (m *Manager) finishAfterRun(e *entry, err error, out string) {
	m.mu.Lock()
	closing := m.closing
	m.mu.Unlock()
	switch {
	case err == nil:
		e.finish(m, StatusSucceeded, nil, out)
	case closing && errors.Is(err, context.Canceled):
		e.finish(m, StatusInterrupted, nil, "")
	case e.ctx.Err() != nil && (errors.Is(err, context.Canceled) || e.cancelRequested()):
		e.finish(m, StatusCanceled, nil, "")
	default:
		e.finish(m, StatusFailed, apperr.From(err), "")
	}
}

// Cancel 请求取消任务。
//   - 排队中：直接移出队列并标记 canceled；
//   - 运行中：取消其 ctx，由 Runner 负责停止（ffmpeg 任务结束进程组，直播任务优雅停止），
//     终态在 Runner 返回后异步落定并通过 task:status 通知；
//   - 已结束：TASK_CONFLICT；不存在：NOT_FOUND。
//
// 对已在取消中的任务重复调用是无操作。
func (m *Manager) Cancel(taskID string) error {
	m.mu.Lock()
	e, ok := m.entries[taskID]
	if !ok {
		m.mu.Unlock()
		if _, err := m.cfg.Store.GetTask(context.Background(), taskID); err != nil {
			return notFoundOr(err, "任务不存在")
		}
		return apperr.New(apperr.TaskConflict, "任务已结束，无法取消")
	}
	for i, q := range m.queue {
		if q == e {
			m.queue = append(m.queue[:i], m.queue[i+1:]...)
			m.mu.Unlock()
			e.markCancelRequested()
			e.cancel()
			e.finish(m, StatusCanceled, nil, "")
			m.mu.Lock()
			delete(m.entries, taskID)
			m.mu.Unlock()
			close(e.done)
			if f, ok := e.runner.(Finalizer); ok {
				f.OnFinish(e.snapshot())
			}
			return nil
		}
	}
	m.mu.Unlock()
	e.markCancelRequested()
	e.cancel()
	return nil
}

// Wait 阻塞到任务结束并返回最终状态（任务已结束则立即返回）。ctx 用于放弃等待。
func (m *Manager) Wait(ctx context.Context, taskID string) (Task, error) {
	m.mu.Lock()
	e, ok := m.entries[taskID]
	m.mu.Unlock()
	if ok {
		select {
		case <-e.done:
		case <-ctx.Done():
			return Task{}, ctx.Err()
		}
	}
	return m.Get(taskID)
}

// Shutdown 在应用退出时调用：不再接受新任务；排队的任务标记 interrupted；
// 取消运行中的任务，等待它们收尾（最多 timeout），收尾后的终态为 interrupted 而不是 canceled。
func (m *Manager) Shutdown(timeout time.Duration) {
	m.mu.Lock()
	m.closing = true
	queued := m.queue
	m.queue = nil
	var all []*entry
	for _, e := range m.entries {
		all = append(all, e)
	}
	m.mu.Unlock()

	for _, e := range queued {
		e.cancel()
		e.finish(m, StatusInterrupted, nil, "")
		m.mu.Lock()
		delete(m.entries, e.task.ID)
		m.mu.Unlock()
		close(e.done)
	}
	for _, e := range all {
		e.cancel()
	}
	done := make(chan struct{})
	go func() { m.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(timeout):
		m.logf("等待任务退出超时（%s）", timeout)
	}
}

// Get 返回任务。运行中的任务带实时进度。
func (m *Manager) Get(taskID string) (Task, error) {
	m.mu.Lock()
	e, ok := m.entries[taskID]
	m.mu.Unlock()
	if ok {
		return e.snapshot(), nil
	}
	t, err := m.cfg.Store.GetTask(context.Background(), taskID)
	if err != nil {
		return Task{}, notFoundOr(err, "任务不存在")
	}
	return t, nil
}

// ListActive 返回所有排队和运行中的任务（含实时进度），按创建时间升序。
func (m *Manager) ListActive() []Task {
	m.mu.Lock()
	es := make([]*entry, 0, len(m.entries))
	for _, e := range m.entries {
		es = append(es, e)
	}
	m.mu.Unlock()
	out := make([]Task, 0, len(es))
	for _, e := range es {
		if s := e.snapshot(); s.Status.Active() {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt != out[j].CreatedAt {
			return out[i].CreatedAt < out[j].CreatedAt
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// List 分页查询（历史任务）。返回结果中仍在运行的任务会叠加实时进度。
func (m *Manager) List(f Filter) (Page, error) {
	p, err := m.cfg.Store.ListTasks(context.Background(), f)
	if err != nil {
		return Page{}, apperr.Wrap(apperr.IOError, "查询任务失败", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, t := range p.Items {
		if e, ok := m.entries[t.ID]; ok {
			p.Items[i] = e.snapshot()
		}
	}
	return p, nil
}

// Retry 用原任务的 Params 重新提交，生成新任务。原任务必须已结束（否则 TASK_CONFLICT）；
// 该类型没有注册 Factory 时返回 INTERNAL。
func (m *Manager) Retry(taskID string) (Task, error) {
	old, err := m.Get(taskID)
	if err != nil {
		return Task{}, err
	}
	if old.Status.Active() {
		return Task{}, apperr.New(apperr.TaskConflict, "任务仍在进行，不能重试")
	}
	m.mu.Lock()
	f := m.factories[old.Type]
	m.mu.Unlock()
	if f == nil {
		return Task{}, apperr.New(apperr.Internal, fmt.Sprintf("%s 类型的任务不支持重试", old.Type))
	}
	r, err := f(old)
	if err != nil {
		return Task{}, apperr.From(err)
	}
	return m.Submit(Spec{Type: old.Type, Title: old.Title, InputPaths: old.InputPaths, OutputPath: old.OutputPath, Params: old.Params}, r)
}

func notFoundOr(err error, msg string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return apperr.New(apperr.NotFound, msg)
	}
	return apperr.Wrap(apperr.IOError, "读取任务失败", err)
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

package task

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"runtime/debug"
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
	namer     *namer // 输出文件名占用登记（见 part.go）
}

// NewManager 创建任务管理器。
func NewManager(cfg Config) *Manager {
	limit := cfg.BatchConcurrency
	if limit <= 0 {
		limit = DefaultBatchConcurrency()
	}
	return &Manager{cfg: cfg, entries: map[string]*entry{}, limit: limit, factories: map[Type]Factory{}, namer: newNamer()}
}

func (m *Manager) logf(format string, args ...any) {
	if m.cfg.Logf != nil {
		m.cfg.Logf(format, args...)
		return
	}
	log.Printf(format, args...)
}

// emit 向前端发事件。Emitter 里的 panic 被拦下并记录日志，不会拖垮任务。
func (m *Manager) emit(event string, payload any) {
	if m.cfg.Emitter == nil {
		return
	}
	defer func() {
		if p := recover(); p != nil {
			m.logf("发送事件 %s 时发生 panic: %v\n%s", event, p, debug.Stack())
		}
	}()
	m.cfg.Emitter.Emit(event, payload)
}

// finalize 调用 Runner 的 OnFinish（如果实现了），同样拦截 panic。
func (m *Manager) finalize(e *entry) {
	f, ok := e.runner.(Finalizer)
	if !ok {
		return
	}
	defer func() {
		if p := recover(); p != nil {
			m.logf("任务 %s 的 OnFinish 发生 panic: %v\n%s", e.task.ID, p, debug.Stack())
		}
	}()
	f.OnFinish(e.snapshot())
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

// SetConcurrency 调整 batch 池并发数（设置里的并发数变化时调用），n<=0 用 DefaultBatchConcurrency()。
// 调大立即生效（排队的任务马上补位）；调小不会打断已在运行的任务，只是暂停从队列取新任务，
// 直到运行数降到新上限以下。可在任意时刻并发调用。
func (m *Manager) SetConcurrency(n int) { m.SetBatchConcurrency(n) }

// SetBatchConcurrency 同 SetConcurrency。
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
//   - live 类任务不排队，立即开始，不占 batch 名额；
//   - task:created 一定先于该任务的任何 task:status / task:progress 发出（先发事件，再入队）；
//   - spec.ID 若指定只能是 ULID 字符（[0-9A-Za-z]，防止日志路径穿越），InputPaths / OutputPath 必须是绝对路径。
func (m *Manager) Submit(spec Spec, r Runner) (Task, error) {
	if r == nil {
		return Task{}, apperr.New(apperr.InvalidArgument, "任务缺少执行体")
	}
	if !validType(spec.Type) {
		return Task{}, apperr.New(apperr.InvalidArgument, fmt.Sprintf("未知的任务类型 %q", spec.Type))
	}
	if err := validateSpec(spec); err != nil {
		return Task{}, err
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

	// 先登记（防重复 ID），再落库。登记但未入队的任务不会被调度。
	m.mu.Lock()
	if m.closing {
		m.mu.Unlock()
		return Task{}, apperr.New(apperr.Internal, "应用正在退出，无法提交新任务")
	}
	if _, dup := m.entries[t.ID]; dup {
		m.mu.Unlock()
		return Task{}, apperr.New(apperr.InvalidArgument, "任务 ID 重复")
	}
	m.entries[t.ID] = e
	m.mu.Unlock()

	if err := m.cfg.Store.InsertTask(context.Background(), t); err != nil {
		m.mu.Lock()
		delete(m.entries, t.ID)
		m.mu.Unlock()
		e.log.close()
		return Task{}, apperr.Wrap(apperr.IOError, "保存任务失败", err)
	}

	if c, ok := r.(Claimer); ok {
		c.Submitted(t.ID)
	}
	// 快照必须在入队前取：入队后任务可能立刻开始甚至跑完，返回给调用方的应当是"刚创建"的状态（queued，version 1），
	// 与 task:created 事件一致；之后的变化由 task:status / task:progress 事件推送。
	created := e.snapshot()
	m.emit(EventCreated, created)

	live := IsLive(spec.Type)
	m.mu.Lock()
	switch {
	case m.closing: // 提交与退出并发：直接标记中断
		m.mu.Unlock()
		e.cancel()
		m.finishNeverRan(e, StatusInterrupted)
	case e.ctx.Err() != nil: // 在 created 事件与入队之间被取消
		m.mu.Unlock()
		m.finishNeverRan(e, StatusCanceled)
	default:
		if !live {
			m.queue = append(m.queue, e)
		}
		m.mu.Unlock()
		if live {
			m.launch(e, false)
		} else {
			m.pump()
		}
	}
	return created, nil
}

// finishNeverRan 结束一个从未开始执行的任务（排队中被取消、退出时还在排队）：落库、发 task:status、调用 OnFinish。
func (m *Manager) finishNeverRan(e *entry, st Status) {
	e.finish(m, st, nil, "")
	m.mu.Lock()
	delete(m.entries, e.task.ID)
	m.mu.Unlock()
	m.namer.releaseOwner(e.task.ID)
	close(e.done)
	m.finalize(e)
}

// validateSpec 检查任务 ID 和路径。
func validateSpec(spec Spec) error {
	if spec.ID != "" && !validTaskID(spec.ID) {
		return apperr.New(apperr.InvalidArgument, "任务 ID 只能包含字母和数字")
	}
	for _, p := range spec.InputPaths {
		if !filepath.IsAbs(p) {
			return apperr.New(apperr.InvalidArgument, "输入路径必须是绝对路径").WithDetail(p)
		}
	}
	if spec.OutputPath != "" && !filepath.IsAbs(spec.OutputPath) {
		return apperr.New(apperr.InvalidArgument, "输出路径必须是绝对路径").WithDetail(spec.OutputPath)
	}
	return nil
}

// validTaskID 只接受 ASCII 字母数字、长度 1~64：ID 直接用作日志文件名，不能含路径分隔符或 ..。
func validTaskID(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
			return false
		}
	}
	return true
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
		m.namer.releaseOwner(e.task.ID)
		close(e.done)
		if countBatch {
			m.pump()
		}
		m.finalize(e)
	}()
}

func (m *Manager) execute(e *entry) {
	if e.ctx.Err() != nil { // 出队前后被取消
		m.finishAfterRun(e, e.ctx.Err(), "")
		return
	}
	e.start()
	ctx := context.WithValue(e.ctx, ctxKey{}, Info{ID: e.task.ID, Type: e.task.Type, log: e.log, m: m})
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
//   - 应用退出中且任务是被退出流程取消的 → interrupted（包括直播被优雅停止后正常返回的情况：
//     那是应用退出造成的停止，不是任务自己完成，重启后用户可以看到它被中断）；
//   - 成功 → succeeded（即使期间收到过用户的取消请求：直播优雅停止就是这种情况，存档已完整）；
//   - 被取消 → canceled；
//   - 其他 → failed。
func (m *Manager) finishAfterRun(e *entry, err error, out string) {
	m.mu.Lock()
	closing := m.closing
	m.mu.Unlock()
	switch {
	case closing && e.ctx.Err() != nil && !e.cancelRequested() && (err == nil || errors.Is(err, context.Canceled)):
		e.finish(m, StatusInterrupted, nil, "")
	case err == nil:
		e.finish(m, StatusSucceeded, nil, out)
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
			m.finishNeverRan(e, StatusCanceled)
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
		m.finishNeverRan(e, StatusInterrupted)
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
// 该类型没有注册 Factory 时返回 UNSUPPORTED。
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
		return Task{}, apperr.New(apperr.Unsupported, fmt.Sprintf("%s 类型的任务不支持重试", old.Type))
	}
	r, err := f(old)
	if err != nil {
		return Task{}, apperr.From(err)
	}
	t, err := m.Submit(Spec{Type: old.Type, Title: old.Title, InputPaths: old.InputPaths, OutputPath: old.OutputPath, Params: old.Params}, r)
	if err != nil {
		if c, ok := r.(Claimer); ok {
			c.Abandoned()
		}
		return Task{}, err
	}
	return t, nil
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

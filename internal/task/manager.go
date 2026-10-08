package task

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
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
	// DeleteWait 是 DeleteRecords 等进行中的任务到达终态的总时长，0 用 10 秒（契约 6.14.4）。
	DeleteWait time.Duration
	// Now 是时钟（删除失败路径的“可打开所在文件夹”有效期用，契约 v0.23.3）；nil 用 time.Now。测试里注入。
	Now func() time.Time
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
	namer     *namer           // 输出文件名占用登记（见 part.go）
	reveal    revealAllow      // 删除失败、文件留下的路径（RevealInFolder 临时放行，见 reveal_allow.go）
	rcCheck   ReconvertChecker // CheckPaths 的重转检查（转换服务注册，契约 6.17.1）
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
		return Task{}, apperr.New(apperr.InvalidArgument, "不支持的任务类型").WithDetail(fmt.Sprintf("未知的任务类型 %q", spec.Type))
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
		Version: 1, CreatedAt: now, SourceID: spec.SourceID,
	}
	if IsLive(spec.Type) {
		t.Progress = -1 // 契约：直播类任务进度恒为 -1
	}
	if er, ok := r.(EncoderReporter); ok {
		ei := er.EncoderInfo()
		t.Encoder, t.EncoderDevice, t.HWFallback, t.HWFallbackReason = ei.Encoder, ei.Device, ei.HWFallback, ei.HWFallbackReason
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

	// 提交时定名并占位（契约 6.14.5）：在落库之前选出最终名，占位人 = 任务 id，一直占到终态（launch / finishNeverRan 释放）。
	if spec.ReserveOutput && t.OutputPath != "" {
		t.OutputPath = m.namer.reserve(t.OutputPath, t.ID, styleFor(t.Type))
		e.mu.Lock()
		e.task.OutputPath = t.OutputPath
		e.mu.Unlock()
	}

	if err := m.cfg.Store.InsertTask(context.Background(), t); err != nil {
		m.mu.Lock()
		delete(m.entries, t.ID)
		m.mu.Unlock()
		m.namer.releaseOwner(t.ID)
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
	m.enqueue(e)
	return created, nil
}

// enqueue 把已登记、已落库、已发事件的任务交给调度池（Submit 和原地 Retry 共用）。
func (m *Manager) enqueue(e *entry) {
	live := IsLive(e.task.Type)
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
}

// NeverRanner 是 Runner 可选实现的接口：任务在 Run 没有执行的情况下就结束（排队中被取消、提交后立即被取消、退出时还在排队）时，
// 管理器在发终态事件之前调用一次 NeverRan，返回值作为该任务的输出路径（同 Run 的返回值，可以是 ClearOutputPath）。
// 直播存档用它清理已经创建的空占位文件。
type NeverRanner interface {
	NeverRan() string
}

func neverRanOutput(r Runner) string {
	if n, ok := r.(NeverRanner); ok {
		return n.NeverRan()
	}
	return ""
}

// finishNeverRan 结束一个从未开始执行的任务（排队中被取消、退出时还在排队）：落库、发 task:status、调用 OnFinish。
func (m *Manager) finishNeverRan(e *entry, st Status) {
	if e.rc != nil {
		m.endReconvert(e, outcomeFor(st), nil)
	} else {
		e.finish(m, st, nil, neverRanOutput(e.runner))
	}
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
	case TypeConvert, TypeEditExport, TypeOfficePDF, TypeLiveFilePush, TypeLiveScreenPush, TypeFFmpegInstall:
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
		m.finishAfterRun(e, e.ctx.Err(), neverRanOutput(e.runner))
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
			err = apperr.New(apperr.Internal, "任务执行时出错，请重试").WithDetail(fmt.Sprintf("panic: %v", p))
		}
	}()
	return r.Run(ctx, report)
}

// finishAfterRun 根据 Run 的结果决定终态：
//   - 应用退出中且任务是被退出流程取消的 → interrupted（包括直播被优雅停止后正常返回的情况：
//     那是应用退出造成的停止，不是任务自己完成，重启后用户可以看到它被中断）；
//   - 成功 → succeeded（即使期间收到过用户的取消请求：直播优雅停止就是这种情况，存档已完整）；
//   - 被取消 → canceled；
//   - 直播任务开始以后被中断（Runner 返回 InterruptedError）→ interrupted，带错误（契约 v0.25.3）；
//   - 其他 → failed。
//
// Runner 返回的输出路径：成功时一律采信；直播任务在 canceled / failed / interrupted 时也采信
// （本地存档在强杀、断流后仍然保留，契约 6.10），其他类型失败 / 取消时不带输出（沿用旧行为）。
// 返回 ClearOutputPath 表示清空 outputPath（存档是空壳被删了）。
func (m *Manager) finishAfterRun(e *entry, err error, out string) {
	m.mu.Lock()
	closing := m.closing
	m.mu.Unlock()
	if e.rc != nil {
		m.finishReconvert(e, closing, err, out)
		return
	}
	carry := out
	if err != nil && !IsLive(e.task.Type) {
		carry = ""
	}
	switch {
	case closing && e.ctx.Err() != nil && !e.cancelRequested() && (err == nil || errors.Is(err, context.Canceled)):
		e.finish(m, StatusInterrupted, nil, carry)
	case err == nil:
		e.finish(m, StatusSucceeded, nil, out, resultOf(e.runner))
	case e.ctx.Err() != nil && (errors.Is(err, context.Canceled) || e.cancelRequested()):
		e.finish(m, StatusCanceled, nil, carry)
	case IsLive(e.task.Type) && IsInterrupted(err):
		// 契约 v0.25.3：直播开始以后被中断（进程被杀、服务器断开等）记为 interrupted，带错误（LIVE_PUSH_INTERRUPTED 等，不会是 INTERNAL）。
		e.finish(m, StatusInterrupted, apperr.From(err), carry)
	default:
		e.finish(m, StatusFailed, apperr.From(err), carry)
	}
}

// resultOf 调用 Runner 的 ResultReporter（如果实现了），拦截 panic。
func resultOf(r Runner) (res *TaskResult) {
	rr, ok := r.(ResultReporter)
	if !ok {
		return nil
	}
	defer func() {
		if recover() != nil {
			res = nil
		}
	}()
	return rr.Result()
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

// usesRunWithPart 是走 RunWithPart 的任务类型：原地重试时清理上一次的 .part、重新占位原输出名（契约 6.6）。
func usesRunWithPart(t Type) bool {
	return t == TypeConvert || t == TypeEditExport || t == TypeOfficePDF
}

// retryLogLine 是原地重试时追加到任务日志末尾的一行（契约 6.6 第 ⑤ 步）。
const retryLogLine = "[FFmpegFree] 重新开始（重试）"

// Retry 原地重试（契约 v0.23，6.6）：复用原任务 id，把同一条记录重置回 queued 重新排队。
//   - 允许 failed / interrupted / canceled；queued / running → TASK_CONFLICT（任务仍在进行，请先取消）；
//     succeeded → TASK_CONFLICT（任务已经成功完成，不能重试）；没有 Factory → UNSUPPORTED；不存在 / 旧类型 → NOT_FOUND；
//   - ① 用 Factory 重建 Runner（失败直接返回，记录不动、不发事件）；② 走 RunWithPart 的类型删掉上一次留下的 .part；
//     ③ 重新占位原输出名（被占时按该类型的重名格式顺延）；④ 重置字段、version +1、落库；⑤ 日志追加一行；
//     ⑥ 发一次 task:status（queued、retried、progress 0、hiddenInTaskCenter false）；⑦ 入队。不发 task:created。
func (m *Manager) Retry(taskID string) (Task, error) {
	old, err := m.Get(taskID)
	if err != nil {
		return Task{}, err
	}
	if old.Status.Active() {
		return Task{}, apperr.New(apperr.TaskConflict, "任务仍在进行，请先取消")
	}
	m.mu.Lock()
	f := m.factories[old.Type]
	m.mu.Unlock()
	if f == nil { // 没有工厂的类型（直播、已移除的剪辑导出）不管什么状态都是 UNSUPPORTED，提示更有用
		if IsLive(old.Type) {
			return Task{}, apperr.New(apperr.Unsupported, "直播会话不能重试，请重新开始推流")
		}
		if old.Type == TypeEditExport || old.Type == TypeEditRender { // 契约 v0.23.5 / v0.25.3：旧版导出记录只能查看和删除
			return Task{}, LegacyExportError("重试")
		}
		return Task{}, apperr.New(apperr.Unsupported, "这类任务不支持重试").WithDetail(fmt.Sprintf("type=%s", old.Type))
	}
	if old.Status == StatusSucceeded {
		return Task{}, apperr.New(apperr.TaskConflict, "任务已经成功完成，不能重试")
	}
	r, err := f(old)
	if err != nil {
		return Task{}, apperr.From(err)
	}
	abandon := func() {
		if c, ok := r.(Claimer); ok {
			c.Abandoned()
		}
	}

	t := old
	t.InputPaths = nonNil(old.InputPaths)
	t.Status = StatusQueued
	t.Progress = 0
	t.Speed, t.EtaSec = "", 0
	t.Fps, t.BitrateKbps, t.DroppedFrames = 0, 0, 0
	t.StartedAt, t.FinishedAt = 0, 0
	t.Encoder, t.EncoderDevice, t.HWFallback, t.HWFallbackReason = "", "", false, ""
	if er, ok := r.(EncoderReporter); ok {
		ei := er.EncoderInfo()
		t.Encoder, t.EncoderDevice, t.HWFallback, t.HWFallbackReason = ei.Encoder, ei.Device, ei.HWFallback, ei.HWFallbackReason
	}
	t.Error, t.Result = nil, nil
	t.HiddenInTaskCenter = false
	t.Version = old.Version + 1
	e := newEntry(m, t, r)
	if t.LogPath == "" {
		t.LogPath = e.log.path()
	}
	e.task = t

	// 先登记（防止并发的两次 Retry / 与 Delete 交错），再确认库里的记录还是我们读到的那一版。
	m.mu.Lock()
	if m.closing {
		m.mu.Unlock()
		abandon()
		return Task{}, apperr.New(apperr.Internal, "应用正在退出，无法重试")
	}
	if _, dup := m.entries[t.ID]; dup {
		m.mu.Unlock()
		abandon()
		return Task{}, apperr.New(apperr.TaskConflict, "任务仍在进行，请先取消")
	}
	m.entries[t.ID] = e
	m.mu.Unlock()
	rollback := func() {
		m.mu.Lock()
		delete(m.entries, t.ID)
		m.mu.Unlock()
		m.namer.releaseOwner(t.ID)
		e.log.close()
		abandon()
	}
	if cur, err := m.cfg.Store.GetTask(context.Background(), t.ID); err != nil {
		rollback()
		return Task{}, notFoundOr(err, "任务不存在")
	} else if cur.Version != old.Version || cur.Status != old.Status {
		rollback()
		return Task{}, apperr.New(apperr.TaskConflict, "任务状态已变化，请刷新后再试")
	}

	if usesRunWithPart(t.Type) && old.OutputPath != "" && filepath.IsAbs(old.OutputPath) {
		// ② 上一次运行留下的 .part（别的任务正占着这个名字时不碰，那是它的 .part）。
		if !m.namer.heldByOther(old.OutputPath, t.ID) {
			if err := removeStalePart(old.OutputPath, old.StartedAt); err != nil {
				m.logf("重试任务 %s 时删除上一次的 .part 失败: %v", t.ID, err)
			}
		}
		// ③ 重新占位原输出名；被占就按该类型的重名格式顺延。
		st := styleFor(t.Type)
		out := old.OutputPath
		if !m.namer.hold(out, t.ID) {
			base := out
			if d, ok := r.(DesiredOutputer); ok && d.DesiredOutput() != "" && filepath.IsAbs(d.DesiredOutput()) {
				base = d.DesiredOutput()
			} else if st == styleSpaced {
				base = stripSpacedSuffix(out)
			}
			out = m.namer.reserve(base, t.ID, st)
		}
		t.OutputPath = out
		e.mu.Lock()
		e.task.OutputPath = out
		e.mu.Unlock()
	}

	// ④ 落库。
	if err := m.cfg.Store.UpdateTask(context.Background(), t); err != nil {
		rollback()
		return Task{}, apperr.Wrap(apperr.IOError, "保存任务失败", err)
	}
	// ⑤ 日志。
	fmt.Fprintf(e.log, "\n%s\n", retryLogLine)
	if c, ok := r.(Claimer); ok {
		c.Submitted(t.ID)
	}
	// ⑥ 事件（快照在入队前取，同 Submit）。
	snap := e.snapshot()
	zero, notHidden := 0.0, false
	m.emit(EventStatus, StatusEvent{
		ID: t.ID, Version: t.Version, Status: StatusQueued, OutputPath: t.OutputPath,
		Encoder: t.Encoder, EncoderDevice: t.EncoderDevice, HWFallback: t.HWFallback, HWFallbackReason: t.HWFallbackReason,
		Progress: &zero, Retried: true, HiddenInTaskCenter: &notHidden, Reconverting: rcFlag(t),
	})
	// ⑦ 入队。
	m.enqueue(e)
	return snap, nil
}

// stripSpacedSuffix 去掉 convert 输出名末尾的 " (n)"（n 为 1~99），得到顺延的起点："/o/a (1).mp4" → "/o/a.mp4"。
func stripSpacedSuffix(p string) string {
	ext := filepath.Ext(p)
	stem := strings.TrimSuffix(p, ext)
	if !strings.HasSuffix(stem, ")") {
		return p
	}
	i := strings.LastIndex(stem, " (")
	if i < 0 {
		return p
	}
	num := stem[i+2 : len(stem)-1]
	if n, err := strconv.Atoi(num); err != nil || n < 1 || n > 99 || strconv.Itoa(n) != num {
		return p
	}
	return stem[:i] + ext
}

// removeStalePart 删除最终输出 final 对应的 .part 残留：只删普通文件（Lstat，不跟随符号链接）、
// 修改时间不早于 startedAt − 3 秒的；startedAt 为 0（从未运行）不删。文件不在返回 nil。
func removeStalePart(final string, startedAt int64) error {
	if final == "" || !filepath.IsAbs(final) || startedAt <= 0 {
		return nil
	}
	p := PartPath(final)
	fi, err := os.Lstat(p)
	if err != nil || !fi.Mode().IsRegular() {
		return nil
	}
	if fi.ModTime().UnixMilli() < startedAt-mtimeSlackMs {
		return nil
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// PeekOutputName 返回此刻按 typ 的重名格式会选中的最终输出路径，不占位（ConvertService.PreviewOutputName）。
func (m *Manager) PeekOutputName(desired string, typ Type) string {
	return m.namer.peek(desired, styleFor(typ))
}

// Live 把 ts 里仍在进行的任务换成带实时进度的快照（转换页列表用，同 List）。
func (m *Manager) Live(ts []Task) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, t := range ts {
		if e, ok := m.entries[t.ID]; ok {
			ts[i] = e.snapshot()
		}
	}
}

// updateOutput 见 RunWithPart：运行时顺延了名字。
func (m *Manager) updateOutput(taskID, p string) {
	m.mu.Lock()
	e := m.entries[taskID]
	m.mu.Unlock()
	if e != nil {
		e.setOutput(p)
	}
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

// setEncoder 见 ReportEncoder。
func (m *Manager) setEncoder(taskID string, info ffmpeg.EncoderInfo) {
	m.mu.Lock()
	e := m.entries[taskID]
	m.mu.Unlock()
	if e != nil {
		e.setEncoder(info)
	}
}

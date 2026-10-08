// Package task 是 v2 的任务管理器（契约第 3、5、6.5 节）。
//
// 职责：
//   - 统一的 Runner 接口，ffmpeg 转换、Office 转 PDF、ffmpeg 下载、直播推流都实现它；
//   - 两个调度池：batch 池按并发数排队，live 池不排队、不占 batch 名额；
//   - 状态机 queued → running → succeeded | failed | canceled | interrupted，
//     状态变化时落库（SQLite tasks 表）并推送 task:created / task:status；
//   - 进度只保存在内存，通过 task:progress 节流推送（每任务最多 4 次/秒）；
//   - 取消、重试、删除、清理、日志读取。
//
// 本包不依赖 Wails：事件通过 Emitter 接口发出，存储通过 Store 接口访问。
package task

import (
	"context"
	"io"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/store"
)

// 类型别名：Task 及其相关类型定义在 store 包（tasks 表的行），这里重新导出，业务代码只需要 import task。
type (
	Task       = store.Task
	Type       = store.TaskType
	Status     = store.TaskStatus
	Filter     = store.TaskFilter
	Page       = store.TaskPage
	TaskFilter = store.TaskFilter
	TaskResult = store.TaskResult
)

// 任务类型与状态常量（契约第 3 节）。
const (
	TypeConvert    = store.TypeConvert
	TypeEditExport = store.TypeEditExport
	// TypeEditRender 已弃用（只为读旧数据），见 store.TypeEditRender。
	TypeEditRender     = store.TypeEditRender
	TypeOfficePDF      = store.TypeOfficePDF
	TypeLiveFilePush   = store.TypeLiveFilePush
	TypeLiveScreenPush = store.TypeLiveScreenPush
	TypeFFmpegInstall  = store.TypeFFmpegInstall

	// Deprecated: 只为读旧数据保留，不再产生，Submit 不接受（契约 v0.10）。
	TypeLiveRelay = store.TypeLiveRelay
	// Deprecated: 同 TypeLiveRelay。
	TypeLiveRecordPush = store.TypeLiveRecordPush

	StatusQueued      = store.StatusQueued
	StatusRunning     = store.StatusRunning
	StatusSucceeded   = store.StatusSucceeded
	StatusFailed      = store.StatusFailed
	StatusCanceled    = store.StatusCanceled
	StatusInterrupted = store.StatusInterrupted
)

// 事件名（契约第 5 节）。
const (
	EventCreated  = "task:created"
	EventProgress = "task:progress"
	EventStatus   = "task:status"
	EventRemoved  = "task:removed"
)

// IsLive 判断任务类型是否属于 live 池（直播类：不排队，不占 batch 名额）。
// 旧的 live_relay / live_record_push 仍返回 true，只是为了常量兼容；它们不再产生，Submit 不接受。
func IsLive(t Type) bool {
	switch t {
	case TypeLiveFilePush, TypeLiveScreenPush, TypeLiveRelay, TypeLiveRecordPush:
		return true
	}
	return false
}

// Progress 是 Runner 汇报的进度快照，只保存在内存。
type Progress struct {
	Fraction   float64 // 0~1；无法确定进度（直播）时传 -1
	Speed      string  // 如 "2.3x"、"3.2 MB/s"
	EtaSec     float64
	OutTimeSec float64 // 已输出的媒体时长，非媒体任务为 0
	// 以下三项只有直播任务用（契约 v0.10）。
	Fps           float64 // 当前输出帧率
	BitrateKbps   float64 // 近 5 秒的输出码率（kbit/s），算不出来为 0
	DroppedFrames int64   // ffmpeg 累计丢帧数
}

// Runner 是任务的执行体（契约 6.5）。
//
// Run 在专属 goroutine 里执行，必须尊重 ctx：取消时尽快返回（返回 ctx.Err() 或包装了它的错误）。
// 成功返回最终输出路径（可为空）。写文件的任务应使用 RunWithPart，保证 .part 临时文件与原子改名。
type Runner interface {
	Run(ctx context.Context, report func(p Progress)) (outputPath string, err error)
}

// RunnerFunc 让普通函数满足 Runner。
type RunnerFunc func(ctx context.Context, report func(p Progress)) (string, error)

func (f RunnerFunc) Run(ctx context.Context, report func(p Progress)) (string, error) {
	return f(ctx, report)
}

// Finalizer 是 Runner 可选实现的接口：任务进入终态（成功、失败、取消、中断）并落库、发出 task:status 之后，
// 管理器调用一次 OnFinish。排队中被取消的任务（Run 从未执行）也会调用。
// 用于任务结束后的收尾，例如 ffmpeg 安装任务结束后刷新 ffmpeg 状态。
type Finalizer interface {
	OnFinish(t Task)
}

// Claimer 是 Runner 可选实现的接口，用来在任务被接受的瞬间"占位"：
//
//   - Submitted 在任务落库并登记之后、发出 task:created 之前同步调用一次（id 是任务 ID）；
//   - Abandoned 在 Retry 用工厂造出了 Runner 但随后 Submit 失败时调用，让 Runner 释放工厂里做的占位。
//
// 例如 ffmpeg 安装任务：Retry 工厂先声明"正在安装"，避免排队期间又提交第二个安装。
type Claimer interface {
	Submitted(id string)
	Abandoned()
}

// EncoderReporter 是 Runner 可选实现的接口：返回任务一开始使用的视频编码器信息（契约 v0.18，9.7）。
// Submit 用它填 Task.Encoder / EncoderDevice / HWFallback / HWFallbackReason（Retry 造出的新 Runner 同样适用），
// 所以 task:created、落库和第一个 task:status 就带着实际编码器；运行中若硬件编码失败回退 CPU，Runner 再用 ReportEncoder 更新。
// 不返回视频编码器（Encoder 为空）的任务不设置这些字段。
type EncoderReporter interface {
	EncoderInfo() ffmpeg.EncoderInfo
}

// Emitter 向前端发事件。生产实现封装 Wails runtime.EventsEmit。
type Emitter interface {
	Emit(event string, payload any)
}

// Store 是任务管理器需要的持久化能力，*store.Store 满足该接口。
type Store interface {
	InsertTask(ctx context.Context, t Task) error
	UpdateTask(ctx context.Context, t Task) error
	GetTask(ctx context.Context, id string) (Task, error)
	ListTasks(ctx context.Context, f Filter) (Page, error)
	DeleteTasks(ctx context.Context, ids []string) ([]string, error)
	// HideFinishedTasks / UnhideTasks 是任务中心的“隐藏已完成 / 取消隐藏”（契约 v0.23）。
	HideFinishedTasks(ctx context.Context) (int64, error)
	UnhideTasks(ctx context.Context, ids []string) ([]Task, error)
}

// ResultReporter 是 Runner 可选实现的接口（契约 v0.23，6.14.6）：Run 返回 nil 之后、发终态事件之前，
// 管理器调用一次 Result，非 nil 时写进 Task.Result、落库并随 succeeded 的 task:status 发出。
type ResultReporter interface {
	Result() *TaskResult
}

// DesiredOutputer 是 Runner 可选实现的接口：返回该任务的期望输出名（重名顺延前的名字）。
// 原地重试时原输出名被占，就从它开始按重名格式顺延（FFmpegRunner 返回 Output）。
type DesiredOutputer interface {
	DesiredOutput() string
}

var _ Store = (*store.Store)(nil)

// ProgressEvent 是 task:progress 的 payload。
type ProgressEvent struct {
	ID         string  `json:"id"`
	Version    int64   `json:"version"`
	Progress   float64 `json:"progress"`
	Speed      string  `json:"speed"`
	EtaSec     float64 `json:"etaSec"`
	OutTimeSec float64 `json:"outTimeSec"`
	// 使用了视频编码器的任务才有（契约 v0.18，9.7），与 Task 里的同名字段一致。
	Encoder          string `json:"encoder,omitempty"`
	EncoderDevice    string `json:"encoderDevice,omitempty"`
	HWFallback       bool   `json:"hwFallback,omitempty"`
	HWFallbackReason string `json:"hwFallbackReason,omitempty"`
	// 只有直播任务才有（契约 v0.10）。
	Fps           float64 `json:"fps,omitempty"`
	BitrateKbps   float64 `json:"bitrateKbps,omitempty"`
	DroppedFrames int64   `json:"droppedFrames,omitempty"`
}

// ClearOutputPath 是 Runner.Run 可以返回的特殊输出路径：表示任务没有输出了，把 outputPath 清空
// （直播存档是空壳被删除时用）。清空在发终态事件之前完成并落库，事件和库里一致（都是空）。
// 普通的空串仍然是没有变化，保留提交时的预期路径。
const ClearOutputPath = "\x00clear-output"

// StatusEvent 是 task:status 的 payload。
//
// 时间字段（Unix 毫秒，为 0 时省略）：
//   - running 事件带 StartedAt，不带 FinishedAt；
//   - 所有终态事件（succeeded / failed / canceled / interrupted）带 FinishedAt，
//     跑过的任务同时带 StartedAt（与 Task.StartedAt / Task.FinishedAt 及落库值一致）；
//   - 从未进入 running 就结束的任务（排队中被取消、退出时还在排队而被中断）没有 StartedAt，
//     事件里省略该字段（Task.StartedAt 为 0），这是正常的。
type StatusEvent struct {
	ID         string           `json:"id"`
	Version    int64            `json:"version"`
	Status     Status           `json:"status"`
	Error      *apperr.AppError `json:"error,omitempty"`
	OutputPath string           `json:"outputPath,omitempty"`
	StartedAt  int64            `json:"startedAt,omitempty"`
	FinishedAt int64            `json:"finishedAt,omitempty"`
	// 使用了视频编码器的任务才有（契约 v0.18，9.7）：running 事件、终态事件、以及运行中硬件编码回退 CPU 时补发的 running 事件都带。
	Encoder          string `json:"encoder,omitempty"`
	EncoderDevice    string `json:"encoderDevice,omitempty"`
	HWFallback       bool   `json:"hwFallback,omitempty"`
	HWFallbackReason string `json:"hwFallbackReason,omitempty"`
	// 以下四项是契约 v0.23（第 5 节）：Progress 在所有终态事件和原地重试的 queued 事件上一定带（指针，0 也带）；
	// Result 只有成功的 convert 任务的 succeeded 事件带；Retried 只在原地重试的那一条 queued 事件上为 true；
	// HiddenInTaskCenter 只出现在 UnhideInTaskCenter 和 retried 事件上（值为 false，指针）。
	Progress           *float64    `json:"progress,omitempty"`
	Result             *TaskResult `json:"result,omitempty"`
	Retried            bool        `json:"retried,omitempty"`
	HiddenInTaskCenter *bool       `json:"hiddenInTaskCenter,omitempty"`
	// 以下三项是契约 v0.24（6.17.2）：Reconverting 在 convert 任务的每条 task:status 上都带（指针，false 也带）；
	// ReconvertOutcome 只在原地重转结束的那一条终态事件上有（succeeded | failed | canceled | interrupted）；
	// LastReconvertError 只在 reconvertOutcome=failed 时带。重转事件不带 params（架构师定）。
	Reconverting       *bool                 `json:"reconverting,omitempty"`
	ReconvertOutcome   string                `json:"reconvertOutcome,omitempty"`
	LastReconvertError *store.ReconvertError `json:"lastReconvertError,omitempty"`
}

// rcFlag 返回 task:status 的 reconverting 字段：只有 convert 任务带。
func rcFlag(t Task) *bool {
	if t.Type != TypeConvert {
		return nil
	}
	v := t.Reconverting
	return &v
}

// RemovedEvent 是 task:removed 的 payload。
type RemovedEvent struct {
	IDs []string `json:"ids"`
}

// Spec 描述要提交的任务。
type Spec struct {
	ID         string // 可选；为空时自动生成 ULID。调用方需要在提交前就知道任务 ID 时使用
	Type       Type
	Title      string
	InputPaths []string
	OutputPath string // 预期的输出路径（可能在完成时被 Runner 返回的实际路径覆盖）
	Params     string // 原始参数 JSON，Retry 用它重新构造 Runner
	// SourceID 是 convert 任务的源文件行（契约 v0.23），其他类型留空。
	SourceID string
	// ReserveOutput 为 true 时在落库之前就以 OutputPath 为期望名定名并占位（占位人 = 任务 id，一直占到终态），
	// 重名格式按任务类型（convert 是 "a (1).mp4"，其余 "a(1).mp4"），见契约 6.14.5。RunWithPart 运行时直接用占好的名字。
	ReserveOutput bool
}

// Info 是任务运行时通过 ctx 传给 Runner 的信息。
type Info struct {
	ID   string
	Type Type
	log  *logSink
	m    *Manager
}

type ctxKey struct{}

// InfoFrom 取出当前任务信息；不在任务里运行时 ok=false。
func InfoFrom(ctx context.Context) (Info, bool) {
	i, ok := ctx.Value(ctxKey{}).(Info)
	return i, ok
}

// LogWriter 返回当前任务的日志写入器（<数据目录>/logs/<任务ID>.log，首次写入时创建）。
// 不在任务里运行或没有配置日志目录时返回 io.Discard。TaskService.GetLog 读取的就是这个文件。
func LogWriter(ctx context.Context) io.Writer {
	if i, ok := InfoFrom(ctx); ok && i.log != nil {
		return i.log
	}
	return io.Discard
}

// ReportEncoder 由 Runner 在运行中更新任务的编码器信息（硬件编码启动失败、改用 CPU 重试时调用）。
// 不在任务里运行、任务已结束或信息没变时什么也不做；有变化时落库并补发一条 task:status（status 仍是 running）。
func ReportEncoder(ctx context.Context, info ffmpeg.EncoderInfo) {
	i, ok := InfoFrom(ctx)
	if !ok || i.m == nil {
		return
	}
	i.m.setEncoder(i.ID, info)
}

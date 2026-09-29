package system

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/id"
)

// 任务事件名（契约第 5 节）。安装任务在任务管理器（internal/task）出现之前由 Manager 自己发这些事件，
// payload 与契约一致，任务管理器接手后只需要把 InstallFFmpeg 换成提交 ffmpeg_install 任务。
const (
	EventTaskCreated  = "task:created"
	EventTaskProgress = "task:progress"
	EventTaskStatus   = "task:status"
)

// TaskTypeFFmpegInstall 是契约 TaskType 中的 ffmpeg_install。
const TaskTypeFFmpegInstall = "ffmpeg_install"

// 契约 TaskStatus 取值（本文件只用到其中几个）。
const (
	TaskQueued    = "queued"
	TaskRunning   = "running"
	TaskSucceeded = "succeeded"
	TaskFailed    = "failed"
	TaskCanceled  = "canceled"
)

// InstallTask 是契约第 3 节 Task 的轻量版，字段与 JSON 名完全一致，任务管理器 PR 可直接换成 task.Task。
type InstallTask struct {
	ID         string           `json:"id"`
	Type       string           `json:"type"`   // 固定 ffmpeg_install
	Status     string           `json:"status"` // running | succeeded | failed | canceled
	Title      string           `json:"title"`
	InputPaths []string         `json:"inputPaths"` // 下载地址不是本地路径，恒为空数组
	OutputPath string           `json:"outputPath"` // <数据目录>/bin
	Progress   float64          `json:"progress"`   // 0~1
	Speed      string           `json:"speed"`      // 如 "3.2 MB/s"
	EtaSec     float64          `json:"etaSec"`
	Params     string           `json:"params"`  // {"mirror":"..."}，用于重试
	Version    int64            `json:"version"` // 每次变更 +1
	Error      *apperr.AppError `json:"error,omitempty"`
	CreatedAt  int64            `json:"createdAt"`
	StartedAt  int64            `json:"startedAt"`
	FinishedAt int64            `json:"finishedAt"`
}

type taskProgressPayload struct {
	ID         string  `json:"id"`
	Version    int64   `json:"version"`
	Progress   float64 `json:"progress"`
	Speed      string  `json:"speed"`
	EtaSec     float64 `json:"etaSec"`
	OutTimeSec float64 `json:"outTimeSec"`
}

type taskStatusPayload struct {
	ID         string           `json:"id"`
	Version    int64            `json:"version"`
	Status     string           `json:"status"`
	Error      *apperr.AppError `json:"error,omitempty"`
	OutputPath string           `json:"outputPath,omitempty"`
	FinishedAt int64            `json:"finishedAt,omitempty"`
}

type installRun struct {
	task   InstallTask
	cancel context.CancelFunc
}

func (m *Manager) installingStatus() (FFmpegStatus, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.install == nil {
		return FFmpegStatus{}, false
	}
	return m.status, true
}

func (m *Manager) emit(event string, payload any) {
	if m.cfg.Emitter != nil {
		m.cfg.Emitter.Emit(event, payload)
	}
}

// Install 启动 ffmpeg 下载安装（契约 9.3），立即返回任务信息，实际工作在后台 goroutine。
//
//   - 幂等：已有进行中的安装时直接返回该任务，不开第二个下载；
//   - mirror 只接受 ""（默认源）和当前平台清单里真正有的镜像（见 InstallOptions）；
//     其他值（包括当前平台没有的 "cn"）返回 INVALID_ARGUMENT，detail 列出可选镜像，不会悄悄改用默认源；
//   - 当前平台没有下载源返回 UNSUPPORTED_PLATFORM；
//   - 状态：任意 -> installing（TaskID 为任务 ID）-> ready | failed；取消后重新检测。
//     每次状态变化推送 ffmpeg:status，进度通过 task:progress 推送，不塞进 ffmpeg:status。
func (m *Manager) Install(ctx context.Context, mirror string) (InstallTask, error) {
	cfg, err := m.ready()
	if err != nil {
		return InstallTask{}, err
	}
	m.mu.Lock()
	if m.install != nil {
		t := m.install.task
		m.mu.Unlock()
		return t, nil
	}
	m.mu.Unlock()

	if cfg.Installer == nil {
		return InstallTask{}, apperr.New(apperr.Internal, "安装功能未初始化")
	}
	if err := preflightInstall(cfg.Installer, mirror); err != nil {
		return InstallTask{}, err
	}

	params, _ := json.Marshal(map[string]string{"mirror": mirror})
	now := time.Now().UnixMilli()

	m.mu.Lock()
	if m.install != nil { // 上面到这里之间可能有并发调用抢先了
		t := m.install.task
		m.mu.Unlock()
		return t, nil
	}
	runCtx, cancel := context.WithCancel(m.appCtx)
	run := &installRun{cancel: cancel, task: InstallTask{
		ID: id.New(), Type: TaskTypeFFmpegInstall, Status: TaskRunning, Title: "安装 ffmpeg",
		InputPaths: []string{}, OutputPath: cfg.Installer.BinDir, Params: string(params),
		Version: 1, CreatedAt: now, StartedAt: now,
	}}
	m.install = run
	task := run.task
	st := FFmpegStatus{State: ffmpeg.StateInstalling, TaskID: task.ID}
	m.status = st
	ffmpeg.SetCurrent(nil) // 安装期间不放行依赖 ffmpeg 的功能，避免与替换文件冲突
	m.emit(EventTaskCreated, task)
	m.emit(EventFFmpegStatus, st)
	m.mu.Unlock()

	go m.runInstall(runCtx, cfg, run, mirror)
	return task, nil
}

func (m *Manager) runInstall(ctx context.Context, cfg Config, run *installRun, mirror string) {
	info, err := cfg.Installer.Install(ctx, mirror, func(p ffmpeg.Progress) {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.install != run {
			return
		}
		run.task.Version++
		run.task.Progress = p.Fraction
		run.task.Speed = formatSpeed(p.Speed)
		run.task.EtaSec = p.EtaSec
		m.emit(EventTaskProgress, taskProgressPayload{
			ID: run.task.ID, Version: run.task.Version, Progress: p.Fraction, Speed: run.task.Speed, EtaSec: p.EtaSec,
		})
	})

	// 用应用 ctx 收尾（run 的 ctx 可能已被取消）。
	fin := m.appCtx
	if fin == nil {
		fin = context.Background()
	}
	m.finishInstall(fin, cfg, run, info, err)
}

func (m *Manager) finishInstall(ctx context.Context, cfg Config, run *installRun, info ffmpeg.Info, err error) {
	now := time.Now().UnixMilli()
	canceled := err != nil && errors.Is(err, context.Canceled)

	m.mu.Lock()
	run.task.Version++
	run.task.FinishedAt = now
	switch {
	case err == nil:
		run.task.Status, run.task.Progress, run.task.Speed, run.task.EtaSec = TaskSucceeded, 1, "", 0
	case canceled:
		run.task.Status = TaskCanceled
	default:
		run.task.Status = TaskFailed
		run.task.Error = installError(err)
	}
	final := run.task
	m.emit(EventTaskStatus, taskStatusPayload{
		ID: final.ID, Version: final.Version, Status: final.Status, Error: final.Error,
		OutputPath: final.OutputPath, FinishedAt: final.FinishedAt,
	})
	m.install = nil
	run.cancel()
	m.mu.Unlock()

	switch {
	case err == nil:
		b := info.Binaries
		m.set(ctx, FFmpegStatus{State: ffmpeg.StateReady, Path: b.FFmpeg, Version: info.Version, Source: info.Source}, &b)
	case canceled:
		// 取消后回到安装前的真实状态，重新检测（保留 .part，下次继续）。
		_, _ = m.Recheck(ctx)
	default:
		// 失败保留 .part；状态 failed 带错误，用户可重试或手动指定路径。
		m.set(ctx, FFmpegStatus{State: ffmpeg.StateFailed, TaskID: final.ID, Error: final.Error}, nil)
	}
}

func installError(err error) *apperr.AppError {
	switch {
	case errors.Is(err, ffmpeg.ErrChecksum):
		return apperr.Wrap(apperr.Internal, "下载的文件校验失败（SHA256 不一致），请重试或换镜像", err)
	case ffmpeg.IsUnavailable(err):
		return apperr.Wrap(apperr.UnsupportedPlatform, "当前系统暂无可用的 ffmpeg 下载源", err)
	}
	return apperr.Wrap(apperr.Internal, "ffmpeg 安装失败，已下载的部分会保留，可点击重试继续", err)
}

func formatSpeed(bps float64) string {
	switch {
	case bps <= 0:
		return ""
	case bps >= 1<<20:
		return fmt.Sprintf("%.1f MB/s", bps/(1<<20))
	default:
		return fmt.Sprintf("%.0f KB/s", bps/(1<<10))
	}
}

// CancelInstall 取消进行中的安装。没有进行中的安装时什么也不做（返回 nil），
// 这样前端点"取消"与安装刚好结束的竞态不会报错。已下载的部分保留在 <数据目录>/tmp，下次安装继续。
func (m *Manager) CancelInstall() error {
	m.mu.Lock()
	run := m.install
	m.mu.Unlock()
	if run != nil {
		run.cancel()
	}
	return nil
}

// InstallOptions 描述当前平台的安装选项，前端据此决定是否显示"使用国内镜像"开关。
type InstallOptions struct {
	Platform  string   `json:"platform"`  // 如 windows-amd64
	Supported bool     `json:"supported"` // 当前平台有可用的下载源
	Mirrors   []string `json:"mirrors"`   // 可用的镜像名（不含默认源 ""），没有时是 []
}

// GetInstallOptions 返回当前平台可选的安装镜像。Installer 未初始化时返回 INTERNAL。
func (m *Manager) GetInstallOptions() (InstallOptions, error) {
	cfg, err := m.ready()
	if err != nil {
		return InstallOptions{}, err
	}
	if cfg.Installer == nil {
		return InstallOptions{}, apperr.New(apperr.Internal, "安装功能未初始化")
	}
	return InstallOptions{
		Platform:  cfg.Installer.PlatformName(),
		Supported: cfg.Installer.Supported(),
		Mirrors:   cfg.Installer.AvailableMirrors(),
	}, nil
}

// preflightInstall 在提交任务前同步检查平台和镜像，错误已映射为契约错误码。
func preflightInstall(in *ffmpeg.Installer, mirror string) error {
	err := in.Preflight(mirror)
	if err == nil {
		return nil
	}
	var me *ffmpeg.MirrorError
	switch {
	case ffmpeg.IsUnavailable(err):
		return apperr.Wrap(apperr.UnsupportedPlatform, "当前系统暂无可用的 ffmpeg 下载源，请手动指定 ffmpeg 所在位置", err)
	case errors.As(err, &me):
		msg := fmt.Sprintf("当前平台不支持镜像 %q", me.Mirror)
		detail := "可用的镜像：无，请使用默认源（mirror 传空字符串）"
		if len(me.Available) > 0 {
			detail = "可用的镜像：" + strings.Join(me.Available, "、") + "；或传空字符串使用默认源"
		}
		return apperr.New(apperr.InvalidArgument, msg).WithDetail(detail)
	}
	return apperr.Wrap(apperr.InvalidArgument, "安装参数不合法", err)
}

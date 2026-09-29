package system

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/task"
)

// 安装任务由任务管理器（internal/task）调度：类型 ffmpeg_install，进 batch 池，
// task:created / task:progress / task:status 事件由任务管理器统一发送（契约第 5 节）。
// 这里负责把任务的生命周期映射成 ffmpeg 状态机：
// 提交 → installing（TaskID 为任务 ID）→ ready | failed；取消 / 中断 → 重新检测。

// installRun 是进行中的安装（从提交到任务结束）。
type installRun struct {
	taskID string
}

func (m *Manager) installingStatus() (FFmpegStatus, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.install == nil {
		return FFmpegStatus{}, false
	}
	return m.status, true
}

// beginInstallLocked 进入 installing 状态。调用方持有 m.mu。已经在同一个任务上安装时无操作。
func (m *Manager) beginInstallLocked(taskID string) {
	if m.install != nil && m.install.taskID == taskID {
		return
	}
	m.install = &installRun{taskID: taskID}
	st := FFmpegStatus{State: ffmpeg.StateInstalling, TaskID: taskID}
	m.status = st
	ffmpeg.SetCurrent(nil) // 安装期间不放行依赖 ffmpeg 的功能，避免与替换文件冲突
	if m.cfg.Emitter != nil {
		m.cfg.Emitter.Emit(EventFFmpegStatus, st)
	}
}

// Install 启动 ffmpeg 下载安装（契约 9.3），立即返回任务，实际工作在任务管理器的 batch 池里进行。
//
//   - 幂等：已有进行中（排队或运行）的安装时直接返回该任务，不开第二个下载；
//   - mirror 只接受 "" 和 "cn"，其他值返回 INVALID_ARGUMENT；
//   - 当前平台没有下载源返回 UNSUPPORTED_PLATFORM；
//   - 状态：任意 → installing（TaskID 为任务 ID）→ ready | failed；取消后重新检测。
//     每次状态变化推送 ffmpeg:status，进度通过 task:progress 推送，不塞进 ffmpeg:status。
func (m *Manager) Install(ctx context.Context, mirror string) (task.Task, error) {
	cfg, err := m.ready()
	if err != nil {
		return task.Task{}, err
	}
	if t, ok := m.currentInstallTask(); ok {
		return t, nil
	}
	if !ffmpeg.ValidMirror(mirror) {
		return task.Task{}, apperr.New(apperr.InvalidArgument, fmt.Sprintf("不支持的镜像 %q，只能是空字符串（默认源）或 \"cn\"", mirror))
	}
	if cfg.Installer == nil {
		return task.Task{}, apperr.New(apperr.Internal, "安装功能未初始化")
	}
	if cfg.Tasks == nil {
		return task.Task{}, apperr.New(apperr.Internal, "任务管理器未初始化")
	}
	if err := cfg.Installer.Preflight(mirror); err != nil {
		if ffmpeg.IsUnavailable(err) {
			return task.Task{}, apperr.Wrap(apperr.UnsupportedPlatform, "当前系统暂无可用的 ffmpeg 下载源，请手动指定 ffmpeg 所在位置", err)
		}
		return task.Task{}, apperr.Wrap(apperr.InvalidArgument, "安装参数不合法", err)
	}

	params, _ := json.Marshal(installParams{Mirror: mirror})

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.install != nil { // 检查之后有并发调用抢先提交了
		return m.installTaskLocked(), nil
	}
	t, err := cfg.Tasks.Submit(task.Spec{
		Type: task.TypeFFmpegInstall, Title: "安装 ffmpeg", OutputPath: cfg.Installer.BinDir, Params: string(params),
	}, m.newInstallRunner(cfg, mirror))
	if err != nil {
		return task.Task{}, err
	}
	m.beginInstallLocked(t.ID)
	return t, nil
}

type installParams struct {
	Mirror string `json:"mirror"`
}

// currentInstallTask 返回进行中的安装任务。
func (m *Manager) currentInstallTask() (task.Task, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.install == nil {
		return task.Task{}, false
	}
	return m.installTaskLocked(), true
}

func (m *Manager) installTaskLocked() task.Task {
	if m.cfg.Tasks != nil {
		if t, err := m.cfg.Tasks.Get(m.install.taskID); err == nil {
			return t
		}
	}
	return task.Task{ID: m.install.taskID, Type: task.TypeFFmpegInstall, Status: task.StatusRunning, InputPaths: []string{}}
}

// installRunner 实现 task.Runner 和 task.Finalizer。
type installRunner struct {
	m      *Manager
	cfg    Config
	mirror string
	info   ffmpeg.Info // Run 成功后的安装结果
}

func (m *Manager) newInstallRunner(cfg Config, mirror string) *installRunner {
	return &installRunner{m: m, cfg: cfg, mirror: mirror}
}

// registerInstallFactory 让 TaskService.Retry 能重新提交 ffmpeg_install 任务（用 Params 里的 mirror）。
// 已有安装在进行时返回 TASK_CONFLICT。
func (m *Manager) registerInstallFactory(cfg Config) {
	cfg.Tasks.RegisterFactory(task.TypeFFmpegInstall, func(old task.Task) (task.Runner, error) {
		var p installParams
		if old.Params != "" {
			if err := json.Unmarshal([]byte(old.Params), &p); err != nil {
				return nil, apperr.Wrap(apperr.InvalidArgument, "安装任务参数无效", err)
			}
		}
		if !ffmpeg.ValidMirror(p.Mirror) {
			return nil, apperr.New(apperr.InvalidArgument, "安装任务参数无效")
		}
		if err := cfg.Installer.Preflight(p.Mirror); err != nil {
			return nil, apperr.Wrap(apperr.UnsupportedPlatform, "当前系统暂无可用的 ffmpeg 下载源", err)
		}
		if _, busy := m.installingStatus(); busy {
			return nil, apperr.New(apperr.TaskConflict, "已有 ffmpeg 安装在进行")
		}
		return m.newInstallRunner(cfg, p.Mirror), nil
	})
}

func (r *installRunner) Run(ctx context.Context, report func(task.Progress)) (string, error) {
	// 通过 Retry 提交的任务没有走 Install()，在真正开始时补上 installing 状态。
	if info, ok := task.InfoFrom(ctx); ok {
		r.m.mu.Lock()
		r.m.beginInstallLocked(info.ID)
		r.m.mu.Unlock()
	}
	logw := task.LogWriter(ctx)
	fmt.Fprintf(logw, "开始安装 ffmpeg（镜像=%q）\n", r.mirror)
	info, err := r.cfg.Installer.Install(ctx, r.mirror, func(p ffmpeg.Progress) {
		report(task.Progress{Fraction: p.Fraction, Speed: formatSpeed(p.Speed), EtaSec: p.EtaSec})
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Fprintln(logw, "安装已取消，已下载的部分保留以便继续")
			return "", err
		}
		fmt.Fprintf(logw, "安装失败: %v\n", err)
		return "", installError(err)
	}
	r.info = info
	fmt.Fprintf(logw, "安装完成: %s (%s)\n", info.FFmpeg, info.Version)
	return r.cfg.Installer.BinDir, nil
}

// OnFinish 在任务进入终态后调用（task.Finalizer）：把结果映射成 ffmpeg 状态。
func (r *installRunner) OnFinish(t task.Task) {
	m := r.m
	m.mu.Lock()
	if m.install != nil && m.install.taskID == t.ID {
		m.install = nil
	}
	m.mu.Unlock()

	ctx := m.appCtx
	if ctx == nil {
		ctx = context.Background()
	}
	switch t.Status {
	case task.StatusSucceeded:
		b := r.info.Binaries
		m.set(ctx, FFmpegStatus{State: ffmpeg.StateReady, Path: b.FFmpeg, Version: r.info.Version, Source: r.info.Source}, &b)
	case task.StatusFailed:
		// 失败保留 .part；状态 failed 带错误，用户可重试或手动指定路径。
		m.set(ctx, FFmpegStatus{State: ffmpeg.StateFailed, TaskID: t.ID, Error: t.Error}, nil)
	default:
		// 取消 / 中断：回到安装前的真实状态，重新检测（保留 .part，下次继续）。
		_, _ = m.Recheck(ctx)
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

// CancelInstall 取消进行中的安装（排队或运行）。没有进行中的安装时什么也不做（返回 nil），
// 这样前端点"取消"与安装刚好结束的竞态不会报错。已下载的部分保留在 <数据目录>/tmp，下次安装继续。
func (m *Manager) CancelInstall() error {
	m.mu.Lock()
	run := m.install
	tasks := m.cfg.Tasks
	m.mu.Unlock()
	if run == nil || tasks == nil {
		return nil
	}
	if err := tasks.Cancel(run.taskID); err != nil && !apperr.Is(err, apperr.TaskConflict) && !apperr.Is(err, apperr.NotFound) {
		return err
	}
	return nil
}

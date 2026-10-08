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
	"FFmpegFree/internal/task"
)

// 安装任务由任务管理器（internal/task）调度：类型 ffmpeg_install，进 batch 池，
// task:created / task:progress / task:status 事件由任务管理器统一发送（契约第 5 节）。
// 这里负责把任务的生命周期映射成 ffmpeg 状态机：
// 提交 → installing（TaskID 为任务 ID）→ ready | failed；取消 / 中断 → 重新检测。

// installRun 是进行中的安装（从提交到任务结束）。
type installRun struct {
	taskID string // 空表示已占位但任务还在提交中
}

func (m *Manager) installingStatus() (FFmpegStatus, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.install == nil {
		return FFmpegStatus{}, false
	}
	return m.status, true
}

// claimInstall 占住"正在安装"这个位置：同一时刻只允许一个安装任务（提交中、排队、运行都算）。
// 已被占用返回 false。占位在 Submitted 时补上任务 ID，Abandoned / 任务结束时释放。
func (m *Manager) claimInstall() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.install != nil {
		return false
	}
	m.install = &installRun{}
	return true
}

// beginInstallLocked 进入 installing 状态。调用方持有 m.mu。已经在同一个任务上安装时无操作。
func (m *Manager) beginInstallLocked(taskID string) {
	if m.install != nil && m.install.taskID == taskID {
		return
	}
	m.install = &installRun{taskID: taskID}
	m.announceInstallingLocked(taskID)
}

func (m *Manager) announceInstallingLocked(taskID string) {
	st := FFmpegStatus{State: ffmpeg.StateInstalling, TaskID: taskID}
	m.status = st
	ffmpeg.SetCurrent(nil) // 安装期间不放行依赖 ffmpeg 的功能，避免与替换文件冲突
	if m.cfg.Emitter != nil {
		m.cfg.Emitter.Emit(EventFFmpegStatus, st)
	}
}

// waitInstallTask 返回进行中的安装任务；占位还没有任务 ID 时最多等 2 秒（提交只有几毫秒）。
func (m *Manager) waitInstallTask() (task.Task, bool) {
	deadline := time.Now().Add(2 * time.Second)
	for {
		m.mu.Lock()
		run := m.install
		if run == nil {
			m.mu.Unlock()
			return task.Task{}, false
		}
		if run.taskID != "" {
			t := m.installTaskLocked()
			m.mu.Unlock()
			return t, true
		}
		m.mu.Unlock()
		if time.Now().After(deadline) {
			return task.Task{}, true
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// Install 启动 ffmpeg 下载安装（契约 9.3），立即返回任务，实际工作在任务管理器的 batch 池里进行。
//
//   - 幂等：已有进行中（排队或运行）的安装时直接返回该任务，不开第二个下载；
//   - mirror 只接受 ""（默认源）和当前平台清单里真正有的镜像（见 InstallOptions）；
//     其他值（包括当前平台没有的 "cn"）返回 INVALID_ARGUMENT，detail 列出可选镜像，不会悄悄改用默认源；
//   - 当前平台没有下载源返回 UNSUPPORTED_PLATFORM；
//   - 状态：任意 → installing（TaskID 为任务 ID）→ ready | failed；取消后重新检测。
//     每次状态变化推送 ffmpeg:status，进度通过 task:progress 推送，不塞进 ffmpeg:status。
func (m *Manager) Install(ctx context.Context, mirror string) (task.Task, error) {
	cfg, err := m.ready()
	if err != nil {
		return task.Task{}, err
	}
	if t, ok := m.waitInstallTask(); ok {
		if t.ID == "" {
			return task.Task{}, apperr.New(apperr.TaskConflict, "转换组件安装正在提交，请稍后再试")
		}
		return t, nil
	}
	if cfg.Installer == nil {
		return task.Task{}, apperr.New(apperr.Internal, "安装功能未初始化")
	}
	if cfg.Tasks == nil {
		return task.Task{}, apperr.New(apperr.Internal, "任务管理器未初始化")
	}
	if err := preflightInstall(cfg.Installer, mirror); err != nil {
		return task.Task{}, err
	}
	if !m.claimInstall() { // 检查之后有并发调用抢先占位了
		if t, ok := m.waitInstallTask(); ok && t.ID != "" {
			return t, nil
		}
		return task.Task{}, apperr.New(apperr.TaskConflict, "已有转换组件安装在进行")
	}
	params, _ := json.Marshal(installParams{Mirror: mirror})
	r := m.newInstallRunner(cfg, mirror)
	r.claimed = true
	t, err := cfg.Tasks.Submit(task.Spec{
		Type: task.TypeFFmpegInstall, Title: "安装转换组件", OutputPath: cfg.Installer.BinDir, Params: string(params),
	}, r)
	if err != nil {
		r.Abandoned()
		return task.Task{}, err
	}
	return t, nil
}

// 目录不在任务里用不到 InputPaths，OutputPath 必须是绝对路径（Installer.BinDir 由 paths.Resolve 给出，本来就是绝对的）。

type installParams struct {
	Mirror string `json:"mirror"`
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
	m       *Manager
	cfg     Config
	mirror  string
	info    ffmpeg.Info // Run 成功后的安装结果
	claimed bool        // 已经占住了 m.install（Install / Retry 工厂里占位）
}

var _ task.Claimer = (*installRunner)(nil)

// Submitted 在任务落库后、task:created 之前调用：把占位补上任务 ID 并进入 installing 状态。
func (r *installRunner) Submitted(id string) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	if r.claimed && r.m.install != nil && r.m.install.taskID == "" {
		r.m.install.taskID = id
		r.m.announceInstallingLocked(id)
		return
	}
	r.m.beginInstallLocked(id)
}

// Abandoned 在占位后提交失败时释放占位。
func (r *installRunner) Abandoned() {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	if r.claimed && r.m.install != nil && r.m.install.taskID == "" {
		r.m.install = nil
	}
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
		if err := preflightInstall(cfg.Installer, p.Mirror); err != nil {
			return nil, err
		}
		// 工厂里就占位：造出 Runner 到任务真正开始之间（包括排队期间）都不允许再提交第二个安装。
		if !m.claimInstall() {
			return nil, apperr.New(apperr.TaskConflict, "已有转换组件安装在进行")
		}
		r := m.newInstallRunner(cfg, p.Mirror)
		r.claimed = true
		return r, nil
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
		return apperr.Wrap(apperr.UnsupportedPlatform, "当前系统暂无可用的转换组件下载源", err)
	}
	return apperr.Wrap(apperr.Internal, "转换组件安装失败，已下载的部分会保留，可点击重试继续", err)
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
	t, ok := m.waitInstallTask()
	m.mu.Lock()
	tasks := m.cfg.Tasks
	m.mu.Unlock()
	if !ok || t.ID == "" || tasks == nil {
		return nil
	}
	if err := tasks.Cancel(t.ID); err != nil && !apperr.Is(err, apperr.TaskConflict) && !apperr.Is(err, apperr.NotFound) {
		return err
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
		return apperr.Wrap(apperr.UnsupportedPlatform, "当前系统暂无可用的转换组件下载源，请手动指定转换组件所在位置", err)
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

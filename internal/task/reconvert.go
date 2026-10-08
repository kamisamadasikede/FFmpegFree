package task

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/store"
)

// ---------- 原地重转（契约 v0.24 / v0.24.1，6.17） ----------

// 重转模式（TaskPathCheck.reconvertMode，v0.24.1 架构师定名）。
const (
	ReconvertReplace    = "replace"    // 旧输出还在：成功后原子替换它
	ReconvertRegenerate = "regenerate" // 旧输出不在了（PM 15a）：按原参数重新生成，只在目标仍不存在时落盘
)

// reconvertBlock 的取值（TaskPathCheck.reconvertBlock，reconvertMode="" 时的原因）。
const (
	BlockInvalidState = "invalid_state"
	BlockOutputMoved  = "output_moved"
	BlockSourceGone   = "source_missing"
	BlockCopyNotReady = "copy_not_ready"
)

// 重转结束的结果（task:status.reconvertOutcome）。
const (
	OutcomeSucceeded   = "succeeded"
	OutcomeFailed      = "failed"
	OutcomeCanceled    = "canceled"
	OutcomeInterrupted = "interrupted"
)

// ReconvertSpec 是一次重转的新参数：成功时才写进记录（重转期间对外仍是旧快照，6.17.3 第 4 条）。
type ReconvertSpec struct {
	Params     string   // 新的 params JSON
	InputPaths []string // 新的 inputPaths（这一行当前的读取路径）
	Summary    string   // 日志里的 “[FFmpegFree] 重转：<summary>”
	Mode       string   // ReconvertReplace | ReconvertRegenerate（调用方用 ReconvertOutputMode 得出）
}

// reconvertPrev 是 reconvert_prev 列的 JSON（6.17.3 第 1 步）。
type reconvertPrev struct {
	Params           string      `json:"params"`
	InputPaths       []string    `json:"inputPaths"`
	OutputPath       string      `json:"outputPath"`
	Result           *TaskResult `json:"result,omitempty"`
	Progress         float64     `json:"progress"`
	StartedAt        int64       `json:"startedAt"`
	FinishedAt       int64       `json:"finishedAt"`
	Encoder          string      `json:"encoder,omitempty"`
	EncoderDevice    string      `json:"encoderDevice,omitempty"`
	HWFallback       bool        `json:"hwFallback,omitempty"`
	HWFallbackReason string      `json:"hwFallbackReason,omitempty"`
	OutSize          int64       `json:"outSize"`
	OutMtimeNs       int64       `json:"outMtimeNs"`
	Mode             string      `json:"mode"`
}

// reconvertPending 是 reconvert_pending 列的 JSON（6.17.5 成功第 3 步）。
type reconvertPending struct {
	Params           string      `json:"params"`
	InputPaths       []string    `json:"inputPaths"`
	Result           *TaskResult `json:"result,omitempty"`
	StartedAt        int64       `json:"startedAt"`
	FinishedAt       int64       `json:"finishedAt"`
	Encoder          string      `json:"encoder,omitempty"`
	EncoderDevice    string      `json:"encoderDevice,omitempty"`
	HWFallback       bool        `json:"hwFallback,omitempty"`
	HWFallbackReason string      `json:"hwFallbackReason,omitempty"`
}

type reconvertState struct {
	prev reconvertPrev
	spec ReconvertSpec
	temp string
}

// ReconvertTempPath 是重转的临时文件：<目标目录>/<目标去扩展名>.reconvert-<taskId>.part.<扩展名>（6.17.4）。
func ReconvertTempPath(output, taskID string) string {
	ext := filepath.Ext(output)
	return strings.TrimSuffix(output, ext) + ".reconvert-" + taskID + ".part" + ext
}

var reconvertTempRe = regexp.MustCompile(`\.reconvert-([0-9A-Za-z]{1,64})\.part(\.[^.]*)?$`)

// ReconvertOutputMode 判断一条已成功的 convert 记录的旧输出能否重转（6.17.1，v0.24.1 PM 15）：
// 输出不在 → regenerate；在且是这条记录的输出（6.14.4 第 3 步的安全条件）→ replace；其余 → block=output_moved。
func ReconvertOutputMode(t Task) (mode, block string) {
	if t.OutputPath == "" || !filepath.IsAbs(t.OutputPath) {
		return "", BlockOutputMoved
	}
	_, err := os.Lstat(t.OutputPath)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return ReconvertRegenerate, ""
	case err != nil:
		return "", BlockOutputMoved
	}
	tt := t
	tt.Status = StatusSucceeded
	if reason := unsafeToDeleteOutput(tt); reason != "" {
		return "", BlockOutputMoved
	}
	return ReconvertReplace, ""
}

// ReconvertChecker 由转换服务注册：返回副本 / 源文件方面挡住重转的原因（copy_not_ready / source_missing），没有问题返回 ""。
type ReconvertChecker func(t Task) string

// SetReconvertChecker 注册 CheckPaths 用的重转检查（转换服务在 New 里调用）。
func (m *Manager) SetReconvertChecker(f ReconvertChecker) {
	m.mu.Lock()
	m.rcCheck = f
	m.mu.Unlock()
}

// reconvertCheck 给 CheckPaths 用：返回 (mode, block)，mode="" 表示不能重转。顺序同 Reconvert 的同步校验（v0.24.1）：
// 状态 → 副本没就绪 → 源文件不在 → 旧输出被换掉。
func (m *Manager) reconvertCheck(t Task) (string, string) {
	if t.Type != TypeConvert || t.Status != StatusSucceeded || t.Reconverting {
		return "", BlockInvalidState
	}
	m.mu.Lock()
	f := m.rcCheck
	m.mu.Unlock()
	if f != nil {
		if b := f(t); b != "" {
			return "", b
		}
	}
	return ReconvertOutputMode(t)
}

// InvalidStateError 是 TASK_CONFLICT reason=invalid_state（6.17.1；不新增错误码，架构师定）。
func InvalidStateError(msg string) error {
	return apperr.New(apperr.TaskConflict, msg).WithDetail("reason=invalid_state")
}

// OutputMovedError 是 TASK_CONFLICT reason=output_moved（开始重转前）。
func OutputMovedError() error {
	return apperr.New(apperr.TaskConflict, "原来的输出文件已被移动或替换，不能重转").WithDetail("reason=output_moved")
}

// Reconvert 在同一条记录上原地重转（6.17.3）：调用方已完成全部同步校验并造好 Runner（直接写 ReconvertTempPath）。
// 这里再确认一次状态与旧输出（防并发），保存快照、更新记录、日志、发一次 task:status（queued、reconverting），入队。
func (m *Manager) Reconvert(taskID string, spec ReconvertSpec, r Runner) (Task, error) {
	if r == nil {
		return Task{}, apperr.New(apperr.InvalidArgument, "任务缺少执行体")
	}
	old, err := m.Get(taskID)
	if err != nil {
		return Task{}, err
	}
	if old.Type != TypeConvert {
		return Task{}, apperr.New(apperr.InvalidArgument, "不是转换记录")
	}
	if old.Reconverting {
		return Task{}, InvalidStateError("这条记录正在重转")
	}
	if old.Status != StatusSucceeded {
		return Task{}, InvalidStateError("只有已完成的记录可以重转")
	}
	mode, block := ReconvertOutputMode(old)
	if block != "" || (spec.Mode != "" && mode != spec.Mode) {
		return Task{}, OutputMovedError()
	}
	prev := reconvertPrev{
		Params: old.Params, InputPaths: nonNil(old.InputPaths), OutputPath: old.OutputPath, Result: old.Result,
		Progress: old.Progress, StartedAt: old.StartedAt, FinishedAt: old.FinishedAt,
		Encoder: old.Encoder, EncoderDevice: old.EncoderDevice, HWFallback: old.HWFallback, HWFallbackReason: old.HWFallbackReason,
		Mode: mode,
	}
	if mode == ReconvertReplace {
		fi, err := os.Lstat(old.OutputPath)
		if err != nil {
			return Task{}, OutputMovedError()
		}
		prev.OutSize, prev.OutMtimeNs = fi.Size(), fi.ModTime().UnixNano()
	}
	pj, _ := json.Marshal(prev)
	spec.Mode = mode
	spec.InputPaths = nonNil(spec.InputPaths)

	t := old
	t.InputPaths = nonNil(old.InputPaths)
	t.Status = StatusQueued
	t.Progress = 0
	t.Speed, t.EtaSec = "", 0
	t.Fps, t.BitrateKbps, t.DroppedFrames = 0, 0, 0
	t.Error = nil
	t.LastReconvertError = nil
	t.Reconverting = true
	t.ReconvertPrev, t.ReconvertPending = string(pj), ""
	t.HiddenInTaskCenter = false
	t.Encoder, t.EncoderDevice, t.HWFallback, t.HWFallbackReason = "", "", false, ""
	if er, ok := r.(EncoderReporter); ok {
		ei := er.EncoderInfo()
		t.Encoder, t.EncoderDevice, t.HWFallback, t.HWFallbackReason = ei.Encoder, ei.Device, ei.HWFallback, ei.HWFallbackReason
	}
	t.Version = old.Version + 1
	e := newEntry(m, t, r)
	if t.LogPath == "" {
		t.LogPath = e.log.path()
	}
	e.task = t
	e.rc = &reconvertState{prev: prev, spec: spec, temp: ReconvertTempPath(old.OutputPath, t.ID)}

	m.mu.Lock()
	if m.closing {
		m.mu.Unlock()
		return Task{}, apperr.New(apperr.Internal, "应用正在退出，无法重转")
	}
	if _, dup := m.entries[t.ID]; dup {
		m.mu.Unlock()
		return Task{}, InvalidStateError("这条记录正在重转")
	}
	m.entries[t.ID] = e
	m.mu.Unlock()
	rollback := func() {
		m.mu.Lock()
		delete(m.entries, t.ID)
		m.mu.Unlock()
		m.namer.releaseOwner(t.ID)
		e.log.close()
	}
	if cur, err := m.cfg.Store.GetTask(context.Background(), t.ID); err != nil {
		rollback()
		return Task{}, notFoundOr(err, "任务不存在")
	} else if cur.Version != old.Version || cur.Status != old.Status {
		rollback()
		return Task{}, apperr.New(apperr.TaskConflict, "任务状态已变化，请刷新后再试")
	}
	// 6.17.3 第 2 步：目标名继续由这条记录占着（被别的任务占着的话不能重转）。
	if !m.namer.claim(old.OutputPath, t.ID) {
		rollback()
		return Task{}, OutputMovedError()
	}
	os.Remove(e.rc.temp) // 上一次异常留下的同名临时文件（名字里带 taskId，只可能是自己的）
	if err := m.cfg.Store.UpdateTask(context.Background(), t); err != nil {
		rollback()
		return Task{}, apperr.Wrap(apperr.IOError, "保存任务失败", err)
	}
	fmt.Fprintf(e.log, "\n[FFmpegFree] 重转：%s\n", spec.Summary)
	snap := e.snapshot()
	zero, notHidden := 0.0, false
	m.emit(EventStatus, StatusEvent{
		ID: t.ID, Version: t.Version, Status: StatusQueued, OutputPath: t.OutputPath,
		Encoder: t.Encoder, EncoderDevice: t.EncoderDevice, HWFallback: t.HWFallback, HWFallbackReason: t.HWFallbackReason,
		Progress: &zero, HiddenInTaskCenter: &notHidden, Reconverting: rcFlag(t),
	})
	m.enqueue(e)
	return snap, nil
}

func outcomeFor(st Status) string {
	switch st {
	case StatusInterrupted:
		return OutcomeInterrupted
	case StatusFailed:
		return OutcomeFailed
	case StatusSucceeded:
		return OutcomeSucceeded
	}
	return OutcomeCanceled
}

// 替换旧输出用的文件操作（测试里替换以模拟“被占用”等）。
var (
	replaceFile = os.Rename
	lstatFile   = os.Lstat
)

// finishReconvert 是重转任务 Run 返回后的收尾（6.17.5）。
func (m *Manager) finishReconvert(e *entry, closing bool, err error, out string) {
	switch {
	case closing && e.ctx.Err() != nil && !e.cancelRequested() && (err == nil || errors.Is(err, context.Canceled)):
		m.endReconvert(e, OutcomeInterrupted, nil)
	case err == nil:
		if aerr := m.commitReconvert(e, resultOf(e.runner)); aerr != nil {
			m.endReconvert(e, OutcomeFailed, aerr)
		}
	case e.ctx.Err() != nil && (errors.Is(err, context.Canceled) || e.cancelRequested()):
		m.endReconvert(e, OutcomeCanceled, nil)
	default:
		m.endReconvert(e, OutcomeFailed, apperr.From(err))
	}
}

func movedDuringError() *apperr.AppError {
	return apperr.New(apperr.TaskConflict, "原来的输出文件在重转期间被移动或替换，新结果没有保存").WithDetail("reason=output_moved")
}

// replaceError 把替换失败归到 6.17.5 的 in_use / permission / io。
func replaceError(err error, target string) *apperr.AppError {
	reason, msg := "io", "替换输出文件失败"
	switch {
	case isInUse(err):
		reason, msg = "in_use", "输出文件正被其他程序使用，新结果没能替换进去"
	case errors.Is(err, fs.ErrPermission):
		reason, msg = "permission", "没有权限替换输出文件"
	}
	return apperr.New(apperr.IOError, msg).WithDetail("reason=" + reason + "\n" + target)
}

// commitReconvert：复核目标 → 写 reconvert_pending → 原子替换 / 无覆盖落盘 → 应用。返回非 nil 表示按失败处理（临时文件由 endReconvert 删）。
func (m *Manager) commitReconvert(e *entry, res *TaskResult) *apperr.AppError {
	rc := e.rc
	target := rc.prev.OutputPath
	fi, err := lstatFile(target)
	switch rc.prev.Mode {
	case ReconvertRegenerate:
		if err == nil || !errors.Is(err, os.ErrNotExist) {
			return movedDuringError() // 期间那里出现了别的文件：绝不覆盖（PM 15a）
		}
	default:
		if err != nil || !fi.Mode().IsRegular() || fi.Size() != rc.prev.OutSize || fi.ModTime().UnixNano() != rc.prev.OutMtimeNs {
			return movedDuringError()
		}
	}
	e.mu.Lock()
	pend := reconvertPending{
		Params: rc.spec.Params, InputPaths: rc.spec.InputPaths, Result: res,
		StartedAt: e.task.StartedAt, FinishedAt: time.Now().UnixMilli(),
		Encoder: e.task.Encoder, EncoderDevice: e.task.EncoderDevice, HWFallback: e.task.HWFallback, HWFallbackReason: e.task.HWFallbackReason,
	}
	pj, _ := json.Marshal(pend)
	e.task.ReconvertPending = string(pj)
	e.persistLocked()
	e.mu.Unlock()

	var rerr error
	if rc.prev.Mode == ReconvertRegenerate {
		rerr = commitPart(rc.temp, target)
		if errors.Is(rerr, errTargetExists) {
			e.clearPending()
			return movedDuringError()
		}
	} else {
		rerr = replaceFile(rc.temp, target)
	}
	if rerr != nil {
		m.logf("重转任务 %s 替换输出 %s 失败: %v", e.task.ID, target, rerr)
		e.clearPending()
		return replaceError(rerr, target)
	}
	e.applyPending(m, pend)
	return nil
}

func (e *entry) clearPending() {
	e.mu.Lock()
	e.task.ReconvertPending = ""
	e.mu.Unlock()
}

// applyPending 是成功的第 5、6 步。
func (e *entry) applyPending(m *Manager, p reconvertPending) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.terminal {
		return
	}
	e.terminal = true
	if e.flushTimer != nil {
		e.flushTimer.Stop()
	}
	t := &e.task
	t.Params, t.InputPaths, t.Result = p.Params, nonNil(p.InputPaths), p.Result
	t.Status, t.Progress, t.Error = StatusSucceeded, 1, nil
	t.StartedAt, t.FinishedAt = p.StartedAt, p.FinishedAt
	t.Encoder, t.EncoderDevice, t.HWFallback, t.HWFallbackReason = p.Encoder, p.EncoderDevice, p.HWFallback, p.HWFallbackReason
	t.Speed, t.EtaSec, t.Fps, t.BitrateKbps, t.DroppedFrames = "", 0, 0, 0, 0
	t.Reconverting, t.ReconvertPrev, t.ReconvertPending, t.LastReconvertError = false, "", "", nil
	t.Version++
	e.persistLocked()
	prog := t.Progress
	m.emit(EventStatus, StatusEvent{
		ID: t.ID, Version: t.Version, Status: StatusSucceeded, OutputPath: t.OutputPath,
		StartedAt: t.StartedAt, FinishedAt: t.FinishedAt,
		Encoder: t.Encoder, EncoderDevice: t.EncoderDevice, HWFallback: t.HWFallback, HWFallbackReason: t.HWFallbackReason,
		Progress: &prog, Result: t.Result, Reconverting: rcFlag(*t), ReconvertOutcome: OutcomeSucceeded,
	})
	e.log.close()
}

// endReconvert 是失败 / 取消 / 中断：删临时文件，按快照恢复成 succeeded，发终态事件（6.17.5）。
// 中断（应用退出）只删临时文件、发事件，不落库：库里仍是 reconverting=1，下次启动由 RecoverReconverts 恢复并计数（v0.24.1 PM 13）。
func (m *Manager) endReconvert(e *entry, outcome string, aerr *apperr.AppError) {
	rc := e.rc
	if err := removeRegular(rc.temp); err != nil {
		m.logf("删除重转临时文件 %s 失败: %v", rc.temp, err)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.terminal {
		return
	}
	e.terminal = true
	if e.flushTimer != nil {
		e.flushTimer.Stop()
	}
	t := &e.task
	restorePrev(t, rc.prev)
	t.Version++
	if outcome == OutcomeFailed && aerr != nil {
		t.LastReconvertError = &store.ReconvertError{Code: string(aerr.Code), Message: aerr.Message, Detail: aerr.Detail, At: time.Now().UnixMilli()}
		fmt.Fprintf(e.log, "[FFmpegFree] 重转失败，原来的文件没有变动：%s\n", aerr.Message)
	}
	if outcome != OutcomeInterrupted {
		e.persistLocked()
	}
	prog := t.Progress
	m.emit(EventStatus, StatusEvent{
		ID: t.ID, Version: t.Version, Status: StatusSucceeded, OutputPath: t.OutputPath,
		StartedAt: t.StartedAt, FinishedAt: t.FinishedAt,
		Encoder: t.Encoder, EncoderDevice: t.EncoderDevice, HWFallback: t.HWFallback, HWFallbackReason: t.HWFallbackReason,
		Progress: &prog, Result: t.Result, Reconverting: rcFlag(*t), ReconvertOutcome: outcome, LastReconvertError: t.LastReconvertError,
	})
	e.log.close()
}

func restorePrev(t *Task, p reconvertPrev) {
	t.Params, t.InputPaths, t.OutputPath, t.Result = p.Params, nonNil(p.InputPaths), p.OutputPath, p.Result
	t.Status, t.Progress, t.Error = StatusSucceeded, p.Progress, nil
	t.StartedAt, t.FinishedAt = p.StartedAt, p.FinishedAt
	t.Encoder, t.EncoderDevice, t.HWFallback, t.HWFallbackReason = p.Encoder, p.EncoderDevice, p.HWFallback, p.HWFallbackReason
	t.Speed, t.EtaSec, t.Fps, t.BitrateKbps, t.DroppedFrames = "", 0, 0, 0, 0
	t.Reconverting, t.ReconvertPrev, t.ReconvertPending, t.LastReconvertError = false, "", "", nil
}

// removeRegular 删除 p（只删普通文件、不跟随符号链接）；不在返回 nil。
func removeRegular(p string) error {
	if p == "" {
		return nil
	}
	fi, err := os.Lstat(p)
	if err != nil {
		return nil
	}
	if !fi.Mode().IsRegular() {
		return nil
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// removeReconvertTemp 是删除记录时的兜底（6.17.6 第 ④ 步）：删这条记录的 *.reconvert-<id>.part.*，失败只记日志。
func (m *Manager) removeReconvertTemp(t Task) {
	if t.Type != TypeConvert || t.OutputPath == "" || !filepath.IsAbs(t.OutputPath) {
		return
	}
	if err := removeRegular(ReconvertTempPath(t.OutputPath, t.ID)); err != nil {
		m.logf("删除任务 %s 的重转临时文件失败: %v", t.ID, err)
	}
}

// ReconvertStore 是启动恢复需要的存储能力（*store.Store 实现）。
type ReconvertStore interface {
	ListReconvertingTasks(ctx context.Context) ([]Task, error)
	ConvertOutputPaths(ctx context.Context) ([]string, error)
	UpdateTask(ctx context.Context, t Task) error
}

var _ ReconvertStore = (*store.Store)(nil)

// RecoverReconverts 是启动时的重转崩溃恢复（6.17.5，必须在 store.MarkInterrupted 之前调用）：
//  1. 有 reconvert_pending、临时文件不在、目标在：改名已完成，按成功收尾（不发事件）；
//  2. 其余：删临时文件，按 reconvert_prev 恢复成 succeeded（不写 lastReconvertError），计入返回值（v0.24.1 PM 13）；
//  3. 孤儿临时文件扫描：每个 convert 记录输出所在目录 + extraDirs（实际输出目录），删匹配 *.reconvert-<id>.part.* 的普通文件，不递归。
//
// 单条失败只记日志。
func RecoverReconverts(ctx context.Context, st ReconvertStore, extraDirs []string, logf func(string, ...any)) (restored int, err error) {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	ts, err := st.ListReconvertingTasks(ctx)
	if err != nil {
		return 0, err
	}
	for _, t := range ts {
		var prev reconvertPrev
		if json.Unmarshal([]byte(t.ReconvertPrev), &prev) != nil || prev.OutputPath == "" {
			// 快照坏了：至少别让它卡在“重转中”。
			prev = reconvertPrev{Params: t.Params, InputPaths: t.InputPaths, OutputPath: t.OutputPath, Result: t.Result,
				Progress: 1, StartedAt: t.StartedAt, FinishedAt: t.FinishedAt}
		}
		temp := ReconvertTempPath(prev.OutputPath, t.ID)
		var pend reconvertPending
		done := t.ReconvertPending != "" && json.Unmarshal([]byte(t.ReconvertPending), &pend) == nil &&
			!exists(temp) && exists(prev.OutputPath)
		nt := t
		if done {
			restorePrev(&nt, prev)
			nt.Params, nt.InputPaths, nt.Result = pend.Params, nonNil(pend.InputPaths), pend.Result
			nt.StartedAt, nt.FinishedAt = pend.StartedAt, pend.FinishedAt
			nt.Encoder, nt.EncoderDevice, nt.HWFallback, nt.HWFallbackReason = pend.Encoder, pend.EncoderDevice, pend.HWFallback, pend.HWFallbackReason
			nt.Progress = 1
		} else {
			if err := removeRegular(temp); err != nil {
				logf("删除重转临时文件 %s 失败: %v", temp, err)
			}
			restorePrev(&nt, prev)
		}
		nt.Version++
		if err := st.UpdateTask(ctx, nt); err != nil {
			logf("恢复重转任务 %s 失败: %v", t.ID, err)
			continue
		}
		if !done {
			restored++
		}
	}
	// 3. 孤儿临时文件。
	dirs := map[string]bool{}
	var order []string
	add := func(d string) {
		if d == "" || !filepath.IsAbs(d) {
			return
		}
		k := nameKey(d)
		if !dirs[k] {
			dirs[k] = true
			order = append(order, d)
		}
	}
	outs, err := st.ConvertOutputPaths(ctx)
	if err != nil {
		logf("读取转换记录的输出位置失败: %v", err)
	}
	for _, p := range outs {
		if p != "" && filepath.IsAbs(p) {
			add(filepath.Dir(p))
		}
	}
	for _, d := range extraDirs {
		add(d)
	}
	for _, d := range order {
		ents, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, de := range ents {
			if !reconvertTempRe.MatchString(de.Name()) {
				continue
			}
			p := filepath.Join(d, de.Name())
			if err := removeRegular(p); err != nil {
				logf("删除重转临时文件 %s 失败: %v", p, err)
			}
		}
	}
	return restored, nil
}

package task

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
)

// FFmpegRunner 是 ffmpeg 类任务的通用 Runner（转换、剪辑渲染、直播都可以复用）。
//
// 写文件的任务设置 Output：Runner 会用 RunWithPart 管理 <name>.part.<ext> 与原子改名，
// BuildArgs 收到 .part 路径，把它放在参数末尾作为输出。不写文件的任务（直播推流）留空 Output，BuildArgs 收到空串。
//
// 两遍编码 / 按目标大小压缩暂缓（契约 v0.7.2），Runner 目前只支持单次 ffmpeg 调用。
type FFmpegRunner struct {
	Exe string // ffmpeg 绝对路径，来自 ffmpeg.Require()
	// BuildArgs 生成 ffmpeg 参数（不含 -progress 等，由 ffmpeg.Run 添加）。
	BuildArgs func(partPath string) []string
	// Output 是期望的输出路径；为空表示任务不产生文件。
	Output string
	// DurationSec 是输入的总时长，用来把 out_time 换算成 0~1 的进度；<=0 时进度保持 0（直播用 -1，见 Live）。
	DurationSec float64
	// ProgressBase / ProgressScale 把本次 ffmpeg 的进度映射到任务整体进度：
	// fraction = ProgressBase + f*ProgressScale。ProgressScale 为 0 时按 1 处理。
	ProgressBase, ProgressScale float64
	// Live 表示直播类任务：进度恒报告 -1，取消时优雅停止（q / SIGINT，最多 5 秒）。
	Live bool
	// Stdin 见 ffmpeg.RunOptions.Stdin。
	Stdin io.Reader
	// Classify 见 ffmpeg.RunOptions.Classify。
	Classify func(stderrTail string, exitErr error) *apperr.AppError
	// TailLines 见 ffmpeg.RunOptions.TailLines。
	TailLines int

	// 以下是直播任务用的扩展（契约 6.10），其他任务留空即可。

	// Redact 见 ffmpeg.RunOptions.Redact：stderr 每行先脱敏，再进入日志、尾部缓冲和 Classify。
	Redact func(string) string
	// GracePeriod 见 ffmpeg.RunOptions.GracePeriod；0 用默认 5 秒（有存档的直播会话设 15 秒）。
	GracePeriod time.Duration
	// GracefulOnlyAfterProgress 为 true 时，"推流已开始"之前取消直接强杀（连接阶段没有需要收尾的东西）。
	// "已开始"由 ReportGate 判定；ReportGate 为空时以第一条 progress 为准。
	GracefulOnlyAfterProgress bool
	// StrictGracefulExit 见 ffmpeg.RunOptions.StrictGracefulExit。
	StrictGracefulExit bool
	// ReportGate 不为空时，只有它对某条 progress 返回 true（此后一直上报）才向任务管理器 report：
	// 直播用它把"已开始"定义为第一条 total_size>0 的 progress，之前的 progress 不上报，
	// 这样前端"收到第一条 task:progress = 已经在推"的判断与 Runner 的错误分类一致。同步调用，要快速返回。
	ReportGate func(u ffmpeg.ProgressUpdate) bool
	// 以下是硬件编码接入（契约 9.7），不用硬件编码的任务留空即可。

	// Encoding 是任务一开始使用的视频编码器信息（实现 EncoderReporter，Submit 时写进 Task）。
	Encoding ffmpeg.EncoderInfo
	// HWEncoder 非空表示 BuildArgs 里用的是这个硬件编码器；此时 BuildCPUArgs 必须给出同一任务用 CPU 编码器的参数。
	HWEncoder string
	// BuildCPUArgs 生成 CPU 编码的参数（硬件编码启动失败后自动重试一次用）。
	BuildCPUArgs func(partPath string) []string
	// CPUEncoding 是回退到 CPU 后的编码器信息（Encoder / Device 已填好，HWFallback 与原因由 Runner 补）。
	CPUEncoding ffmpeg.EncoderInfo
	// NoBitrate 为 true 时不计算 bitrateKbps（走 tee 时 ffmpeg 的 total_size 恒为 N/A，契约 6.10：有存档时没有 bitrateKbps）。
	NoBitrate bool

	// 以下是 v0.24 的扩展。

	// DirectOutput 非空时不走 RunWithPart：直接写到这个路径（原地重转的临时文件，契约 6.17.4），
	// 失败 / 取消时删掉它；成功时返回它，由任务管理器复核并原子替换。不能和 Output 同时用。
	DirectOutput string
	// BuildFallbackArgs 非空时：ffmpeg 结束后没有产出文件（图片输出、时长未知时 -ss 1 越过结尾；不论退出码），用它再跑一次（契约 6.16.5）。
	BuildFallbackArgs func(partPath string) []string
}

// DesiredOutput 实现 DesiredOutputer：期望的输出路径（重名顺延前）。
func (r *FFmpegRunner) DesiredOutput() string { return r.Output }

// EncoderInfo 实现 EncoderReporter。
func (r *FFmpegRunner) EncoderInfo() ffmpeg.EncoderInfo { return r.Encoding }

// Run 实现 Runner。ffmpeg 的 stderr 会写入任务日志。
func (r *FFmpegRunner) Run(ctx context.Context, report func(Progress)) (string, error) {
	logw := LogWriter(ctx)
	// one 运行一次 ffmpeg，进度映射为 base + f*scale。
	// 返回 ffmpeg 的 stderr 尾部和"是否已经有输出进度（out_time > 0）"，供硬件编码回退判断。
	one := func(args []string, base, scale float64) (string, bool, error) {
		if scale == 0 {
			scale = 1
		}
		var lastOut float64
		var rate rateWindow
		var started atomic.Bool
		opts := ffmpeg.RunOptions{
			Exe:          r.Exe,
			Args:         args,
			Stdin:        r.Stdin,
			GracefulStop: r.Live,
			GracePeriod:  r.GracePeriod,
			TailLines:    r.TailLines,
			Classify:     r.Classify,
			Redact:       r.Redact,

			StrictGracefulExit: r.StrictGracefulExit,
			OnStderr:           func(line string) { io.WriteString(logw, line+"\n") },
			OnProgress: func(u ffmpeg.ProgressUpdate) {
				if !started.Load() {
					if r.ReportGate != nil && !r.ReportGate(u) {
						return
					}
					started.Store(true)
				}
				out := u.OutTimeSec
				if out <= 0 && !u.End {
					out = lastOut // out_time=N/A：沿用上一次的值，进度不回退
				}
				if out > lastOut {
					lastOut = out
				}
				p := Progress{Speed: u.Speed, OutTimeSec: out}
				switch {
				case r.Live:
					p.Fraction = -1
					p.Fps, p.DroppedFrames = u.Fps, u.Dropped
					if !r.NoBitrate {
						p.BitrateKbps = rate.add(out, u.TotalSize)
					}
				case r.DurationSec > 0:
					f := out / r.DurationSec
					if u.End {
						f = 1
					}
					if f > 1 {
						f = 1
					}
					p.Fraction = base + f*scale
					if u.SpeedX > 0 && f < 1 {
						p.EtaSec = (r.DurationSec - out) / u.SpeedX
					}
				default:
					p.Fraction = base
				}
				report(p)
			},
		}
		if r.GracefulOnlyAfterProgress {
			opts.CanGraceful = started.Load
		}
		res, err := ffmpeg.Run(ctx, opts)
		if err != nil && res.ExitCode != 0 && ctx.Err() == nil {
			// 退出码不放在界面提示里，写进任务日志方便排查（-1 通常是进程被外部结束或启动后立即崩溃）。
			io.WriteString(logw, fmt.Sprintf("[FFmpegFree] ffmpeg 退出码 %d\n", res.ExitCode))
		}
		return res.StderrTail, lastOut > 0, err
	}

	run := func(part string) error {
		tail, progressed, err := one(r.BuildArgs(part), r.ProgressBase, r.ProgressScale)
		// 没有产出就退回第一帧：ffmpeg 6 越过结尾时正常退出但不写文件，ffmpeg 7 会以 “Error while opening encoder”（退出码 234）失败，
		// 两种都按“第 1 秒没有画面”处理；第一帧也失败时报第二次的错误。
		if r.BuildFallbackArgs != nil && part != "" && !partProduced(part) && ctx.Err() == nil {
			io.WriteString(logw, "[FFmpegFree] 第 1 秒没有画面，改取第一帧\n")
			_, _, err = one(r.BuildFallbackArgs(part), r.ProgressBase, r.ProgressScale)
			return err
		}
		if err == nil || r.HWEncoder == "" || r.BuildCPUArgs == nil || ctx.Err() != nil {
			return err // 成功 / 没用硬件 / 已取消：取消绝不触发回退
		}
		if progressed && r.Live {
			return err // 直播：推流已经建立（有过输出）后中途失败不自动重试
		}
		reason, ok := ffmpeg.HWInitFailure(r.HWEncoder, tail)
		if !ok && !progressed && ffmpeg.HWStartCrash(r.HWEncoder, tail, false, partProduced(part)) {
			reason, ok = ffmpeg.ReasonEncoderStart, true
		}
		if !ok {
			return err
		}
		info := r.CPUEncoding
		info.HWFallback, info.HWFallbackReason = true, reason
		// 日志里 FFmpegFree 自己写的这一行不带编码器名（界面 / 日志统一叫“显卡编码”）；具体原因在上面 ffmpeg 自己的 stderr 里。
		io.WriteString(logw, "[FFmpegFree] 显卡编码启动失败，已自动改用 CPU 重试一次\n")
		ReportEncoder(ctx, info)
		_, _, err = one(r.BuildCPUArgs(part), r.ProgressBase, r.ProgressScale)
		return err
	}
	if r.DirectOutput != "" {
		if err := mkdirAll(filepath.Dir(r.DirectOutput), 0o755); err != nil {
			return "", outputIOError("创建输出目录失败", err)
		}
		if err := run(r.DirectOutput); err != nil {
			os.Remove(r.DirectOutput)
			return "", err
		}
		if !partProduced(r.DirectOutput) {
			os.Remove(r.DirectOutput)
			return "", apperr.New(apperr.ProcessFailed, "转换没有产生输出文件")
		}
		return r.DirectOutput, nil
	}
	if r.Output == "" {
		return "", run("")
	}
	return RunWithPart(ctx, r.Output, run)
}

// partProduced 判断输出临时文件是否已经有内容（不写文件的任务 part 为空，视为没有）。
func partProduced(part string) bool {
	if part == "" {
		return false
	}
	fi, err := os.Stat(part)
	return err == nil && fi.Size() > 0
}

// bitrateWindowSec 是直播码率滑动均值的窗口（秒，媒体时间）。
const bitrateWindowSec = 5

// rateWindow 用相邻 progress 的 total_size / out_time 增量算近 5 秒的输出码率（kbit/s）。
// out_time 不增长、total_size 未知（N/A，如 tee 输出）或回退时沿用上一个值，永远不会产生 NaN / Inf。
type rateWindow struct {
	pts  []ratePoint
	last float64
}

type ratePoint struct {
	out  float64
	size int64
}

func (w *rateWindow) add(out float64, size int64) float64 {
	if size <= 0 || out <= 0 {
		return w.last
	}
	if n := len(w.pts); n > 0 && (out <= w.pts[n-1].out || size < w.pts[n-1].size) {
		return w.last
	}
	w.pts = append(w.pts, ratePoint{out, size})
	// 丢掉窗口之外的旧点，但保留窗口起点之前最近的一个，让窗口覆盖满 5 秒。
	i := 0
	for i+1 < len(w.pts) && w.pts[i+1].out <= out-bitrateWindowSec {
		i++
	}
	w.pts = w.pts[i:]
	first := w.pts[0]
	dt := out - first.out
	if dt <= 0 {
		return w.last
	}
	v := float64(size-first.size) * 8 / dt / 1000
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return w.last
	}
	w.last = v
	return v
}

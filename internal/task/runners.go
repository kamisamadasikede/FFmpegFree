package task

import (
	"context"
	"io"

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
}

// Run 实现 Runner。ffmpeg 的 stderr 会写入任务日志。
func (r *FFmpegRunner) Run(ctx context.Context, report func(Progress)) (string, error) {
	logw := LogWriter(ctx)
	// one 运行一次 ffmpeg，进度映射为 base + f*scale。
	one := func(args []string, base, scale float64) error {
		if scale == 0 {
			scale = 1
		}
		var lastOut float64
		_, err := ffmpeg.Run(ctx, ffmpeg.RunOptions{
			Exe:          r.Exe,
			Args:         args,
			Stdin:        r.Stdin,
			GracefulStop: r.Live,
			TailLines:    r.TailLines,
			Classify:     r.Classify,
			OnStderr:     func(line string) { io.WriteString(logw, line+"\n") },
			OnProgress: func(u ffmpeg.ProgressUpdate) {
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
		})
		return err
	}

	run := func(part string) error { return one(r.BuildArgs(part), r.ProgressBase, r.ProgressScale) }
	if r.Output == "" {
		return "", run("")
	}
	return RunWithPart(ctx, r.Output, run)
}

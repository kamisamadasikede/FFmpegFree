package task

import (
	"context"
	"io"
	"path/filepath"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
)

// FFmpegRunner 是 ffmpeg 类任务的通用 Runner（转换、剪辑渲染、直播都可以复用）。
//
// 写文件的任务设置 Output：Runner 会用 RunWithPart 管理 <name>.part.<ext> 与原子改名，
// BuildArgs 收到 .part 路径，把它放在参数末尾作为输出。不写文件的任务（直播推流）留空 Output，BuildArgs 收到空串。
//
// 两遍编码：设置 BuildPassArgs（此时忽略 BuildArgs）。Runner 在任务专属临时目录里放 -passlogfile，
// 依次运行 pass 1（输出到 null，进度映射到 0~0.5）和 pass 2（输出 .part，进度映射到 0.5~1），结束后删除临时目录。
type FFmpegRunner struct {
	Exe string // ffmpeg 绝对路径，来自 ffmpeg.Require()
	// BuildArgs 生成 ffmpeg 参数（不含 -progress 等，由 ffmpeg.Run 添加）。
	BuildArgs func(partPath string) []string
	// BuildPassArgs 生成两遍编码每一遍的参数：pass 为 1 或 2，passLog 是 -passlogfile 的前缀（在任务临时目录里）。
	// pass 1 的 partPath 为空（应输出到 -f null -），pass 2 的 partPath 是 .part 输出。
	BuildPassArgs func(pass int, partPath, passLog string) []string
	// TempDir 是任务临时目录的父目录（<数据目录>/tmp）；为空用系统临时目录。仅两遍编码使用。
	TempDir string
	// Output 是期望的输出路径；为空表示任务不产生文件。
	Output string
	// DurationSec 是输入的总时长，用来把 out_time 换算成 0~1 的进度；<=0 时进度保持 0（直播用 -1，见 Live）。
	DurationSec float64
	// ProgressBase / ProgressScale 把本次 ffmpeg 的进度映射到任务整体进度：
	// fraction = ProgressBase + f*ProgressScale。ProgressScale 为 0 时按 1 处理。
	// 两遍编码由 Runner 自己映射，不需要设置。
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

	if r.BuildPassArgs != nil {
		if r.Output == "" {
			return "", apperr.New(apperr.Internal, "两遍编码必须指定输出文件")
		}
		dir, cleanup, err := MkTaskTemp(ctx, r.TempDir)
		if err != nil {
			return "", apperr.Wrap(apperr.IOError, "创建临时目录失败", err)
		}
		defer cleanup()
		passLog := filepath.Join(dir, "pass")
		return RunWithPart(ctx, r.Output, func(part string) error {
			if err := one(r.BuildPassArgs(1, "", passLog), 0, 0.5); err != nil {
				return err
			}
			return one(r.BuildPassArgs(2, part, passLog), 0.5, 0.5)
		})
	}

	run := func(part string) error { return one(r.BuildArgs(part), r.ProgressBase, r.ProgressScale) }
	if r.Output == "" {
		return "", run("")
	}
	return RunWithPart(ctx, r.Output, run)
}

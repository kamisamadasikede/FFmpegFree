package convert

import (
	"context"

	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/task"
)

// hwGateRunner 在高清 ASR 运行时串行化硬件编码任务（契约 6.18.2）。
type hwGateRunner struct {
	inner   *resultRunner
	acquire func(ctx context.Context) (release func(), err error)
}

func (r *hwGateRunner) Run(ctx context.Context, report func(task.Progress)) (string, error) {
	release, err := r.acquire(ctx)
	if err != nil {
		return "", err
	}
	defer release()
	return r.inner.Run(ctx, report)
}

func (r *hwGateRunner) Result() *task.TaskResult { return r.inner.Result() }

func (r *hwGateRunner) EncoderInfo() ffmpeg.EncoderInfo {
	if r.inner.FFmpegRunner != nil {
		return r.inner.FFmpegRunner.EncoderInfo()
	}
	return ffmpeg.EncoderInfo{}
}

func (r *hwGateRunner) DesiredOutput() string {
	if r.inner.FFmpegRunner != nil {
		return r.inner.FFmpegRunner.DesiredOutput()
	}
	return ""
}

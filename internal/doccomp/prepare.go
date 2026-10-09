package doccomp

import (
	"context"
	"errors"
	"os/exec"
	"strconv"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/proc"
)

// runTree 启动 cmd 并等它结束；ctx 取消时结束整个进程树。返回退出码（启动失败 -2）。
func runTree(ctx context.Context, cmd *exec.Cmd) (int, error) {
	if err := proc.Start(cmd); err != nil {
		return -2, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode(), err
		}
		if err != nil {
			return -1, err
		}
		return 0, nil
	case <-ctx.Done():
		_ = proc.Kill(cmd)
		<-done
		return -1, ctx.Err()
	}
}

func installFailed(detail string) *apperr.AppError {
	return apperr.New(apperr.DocComponentInstallFailed, "文档组件准备失败，请重试。").WithDetail(detail)
}

func codeDetail(tool string, code int, tail string) string {
	d := tool + "=" + strconv.Itoa(code)
	if tail != "" {
		d += "\n" + tail
	}
	return d
}

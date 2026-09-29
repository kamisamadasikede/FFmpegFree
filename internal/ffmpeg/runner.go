package ffmpeg

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"FFmpegFree/internal/proc"
)

// Runner 运行一个可执行文件并返回标准输出。检测逻辑通过它调用 ffmpeg / ffprobe，
// 测试可以注入假实现。
type Runner func(ctx context.Context, exe string, args ...string) (stdout string, err error)

// defaultCheckTimeout 是单次探测命令（-version、-encoders）的超时。
const defaultCheckTimeout = 10 * time.Second

// NewCommand 创建所有检测 / 校验用的子进程命令。这是本包启动外部程序的唯一入口：
// 一律经过 proc.Configure，Windows 下隐藏控制台窗口（否则每次检测都会闪一个黑框），
// 其他平台让子进程单独成组。新增任何会启动 ffmpeg / ffprobe / codesign 的代码都应使用它。
func NewCommand(ctx context.Context, exe string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, exe, args...)
	proc.Configure(cmd)
	// 取消 / 超时时结束整个进程组（Windows 是进程树），而不是只杀主进程留下孙进程。
	cmd.Cancel = func() error { return proc.Kill(cmd) }
	// 超时杀掉主进程后，如果子孙进程还握着管道，Wait 不会返回；WaitDelay 兜底。
	cmd.WaitDelay = 2 * time.Second
	return cmd
}

// ExecRunner 返回真实的命令执行器：每次调用带超时，Windows 上隐藏控制台窗口。
func ExecRunner(timeout time.Duration) Runner {
	if timeout <= 0 {
		timeout = defaultCheckTimeout
	}
	return func(ctx context.Context, exe string, args ...string) (string, error) {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		cmd := NewCommand(ctx, exe, args...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return stdout.String(), fmt.Errorf("运行超时（%s）: %s", timeout, exe)
			}
			msg := strings.TrimSpace(stderr.String())
			if len(msg) > 500 {
				msg = msg[len(msg)-500:]
			}
			if msg != "" {
				return stdout.String(), fmt.Errorf("%w: %s", err, msg)
			}
			return stdout.String(), err
		}
		return stdout.String(), nil
	}
}

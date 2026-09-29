package ffmpeg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/proc"
)

// DefaultGracePeriod 是优雅停止的最长等待时间（契约 6.5：等待最多 5 秒让 ffmpeg 写完文件尾）。
const DefaultGracePeriod = 5 * time.Second

// RunOptions 描述一次 ffmpeg 长任务的执行方式。转换、剪辑、直播等 Service 都通过 Run 启动 ffmpeg，
// 统一得到：进度解析、stderr 尾部、进程组管理、取消 / 优雅停止。
type RunOptions struct {
	// Exe 是 ffmpeg 的绝对路径（来自 ffmpeg.Require()）。
	Exe string
	// Args 是 ffmpeg 参数（不含可执行文件本身）。Run 会在最前面加上
	// -hide_banner -nostats -y [-nostdin] -progress pipe:1，调用方不要重复添加。
	Args []string
	// Stdin 不为空时接到 ffmpeg 的标准输入（录屏分片写入等）。此时无法通过 stdin 发 q，
	// 优雅停止改为发中断信号（Windows 不支持信号，直接结束进程）。
	Stdin io.Reader
	// OnProgress 每解析出一个 progress 块回调一次（同步调用，请快速返回）。
	OnProgress func(ProgressUpdate)
	// OnStderr 每收到 stderr 一行回调一次，可用于写日志文件。可为空。
	OnStderr func(line string)
	// TailLines 保留 stderr 最后多少行放进错误 detail，默认 50。
	TailLines int
	// GracefulStop 为 true 时，ctx 被取消不是直接结束进程，而是发 q（或中断信号），
	// 等待最多 GracePeriod 让 ffmpeg 写完文件尾，超时再强制结束。
	// 优雅退出成功时 Run 返回 nil（输出文件完整），并且 Result.Stopped 为 true。
	GracefulStop bool
	GracePeriod  time.Duration // 默认 DefaultGracePeriod
	// Classify 把非零退出转换成具体的错误码（直播的连接失败、推流被拒绝等）。
	// 返回 nil 表示不认识，使用默认的 PROCESS_FAILED。
	Classify func(stderrTail string, exitErr error) *apperr.AppError
}

// RunResult 是 Run 的附加信息。
type RunResult struct {
	// Stopped 表示进程是因为 ctx 取消而停止的（无论是优雅退出还是被强制结束）。
	Stopped bool
	// StderrTail 是 stderr 的最后若干行。
	StderrTail string
}

// Run 启动 ffmpeg 并等待结束。
//
//   - 正常退出返回 nil；
//   - 非零退出返回 apperr（默认 PROCESS_FAILED，detail 为 stderr 最后 N 行），可用 Classify 细分；
//   - ctx 取消：非优雅模式立即 proc.Kill 整个进程组并返回 ctx.Err()；优雅模式见 GracefulStop。
func Run(ctx context.Context, opts RunOptions) (RunResult, error) {
	if opts.Exe == "" {
		return RunResult{}, apperr.New(apperr.FFmpegNotFound, "未指定 ffmpeg 路径")
	}
	// -y：输出是任务自己选好名字的 .part 临时文件，遇到上次残留直接覆盖，绝不能停下来问 y/N；
	// -nostdin：不需要 stdin 的任务禁止 ffmpeg 读键盘（否则后台运行时可能被 SIGTTIN 挂起或吞掉输入）。
	// 直播优雅停止要通过 stdin 发 q、录屏由调用方给 stdin，这两种情况不加 -nostdin。
	pre := []string{"-hide_banner", "-nostats", "-y"}
	if !opts.GracefulStop && opts.Stdin == nil {
		pre = append(pre, "-nostdin")
	}
	pre = append(pre, "-progress", "pipe:1")
	args := append(pre, opts.Args...)

	// 进程自身的生命周期与调用方 ctx 解耦：优雅停止时 ctx 已经取消，但进程还要活一小会儿。
	procCtx, killProc := context.WithCancel(context.Background())
	defer killProc()
	cmd := NewCommand(procCtx, opts.Exe, args...)
	cmd.Cancel = func() error { return proc.Kill(cmd) }

	tail := NewTailBuffer(opts.TailLines)
	var parser ProgressParser
	cmd.Stdout = newLineWriter(func(line string) {
		if u, ok := parser.Feed(line); ok && opts.OnProgress != nil {
			opts.OnProgress(u)
		}
	})
	cmd.Stderr = newLineWriter(func(line string) {
		tail.Add(line)
		if opts.OnStderr != nil {
			opts.OnStderr(line)
		}
	})

	var stdinW io.WriteCloser
	switch {
	case opts.Stdin != nil:
		cmd.Stdin = opts.Stdin
	case opts.GracefulStop:
		w, err := cmd.StdinPipe()
		if err != nil {
			return RunResult{}, apperr.Wrap(apperr.Internal, "创建 ffmpeg 输入管道失败", err)
		}
		stdinW = w
	}

	if err := proc.Start(cmd); err != nil {
		return RunResult{}, apperr.Wrap(apperr.ProcessFailed, "启动 ffmpeg 失败", err)
	}

	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()

	grace := opts.GracePeriod
	if grace <= 0 {
		grace = DefaultGracePeriod
	}
	var stopped, exitedGracefully bool
	var waitErr error
	select {
	case waitErr = <-waitDone:
	case <-ctx.Done():
		stopped = true
		if opts.GracefulStop {
			requestGracefulExit(cmd, stdinW, opts.Stdin != nil)
			timer := time.NewTimer(grace)
			select {
			case waitErr = <-waitDone:
				exitedGracefully = true
				timer.Stop()
			case <-timer.C:
				killProc()
				waitErr = <-waitDone
			}
		} else {
			killProc()
			waitErr = <-waitDone
		}
	}
	if stdinW != nil {
		stdinW.Close()
	}
	res := RunResult{Stopped: stopped, StderrTail: tail.String()}

	if stopped {
		if exitedGracefully {
			return res, nil // 文件尾已写完；ffmpeg 收到 q / SIGINT 后的退出码不一定是 0，不当作失败
		}
		return res, ctx.Err()
	}
	if waitErr != nil {
		return res, classifyExit(opts, res.StderrTail, waitErr)
	}
	return res, nil
}

// requestGracefulExit 请求 ffmpeg 自己收尾退出。
func requestGracefulExit(cmd *exec.Cmd, stdin io.WriteCloser, callerOwnsStdin bool) {
	if stdin != nil {
		stdin.Write([]byte("q")) // ffmpeg 交互命令：q 表示正常结束
		stdin.Close()
		return
	}
	if callerOwnsStdin {
		if err := proc.Interrupt(cmd); err != nil {
			proc.Kill(cmd) // 平台不支持信号：只能结束进程
		}
	}
}

func classifyExit(opts RunOptions, tail string, exitErr error) error {
	if opts.Classify != nil {
		if e := opts.Classify(tail, exitErr); e != nil {
			return e.WithDetail(tail)
		}
	}
	msg := "ffmpeg 执行失败"
	var ee *exec.ExitError
	if errors.As(exitErr, &ee) {
		msg = fmt.Sprintf("ffmpeg 异常退出（退出码 %d）", ee.ExitCode())
	}
	e := apperr.Wrap(apperr.ProcessFailed, msg, exitErr)
	if tail != "" {
		return e.WithDetail(tail)
	}
	return e
}

// lineWriter 把字节流按行切开回调。ffmpeg 的进度行以 \n 结尾；stderr 里的进度刷新可能用 \r，一并当作换行。
type lineWriter struct {
	mu  sync.Mutex
	buf bytes.Buffer
	fn  func(string)
}

func newLineWriter(fn func(string)) *lineWriter { return &lineWriter{fn: fn} }

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf.Write(p)
	for {
		b := w.buf.Bytes()
		i := bytes.IndexAny(b, "\r\n")
		if i < 0 {
			break
		}
		line := string(b[:i])
		w.buf.Next(i + 1)
		if strings.TrimSpace(line) != "" {
			w.fn(line)
		}
	}
	if w.buf.Len() > 1<<20 { // 防止没有换行的异常输出无限增长
		w.buf.Reset()
	}
	return len(p), nil
}

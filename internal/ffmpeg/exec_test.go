//go:build !windows

package ffmpeg

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
)

// 假 ffmpeg：按第一个非选项参数（模式）决定行为。
func fakeFFmpeg(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "ffmpeg")
	script := `#!/bin/sh
mode=""
for a in "$@"; do mode="$a"; done
case "$mode" in
  ok)
    echo "out_time_us=1000000"; echo "speed=2.0x"; echo "progress=continue"
    echo "out_time_us=2000000"; echo "speed=2.0x"; echo "progress=end"
    echo "some info line" >&2
    exit 0 ;;
  fail)
    i=1; while [ $i -le 80 ]; do echo "line $i" >&2; i=$((i+1)); done
    echo "Connection refused" >&2
    exit 3 ;;
  hang)
    echo "out_time_us=1000000"; echo "progress=continue"
    sleep 30 ;;
  waitq)
    # 收到 q 后写文件尾并正常退出
    echo "out_time_us=1000000"; echo "progress=continue"
    read -r x
    echo "trailer written" >&2
    exit 0 ;;
  ignoreq)
    trap '' INT
    echo "started" >&2
    sleep 30 ;;
  sigint)
    trap 'echo "got sigint" >&2; exit 130' INT
    echo "ready" >&2
    while true; do sleep 0.05; done ;;
esac
`
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRunSuccessParsesProgress(t *testing.T) {
	var ups []ProgressUpdate
	res, err := Run(context.Background(), RunOptions{Exe: fakeFFmpeg(t), Args: []string{"ok"}, OnProgress: func(u ProgressUpdate) { ups = append(ups, u) }})
	if err != nil {
		t.Fatal(err)
	}
	if len(ups) != 2 || ups[0].OutTimeSec != 1 || !ups[1].End || ups[1].OutTimeSec != 2 {
		t.Fatalf("%+v", ups)
	}
	if !strings.Contains(res.StderrTail, "some info line") || res.Stopped {
		t.Fatalf("%+v", res)
	}
}

func TestRunFailureTailAndClassify(t *testing.T) {
	_, err := Run(context.Background(), RunOptions{Exe: fakeFFmpeg(t), Args: []string{"fail"}, TailLines: 50})
	var ae *apperr.AppError
	if !errors.As(err, &ae) || ae.Code != apperr.ProcessFailed || !strings.Contains(ae.Message, "3") {
		t.Fatalf("%v", err)
	}
	lines := strings.Split(ae.Detail, "\n")
	if len(lines) != 50 || lines[49] != "Connection refused" || lines[0] != "line 32" {
		t.Fatalf("detail 应是最后 50 行: %d %q %q", len(lines), lines[0], lines[len(lines)-1])
	}
	// 自定义分类
	_, err = Run(context.Background(), RunOptions{Exe: fakeFFmpeg(t), Args: []string{"fail"}, TailLines: 5,
		Classify: func(tail string, _ error) *apperr.AppError {
			if strings.Contains(tail, "Connection refused") {
				return apperr.New(apperr.LiveConnectFailed, "连接失败")
			}
			return nil
		}})
	if !apperr.Is(err, apperr.LiveConnectFailed) || !strings.Contains(err.Error(), "Connection refused") {
		t.Fatalf("分类应生效且 detail 带尾部日志: %v", err)
	}
	// 分类返回 nil 时回退默认
	_, err = Run(context.Background(), RunOptions{Exe: fakeFFmpeg(t), Args: []string{"fail"}, Classify: func(string, error) *apperr.AppError { return nil }})
	if !apperr.Is(err, apperr.ProcessFailed) {
		t.Fatalf("%v", err)
	}
}

func TestRunCancelKillsProcess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{}, 1)
	go func() {
		<-started
		cancel()
	}()
	begin := time.Now()
	res, err := Run(ctx, RunOptions{Exe: fakeFFmpeg(t), Args: []string{"hang"}, OnProgress: func(ProgressUpdate) {
		select {
		case started <- struct{}{}:
		default:
		}
	}})
	if !errors.Is(err, context.Canceled) || !res.Stopped {
		t.Fatalf("%v %+v", err, res)
	}
	if time.Since(begin) > 5*time.Second {
		t.Fatal("取消应立即结束进程")
	}
}

func TestRunGracefulStopViaQ(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(200 * time.Millisecond); cancel() }()
	res, err := Run(ctx, RunOptions{Exe: fakeFFmpeg(t), Args: []string{"waitq"}, GracefulStop: true})
	if err != nil || !res.Stopped {
		t.Fatalf("优雅停止成功应返回 nil: %v %+v", err, res)
	}
	if !strings.Contains(res.StderrTail, "trailer written") {
		t.Fatalf("应等到 ffmpeg 写完文件尾: %q", res.StderrTail)
	}
}

func TestRunGracefulStopTimeoutKills(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(200 * time.Millisecond); cancel() }()
	begin := time.Now()
	res, err := Run(ctx, RunOptions{Exe: fakeFFmpeg(t), Args: []string{"ignoreq"}, GracefulStop: true, GracePeriod: 400 * time.Millisecond})
	if !errors.Is(err, context.Canceled) || !res.Stopped {
		t.Fatalf("超时后应强制结束并返回 ctx 错误: %v %+v", err, res)
	}
	if d := time.Since(begin); d < 500*time.Millisecond || d > 5*time.Second {
		t.Fatalf("应等满宽限期再杀: %v", d)
	}
}

func TestRunGracefulStopViaSIGINTWhenCallerOwnsStdin(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(300 * time.Millisecond); cancel() }()
	pr, pw, _ := os.Pipe() // 模拟调用方持有的 stdin（录屏分片）
	defer pw.Close()
	res, err := Run(ctx, RunOptions{Exe: fakeFFmpeg(t), Args: []string{"sigint"}, Stdin: pr, GracefulStop: true})
	if err != nil || !res.Stopped || !strings.Contains(res.StderrTail, "got sigint") {
		t.Fatalf("%v %+v", err, res)
	}
}

func TestRunStartFailure(t *testing.T) {
	_, err := Run(context.Background(), RunOptions{Exe: "/nonexistent/ffmpeg"})
	if !apperr.Is(err, apperr.ProcessFailed) {
		t.Fatalf("%v", err)
	}
	if _, err := Run(context.Background(), RunOptions{}); !apperr.Is(err, apperr.FFmpegNotFound) {
		t.Fatalf("%v", err)
	}
}

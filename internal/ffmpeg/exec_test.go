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
  qexit224)
    # 收到 q 后以非零码退出（模拟取消之后连接断开、Broken pipe 等）
    echo "out_time_us=1000000"; echo "progress=continue"
    read -r x
    echo "Broken pipe" >&2
    exit 224 ;;
  secretfail)
    echo "Error opening output rtmp://h/live/topsecretkey: Connection refused" >&2
    exit 3 ;;
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
	if !errors.As(err, &ae) || ae.Code != apperr.ProcessFailed || ae.Message != "转换被意外中断，可以重试；如果反复出现，请查看日志。" {
		t.Fatalf("%v", err)
	}
	if strings.ContainsAny(ae.Message, "0123456789") || strings.Contains(ae.Message, "退出码") {
		t.Fatalf("message 不应带退出码: %q", ae.Message)
	}
	if !strings.HasPrefix(ae.Detail, "ffmpeg 退出码 3\n") {
		t.Fatalf("退出码应在 detail 第一行: %q", ae.Detail[:20])
	}
	lines := strings.Split(strings.TrimPrefix(ae.Detail, "ffmpeg 退出码 3\n"), "\n")
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

func TestRunAlwaysAddsYAndNostdin(t *testing.T) {
	dir := t.TempDir()
	rec := filepath.Join(dir, "args")
	p := filepath.Join(dir, "ffmpeg")
	os.WriteFile(p, []byte("#!/bin/sh\necho \"$@\" > "+rec+"\necho progress=end\nexit 0\n"), 0o755)
	get := func() string { b, _ := os.ReadFile(rec); return strings.TrimSpace(string(b)) }

	if _, err := Run(context.Background(), RunOptions{Exe: p, Args: []string{"-i", "file:x", "out"}}); err != nil {
		t.Fatal(err)
	}
	if got := get(); got != "-hide_banner -nostats -y -nostdin -progress pipe:1 -i file:x out" {
		t.Fatalf("普通任务应带 -y -nostdin: %q", got)
	}
	// 直播优雅停止需要 stdin 发 q：不能加 -nostdin，但仍有 -y
	if _, err := Run(context.Background(), RunOptions{Exe: p, Args: []string{"x"}, GracefulStop: true}); err != nil {
		t.Fatal(err)
	}
	if got := get(); strings.Contains(got, "-nostdin") || !strings.Contains(got, " -y ") {
		t.Fatalf("GracefulStop 不应加 -nostdin: %q", got)
	}
	// 调用方自带 stdin（录屏）：同样不能加 -nostdin
	if _, err := Run(context.Background(), RunOptions{Exe: p, Args: []string{"x"}, Stdin: strings.NewReader("")}); err != nil {
		t.Fatal(err)
	}
	if got := get(); strings.Contains(got, "-nostdin") || !strings.Contains(got, " -y ") {
		t.Fatalf("自带 stdin 不应加 -nostdin: %q", got)
	}
}

// 已请求取消之后 ffmpeg 无论怎样非零退出，Run 都返回 ctx 错误（任务落 canceled，不做分类）。
func TestRunStrictGracefulNonzeroExitAfterCancelIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(200 * time.Millisecond); cancel() }()
	classified := false
	res, err := Run(ctx, RunOptions{Exe: fakeFFmpeg(t), Args: []string{"qexit224"}, GracefulStop: true, StrictGracefulExit: true,
		Classify: func(string, error) *apperr.AppError {
			classified = true
			return apperr.New(apperr.LivePushInterrupted, "x")
		}})
	if !errors.Is(err, context.Canceled) || !res.Stopped || classified {
		t.Fatalf("取消后非零退出应是 ctx 错误且不分类: err=%v res=%+v classified=%v", err, res, classified)
	}
	// 不严格时沿用旧语义：q 之后退出即成功。
	ctx2, cancel2 := context.WithCancel(context.Background())
	go func() { time.Sleep(200 * time.Millisecond); cancel2() }()
	if _, err := Run(ctx2, RunOptions{Exe: fakeFFmpeg(t), Args: []string{"qexit224"}, GracefulStop: true}); err != nil {
		t.Fatalf("默认语义 q 后退出即成功: %v", err)
	}
}

// 连接阶段（CanGraceful=false）取消直接强杀：不发 q、不等宽限期。
func TestRunCanGracefulFalseKillsImmediately(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(200 * time.Millisecond); cancel() }()
	begin := time.Now()
	res, err := Run(ctx, RunOptions{Exe: fakeFFmpeg(t), Args: []string{"ignoreq"}, GracefulStop: true, GracePeriod: 10 * time.Second,
		CanGraceful: func() bool { return false }})
	if !errors.Is(err, context.Canceled) || !res.Stopped {
		t.Fatalf("%v %+v", err, res)
	}
	if d := time.Since(begin); d > 3*time.Second {
		t.Fatalf("连接阶段取消应直接强杀，实际 %v", d)
	}
}

// Redact 在 stderr 进入尾部缓冲、OnStderr、Classify 之前生效。
func TestRunRedactsBeforeTailLogAndClassify(t *testing.T) {
	redact := func(l string) string { return strings.ReplaceAll(l, "topsecretkey", "***") }
	var logged []string
	var classifiedTail string
	_, err := Run(context.Background(), RunOptions{Exe: fakeFFmpeg(t), Args: []string{"secretfail"}, Redact: redact,
		OnStderr: func(l string) { logged = append(logged, l) },
		Classify: func(tail string, _ error) *apperr.AppError {
			classifiedTail = tail
			return apperr.New(apperr.LiveConnectFailed, "x").WithDetail("scheme=rtmp\n" + tail)
		}})
	all := strings.Join(logged, "\n") + classifiedTail
	if err == nil || strings.Contains(all, "topsecretkey") || strings.Contains(err.Error(), "topsecretkey") || !strings.Contains(all, "rtmp://h/live/***") {
		t.Fatalf("脱敏未生效: err=%v all=%q", err, all)
	}
	// 分类器自己写的 detail（固定首行）保留，不被 tail 覆盖。
	var ae *apperr.AppError
	if !errors.As(err, &ae) || !strings.HasPrefix(ae.Detail, "scheme=rtmp\n") {
		t.Fatalf("%v", err)
	}
}

// 严格模式下：q 之后退出码 0 才算成功（返回 nil、Stopped=true）；宽限期超时被强杀仍是 ctx 错误。
// 非严格（默认，除直播外的调用方）行为不变：见 TestRunGracefulStopViaQ / TestRunStrictGracefulNonzeroExitAfterCancelIsCanceled 的第二段。
func TestRunStrictGracefulExitZeroIsSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(200 * time.Millisecond); cancel() }()
	res, err := Run(ctx, RunOptions{Exe: fakeFFmpeg(t), Args: []string{"waitq"}, GracefulStop: true, StrictGracefulExit: true})
	if err != nil || !res.Stopped || !strings.Contains(res.StderrTail, "trailer written") {
		t.Fatalf("q 后退出码 0 应成功: %v %+v", err, res)
	}
	ctx2, cancel2 := context.WithCancel(context.Background())
	go func() { time.Sleep(200 * time.Millisecond); cancel2() }()
	_, err = Run(ctx2, RunOptions{Exe: fakeFFmpeg(t), Args: []string{"ignoreq"}, GracefulStop: true, StrictGracefulExit: true, GracePeriod: 300 * time.Millisecond})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("超时强杀应是 ctx 错误: %v", err)
	}
}

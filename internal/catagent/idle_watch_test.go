package catagent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
)

func shortIdle(t *testing.T, idle time.Duration) {
	t.Helper()
	oldI, oldE := DefaultIdleTimeout, idleCheckEvery
	DefaultIdleTimeout, idleCheckEvery = idle, 10*time.Millisecond
	t.Cleanup(func() { DefaultIdleTimeout, idleCheckEvery = oldI, oldE })
}

func scriptClaude(t *testing.T, script string) *CLIAgentAdapter {
	t.Helper()
	fake := filepath.Join(t.TempDir(), "claude-fake")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewClaudeAdapter(CLIAgentConfig{
		DataTemp: t.TempDir(),
		LookPath: func(string) (string, error) { return fake, nil },
		Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			for _, arg := range args {
				if arg == "-p" {
					return exec.CommandContext(ctx, fake)
				}
			}
			return exec.CommandContext(ctx, "true")
		},
	})
	a.detect()
	return a
}

func detailOf(err error) string {
	ae := apperr.From(err)
	if ae == nil {
		return ""
	}
	return strings.SplitN(ae.Detail, "\n", 2)[0]
}

// 假时钟：看门狗只看 now()，持续 touch 就永不超时；停止 touch 后推进时钟即超时。
func TestIdleWatch_FakeClock(t *testing.T) {
	shortIdle(t, time.Minute)
	var mu = make(chan struct{}, 1)
	mu <- struct{}{}
	clock := time.Unix(0, 0)
	now := func() time.Time { <-mu; c := clock; mu <- struct{}{}; return c }
	advance := func(d time.Duration) { <-mu; clock = clock.Add(d); mu <- struct{}{} }
	ctx, w, cancel := newTurnContext(context.Background(), 0, now)
	defer cancel()
	// 30 分钟（远超旧的 10 分钟总上限）一直有输出 → 不超时
	for i := 0; i < 60; i++ {
		advance(30 * time.Second)
		w.touch()
		time.Sleep(2 * time.Millisecond)
	}
	if ctx.Err() != nil {
		t.Fatalf("active turn must not time out: %v", context.Cause(ctx))
	}
	advance(61 * time.Second)
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("silent turn should time out")
	}
	if !idleTimedOut(ctx) {
		t.Fatalf("cause = %v", context.Cause(ctx))
	}
}

// 定时任务（Timeout > 0）仍是总时长，不挂看门狗。
func TestNewTurnContext_ScheduleKeepsTotalCap(t *testing.T) {
	ctx, w, cancel := newTurnContext(context.Background(), 50*time.Millisecond, nil)
	defer cancel()
	if w != nil {
		t.Fatal("schedule run must not use idle watch")
	}
	<-ctx.Done()
	if idleTimedOut(ctx) {
		t.Fatal("total cap must not be reported as idle")
	}
}

// 真进程：一直有输出、总时长远超无响应阈值 → 不会被无响应超时停掉。
func TestRunTurn_ActiveStreamBeyondIdle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix script fake")
	}
	shortIdle(t, 300*time.Millisecond)
	done := filepath.Join(t.TempDir(), "done")
	a := scriptClaude(t, "i=0\nwhile [ $i -lt 12 ]; do echo '{\"type\":\"ping\"}'; sleep 0.1; i=$((i+1)); done\n: > '"+done+"'\n")
	start := time.Now()
	_, err := a.RunTurn(TurnOptions{ConversationID: "c-active", Messages: []WireMessage{{Role: "user", Content: "x"}}})
	if d := detailOf(err); d == ReasonIdleTimeout || d == "reason=timeout" {
		t.Fatalf("active stream timed out: %v", err)
	}
	if _, serr := os.Stat(done); serr != nil {
		t.Fatalf("script was killed early after %v: %v", time.Since(start), err)
	}
}

// 真进程：没有任何输出 → idle_timeout，文案为专用文案。
func TestRunTurn_SilentStreamIdleTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix script fake")
	}
	shortIdle(t, 200*time.Millisecond)
	a := scriptClaude(t, "exec sleep 30\n")
	start := time.Now()
	_, err := a.RunTurn(TurnOptions{ConversationID: "c-silent", Messages: []WireMessage{{Role: "user", Content: "x"}}})
	if !apperr.Is(err, apperr.CatReplyFailed) || detailOf(err) != ReasonIdleTimeout {
		t.Fatalf("want idle_timeout, got %v (%s)", err, detailOf(err))
	}
	if apperr.From(err).Message != MsgIdleTimeout {
		t.Fatalf("msg = %q", apperr.From(err).Message)
	}
	if time.Since(start) > 10*time.Second {
		t.Fatalf("not killed promptly: %v", time.Since(start))
	}
}

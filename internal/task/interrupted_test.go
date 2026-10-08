package task

import (
	"context"
	"errors"
	"testing"

	"FFmpegFree/internal/apperr"
)

// 契约 v0.25.3（N4）：直播任务开始以后被中断（Runner 返回 InterruptedError）记为 interrupted，带直播码；
// 状态筛选、任务中心"隐藏已结束"按已结束处理；直播任务不管什么状态都不能重试。非直播任务返回它按普通失败处理。
func TestLiveInterruptedIsTerminalInterrupted(t *testing.T) {
	f := newFx(t, 1)
	live := RunnerFunc(func(context.Context, func(Progress)) (string, error) {
		return "", Interrupted(apperr.New(apperr.LivePushInterrupted, "推流被中断，请回到直播页重新推流。").WithDetail("Killed"))
	})
	tk, err := f.m.Submit(Spec{Type: TypeLiveFilePush}, live)
	if err != nil {
		t.Fatal(err)
	}
	d := waitTask(t, f.m, tk.ID)
	if d.Status != StatusInterrupted || d.Error == nil || d.Error.Code != apperr.LivePushInterrupted || d.FinishedAt == 0 {
		t.Fatalf("应是 interrupted + LIVE_PUSH_INTERRUPTED: %+v %+v", d, d.Error)
	}
	got, err := f.m.Get(tk.ID)
	if err != nil || got.Status != StatusInterrupted || got.Error == nil || got.Error.Code != apperr.LivePushInterrupted {
		t.Fatalf("落库: %+v %v", got, err)
	}
	page, err := f.m.List(Filter{Statuses: []Status{StatusInterrupted}, Limit: 50})
	if err != nil || page.Total != 1 || page.Items[0].ID != tk.ID {
		t.Fatalf("按 interrupted 筛选: %+v %v", page, err)
	}
	if page, _ := f.m.List(Filter{Statuses: []Status{StatusFailed}, Limit: 50}); page.Total != 0 {
		t.Fatalf("不应算在 failed 里: %+v", page)
	}
	if _, err := f.m.Retry(tk.ID); !apperr.Is(err, apperr.Unsupported) {
		t.Fatalf("直播任务被中断后也不能重试: %v", err)
	}
	if n, err := f.m.HideFinishedInTaskCenter(); err != nil || n != 1 {
		t.Fatalf("隐藏已结束应包含 interrupted: %d %v", n, err)
	}

	// 非直播任务：InterruptedError 不生效，按 failed
	conv := RunnerFunc(func(context.Context, func(Progress)) (string, error) {
		return "", Interrupted(apperr.New(apperr.ProcessFailed, "转换失败"))
	})
	tk2, err := f.m.Submit(Spec{Type: TypeOfficePDF}, conv)
	if err != nil {
		t.Fatal(err)
	}
	if d := waitTask(t, f.m, tk2.ID); d.Status != StatusFailed {
		t.Fatalf("非直播任务应是 failed: %+v", d)
	}
	if Interrupted(nil) != nil || !IsInterrupted(Interrupted(errors.New("x"))) || IsInterrupted(errors.New("x")) {
		t.Fatal("Interrupted / IsInterrupted")
	}
}

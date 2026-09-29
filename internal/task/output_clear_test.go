package task

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"FFmpegFree/internal/apperr"
)

func lastStatusEvent(f *fx, id string) StatusEvent {
	var last StatusEvent
	for _, e := range f.em.all() {
		if se, ok := e.payload.(StatusEvent); ok && se.ID == id {
			last = se
		}
	}
	return last
}

// 直播任务 Runner 返回 ClearOutputPath：outputPath 在终态事件之前清空并落库，事件、快照、库三处一致；
// 各终态（succeeded / canceled / failed）都生效；返回空串仍保持提交时的预期路径；返回实际路径则采信。
func TestClearOutputPathMechanism(t *testing.T) {
	arch := func(f *fx, name string) string { return filepath.Join(f.dir, name) }
	cases := []struct {
		name    string
		typ     Type
		runErr  func(ctx context.Context) error
		ret     func(exp string) string
		want    Status
		wantOut func(exp string) string
	}{
		{"直播成功+清空", TypeLiveScreenPush, func(context.Context) error { return nil }, func(string) string { return ClearOutputPath }, StatusSucceeded, func(string) string { return "" }},
		{"直播失败+清空", TypeLiveScreenPush, func(context.Context) error { return apperr.New(apperr.LivePushInterrupted, "x") }, func(string) string { return ClearOutputPath }, StatusFailed, func(string) string { return "" }},
		{"直播取消+清空", TypeLiveScreenPush, func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }, func(string) string { return ClearOutputPath }, StatusCanceled, func(string) string { return "" }},
		{"直播取消+保留（空串=不变）", TypeLiveScreenPush, func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }, func(string) string { return "" }, StatusCanceled, func(exp string) string { return exp }},
		{"直播取消+返回实际路径（强杀后存档保留）", TypeLiveScreenPush, func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }, func(exp string) string { return exp }, StatusCanceled, func(exp string) string { return exp }},
		{"直播失败+返回实际路径", TypeLiveScreenPush, func(context.Context) error { return errors.New("boom") }, func(exp string) string { return exp }, StatusFailed, func(exp string) string { return exp }},
		{"convert 成功+清空（机制通用）", TypeConvert, func(context.Context) error { return nil }, func(string) string { return ClearOutputPath }, StatusSucceeded, func(string) string { return "" }},
		// 旧行为不变：非直播任务失败 / 取消时不采信 Runner 返回的路径，保留提交时的预期路径
		{"convert 失败不采信返回路径", TypeConvert, func(context.Context) error { return errors.New("boom") }, func(string) string { return "/other/x.mp4" }, StatusFailed, func(exp string) string { return exp }},
		{"convert 失败+清空标记也不生效", TypeConvert, func(context.Context) error { return errors.New("boom") }, func(string) string { return ClearOutputPath }, StatusFailed, func(exp string) string { return exp }},
		{"convert 成功采信返回路径", TypeConvert, func(context.Context) error { return nil }, func(exp string) string { return exp + ".real" }, StatusSucceeded, func(exp string) string { return exp + ".real" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFx(t, 1)
			exp := arch(f, "archive.mp4")
			started := make(chan struct{})
			tk, err := f.m.Submit(Spec{Type: tc.typ, OutputPath: exp, InputPaths: []string{}}, RunnerFunc(func(ctx context.Context, _ func(Progress)) (string, error) {
				close(started)
				err := tc.runErr(ctx)
				return tc.ret(exp), err
			}))
			if err != nil {
				t.Fatal(err)
			}
			<-started
			if tc.want == StatusCanceled {
				if err := f.m.Cancel(tk.ID); err != nil {
					t.Fatal(err)
				}
			}
			d := waitTask(t, f.m, tk.ID)
			want := tc.wantOut(exp)
			if d.Status != tc.want || d.OutputPath != want {
				t.Fatalf("Get: status=%s out=%q want %s %q", d.Status, d.OutputPath, tc.want, want)
			}
			stored, err := f.st.GetTask(context.Background(), tk.ID)
			if err != nil || stored.OutputPath != want || stored.Status != tc.want {
				t.Fatalf("库里: %+v %v", stored, err)
			}
			ev := lastStatusEvent(f, tk.ID)
			if ev.Status != tc.want || ev.OutputPath != want || ev.Version != stored.Version {
				t.Fatalf("终态事件与库不一致: event=%+v stored version=%d out=%q", ev, stored.Version, stored.OutputPath)
			}
		})
	}
}

// 终态事件只有一条，且它带的就是已经清空的 outputPath（不会先发带路径的事件再补一条）。
func TestClearOutputPathSingleTerminalEvent(t *testing.T) {
	f := newFx(t, 1)
	exp := filepath.Join(f.dir, "a.mp4")
	tk, err := f.m.Submit(Spec{Type: TypeLiveFilePush, OutputPath: exp, InputPaths: []string{}}, RunnerFunc(func(context.Context, func(Progress)) (string, error) {
		return ClearOutputPath, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	waitTask(t, f.m, tk.ID)
	n := 0
	for _, e := range f.em.all() {
		if se, ok := e.payload.(StatusEvent); ok && se.ID == tk.ID && !se.Status.Active() {
			n++
			if se.OutputPath != "" {
				t.Fatalf("终态事件带了路径: %+v", se)
			}
		}
	}
	if n != 1 {
		t.Fatalf("终态事件应只有 1 条: %d", n)
	}
}

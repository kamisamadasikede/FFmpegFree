package task

import (
	"context"
	"testing"
	"time"
)

type docPoolRunner struct {
	release chan struct{}
	started chan string
	id      string
}

func (r *docPoolRunner) Pool() Pool { return PoolDoc }
func (r *docPoolRunner) Run(ctx context.Context, report func(Progress)) (string, error) {
	r.started <- r.id
	select {
	case <-r.release:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	return "/out/" + r.id + ".pdf", nil
}

// 架构师确认（v0.26 实现 PR）：doc_convert 的 startedAt 只在真正开始处理时写；在文档组件池里排队时为 0，queuePosition 从 0 起。
func TestDocPoolStartedAtOnlyWhenRunning(t *testing.T) {
	f := newFx(t, 1)
	release := make(chan struct{})
	started := make(chan string, 4)
	var ids []string
	for i, name := range []string{"a", "b", "c"} {
		tk, err := f.m.Submit(Spec{Type: TypeDocConvert, Title: name, InputPaths: []string{"/in/" + name + ".docx"}, Params: `{"target":"pdf"}`},
			&docPoolRunner{release: release, started: started, id: name})
		if err != nil {
			t.Fatal(i, err)
		}
		ids = append(ids, tk.ID)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			t.Fatal("前两个应立即开始（并发 2，与 batch 池无关）")
		}
	}
	var third Task
	eventually(t, func() bool {
		var err error
		third, err = f.m.Get(ids[2])
		return err == nil && third.Status == StatusQueued
	})
	if third.StartedAt != 0 || third.QueuePosition == nil || *third.QueuePosition != 0 {
		t.Fatalf("排队中的第三个：startedAt=%d queuePosition=%v", third.StartedAt, third.QueuePosition)
	}
	for _, id := range ids[:2] {
		if tk, _ := f.m.Get(id); tk.Status != StatusRunning || tk.StartedAt == 0 || tk.QueuePosition != nil {
			t.Fatalf("运行中的任务 %+v", tk)
		}
	}
	before := time.Now().UnixMilli()
	close(release)
	done := waitTask(t, f.m, ids[2])
	if done.Status != StatusSucceeded || done.StartedAt < before || done.FinishedAt < done.StartedAt {
		t.Fatalf("第三个：%+v（释放于 %d）", done, before)
	}
}

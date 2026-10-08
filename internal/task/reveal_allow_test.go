package task

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestRevealAllowTTLCapAndExactMatch(t *testing.T) {
	f := newFx(t, 1)
	now := time.Unix(1_800_000_000, 0)
	f.m.cfg.Now = func() time.Time { return now }
	p := filepath.Join(f.dir, "a.mp4")
	os.WriteFile(p, []byte("x"), 0o644)

	// 只收记录登记的输出路径本身
	f.m.allowRevealOfFailure(p, filepath.Join(f.dir, "other.mp4"))
	if f.m.IsRecentDeleteFailure(p) {
		t.Fatal("路径与登记的输出不一致时不应放行")
	}
	f.m.allowRevealOfFailure("rel.mp4", "rel.mp4")
	f.m.allowRevealOfFailure(p, p)
	if !f.m.IsRecentDeleteFailure(p) || !f.m.IsRecentDeleteFailure(filepath.Join(f.dir, ".", "a.mp4")) {
		t.Fatal("Clean 后精确匹配")
	}
	if f.m.IsRecentDeleteFailure(f.dir) || f.m.IsRecentDeleteFailure(filepath.Join(f.dir, "a.mp4.bak")) {
		t.Fatal("不放行别的路径")
	}
	// 符号链接不算
	link := filepath.Join(f.dir, "link.mp4")
	if os.Symlink(p, link) == nil {
		f.m.allowRevealOfFailure(link, link)
		if f.m.IsRecentDeleteFailure(link) {
			t.Fatal("符号链接不放行")
		}
	}
	// TTL
	now = now.Add(revealAllowTTL - time.Second)
	if !f.m.IsRecentDeleteFailure(p) {
		t.Fatal("有效期内")
	}
	now = now.Add(2 * time.Second)
	if f.m.IsRecentDeleteFailure(p) {
		t.Fatal("过期")
	}
	// 上限：只保留最新的 100 个
	for i := 0; i <= revealAllowMax; i++ {
		q := filepath.Join(f.dir, "f"+strconv.Itoa(i)+".mp4")
		os.WriteFile(q, []byte("x"), 0o644)
		f.m.allowRevealOfFailure(q, q)
	}
	if f.m.IsRecentDeleteFailure(filepath.Join(f.dir, "f0.mp4")) || !f.m.IsRecentDeleteFailure(filepath.Join(f.dir, "f1.mp4")) ||
		!f.m.IsRecentDeleteFailure(filepath.Join(f.dir, "f100.mp4")) || len(f.m.reveal.items) != revealAllowMax {
		t.Fatalf("上限 %d: %d", revealAllowMax, len(f.m.reveal.items))
	}
}

// DeleteRecords：只有文件类失败（路径非空）进放行表；成功删掉的、still_running 的不进。
func TestDeleteRecordsRegistersFailurePathsForReveal(t *testing.T) {
	f := newFx(t, 2)
	in := filepath.Join(f.dir, "src.mov")
	os.WriteFile(in, []byte("src"), 0o644)
	ok1 := succeededConvert(t, f, "ok.mp4", in)
	busy := succeededConvert(t, f, "busy.mp4", in)
	removeFile = func(p string) error {
		if p == busy.OutputPath {
			return &os.PathError{Op: "remove", Path: p, Err: inUseErrno}
		}
		return os.Remove(p)
	}
	defer func() { removeFile = os.Remove }()
	res, err := f.m.DeleteRecords([]string{ok1.ID, busy.ID}, TypeConvert, true, nil)
	if err != nil || len(res.Failures) != 1 || res.Failures[0].Path != busy.OutputPath {
		t.Fatalf("%+v %v", res, err)
	}
	if !f.m.IsRecentDeleteFailure(busy.OutputPath) {
		t.Fatal("in_use 留下的文件应放行")
	}
	if f.m.IsRecentDeleteFailure(ok1.OutputPath) || f.m.IsRecentDeleteFailure(in) {
		t.Fatal("已删掉的输出、源文件不放行")
	}
	// still_running：没有路径，不放行任何东西
	stuck := make(chan struct{})
	hard, _ := f.m.Submit(Spec{Type: TypeConvert, OutputPath: filepath.Join(f.dir, "out", "hard.mp4")}, RunnerFunc(func(ctx context.Context, _ func(Progress)) (string, error) {
		<-stuck
		return "", errors.New("x")
	}))
	eventually(t, func() bool { return f.m.mustGet(t, hard.ID).Status == StatusRunning })
	f.m.cfg.DeleteWait = 100 * time.Millisecond
	before := len(f.m.reveal.items)
	res, _ = f.m.DeleteRecords([]string{hard.ID}, TypeConvert, true, nil)
	close(stuck)
	waitTask(t, f.m, hard.ID)
	if len(res.Failures) != 1 || res.Failures[0].Path != "" || len(f.m.reveal.items) != before {
		t.Fatalf("%+v", res)
	}
}

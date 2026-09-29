package proc

import (
	"errors"
	"sync"
	"testing"
)

// 这些测试不带构建标签：Job Object 的回退顺序和登记逻辑是纯逻辑，Linux 上也能验证。

func TestKillTreeOrder(t *testing.T) {
	errX := errors.New("x")
	rec := func(log *[]string, name string, err error) func() error {
		return func() error { *log = append(*log, name); return err }
	}
	cases := []struct {
		name          string
		job, tk, main error
		wantCalls     []string
		wantErr       error
	}{
		{"Job 成功就不再往下", nil, errX, errX, []string{"job"}, nil},
		{"Job 失败退回 taskkill", errX, nil, errX, []string{"job", "taskkill"}, nil},
		{"都失败退回只结束主进程", errX, errX, nil, []string{"job", "taskkill", "main"}, nil},
		{"全部失败返回最后的错误", errX, errX, errNoJob, []string{"job", "taskkill", "main"}, errNoJob},
		{"没有 Job（errNoJob）走 taskkill", errNoJob, nil, errX, []string{"job", "taskkill"}, nil},
	}
	for _, c := range cases {
		var log []string
		err := killTree(rec(&log, "job", c.job), rec(&log, "taskkill", c.tk), rec(&log, "main", c.main))
		if !errors.Is(err, c.wantErr) && !(err == nil && c.wantErr == nil) {
			t.Errorf("%s: err=%v want %v", c.name, err, c.wantErr)
		}
		if len(log) != len(c.wantCalls) {
			t.Errorf("%s: calls=%v want %v", c.name, log, c.wantCalls)
			continue
		}
		for i := range log {
			if log[i] != c.wantCalls[i] {
				t.Errorf("%s: calls=%v want %v", c.name, log, c.wantCalls)
			}
		}
	}
	// 某一步为 nil 时跳过
	called := false
	if err := killTree(nil, nil, func() error { called = true; return nil }); err != nil || !called {
		t.Errorf("nil 回退应跳过: %v %v", err, called)
	}
}

func TestJobRegistry(t *testing.T) {
	var r jobRegistry
	if _, ok := r.get(1); ok || r.len() != 0 {
		t.Fatal("空表")
	}
	r.add(10, 111)
	r.add(11, 222)
	if h, ok := r.get(10); !ok || h != 111 {
		t.Fatalf("get: %v %v", h, ok)
	}
	if r.len() != 2 {
		t.Fatal("get 不应移除")
	}
	if h, ok := r.take(10); !ok || h != 111 || r.len() != 1 {
		t.Fatalf("take: %v %v", h, ok)
	}
	if _, ok := r.take(10); ok {
		t.Fatal("take 只能拿一次（句柄只关闭一次）")
	}
	// 并发安全
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r.add(1000+i, uintptr(i))
			r.get(1000 + i)
			r.take(1000 + i)
		}(i)
	}
	wg.Wait()
	if r.len() != 1 { // 只剩 11
		t.Fatalf("len=%d", r.len())
	}
}

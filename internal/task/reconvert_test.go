package task

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/store"
)

// ---------- 原地重转（契约 v0.24 / v0.24.1，6.17） ----------

func readS(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("读取 %s: %v", p, err)
	}
	return string(b)
}

func exists2(p string) bool { _, err := os.Lstat(p); return err == nil }

// doneConvert 提交一条 convert 任务，写 content 到 out，等它成功。
func (f *fx) doneConvert(t *testing.T, out, content string) Task {
	t.Helper()
	in := filepath.Join(f.dir, "in.mov")
	os.WriteFile(in, []byte("src"), 0o644)
	tk, err := f.m.Submit(Spec{Type: TypeConvert, Title: "T", InputPaths: []string{in}, OutputPath: out, Params: `{"v":1}`},
		RunnerFunc(func(ctx context.Context, _ func(Progress)) (string, error) {
			return RunWithPart(ctx, out, func(part string) error { return os.WriteFile(part, []byte(content), 0o644) })
		}))
	if err != nil {
		t.Fatal(err)
	}
	d := waitTask(t, f.m, tk.ID)
	if d.Status != StatusSucceeded {
		t.Fatalf("%+v", d)
	}
	return d
}

// rcRunner 写临时文件；gate 非 nil 时写完后等 gate 关闭或 ctx 结束；fail 非 nil 时返回它。
func rcRunner(out, id, content string, gate chan struct{}, fail error) RunnerFunc {
	return func(ctx context.Context, _ func(Progress)) (string, error) {
		tmp := ReconvertTempPath(out, id)
		if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
			return "", err
		}
		if gate != nil {
			select {
			case <-gate:
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
		if fail != nil {
			return "", fail
		}
		return tmp, nil
	}
}

func (f *fx) lastStatus(id string) StatusEvent {
	var last StatusEvent
	for _, e := range f.em.all() {
		if s, ok := e.payload.(StatusEvent); ok && e.name == EventStatus && s.ID == id {
			last = s
		}
	}
	return last
}

func waitRC(t *testing.T, f *fx, id string) Task {
	t.Helper()
	var got Task
	eventually(t, func() bool {
		got, _ = f.m.Get(id)
		return !got.Reconverting && got.Status == StatusSucceeded && f.lastStatus(id).ReconvertOutcome != ""
	})
	return got
}

func wantDetail(t *testing.T, err error, code apperr.Code, reason string) {
	t.Helper()
	var ae *apperr.AppError
	if !errors.As(err, &ae) || ae.Code != code || !strings.Contains(ae.Detail, reason) {
		t.Fatalf("want %s %s, got %v", code, reason, err)
	}
}

func TestReconvertSuccessReplacesInPlace(t *testing.T) {
	f := newFx(t, 2)
	out := filepath.Join(f.dir, "o.mp4")
	d := f.doneConvert(t, out, "v1")
	if mode, block := ReconvertOutputMode(d); mode != ReconvertReplace || block != "" {
		t.Fatalf("%s %s", mode, block)
	}
	gate := make(chan struct{})
	r, err := f.m.Reconvert(d.ID, ReconvertSpec{Params: `{"v":2}`, InputPaths: []string{"/new/in"}, Summary: "s", Mode: ReconvertReplace},
		rcRunner(out, d.ID, "v2", gate, nil))
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != d.ID || !r.Reconverting || r.OutputPath != out || r.Params != `{"v":1}` {
		t.Fatalf("重转期间对外仍是旧快照: %+v", r)
	}
	// 重转中：再次重转 invalid_state；CheckPaths 报 invalid_state，输出仍算存在
	_, err = f.m.Reconvert(d.ID, ReconvertSpec{Params: "{}"}, rcRunner(out, d.ID, "x", nil, nil))
	wantDetail(t, err, apperr.TaskConflict, "reason=invalid_state")
	if cs, _ := f.m.CheckPaths([]string{d.ID}); cs[0].ReconvertMode != "" || cs[0].ReconvertBlock != BlockInvalidState || !cs[0].OutputExists {
		t.Fatalf("%+v", cs[0])
	}
	if readS(t, out) != "v1" {
		t.Fatal("重转期间旧文件不动")
	}
	close(gate)
	got := waitRC(t, f, d.ID)
	if readS(t, out) != "v2" || got.Params != `{"v":2}` || got.InputPaths[0] != "/new/in" || got.LastReconvertError != nil || got.Version <= d.Version {
		t.Fatalf("%+v", got)
	}
	if exists2(ReconvertTempPath(out, d.ID)) {
		t.Fatal("临时文件应已改名")
	}
	if ev := f.lastStatus(d.ID); ev.ReconvertOutcome != OutcomeSucceeded || ev.Reconverting == nil || *ev.Reconverting {
		t.Fatalf("%+v", ev)
	}
	db, _ := f.st.GetTask(context.Background(), d.ID)
	if db.Reconverting || db.ReconvertPrev != "" || db.ReconvertPending != "" || db.Params != `{"v":2}` || db.Status != StatusSucceeded {
		t.Fatalf("%+v", db)
	}
	if cs, _ := f.m.CheckPaths([]string{d.ID}); cs[0].ReconvertMode != ReconvertReplace || cs[0].ReconvertBlock != "" {
		t.Fatalf("%+v", cs[0])
	}
}

func TestReconvertFailureRestores(t *testing.T) {
	f := newFx(t, 2)
	out := filepath.Join(f.dir, "o.mp4")
	d := f.doneConvert(t, out, "v1")
	_, err := f.m.Reconvert(d.ID, ReconvertSpec{Params: `{"v":2}`, Mode: ReconvertReplace},
		rcRunner(out, d.ID, "half", nil, apperr.New(apperr.ProcessFailed, "转换失败").WithDetail("boom")))
	if err != nil {
		t.Fatal(err)
	}
	got := waitRC(t, f, d.ID)
	if readS(t, out) != "v1" || got.Params != `{"v":1}` || got.FinishedAt != d.FinishedAt || got.Error != nil {
		t.Fatalf("%+v", got)
	}
	if got.LastReconvertError == nil || got.LastReconvertError.Code != string(apperr.ProcessFailed) || got.LastReconvertError.At == 0 {
		t.Fatalf("%+v", got.LastReconvertError)
	}
	if exists2(ReconvertTempPath(out, d.ID)) {
		t.Fatal("失败时临时文件应删除")
	}
	if ev := f.lastStatus(d.ID); ev.ReconvertOutcome != OutcomeFailed || ev.LastReconvertError == nil || ev.Status != StatusSucceeded {
		t.Fatalf("%+v", ev)
	}
	db, _ := f.st.GetTask(context.Background(), d.ID)
	if db.LastReconvertError == nil || db.Reconverting || db.Params != `{"v":1}` {
		t.Fatalf("%+v", db)
	}
	// 下一次重转开始时清掉 lastReconvertError
	gate := make(chan struct{})
	r, err := f.m.Reconvert(d.ID, ReconvertSpec{Params: `{"v":3}`, Mode: ReconvertReplace}, rcRunner(out, d.ID, "v3", gate, nil))
	if err != nil || r.LastReconvertError != nil {
		t.Fatalf("%+v %v", r, err)
	}
	close(gate)
	waitRC(t, f, d.ID)
}

func TestReconvertCancelRestoresWithoutError(t *testing.T) {
	f := newFx(t, 2)
	out := filepath.Join(f.dir, "o.mp4")
	d := f.doneConvert(t, out, "v1")
	gate := make(chan struct{})
	if _, err := f.m.Reconvert(d.ID, ReconvertSpec{Params: `{"v":2}`, Mode: ReconvertReplace}, rcRunner(out, d.ID, "v2", gate, nil)); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return exists2(ReconvertTempPath(out, d.ID)) })
	if err := f.m.Cancel(d.ID); err != nil {
		t.Fatal(err)
	}
	got := waitRC(t, f, d.ID)
	if readS(t, out) != "v1" || got.LastReconvertError != nil || got.Params != `{"v":1}` || got.Status != StatusSucceeded {
		t.Fatalf("%+v", got)
	}
	if exists2(ReconvertTempPath(out, d.ID)) {
		t.Fatal("取消时临时文件应删除")
	}
	if ev := f.lastStatus(d.ID); ev.ReconvertOutcome != OutcomeCanceled || ev.LastReconvertError != nil {
		t.Fatalf("%+v", ev)
	}
}

func TestReconvertInvalidState(t *testing.T) {
	f := newFx(t, 2)
	tk, _ := f.m.Submit(Spec{Type: TypeConvert, OutputPath: filepath.Join(f.dir, "f.mp4")}, RunnerFunc(func(ctx context.Context, _ func(Progress)) (string, error) {
		return "", apperr.New(apperr.ProcessFailed, "x")
	}))
	waitTask(t, f.m, tk.ID)
	_, err := f.m.Reconvert(tk.ID, ReconvertSpec{}, rcRunner("", tk.ID, "", nil, nil))
	wantDetail(t, err, apperr.TaskConflict, "reason=invalid_state")
	if cs, _ := f.m.CheckPaths([]string{tk.ID}); cs[0].ReconvertBlock != BlockInvalidState {
		t.Fatalf("%+v", cs[0])
	}
}

// PM 15(b)：原位置是别的文件（修改时间早于这次转换）→ output_moved。
func TestReconvertOutputMovedBefore(t *testing.T) {
	f := newFx(t, 2)
	out := filepath.Join(f.dir, "o.mp4")
	d := f.doneConvert(t, out, "v1")
	os.Remove(out)
	os.WriteFile(out, []byte("foreign"), 0o644)
	old := time.Now().Add(-time.Hour)
	os.Chtimes(out, old, old)
	if mode, block := ReconvertOutputMode(d); mode != "" || block != BlockOutputMoved {
		t.Fatalf("%s %s", mode, block)
	}
	_, err := f.m.Reconvert(d.ID, ReconvertSpec{}, rcRunner(out, d.ID, "v2", nil, nil))
	wantDetail(t, err, apperr.TaskConflict, "reason=output_moved")
	if cs, _ := f.m.CheckPaths([]string{d.ID}); cs[0].ReconvertMode != "" || cs[0].ReconvertBlock != BlockOutputMoved {
		t.Fatalf("%+v", cs[0])
	}
	if readS(t, out) != "foreign" {
		t.Fatal("别人的文件不能动")
	}
}

// 重转期间旧输出被换掉：不替换，按失败恢复（lastReconvertError reason=output_moved）。
func TestReconvertOutputMovedDuring(t *testing.T) {
	f := newFx(t, 2)
	out := filepath.Join(f.dir, "o.mp4")
	d := f.doneConvert(t, out, "v1")
	gate := make(chan struct{})
	if _, err := f.m.Reconvert(d.ID, ReconvertSpec{Params: `{"v":2}`, Mode: ReconvertReplace}, rcRunner(out, d.ID, "v2", gate, nil)); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(out, []byte("someone else's longer file"), 0o644)
	close(gate)
	got := waitRC(t, f, d.ID)
	if readS(t, out) != "someone else's longer file" || got.LastReconvertError == nil || !strings.Contains(got.LastReconvertError.Detail, "reason=output_moved") {
		t.Fatalf("%+v %+v", got, got.LastReconvertError)
	}
	if exists2(ReconvertTempPath(out, d.ID)) {
		t.Fatal("临时文件应删除")
	}
}

// 替换时旧文件被占用 / 没权限：IO_ERROR reason=in_use / permission，旧文件不动（架构师定：不预警，直接给失败）。
func TestReconvertReplaceErrors(t *testing.T) {
	f := newFx(t, 2)
	out := filepath.Join(f.dir, "o.mp4")
	d := f.doneConvert(t, out, "v1")
	defer func(r func(string, string) error) { replaceFile = r }(replaceFile)
	replaceFile = func(a, b string) error { return &os.LinkError{Op: "rename", Old: a, New: b, Err: fs.ErrPermission} }
	if _, err := f.m.Reconvert(d.ID, ReconvertSpec{Params: `{"v":2}`, Mode: ReconvertReplace}, rcRunner(out, d.ID, "v2", nil, nil)); err != nil {
		t.Fatal(err)
	}
	got := waitRC(t, f, d.ID)
	if readS(t, out) != "v1" || got.LastReconvertError == nil || got.LastReconvertError.Code != string(apperr.IOError) ||
		!strings.Contains(got.LastReconvertError.Detail, "reason=permission") {
		t.Fatalf("%+v", got.LastReconvertError)
	}
	db, _ := f.st.GetTask(context.Background(), d.ID)
	if db.ReconvertPending != "" || db.Reconverting {
		t.Fatalf("pending 应清掉: %+v", db)
	}
}

// PM 15(a)：输出不在 → regenerate；成功时落盘到原路径。
func TestReconvertRegenerate(t *testing.T) {
	f := newFx(t, 2)
	out := filepath.Join(f.dir, "o.mp4")
	d := f.doneConvert(t, out, "v1")
	os.Remove(out)
	if mode, block := ReconvertOutputMode(d); mode != ReconvertRegenerate || block != "" {
		t.Fatalf("%s %s", mode, block)
	}
	if cs, _ := f.m.CheckPaths([]string{d.ID}); cs[0].ReconvertMode != ReconvertRegenerate || cs[0].OutputExists {
		t.Fatalf("%+v", cs[0])
	}
	// 调用方以为是 replace（状态过时）→ output_moved，不开始
	_, err := f.m.Reconvert(d.ID, ReconvertSpec{Mode: ReconvertReplace}, rcRunner(out, d.ID, "v2", nil, nil))
	wantDetail(t, err, apperr.TaskConflict, "reason=output_moved")
	if _, err := f.m.Reconvert(d.ID, ReconvertSpec{Params: `{"v":1}`, Mode: ReconvertRegenerate}, rcRunner(out, d.ID, "v2", nil, nil)); err != nil {
		t.Fatal(err)
	}
	got := waitRC(t, f, d.ID)
	if readS(t, out) != "v2" || got.LastReconvertError != nil || f.lastStatus(d.ID).ReconvertOutcome != OutcomeSucceeded {
		t.Fatalf("%+v", got)
	}
}

// PM 15(a)：regenerate 失败 / 取消 → 记录仍是 succeeded、输出仍不在；失败写 lastReconvertError。
func TestReconvertRegenerateFailureAndCancel(t *testing.T) {
	f := newFx(t, 2)
	out := filepath.Join(f.dir, "o.mp4")
	d := f.doneConvert(t, out, "v1")
	os.Remove(out)
	if _, err := f.m.Reconvert(d.ID, ReconvertSpec{Mode: ReconvertRegenerate}, rcRunner(out, d.ID, "x", nil, apperr.New(apperr.ProcessFailed, "坏了"))); err != nil {
		t.Fatal(err)
	}
	got := waitRC(t, f, d.ID)
	if exists2(out) || got.Status != StatusSucceeded || got.LastReconvertError == nil || exists2(ReconvertTempPath(out, d.ID)) {
		t.Fatalf("%+v", got)
	}
	gate := make(chan struct{})
	if _, err := f.m.Reconvert(d.ID, ReconvertSpec{Mode: ReconvertRegenerate}, rcRunner(out, d.ID, "x", gate, nil)); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return exists2(ReconvertTempPath(out, d.ID)) })
	f.m.Cancel(d.ID)
	got = waitRC(t, f, d.ID)
	if exists2(out) || got.LastReconvertError != nil || exists2(ReconvertTempPath(out, d.ID)) {
		t.Fatalf("%+v", got)
	}
}

// PM 15(a)：regenerate 期间原路径出现了别的文件 → 绝不覆盖，按失败（output_moved）。
func TestReconvertRegenerateNeverOverwrites(t *testing.T) {
	f := newFx(t, 2)
	out := filepath.Join(f.dir, "o.mp4")
	d := f.doneConvert(t, out, "v1")
	os.Remove(out)
	gate := make(chan struct{})
	if _, err := f.m.Reconvert(d.ID, ReconvertSpec{Mode: ReconvertRegenerate}, rcRunner(out, d.ID, "new", gate, nil)); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(out, []byte("appeared"), 0o644)
	close(gate)
	got := waitRC(t, f, d.ID)
	if readS(t, out) != "appeared" || got.LastReconvertError == nil || !strings.Contains(got.LastReconvertError.Detail, "reason=output_moved") {
		t.Fatalf("%+v", got.LastReconvertError)
	}
	if exists2(ReconvertTempPath(out, d.ID)) {
		t.Fatal("临时文件应删除")
	}
}

// 重转中删除：先取消、删临时文件；deleteOutputs=true 时删旧输出。
func TestReconvertDeleteDuring(t *testing.T) {
	f := newFx(t, 2)
	out := filepath.Join(f.dir, "o.mp4")
	d := f.doneConvert(t, out, "v1")
	gate := make(chan struct{})
	defer close(gate)
	if _, err := f.m.Reconvert(d.ID, ReconvertSpec{Mode: ReconvertReplace}, rcRunner(out, d.ID, "v2", gate, nil)); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return exists2(ReconvertTempPath(out, d.ID)) })
	res, err := f.m.DeleteRecords([]string{d.ID}, TypeConvert, true, nil)
	if err != nil || len(res.DeletedTaskIDs) != 1 || len(res.Failures) != 0 {
		t.Fatalf("%+v %v", res, err)
	}
	if exists2(out) || exists2(ReconvertTempPath(out, d.ID)) {
		t.Fatal("旧输出和临时文件都应删除")
	}
	if _, err := f.st.GetTask(context.Background(), d.ID); err == nil {
		t.Fatal("记录应删除")
	}
}

// 应用退出时中断：删临时文件、发事件，但不落库；下次启动 RecoverReconverts 恢复并计数（PM 13）。
func TestReconvertInterruptedAtShutdownCountedOnRecovery(t *testing.T) {
	f := newFx(t, 2)
	out := filepath.Join(f.dir, "o.mp4")
	d := f.doneConvert(t, out, "v1")
	gate := make(chan struct{})
	defer close(gate)
	if _, err := f.m.Reconvert(d.ID, ReconvertSpec{Params: `{"v":2}`, Mode: ReconvertReplace}, rcRunner(out, d.ID, "v2", gate, nil)); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return exists2(ReconvertTempPath(out, d.ID)) })
	f.m.Shutdown(2 * time.Second)
	if ev := f.lastStatus(d.ID); ev.ReconvertOutcome != OutcomeInterrupted {
		t.Fatalf("%+v", ev)
	}
	if exists2(ReconvertTempPath(out, d.ID)) || readS(t, out) != "v1" {
		t.Fatal("退出时删临时文件，旧文件不动")
	}
	db, _ := f.st.GetTask(context.Background(), d.ID)
	if !db.Reconverting {
		t.Fatal("中断不落库：库里仍是 reconverting")
	}
	n, err := RecoverReconverts(context.Background(), f.st, nil, nil)
	if err != nil || n != 1 {
		t.Fatalf("%d %v", n, err)
	}
	db, _ = f.st.GetTask(context.Background(), d.ID)
	if db.Reconverting || db.Status != StatusSucceeded || db.Params != `{"v":1}` || db.LastReconvertError != nil || db.ReconvertPrev != "" {
		t.Fatalf("%+v", db)
	}
	// 再跑一次：没有可恢复的
	if n, _ := RecoverReconverts(context.Background(), f.st, nil, nil); n != 0 {
		t.Fatal(n)
	}
}

// 崩溃恢复（进程直接没了）：按库里的快照恢复；改名已完成（有 pending、临时文件不在、目标在）的按成功收尾、不计数。
func TestRecoverReconvertsCrash(t *testing.T) {
	f := newFx(t, 2)
	ctx := context.Background()
	outA := filepath.Join(f.dir, "a.mp4")
	outB := filepath.Join(f.dir, "b.mp4")
	a := f.doneConvert(t, outA, "a1")
	b := f.doneConvert(t, outB, "b1")
	f.m.Shutdown(time.Second)

	crash := func(tk Task, pending *reconvertPending) {
		prev := reconvertPrev{Params: tk.Params, InputPaths: tk.InputPaths, OutputPath: tk.OutputPath, Result: tk.Result, Progress: 1,
			StartedAt: tk.StartedAt, FinishedAt: tk.FinishedAt, Mode: ReconvertReplace}
		pj, _ := json.Marshal(prev)
		tk.Reconverting, tk.Status, tk.ReconvertPrev, tk.Params = true, StatusRunning, string(pj), `{"v":"new"}`
		if pending != nil {
			b, _ := json.Marshal(pending)
			tk.ReconvertPending = string(b)
		}
		tk.Version++
		if err := f.st.UpdateTask(ctx, tk); err != nil {
			t.Fatal(err)
		}
	}
	// a：跑到一半崩了，临时文件还在
	crash(a, nil)
	os.WriteFile(ReconvertTempPath(outA, a.ID), []byte("half"), 0o644)
	// b：pending 已写、改名已完成
	crash(b, &reconvertPending{Params: `{"v":"b2"}`, InputPaths: []string{"/b2"}, StartedAt: 5, FinishedAt: 6})
	os.WriteFile(outB, []byte("b2"), 0o644)
	// 孤儿临时文件：记录输出目录和 extraDirs 里的都删；不匹配的、子目录里的不动
	extra := filepath.Join(f.dir, "extra")
	os.MkdirAll(filepath.Join(extra, "sub"), 0o755)
	orphan1 := filepath.Join(f.dir, "x.reconvert-ABC123.part.mp4")
	orphan2 := filepath.Join(extra, "y.reconvert-Z9.part.mkv")
	keep1 := filepath.Join(extra, "y.part.mkv")
	keep2 := filepath.Join(extra, "sub", "z.reconvert-Q1.part.mp4")
	for _, p := range []string{orphan1, orphan2, keep1, keep2} {
		os.WriteFile(p, []byte("x"), 0o644)
	}
	n, err := RecoverReconverts(ctx, f.st, []string{extra}, nil)
	if err != nil || n != 1 {
		t.Fatalf("只有 a 算被中断: %d %v", n, err)
	}
	ga, _ := f.st.GetTask(ctx, a.ID)
	if ga.Reconverting || ga.Status != StatusSucceeded || ga.Params != `{"v":1}` || exists2(ReconvertTempPath(outA, a.ID)) || readS(t, outA) != "a1" {
		t.Fatalf("%+v", ga)
	}
	gb, _ := f.st.GetTask(ctx, b.ID)
	if gb.Reconverting || gb.Status != StatusSucceeded || gb.Params != `{"v":"b2"}` || gb.InputPaths[0] != "/b2" || gb.FinishedAt != 6 || gb.ReconvertPending != "" {
		t.Fatalf("%+v", gb)
	}
	if exists2(orphan1) || exists2(orphan2) || !exists2(keep1) || !exists2(keep2) {
		t.Fatal("孤儿扫描范围不对")
	}
}

// 删除记录的兜底：DeleteRecords 删这条记录的重转临时文件。
func TestDeleteRecordsRemovesReconvertTemp(t *testing.T) {
	f := newFx(t, 2)
	out := filepath.Join(f.dir, "o.mp4")
	d := f.doneConvert(t, out, "v1")
	tmp := ReconvertTempPath(out, d.ID)
	os.WriteFile(tmp, []byte("x"), 0o644)
	if _, err := f.m.DeleteRecords([]string{d.ID}, TypeConvert, false, nil); err != nil {
		t.Fatal(err)
	}
	if exists2(tmp) || !exists2(out) {
		t.Fatal("删临时文件，不删输出（deleteOutputs=false）")
	}
}

func TestReconvertTempPath(t *testing.T) {
	p := ReconvertTempPath(filepath.Join("/a", "视频 1.mp4"), "T1")
	if filepath.Base(p) != "视频 1.reconvert-T1.part.mp4" || !reconvertTempRe.MatchString(filepath.Base(p)) {
		t.Fatal(p)
	}
	if reconvertTempRe.MatchString("a.part.mp4") || reconvertTempRe.MatchString("a.reconvert-.part.mp4") {
		t.Fatal("不该匹配")
	}
	_ = store.ReconvertError{}
}

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
)

func mkTask(id string, typ TaskType, st TaskStatus, created int64) Task {
	return Task{ID: id, Type: typ, Status: st, Title: "t-" + id, InputPaths: []string{"/a/" + id}, OutputPath: "/o/" + id,
		Params: `{"k":1}`, Progress: 0, Version: 1, CreatedAt: created}
}

func TestTaskRoundTrip(t *testing.T) {
	ctx := context.Background()
	s, _ := openTemp(t)
	in := mkTask("A", TypeConvert, StatusQueued, 100)
	in.LogPath = "/logs/A.log"
	if err := s.InsertTask(ctx, in); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetTask(ctx, "A")
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != TypeConvert || got.Status != StatusQueued || got.Title != "t-A" || got.InputPaths[0] != "/a/A" ||
		got.Params != `{"k":1}` || got.LogPath != "/logs/A.log" || got.Error != nil || got.CreatedAt != 100 {
		t.Fatalf("%+v", got)
	}
	// 更新为失败，错误 JSON 往返
	got.Status, got.Version, got.FinishedAt = StatusFailed, 3, 999
	got.Error = apperr.New(apperr.ProcessFailed, "boom").WithDetail("last lines")
	if err := s.UpdateTask(ctx, got); err != nil {
		t.Fatal(err)
	}
	back, _ := s.GetTask(ctx, "A")
	if back.Status != StatusFailed || back.Version != 3 || back.Error == nil || back.Error.Code != apperr.ProcessFailed ||
		back.Error.Message != "boom" || back.Error.Detail != "last lines" || back.FinishedAt != 999 {
		t.Fatalf("%+v", back)
	}
	// 不存在
	if _, err := s.GetTask(ctx, "nope"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("%v", err)
	}
	if err := s.UpdateTask(ctx, Task{ID: "nope"}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("%v", err)
	}
	// 空输入列表落库为 []
	e := mkTask("E", TypeOfficePDF, StatusQueued, 1)
	e.InputPaths, e.Params = nil, ""
	s.InsertTask(ctx, e)
	if g, _ := s.GetTask(ctx, "E"); g.InputPaths == nil || len(g.InputPaths) != 0 || g.Params != "{}" {
		t.Fatalf("%+v", g)
	}
}

func TestListTasksFilterAndPaging(t *testing.T) {
	ctx := context.Background()
	s, _ := openTemp(t)
	types := []TaskType{TypeConvert, TypeEditExport, TypeConvert}
	statuses := []TaskStatus{StatusSucceeded, StatusFailed, StatusQueued}
	for i := 0; i < 30; i++ {
		s.InsertTask(ctx, mkTask(fmt.Sprintf("T%02d", i), types[i%3], statuses[i%3], int64(1000+i)))
	}
	p, err := s.ListTasks(ctx, TaskFilter{Limit: 10})
	if err != nil || p.Total != 30 || len(p.Items) != 10 || p.Items[0].ID != "T29" || p.Items[9].ID != "T20" {
		t.Fatalf("%+v %v", p, err)
	}
	p, _ = s.ListTasks(ctx, TaskFilter{Limit: 10, Offset: 25})
	if len(p.Items) != 5 || p.Items[0].ID != "T04" {
		t.Fatalf("%d %s", len(p.Items), p.Items[0].ID)
	}
	p, _ = s.ListTasks(ctx, TaskFilter{Types: []TaskType{TypeEditExport}})
	if p.Total != 10 {
		t.Fatalf("按类型: %d", p.Total)
	}
	p, _ = s.ListTasks(ctx, TaskFilter{Types: []TaskType{TypeConvert}, Statuses: []TaskStatus{StatusQueued, StatusFailed}})
	if p.Total != 10 { // convert 且 queued：i%3==2 的 10 条
		t.Fatalf("组合过滤: %d", p.Total)
	}
	// 默认与上限
	p, _ = s.ListTasks(ctx, TaskFilter{})
	if len(p.Items) != 30 {
		t.Fatal("默认 limit 50 应返回全部 30 条")
	}
	if p, _ = s.ListTasks(ctx, TaskFilter{Limit: 100000}); len(p.Items) != 30 {
		t.Fatal("超大 limit 应被限制但仍可用")
	}
	if p, _ = s.ListTasks(ctx, TaskFilter{Statuses: []TaskStatus{StatusCanceled}}); p.Total != 0 || p.Items == nil {
		t.Fatalf("空结果 Items 应为空数组而不是 nil: %+v", p)
	}
}

func TestDeleteTasks(t *testing.T) {
	ctx := context.Background()
	s, _ := openTemp(t)
	s.InsertTask(ctx, mkTask("Q", TypeConvert, StatusQueued, 1))
	s.InsertTask(ctx, mkTask("R", TypeConvert, StatusRunning, 2))
	s.InsertTask(ctx, mkTask("S", TypeConvert, StatusSucceeded, 3))
	s.InsertTask(ctx, mkTask("F", TypeConvert, StatusFailed, 4))
	s.InsertTask(ctx, mkTask("I", TypeConvert, StatusInterrupted, 5))
	del, err := s.DeleteTasks(ctx, []string{"Q", "R", "S", "missing"})
	if err != nil || len(del) != 1 || del[0] != "S" {
		t.Fatalf("进行中的任务不能删: %v %v", del, err)
	}
	gone, err := s.DeleteFinishedTasks(ctx)
	if err != nil || len(gone) != 2 {
		t.Fatalf("%v %v", gone, err)
	}
	p, _ := s.ListTasks(ctx, TaskFilter{})
	if p.Total != 2 {
		t.Fatalf("应只剩排队和运行中: %d", p.Total)
	}
	act, _ := s.ListTasksByStatus(ctx, StatusQueued, StatusRunning)
	if len(act) != 2 || act[0].ID != "Q" {
		t.Fatalf("%+v", act)
	}
}

func TestMarkInterruptedAffectsOnlyActive(t *testing.T) {
	ctx := context.Background()
	s, _ := openTemp(t)
	s.InsertTask(ctx, mkTask("Q", TypeConvert, StatusQueued, 1))
	s.InsertTask(ctx, mkTask("R", TypeConvert, StatusRunning, 2))
	s.InsertTask(ctx, mkTask("S", TypeConvert, StatusSucceeded, 3))
	n, err := s.MarkInterrupted(ctx, time.UnixMilli(5000))
	if err != nil || n != 2 {
		t.Fatalf("%d %v", n, err)
	}
	for _, id := range []string{"Q", "R"} {
		g, _ := s.GetTask(ctx, id)
		if g.Status != StatusInterrupted || g.Version != 2 || g.FinishedAt != 5000 {
			t.Fatalf("%s %+v", id, g)
		}
	}
	if g, _ := s.GetTask(ctx, "S"); g.Status != StatusSucceeded || g.Version != 1 {
		t.Fatalf("%+v", g)
	}
}

func TestTaskOutputsByBase(t *testing.T) {
	ctx := context.Background()
	s, _ := openTemp(t)
	for i, out := range []string{"/o/a_b.mp4", "/o/axb.mp4", "/o/100%.mp4", "/p/A_B.MP4", ""} {
		tk := mkTask(fmt.Sprintf("T%d", i), TypeConvert, StatusSucceeded, int64(i))
		tk.OutputPath = out
		if err := s.InsertTask(ctx, tk); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.TaskOutputsByBase(ctx, "a_b.mp4")
	if err != nil {
		t.Fatal(err)
	}
	// _ 不是通配符：axb.mp4 不能匹配；LIKE 对 ASCII 大小写不敏感，所以 A_B.MP4 会被粗筛进来（调用方再精确比较）
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
	if got, _ := s.TaskOutputsByBase(ctx, "100%.mp4"); len(got) != 1 || got[0] != "/o/100%.mp4" {
		t.Fatalf("%% 应按字面匹配: %v", got)
	}
	if got, _ := s.TaskOutputsByBase(ctx, ""); got != nil {
		t.Fatalf("空文件名应返回 nil: %v", got)
	}
}

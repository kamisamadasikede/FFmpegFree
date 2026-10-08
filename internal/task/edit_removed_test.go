package task

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/store"
)

// 契约 v0.23.5：剪辑功能已移除。库里旧的 edit_export 记录照常出现在任务列表（含任务中心），可以移除；
// Retry 不论什么状态都是 UNSUPPORTED（detail 说明原因）；启动时没跑完的旧导出变成 interrupted，没有 Runner、不会被重新执行。
func TestLegacyEditExportRecords(t *testing.T) {
	f := newFx(t, 1)
	ctx := context.Background()
	out := filepath.Join(f.dir, "cut.mp4")
	os.WriteFile(out, []byte("x"), 0o644)
	ins := func(id string, st store.TaskStatus, created int64, outPath string) {
		t.Helper()
		if err := f.st.InsertTask(ctx, store.Task{ID: id, Type: store.TypeEditExport, Status: st, Title: "剪辑导出 " + id,
			InputPaths: []string{}, OutputPath: outPath, Version: 1, CreatedAt: created, StartedAt: created}); err != nil {
			t.Fatal(err)
		}
	}
	ins("EOK", store.StatusSucceeded, 100, out)
	ins("EFAIL", store.StatusFailed, 101, "")
	ins("ERUN", store.StatusRunning, 102, "") // 上次退出时还在导出
	if _, err := f.st.MarkInterrupted(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}

	page, err := f.m.List(Filter{})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Status{}
	for _, it := range page.Items {
		if it.Type == TypeEditExport {
			got[it.ID] = it.Status
		}
	}
	if len(got) != 3 || got["ERUN"] != StatusInterrupted || got["EOK"] != StatusSucceeded {
		t.Fatalf("旧的剪辑导出记录要照常列出（中断的保持 interrupted）: %v", got)
	}
	if acts := f.m.ListActive(); len(acts) != 0 {
		t.Fatalf("不应有进行中的任务: %+v", acts)
	}
	for _, id := range []string{"EOK", "EFAIL", "ERUN"} {
		_, err := f.m.Retry(id)
		ae := apperr.From(err)
		// v0.25.3：任务中心叫“旧版导出”，只能查看和删除；用户文字里不出现“剪辑”。
		if !apperr.Is(err, apperr.Unsupported) || !strings.HasPrefix(ae.Detail, "reason=feature_removed") ||
			ae.Message != "旧版导出记录只能查看和删除，不能重试" || strings.Contains(ae.Message+ae.Detail, "剪辑") {
			t.Fatalf("Retry(%s) 应 UNSUPPORTED reason=feature_removed: %v (%q)", id, err, ae.Detail)
		}
		_, err = f.m.Reconvert(id, ReconvertSpec{}, RunnerFunc(func(context.Context, func(Progress)) (string, error) { return "", nil }))
		if ae := apperr.From(err); !apperr.Is(err, apperr.Unsupported) || ae.Message != "旧版导出记录只能查看和删除，不能重转" || !strings.HasPrefix(ae.Detail, "reason=feature_removed") {
			t.Fatalf("Reconvert(%s) 应 UNSUPPORTED reason=feature_removed: %v", id, err)
		}
	}
	if tk, _ := f.m.Get("ERUN"); tk.Status != StatusInterrupted || tk.Version != 2 {
		t.Fatalf("Retry 失败后记录不变: %+v", tk)
	}
	// 移除：记录删掉；deleteOutput=true 时成功记录的输出文件一起删
	if err := f.m.Remove([]string{"EFAIL", "ERUN"}, false); err != nil {
		t.Fatal(err)
	}
	if err := f.m.Remove([]string{"EOK"}, true); err != nil {
		t.Fatal(err)
	}
	if page, _ := f.m.List(Filter{}); page.Total != 0 {
		t.Fatalf("移除后列表为空: %+v", page.Items)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("deleteOutput=true 应删输出文件: %v", err)
	}
}

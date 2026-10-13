package system

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"FFmpegFree/internal/apperr"
)

func TestOpenStorageFolderOfficeCanvas(t *testing.T) {
	f, _, _ := storageFx(t)
	ctx := context.Background()

	// 未注入 hook → NOT_FOUND
	err := f.mgr.OpenStorageFolder(ctx, StorageOfficeCanvas)
	wantErr(t, err, apperr.NotFound, "办公画布组件还没有就绪。")
	if ae := err.(*apperr.AppError); ae.Detail != "reason=component" {
		t.Fatalf("detail=%q", ae.Detail)
	}

	// hook 返回空 → NOT_FOUND
	f.mgr.OfficeCanvasDir = func() (string, error) { return "", nil }
	wantErr(t, f.mgr.OpenStorageFolder(ctx, StorageOfficeCanvas), apperr.NotFound, "办公画布组件还没有就绪。")

	// hook 返回不存在目录 → NOT_FOUND
	f.mgr.OfficeCanvasDir = func() (string, error) {
		return filepath.Join(f.root, "missing-canvas"), nil
	}
	wantErr(t, f.mgr.OpenStorageFolder(ctx, StorageOfficeCanvas), apperr.NotFound, "办公画布组件还没有就绪。")

	// ready：打开目录
	dir := filepath.Join(f.root, "office-canvas-ready")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	f.mgr.OfficeCanvasDir = func() (string, error) { return dir, nil }
	n := len(f.launched)
	if err := f.mgr.OpenStorageFolder(ctx, StorageOfficeCanvas); err != nil {
		t.Fatal(err)
	}
	if len(f.launched) != n+1 {
		t.Fatalf("launched=%v", f.launched)
	}
}

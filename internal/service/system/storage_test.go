package system

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/paths"
)

func storageFx(t *testing.T) (*revealFx, paths.Storage, string) {
	t.Helper()
	f := newRevealFx(t)
	data := filepath.Join(f.root, "data")
	base := filepath.Join(f.root, "base")
	st := paths.Storage{Base: base, Output: filepath.Join(base, "output"), Uploads: filepath.Join(base, "uploads"), BaseKind: paths.BaseExeDir}
	for _, d := range []string{data, st.Output, st.Uploads} {
		os.MkdirAll(d, 0o755)
	}
	f.mgr.SetStorage(st, data)
	f.mgr.cfg.Locator, f.mgr.started = &ffmpeg.Locator{}, true // GetSettings / UpdateSettings 需要
	return f, st, data
}

func wantErr(t *testing.T, err error, code apperr.Code, msg string) {
	t.Helper()
	ae, ok := err.(*apperr.AppError)
	if !ok || ae.Code != code || (msg != "" && ae.Message != msg) {
		t.Fatalf("want %s %q, got %v", code, msg, err)
	}
}

func TestGetStorageDirsDefaults(t *testing.T) {
	f, st, _ := storageFx(t)
	ctx := context.Background()
	d, err := f.mgr.GetStorageDirs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if d.OutputDir != st.Output || d.UploadsDir != st.Uploads || d.OutputCustom || d.UploadsCustom ||
		d.DefaultOutputDir != st.Output || d.DefaultUploadsDir != st.Uploads || d.BaseKind != "exe_dir" || d.FellBack ||
		!d.OutputAvailable || !d.UploadsAvailable {
		t.Fatalf("%+v", d)
	}
	if f.mgr.ActualOutputDir(ctx) != st.Output || f.mgr.ActualUploadsDir(ctx) != st.Uploads {
		t.Fatal("实际目录应是默认目录")
	}
	os.Remove(st.Uploads)
	if d, _ := f.mgr.GetStorageDirs(ctx); d.UploadsAvailable {
		t.Fatal("目录不在时 uploadsAvailable=false")
	}
}

func TestSetStorageDirsValidation(t *testing.T) {
	f, st, data := storageFx(t)
	ctx := context.Background()
	custom := filepath.Join(f.root, "mine")
	os.MkdirAll(custom, 0o755)
	file := f.file(t, filepath.Join(f.root, "afile"))
	cases := []struct {
		req StorageDirsUpdate
		msg string
	}{
		{StorageDirsUpdate{OutputDir: "rel"}, "保存位置必须是绝对路径"},
		{StorageDirsUpdate{OutputDir: filepath.Join(f.root, "nope")}, "保存位置不存在"},
		{StorageDirsUpdate{OutputDir: file}, "保存位置不是文件夹"},
		{StorageDirsUpdate{UploadsDir: "rel"}, "上传位置必须是绝对路径"},
		{StorageDirsUpdate{UploadsDir: filepath.Join(f.root, "nope")}, "上传位置不存在"},
		{StorageDirsUpdate{OutputDir: custom, UploadsDir: file}, "上传位置不是文件夹"}, // 任一失败整体不生效
		{StorageDirsUpdate{OutputDir: data}, "保存位置不能在应用数据目录内"},
		{StorageDirsUpdate{OutputDir: filepath.Join(data)}, "保存位置不能在应用数据目录内"},
	}
	for _, c := range cases {
		_, err := f.mgr.SetStorageDirs(ctx, c.req)
		wantErr(t, err, apperr.InvalidArgument, c.msg)
	}
	if f.mgr.DefaultOutputDir(ctx) != "" || f.mgr.UploadsDir(ctx) != "" {
		t.Fatal("失败时什么都不应保存")
	}
	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		ro := filepath.Join(f.root, "ro")
		os.MkdirAll(ro, 0o555)
		t.Cleanup(func() { os.Chmod(ro, 0o755) })
		_, err := f.mgr.SetStorageDirs(ctx, StorageDirsUpdate{OutputDir: ro})
		wantErr(t, err, apperr.InvalidArgument, "保存位置无法写入")
	}
	// 成功：自定义输出目录；上传目录传默认目录的路径 → 按 "" 保存
	d, err := f.mgr.SetStorageDirs(ctx, StorageDirsUpdate{OutputDir: custom + string(filepath.Separator), UploadsDir: st.Uploads})
	if err != nil {
		t.Fatal(err)
	}
	if d.OutputDir != custom || !d.OutputCustom || d.UploadsCustom || d.UploadsDir != st.Uploads {
		t.Fatalf("%+v", d)
	}
	if f.mgr.UploadsDir(ctx) != "" || f.mgr.DefaultOutputDir(ctx) != custom || f.mgr.ActualOutputDir(ctx) != custom {
		t.Fatal("保存的值不对")
	}
	// UpdateSettings 走同一套校验，读写同一组键
	s, _ := f.mgr.GetSettings(ctx)
	if s.DefaultOutputDir != custom || s.UploadsDir != "" {
		t.Fatalf("%+v", s)
	}
	s.UploadsDir = "rel"
	wantErr(t, f.mgr.UpdateSettings(ctx, s), apperr.InvalidArgument, "上传位置必须是绝对路径")
	s.UploadsDir, s.DefaultOutputDir = custom, st.Output
	if err := f.mgr.UpdateSettings(ctx, s); err != nil {
		t.Fatal(err)
	}
	if d, _ := f.mgr.GetStorageDirs(ctx); d.OutputCustom || !d.UploadsCustom || d.UploadsDir != custom {
		t.Fatalf("%+v", d)
	}
}

// v0.24.1：<dataDir>/output 放行（macOS / Linux 回退后 <base> 就是数据目录）。
func TestSetStorageDirsDataDirOutputAllowed(t *testing.T) {
	f, _, data := storageFx(t)
	ctx := context.Background()
	out := filepath.Join(data, "output", "mine")
	os.MkdirAll(out, 0o755)
	if _, err := f.mgr.SetStorageDirs(ctx, StorageDirsUpdate{OutputDir: out}); err != nil {
		t.Fatalf("<dataDir>/output 的子文件夹应放行: %v", err)
	}
	logs := filepath.Join(data, "logs")
	os.MkdirAll(logs, 0o755)
	_, err := f.mgr.SetStorageDirs(ctx, StorageDirsUpdate{OutputDir: logs})
	wantErr(t, err, apperr.InvalidArgument, "保存位置不能在应用数据目录内")
}

func TestOpenStorageFolder(t *testing.T) {
	f, st, _ := storageFx(t)
	ctx := context.Background()
	wantErr(t, f.mgr.OpenStorageFolder(ctx, "logs"), apperr.InvalidArgument, "")
	// 默认目录不在：先建再打开
	os.RemoveAll(st.Output)
	if err := f.mgr.OpenStorageFolder(ctx, "output"); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(st.Output); err != nil || !fi.IsDir() {
		t.Fatal("默认目录应被建出来")
	}
	if len(f.launched) != 1 || f.launched[0][len(f.launched[0])-1] != st.Output {
		t.Fatalf("%v", f.launched)
	}
	if err := f.mgr.OpenStorageFolder(ctx, "uploads"); err != nil || len(f.launched) != 2 {
		t.Fatalf("%v %v", err, f.launched)
	}
	// 自定义目录不在：NOT_FOUND reason=file，不替用户建
	custom := filepath.Join(f.root, "usb")
	os.MkdirAll(custom, 0o755)
	if _, err := f.mgr.SetStorageDirs(ctx, StorageDirsUpdate{OutputDir: custom}); err != nil {
		t.Fatal(err)
	}
	os.Remove(custom)
	err := f.mgr.OpenStorageFolder(ctx, "output")
	wantErr(t, err, apperr.NotFound, "保存位置不存在，请在设置里重新选择")
	if !strings.HasPrefix(err.(*apperr.AppError).Detail, "reason=file") {
		t.Fatal(err.(*apperr.AppError).Detail)
	}
	if _, err := os.Stat(custom); !os.IsNotExist(err) {
		t.Fatal("不应替用户建自定义目录")
	}
	// 自定义目录不在时不悄悄改用默认目录
	if f.mgr.ActualOutputDir(ctx) != custom {
		t.Fatal("实际输出目录仍是自定义目录")
	}
}

// 6.15.2 第 7 条：RevealInFolder 第 2 类放行实际输出目录和实际上传目录（默认目录也放行）。
func TestRevealAllowsStorageDirs(t *testing.T) {
	f, st, _ := storageFx(t)
	for _, p := range []string{f.file(t, filepath.Join(st.Output, "a.mp4")), f.file(t, filepath.Join(st.Uploads, "id", "b.mov")), st.Uploads} {
		if err := f.mgr.RevealInFolder(p); err != nil {
			t.Fatalf("%s 应允许: %v", p, err)
		}
	}
	if c := revealCode(f.mgr.RevealInFolder(f.outside)); c != apperr.InvalidArgument {
		t.Fatalf("范围外应拒绝: %v", c)
	}
}

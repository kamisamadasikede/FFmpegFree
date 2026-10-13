package office

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/officecanvas"
)

func TestExportNotReady(t *testing.T) {
	dir := t.TempDir()
	m := officecanvas.New(officecanvas.Config{Dir: dir})
	m.Start()
	m.WaitChecked()
	svc := New(Config{Canvas: m, TempRoot: filepath.Join(dir, "tmp"), ActualOutputDir: func(context.Context) string {
		return filepath.Join(dir, "out")
	}})
	_, err := svc.ExportOfficeCanvasPptx(context.Background(), ExportOfficeCanvasPptxRequest{
		ConvID: "c1", ArtifactID: "a1",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	ae := apperr.From(err)
	if !strings.HasPrefix(ae.Message, "导出失败：") {
		t.Fatalf("message=%q", ae.Message)
	}
}

func TestExportArtifactNotFound(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "office", "canvas", "1.0.0")
	os.MkdirAll(root, 0o755)
	name := "office-canvas"
	if runtime.GOOS == "windows" {
		name = "office-canvas.exe"
	}
	os.WriteFile(filepath.Join(root, name), []byte("x"), 0o755)
	m := officecanvas.New(officecanvas.Config{Dir: dir, Manifest: officecanvas.Manifest{Version: "1.0.0"}})
	m.Start()
	m.WaitChecked()
	if m.Status().State != officecanvas.StateReady {
		t.Fatalf("%+v", m.Status())
	}
	svc := New(Config{Canvas: m, TempRoot: filepath.Join(dir, "tmp"), ActualOutputDir: func(context.Context) string {
		return filepath.Join(dir, "out")
	}})
	_, err := svc.ExportOfficeCanvasPptx(context.Background(), ExportOfficeCanvasPptxRequest{
		ConvID: "c1", ArtifactID: "missing",
	})
	ae := apperr.From(err)
	if ae.Code != apperr.NotFound {
		t.Fatalf("code=%s msg=%s", ae.Code, ae.Message)
	}
	if ae.Message != officecanvas.MsgExportNoDeck {
		t.Fatalf("message=%q", ae.Message)
	}
}

func TestInstallViaServiceNotConfigured(t *testing.T) {
	dir := t.TempDir()
	m := officecanvas.New(officecanvas.Config{Dir: dir})
	m.Start()
	m.WaitChecked()
	svc := New(Config{Canvas: m})
	err := svc.InstallOfficeCanvas(context.Background())
	ae := apperr.From(err)
	if ae.Message != officecanvas.MsgNotPublished {
		t.Fatalf("%q", ae.Message)
	}
	if err := svc.CancelOfficeCanvasInstall(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestComponentDirNotReady(t *testing.T) {
	dir := t.TempDir()
	m := officecanvas.New(officecanvas.Config{Dir: dir})
	m.Start()
	m.WaitChecked()
	svc := New(Config{Canvas: m})
	_, err := svc.ComponentDir()
	ae := apperr.From(err)
	if ae.Code != apperr.NotFound || !strings.Contains(ae.Detail, "reason=component") {
		t.Fatalf("%#v", ae)
	}
}

package officecanvas

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
)

func TestDetectMissingWhenNoInstall(t *testing.T) {
	dir := t.TempDir()
	m := New(Config{Dir: dir})
	m.Start()
	m.WaitChecked()
	st := m.Status()
	if st.State != StateMissing {
		t.Fatalf("state=%s want missing", st.State)
	}
	if st.CanDownload {
		t.Fatal("placeholder manifest must have CanDownload=false")
	}
	if st.DownloadBytes <= 0 {
		t.Fatal("guide DownloadBytes")
	}
	if st.Source != "" {
		t.Fatalf("source should be empty when not ready: %q", st.Source)
	}
}

func TestDetectReadyWhenEntryPresent(t *testing.T) {
	dir := t.TempDir()
	ver := "1.2.3"
	root := filepath.Join(dir, "office", "canvas", ver)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	name := "office-canvas"
	if runtime.GOOS == "windows" {
		name = "office-canvas.exe"
	}
	if err := os.WriteFile(filepath.Join(root, name), []byte("#!/bin/true\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := New(Config{Dir: dir, Manifest: Manifest{Version: ver}})
	m.Start()
	m.WaitChecked()
	st := m.Status()
	if st.State != StateReady || st.Source != SourceDownloaded || st.Version != ver {
		t.Fatalf("%+v", st)
	}
	if m.ComponentRoot() != root {
		t.Fatalf("path=%s", m.ComponentRoot())
	}
}

func TestInstallNotConfiguredMessage(t *testing.T) {
	dir := t.TempDir()
	m := New(Config{Dir: dir})
	m.Start()
	m.WaitChecked()
	err := m.Install()
	if err == nil {
		t.Fatal("expected error")
	}
	ae := apperr.From(err)
	if ae.Code != apperr.LangDownloadFailed {
		t.Fatalf("code=%s", ae.Code)
	}
	if ae.Message != MsgNotPublished {
		t.Fatalf("message=%q", ae.Message)
	}
	if !strings.Contains(ae.Detail, "reason=not_configured") {
		t.Fatalf("detail=%q", ae.Detail)
	}
	// 状态保持 missing，不进入 failed
	if m.Status().State != StateMissing {
		t.Fatalf("state after failed install attempt: %s", m.Status().State)
	}
}

func TestCancelInstallIdleOK(t *testing.T) {
	dir := t.TempDir()
	m := New(Config{Dir: dir})
	m.CancelInstall() // 不应 panic
	if err := func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = apperr.New(apperr.Internal, "panic")
			}
		}()
		m.CancelInstall()
		return nil
	}(); err != nil {
		t.Fatal(err)
	}
}

func TestRecheckAfterMissing(t *testing.T) {
	dir := t.TempDir()
	m := New(Config{Dir: dir})
	m.Start()
	m.WaitChecked()
	st := m.Recheck()
	if st.State != StateChecking {
		// Recheck 立即返回 checking，随后异步 detect
		t.Fatalf("immediate recheck state=%s", st.State)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if m.Status().State == StateMissing {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("never reached missing: %s", m.Status().State)
}

func TestDefaultDir(t *testing.T) {
	got := DefaultDir("/data/root")
	if runtime.GOOS == "windows" {
		if !strings.Contains(got, "FFmpegFree") || !strings.Contains(got, "components") {
			t.Fatalf("%s", got)
		}
	} else if got != filepath.Join("/data/root", "components") {
		t.Fatalf("%s", got)
	}
}

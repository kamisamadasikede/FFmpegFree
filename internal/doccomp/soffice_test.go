package doccomp

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
)

// realSoffice 返回箱子上装的系统组件；没有就跳过（集成测试）。
func realSoffice(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("-short")
	}
	for _, p := range systemCandidates() {
		if regular(p) {
			return p
		}
	}
	t.Skip("没有安装文档组件")
	return ""
}

func TestRealDetectAndValidate(t *testing.T) {
	exe := realSoffice(t)
	m := New(Config{Dir: t.TempDir(), Logf: t.Logf})
	t0 := time.Now()
	m.Start()
	s := m.Wait(context.Background(), 3*time.Minute)
	t.Logf("检测用时 %s：%+v", time.Since(t0).Round(time.Millisecond), s)
	if s.State != StateReady || s.Source != SourceSystem || s.Version == "" || s.Path == "" {
		t.Fatalf("%+v (候选 %s)", s, exe)
	}
	if runtime.GOOS == "linux" && (s.CanDownload || s.InstallBytes != 0) {
		t.Fatalf("%+v", s)
	}
}

func TestRealConvertCorruptAndCrash(t *testing.T) {
	exe := realSoffice(t)
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.docx")
	os.WriteFile(bad, []byte("PK\x03\x04 this is not a real zip"), 0o644)
	_, err := Convert(context.Background(), Job{Exe: exe, Input: bad, ConvertTo: "pdf:writer_pdf_Export", WorkDir: filepath.Join(dir, "w"), OutExt: "pdf", Timeout: 2 * time.Minute})
	ae := apperr.From(err)
	if ae == nil || (ae.Code != apperr.DocCorrupt && ae.Code != apperr.DocComponentCrashed) {
		t.Fatalf("%v", err)
	}
	if strings.Contains(ae.Detail, dir) {
		t.Fatalf("detail 不能有路径: %q", ae.Detail)
	}
	t.Logf("坏文件：%s %q", ae.Code, ae.Detail)
}

// 超时：用一个会派生子进程并一直睡的假组件，确认整个进程树都被结束。
func TestConvertTimeoutKillsTree(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix 脚本")
	}
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")
	exe := filepath.Join(dir, "fake-soffice")
	os.WriteFile(exe, []byte("#!/bin/sh\nsleep 60 &\necho $! > "+pidFile+"\nwait\n"), 0o755)
	in := filepath.Join(dir, "a.txt")
	os.WriteFile(in, []byte("x"), 0o644)
	t0 := time.Now()
	_, err := Convert(context.Background(), Job{Exe: exe, Input: in, ConvertTo: "pdf", WorkDir: filepath.Join(dir, "w"), OutExt: "pdf", Timeout: time.Second})
	if !apperr.Is(err, apperr.DocTimeout) || time.Since(t0) > 10*time.Second {
		t.Fatalf("%v %s", err, time.Since(t0))
	}
	time.Sleep(200 * time.Millisecond)
	b, _ := os.ReadFile(pidFile)
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	if pid == 0 {
		t.Fatal("没拿到子进程 pid")
	}
	if err := syscall.Kill(pid, 0); err == nil {
		// 可能是僵尸：看 /proc 状态
		st, _ := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
		if !strings.Contains(string(st), ") Z") && len(st) > 0 {
			t.Fatalf("子进程 %d 还活着: %s", pid, st)
		}
	}
	// 非零退出、没有产出 → DOC_COMPONENT_CRASHED，detail 第一行 exit=
	crash := filepath.Join(dir, "crash")
	os.WriteFile(crash, []byte("#!/bin/sh\necho 'boom at "+dir+"/x' >&2\nexit 3\n"), 0o755)
	_, err = Convert(context.Background(), Job{Exe: crash, Input: in, ConvertTo: "pdf", WorkDir: filepath.Join(dir, "w2"), OutExt: "pdf"})
	ae := apperr.From(err)
	if ae == nil || ae.Code != apperr.DocComponentCrashed || !strings.HasPrefix(ae.Detail, "exit=3\n") || strings.Contains(ae.Detail, dir) {
		t.Fatalf("%v", err)
	}
	// 组件明确报告打不开 → DOC_CORRUPT
	cor := filepath.Join(dir, "corrupt")
	os.WriteFile(cor, []byte("#!/bin/sh\necho 'Error: source file could not be loaded' >&2\nexit 0\n"), 0o755)
	_, err = Convert(context.Background(), Job{Exe: cor, Input: in, ConvertTo: "pdf", WorkDir: filepath.Join(dir, "w3"), OutExt: "pdf"})
	if !apperr.Is(err, apperr.DocCorrupt) {
		t.Fatal(err)
	}
	// 取消：结束进程树，返回 ctx 错误
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_, err = Convert(ctx, Job{Exe: exe, Input: in, ConvertTo: "pdf", WorkDir: filepath.Join(dir, "w4"), OutExt: "pdf"})
	if err != context.DeadlineExceeded {
		t.Fatal(err)
	}
	_ = exec.Command
}

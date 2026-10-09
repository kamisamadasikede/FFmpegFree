package doccomp

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

package doccomp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
)

type events struct {
	mu   sync.Mutex
	list []any
	name []string
}

func (e *events) emit(n string, p any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.name = append(e.name, n)
	e.list = append(e.list, p)
}

func (e *events) progress() []Progress {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []Progress
	for _, p := range e.list {
		if pp, ok := p.(Progress); ok {
			out = append(out, pp)
		}
	}
	return out
}

func payload(n int) ([]byte, string) {
	b := bytes.Repeat([]byte("0123456789abcdef"), n/16)
	h := sha256.Sum256(b)
	return b, hex.EncodeToString(h[:])
}

func fakePrepare(ctx context.Context, pkg, staging, tmp string) error {
	p := filepath.Join(staging, "LibreOffice", "program", exeName())
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte("x"), 0o755)
}

func fakeValidate(ctx context.Context, exe, tmp string) (string, error) { return "26.2.6.3", nil }

func newTestManager(t *testing.T, urls []string, size int64, sum string, ev *events) *Manager {
	t.Helper()
	return New(Config{
		Dir: t.TempDir(), Emit: ev.emit, Logf: t.Logf,
		Package:    &Package{Type: "msi", Size: size, SHA256: sum, URLs: urls},
		Version:    "26.2.6",
		RetryWait:  10 * time.Millisecond,
		Candidates: func() []string { return nil },
		Prepare:    fakePrepare,
		Validate:   fakeValidate,
		DiskFree:   func(string) (int64, error) { return 1 << 50, nil },
	})
}

func waitState(t *testing.T, m *Manager, want ...string) Status {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		s := m.Status()
		for _, w := range want {
			if s.State == w {
				return s
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("等不到 %v，当前 %+v", want, m.Status())
	return Status{}
}

func TestInstallResumeVerifyPrepare(t *testing.T) {
	data, sum := payload(1 << 20)
	var ranges atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "" {
			ranges.Add(1)
		}
		http.ServeContent(w, r, "x.msi", time.Time{}, bytes.NewReader(data))
	}))
	defer srv.Close()
	ev := &events{}
	m := newTestManager(t, []string{srv.URL + "/x.msi"}, int64(len(data)), sum, ev)
	m.Start()
	if s := waitState(t, m, StateMissing); !s.CanDownload || s.DownloadBytes != int64(len(data)) || s.InstallBytes != InstallBytesApprox || s.Error != nil ||
		s.ComponentState != StateMissing || len(s.Engines) != 1 || s.Engines[0].ID != "component" || s.Engines[0].Installed || s.Engines[0].Available || s.Engines[0].Name != "文档组件" {
		t.Fatalf("%+v", s)
	}
	// 先放半个 .part：应当带 Range 续传
	os.MkdirAll(m.tmpDir(), 0o755)
	os.WriteFile(m.partPath(), data[:len(data)/2], 0o644)
	s, err := m.Install("")
	if err != nil || s.State != StateDownloading || s.ReceivedBytes != int64(len(data)/2) {
		t.Fatalf("%+v %v", s, err)
	}
	s2, _ := m.Install("") // 幂等
	if s2.State != StateDownloading && s2.State != StatePreparing && s2.State != StateReady {
		t.Fatalf("%+v", s2)
	}
	s = waitState(t, m, StateReady, StateFailed)
	if s.State != StateReady || s.Source != SourceDownloaded || s.Version != "26.2.6.3" || !strings.HasPrefix(s.Path, m.finalDir()) ||
		s.ComponentState != StateReady || len(s.Engines) != 1 || !s.Engines[0].Installed || !s.Engines[0].Available || s.Engines[0].Source != SourceDownloaded || s.Engines[0].Version != "26.2.6.3" {
		t.Fatalf("%+v", s)
	}
	if ranges.Load() == 0 {
		t.Fatal("没有用 Range 续传")
	}
	if regular(m.partPath()) || regular(m.pkgPath()) {
		t.Fatal("安装包应已删除")
	}
	if _, err := os.Stat(m.stagingDir()); !os.IsNotExist(err) {
		t.Fatal("staging 应已改名")
	}
	pr := ev.progress()
	if len(pr) < 2 || pr[0].Phase != StateDownloading || pr[0].ReceivedBytes != int64(len(data)/2) || pr[len(pr)-1].Phase != StatePreparing || pr[len(pr)-1].Progress != nil {
		t.Fatalf("%+v", pr)
	}
	// 重新检测：应用下载的优先
	m.Recheck()
	if s := waitState(t, m, StateReady); s.Source != SourceDownloaded {
		t.Fatalf("%+v", s)
	}
}

func TestInstallChecksumRestart(t *testing.T) {
	data, sum := payload(256 << 10)
	bad := append([]byte(nil), data...)
	bad[100] ^= 1
	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "x", time.Time{}, bytes.NewReader(bad))
	}))
	defer badSrv.Close()
	goodSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "x", time.Time{}, bytes.NewReader(data))
	}))
	defer goodSrv.Close()

	ev := &events{}
	m := newTestManager(t, []string{badSrv.URL, goodSrv.URL}, int64(len(data)), sum, ev)
	m.Start()
	waitState(t, m, StateMissing)
	if _, err := m.Install(MirrorCN); err != nil { // cn 没有镜像条目：退回默认地址
		t.Fatal(err)
	}
	if s := waitState(t, m, StateReady, StateFailed); s.State != StateReady {
		t.Fatalf("%+v", s)
	}
	reset := false
	for _, p := range ev.progress() {
		if p.Phase == StateDownloading && p.ReceivedBytes == 0 && p.Progress != nil && *p.Progress == 0 {
			reset = true
		}
	}
	if !reset {
		t.Fatal("校验失败重下应发一条 receivedBytes=0")
	}

	// 两次都不通过：DOC_CHECKSUM_FAILED，.part 已删
	ev2 := &events{}
	m2 := newTestManager(t, []string{badSrv.URL, badSrv.URL, goodSrv.URL}, int64(len(data)), sum, ev2)
	m2.Start()
	waitState(t, m2, StateMissing)
	m2.Install("")
	s := waitState(t, m2, StateFailed, StateReady)
	if s.State != StateFailed || s.Error == nil || s.Error.Code != apperr.DocChecksumFailed || s.Error.Message != "下载的文档组件校验失败，请重试。" {
		t.Fatalf("%+v", s)
	}
	if regular(m2.partPath()) {
		t.Fatal(".part 应删除")
	}
}

func TestInstallDownloadFailedKeepsPart(t *testing.T) {
	data, sum := payload(256 << 10)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		// 只给一部分就断开
		w.Header().Set("Content-Length", "262144")
		w.WriteHeader(200)
		w.Write(data[:1000])
		w.(http.Flusher).Flush()
		hj, _ := w.(http.Hijacker)
		c, _, _ := hj.Hijack()
		c.Close()
	}))
	defer srv.Close()
	ev := &events{}
	m := newTestManager(t, []string{srv.URL}, int64(len(data)), sum, ev)
	m.Start()
	waitState(t, m, StateMissing)
	m.Install("")
	s := waitState(t, m, StateFailed)
	if s.Error == nil || s.Error.Code != apperr.DocDownloadFailed || strings.Contains(s.Error.Detail, m.cfg.Dir) {
		t.Fatalf("%+v", s.Error)
	}
	if hits.Load() != 4 {
		t.Fatalf("应重试 3 次，实际请求 %d 次", hits.Load())
	}
	if fi, err := os.Stat(m.partPath()); err != nil || fi.Size() == 0 {
		t.Fatal("失败应保留 .part")
	}
}

func TestInstallCancelKeepsPart(t *testing.T) {
	data, sum := payload(1 << 20)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1048576")
		w.WriteHeader(200)
		w.Write(data[:4096])
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()
	m := newTestManager(t, []string{srv.URL}, int64(len(data)), sum, &events{})
	m.Start()
	waitState(t, m, StateMissing)
	m.Install("")
	deadline := time.Now().Add(5 * time.Second)
	for m.Status().ReceivedBytes < 4096 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	m.Cancel()
	if s := waitState(t, m, StateMissing); s.Error != nil {
		t.Fatalf("%+v", s)
	}
	if fi, err := os.Stat(m.partPath()); err != nil || fi.Size() != 4096 {
		t.Fatalf("取消应保留 .part: %v", err)
	}
}

func TestInstallDiskFullAndMirror(t *testing.T) {
	m := newTestManager(t, []string{"http://127.0.0.1:1/x"}, 300<<20, strings.Repeat("a", 64), &events{})
	m.cfg.DiskFree = func(string) (int64, error) { return 1 << 30, nil }
	_, err := m.Install("")
	ae := apperr.From(err)
	if ae == nil || ae.Code != apperr.ConvertDiskFull || !strings.HasPrefix(ae.Detail, "reason=no_space\nneedBytes=") || !strings.Contains(ae.Detail, "freeBytes=1073741824") {
		t.Fatalf("%v", err)
	}
	if _, err := m.Install("us"); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatal(err)
	}
	p := Package{URLs: []string{"a"}, Mirrors: map[string][]string{"cn": {"c"}}}
	if got := p.urlsFor("cn"); len(got) != 2 || got[0] != "c" {
		t.Fatal(got)
	}
}

func TestLinuxNoPackage(t *testing.T) {
	m := New(Config{Dir: t.TempDir(), Candidates: func() []string { return nil }})
	m.pkg = nil
	m.st = m.base(StateChecking)
	if _, err := m.Install(""); !apperr.Is(err, apperr.UnsupportedPlatform) || apperr.From(err).Message != LinuxHint {
		t.Fatal(err)
	}
	m.Start()
	s := waitState(t, m, StateMissing)
	if s.CanDownload || s.DownloadBytes != 0 || s.InstallBytes != 0 || s.Error == nil || s.Error.Code != apperr.DocComponentNotReady || s.Error.Message != LinuxHint ||
		s.Engines == nil || len(s.Engines) != 0 {
		t.Fatalf("%+v", s)
	}
}

func TestOutdatedSystem(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "soffice")
	os.WriteFile(exe, []byte("x"), 0o755)
	m := New(Config{Dir: t.TempDir(), Candidates: func() []string { return []string{exe} },
		Validate: func(ctx context.Context, e, tmp string) (string, error) {
			return "7.1.4.2", installFailed("check=outdated")
		}})
	m.Start()
	s := waitState(t, m, StateOutdated)
	if s.Version != "7.1.4.2" || s.Error == nil || s.Error.Code != apperr.DocComponentNotReady || s.Source != "" || s.ComponentState != StateOutdated {
		t.Fatalf("%+v", s)
	}
	if m.pkg == nil && s.Error.Message != LinuxOutdatedHint {
		t.Fatalf("Linux 版本太旧的提示: %q", s.Error.Message)
	}
}

// 架构师确认：Win/mac（有下载源）版本太旧时 componentState=outdated、canDownload=true，提交报 NOT_READY + reason=outdated。
func TestOutdatedWithPackage(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "soffice")
	os.WriteFile(exe, []byte("x"), 0o755)
	m := New(Config{Dir: t.TempDir(), Candidates: func() []string { return []string{exe} },
		Package: &Package{Type: "msi", Size: 1000, SHA256: strings.Repeat("a", 64), URLs: []string{"http://127.0.0.1:1/x.msi"}},
		Validate: func(ctx context.Context, e, tmp string) (string, error) {
			return "7.1.4.2", installFailed("check=outdated")
		}})
	m.Start()
	s := waitState(t, m, StateOutdated)
	if s.ComponentState != StateOutdated || !s.CanDownload || s.DownloadBytes != 1000 || s.InstallBytes != InstallBytesApprox ||
		s.Error == nil || s.Error.Message != "需要先下载文档组件。" || !strings.Contains(s.Error.Detail, "reason=outdated") {
		t.Fatalf("%+v", s)
	}
	e := m.NotReadyError()
	if e.Code != apperr.DocComponentNotReady || !strings.HasPrefix(e.Detail, "reason=outdated\n") || strings.Contains(e.Message, "LibreOffice") {
		t.Fatalf("%+v", e)
	}
	if len(s.Engines) != 1 || s.Engines[0].Installed || s.Engines[0].Available {
		t.Fatalf("engines %+v", s.Engines)
	}
}

func TestParseVersionAndFileURL(t *testing.T) {
	for _, c := range []struct {
		in    string
		full  string
		ok    bool
		newer bool
	}{
		{"LibreOffice 25.2.3.2 520(Build:2)", "25.2.3.2", true, true},
		{"LibreOffice 7.2.0.4 10(Build:4)", "7.2.0.4", true, true},
		{"LibreOffice 7.1.8.1 e1f30c80", "7.1.8.1", true, false},
		{"LibreOffice 26.2.6.3 abc", "26.2.6.3", true, true},
		{"garbage", "", false, false},
	} {
		full, ma, mi, ok := ParseVersion(c.in)
		if full != c.full || ok != c.ok || (ok && VersionOK(ma, mi) != c.newer) {
			t.Errorf("%q → %q %d.%d %v", c.in, full, ma, mi, ok)
		}
	}
	if got := FileURL("/tmp/a b/profile"); got != "file:///tmp/a%20b/profile" {
		t.Fatal(got)
	}
	if got := FileURL(`C:/Users/张 三/p`); !strings.HasPrefix(got, "file:///C:/Users/") || strings.Contains(got, " ") {
		t.Fatal(got)
	}
}

var _ = net.Listen

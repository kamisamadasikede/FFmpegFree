package system

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
)

// 读取文件内容决定行为的假运行器："GOOD" 开头合格。
func contentRun(_ context.Context, exe string, args ...string) (string, error) {
	b, err := os.ReadFile(exe)
	if err != nil {
		return "", err
	}
	for _, a := range args {
		if a == "-encoders" {
			return encodersOK, nil
		}
	}
	if bytes.HasPrefix(b, []byte("GOOD")) {
		return strings.TrimSuffix(filepath.Base(exe), ".exe") + " version 6.1 x\n", nil
	}
	return "", errors.New("exec format error")
}

// anyEmitter 记录所有事件。
type anyEmitter struct {
	mu   sync.Mutex
	evts []struct {
		name    string
		payload any
	}
}

func (e *anyEmitter) Emit(name string, payload any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.evts = append(e.evts, struct {
		name    string
		payload any
	}{name, payload})
}

func (e *anyEmitter) names() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []string
	for _, x := range e.evts {
		out = append(out, x.name)
	}
	return out
}

func (e *anyEmitter) statuses() []FFmpegStatus {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []FFmpegStatus
	for _, x := range e.evts {
		if s, ok := x.payload.(FFmpegStatus); ok {
			out = append(out, s)
		}
	}
	return out
}

func (e *anyEmitter) count(name string) int {
	n := 0
	for _, x := range e.names() {
		if x == name {
			n++
		}
	}
	return n
}

type instFixture struct {
	mgr   *Manager
	em    *anyEmitter
	root  string
	gate  chan struct{} // 非 nil 时，服务器在发送前等待
	srv   *httptest.Server
	hits  int
	hitMu sync.Mutex
}

func newInstFixture(t *testing.T, ffmpegBody string) *instFixture {
	t.Helper()
	root := t.TempDir()
	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	for n, b := range map[string]string{"ffmpeg": ffmpegBody, "ffprobe": "GOOD-probe"} {
		w, _ := zw.Create(n)
		w.Write([]byte(b))
	}
	zw.Close()
	data := zbuf.Bytes()
	h := sha256.Sum256(data)

	f := &instFixture{em: &anyEmitter{}, root: root, gate: make(chan struct{})}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hitMu.Lock()
		f.hits++
		f.hitMu.Unlock()
		select {
		case <-f.gate:
		case <-r.Context().Done():
			return
		}
		http.ServeContent(w, r, "a.zip", time.Time{}, bytes.NewReader(data))
	}))
	t.Cleanup(f.srv.Close)
	t.Cleanup(func() { ffmpeg.SetCurrent(nil) })

	manifest := &ffmpeg.Manifest{Platforms: map[string]ffmpeg.Platform{"linux-amd64": {
		Available: true, Version: "6.1",
		Archives: []ffmpeg.Archive{{URL: f.srv.URL, SHA256: hex.EncodeToString(h[:]), Size: int64(len(data)), Type: "zip",
			Extract: []ffmpeg.ExtractItem{{Path: "ffmpeg", Name: "ffmpeg"}, {Path: "ffprobe", Name: "ffprobe"}}}},
	}}}
	binDir := filepath.Join(root, "bin")
	loc := &ffmpeg.Locator{
		Run:      contentRun,
		LookPath: func(string) (string, error) { return "", errors.New("no") },
		ExeDir:   func() (string, error) { return filepath.Join(root, "app"), nil },
		BinDir:   binDir, GOOS: "linux",
	}
	inst := ffmpeg.NewInstaller(manifest, loc, binDir, filepath.Join(root, "tmp"))
	inst.Platform, inst.GOOS, inst.Retries, inst.ProgressInterval = "linux-amd64", "linux", -1, -1
	f.mgr = NewManager()
	f.mgr.Start(context.Background(), Config{Locator: loc, Settings: newMemSettings(), Emitter: f.em, Installer: inst})
	waitFor(t, func() bool { return f.mgr.Status().State == ffmpeg.StateMissing })
	return f
}

func (f *instFixture) release() { close(f.gate) }

func TestInstallHappyPath(t *testing.T) {
	f := newInstFixture(t, "GOOD-ffmpeg")
	task, err := f.mgr.Install(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if task.ID == "" || task.Type != "ffmpeg_install" || task.Status != "running" || task.InputPaths == nil {
		t.Fatalf("%+v", task)
	}
	st := f.mgr.Status()
	if st.State != ffmpeg.StateInstalling || st.TaskID != task.ID {
		t.Fatalf("安装中状态: %+v", st)
	}
	if _, err := ffmpeg.Require(); err == nil {
		t.Fatal("安装期间门控应保持关闭")
	}
	f.release()
	waitFor(t, func() bool { return f.mgr.Status().State == ffmpeg.StateReady })

	st = f.mgr.Status()
	if st.Source != ffmpeg.SourceBundled || st.Error != nil || st.TaskID != "" {
		t.Fatalf("%+v", st)
	}
	if _, err := ffmpeg.Require(); err != nil {
		t.Fatal(err)
	}
	// 状态序列：missing(启动前的 checking→missing) → installing → ready
	var states []string
	for _, s := range f.em.statuses() {
		states = append(states, s.State)
	}
	if got := strings.Join(states, ","); got != "checking,missing,installing,ready" {
		t.Fatalf("事件序列: %s", got)
	}
	if f.em.count(EventTaskCreated) != 1 || f.em.count(EventTaskStatus) != 1 || f.em.count(EventTaskProgress) == 0 {
		t.Fatalf("任务事件: %v", f.em.names())
	}
	// task:status 是 succeeded，且版本号大于 created
	f.em.mu.Lock()
	var last taskStatusPayload
	for _, x := range f.em.evts {
		if p, ok := x.payload.(taskStatusPayload); ok {
			last = p
		}
	}
	f.em.mu.Unlock()
	if last.Status != "succeeded" || last.ID != task.ID || last.Version <= 1 {
		t.Fatalf("%+v", last)
	}
}

func TestInstallIsIdempotent(t *testing.T) {
	f := newInstFixture(t, "GOOD-ffmpeg")
	t1, err := f.mgr.Install(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	ids := make([]string, 8)
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tk, err := f.mgr.Install(context.Background(), "")
			if err != nil {
				t.Error(err)
			}
			ids[i] = tk.ID
		}(i)
	}
	wg.Wait()
	for _, id := range ids {
		if id != t1.ID {
			t.Fatalf("应返回同一任务: %s vs %s", id, t1.ID)
		}
	}
	f.release()
	waitFor(t, func() bool { return f.mgr.Status().State == ffmpeg.StateReady })
	f.hitMu.Lock()
	defer f.hitMu.Unlock()
	if f.hits != 1 {
		t.Fatalf("只应下载一次, 实际请求 %d 次", f.hits)
	}
	if f.em.count(EventTaskCreated) != 1 {
		t.Fatal("只应创建一个任务")
	}
}

func TestInstallRejectsBadMirror(t *testing.T) {
	f := newInstFixture(t, "GOOD-ffmpeg")
	for _, m := range []string{"eu", "CN", "https://mirror.example.com", " "} {
		if _, err := f.mgr.Install(context.Background(), m); !apperr.Is(err, apperr.InvalidArgument) {
			t.Fatalf("mirror %q: %v", m, err)
		}
	}
	if f.mgr.Status().State != ffmpeg.StateMissing {
		t.Fatal("参数错误不应改变状态")
	}
	// linux-amd64 的清单里没有 cn 镜像：明确拒绝，detail 说明可选项，不创建任务
	_, err := f.mgr.Install(context.Background(), "cn")
	var ae *apperr.AppError
	if !errors.As(err, &ae) || ae.Code != apperr.InvalidArgument || !strings.Contains(ae.Detail, "默认源") {
		t.Fatalf("cn 应被拒绝并说明: %v", err)
	}
	if f.mgr.Status().State != ffmpeg.StateMissing {
		t.Fatal("被拒绝的安装不应改变状态")
	}
	// 默认源可以
	if _, err := f.mgr.Install(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	f.release()
	waitFor(t, func() bool { return f.mgr.Status().State == ffmpeg.StateReady })
}

func TestInstallOptions(t *testing.T) {
	f := newInstFixture(t, "GOOD-ffmpeg")
	o, err := f.mgr.GetInstallOptions()
	if err != nil || o.Platform != "linux-amd64" || !o.Supported || o.Mirrors == nil || len(o.Mirrors) != 0 {
		t.Fatalf("%+v %v", o, err)
	}
	// 给当前平台加上 cn 镜像后：出现在可选项里，并且 Install("cn") 被接受
	inst := f.mgr.cfg.Installer
	p := inst.Manifest.Platforms["linux-amd64"]
	for i := range p.Archives {
		p.Archives[i].Mirrors = map[string][]string{"cn": {p.Archives[i].URL}}
	}
	inst.Manifest.Platforms["linux-amd64"] = p
	if o, _ = f.mgr.GetInstallOptions(); len(o.Mirrors) != 1 || o.Mirrors[0] != "cn" {
		t.Fatalf("%+v", o)
	}
	if _, err := f.mgr.Install(context.Background(), "cn"); err != nil {
		t.Fatal(err)
	}
	f.release()
	waitFor(t, func() bool { return f.mgr.Status().State == ffmpeg.StateReady })
	// 不支持的平台
	inst.Platform = "plan9-mips"
	if o, _ = f.mgr.GetInstallOptions(); o.Supported || len(o.Mirrors) != 0 {
		t.Fatalf("%+v", o)
	}
}

func TestInstallUnsupportedPlatform(t *testing.T) {
	f := newInstFixture(t, "GOOD-ffmpeg")
	f.mgr.cfg.Installer.Platform = "plan9-mips"
	if _, err := f.mgr.Install(context.Background(), ""); !apperr.Is(err, apperr.UnsupportedPlatform) {
		t.Fatalf("%v", err)
	}
}

func TestInstallFailureSetsFailedAndKeepsPartial(t *testing.T) {
	f := newInstFixture(t, "BAD-not-runnable") // 下载成功但校验不通过
	if _, err := f.mgr.Install(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	f.release()
	waitFor(t, func() bool { return f.mgr.Status().State == ffmpeg.StateFailed })
	st := f.mgr.Status()
	if st.Error == nil || st.Error.Code != apperr.Internal || st.TaskID == "" {
		t.Fatalf("%+v", st)
	}
	if len(f.mgr.cfg.Installer.PartFiles()) != 1 {
		t.Fatal("失败应保留 .part")
	}
	if f.em.count(EventTaskStatus) != 1 {
		t.Fatal("应发一次 task:status(failed)")
	}
	// failed 之后可以重新安装（此时不再是进行中）
	if _, err := f.mgr.Install(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return f.em.count(EventTaskCreated) == 2 })
	// 重新检测在失败后可用
	f.mgr.CancelInstall()
	waitFor(t, func() bool { s := f.mgr.Status().State; return s == ffmpeg.StateMissing })
}

func TestCancelInstall(t *testing.T) {
	f := newInstFixture(t, "GOOD-ffmpeg")
	if err := f.mgr.CancelInstall(); err != nil {
		t.Fatalf("没有安装时取消应无副作用: %v", err)
	}
	task, err := f.mgr.Install(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	// 服务器还卡在 gate 上，此时取消
	waitFor(t, func() bool { f.hitMu.Lock(); defer f.hitMu.Unlock(); return f.hits == 1 })
	if err := f.mgr.CancelInstall(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return f.mgr.Status().State == ffmpeg.StateMissing })
	f.em.mu.Lock()
	var statusEvt taskStatusPayload
	for _, x := range f.em.evts {
		if p, ok := x.payload.(taskStatusPayload); ok {
			statusEvt = p
		}
	}
	f.em.mu.Unlock()
	if statusEvt.ID != task.ID || statusEvt.Status != "canceled" {
		t.Fatalf("%+v", statusEvt)
	}
	if _, err := ffmpeg.Require(); err == nil {
		t.Fatal("取消后仍未安装")
	}
}

func TestSetPathAndRecheckDuringInstall(t *testing.T) {
	f := newInstFixture(t, "GOOD-ffmpeg")
	if _, err := f.mgr.Install(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := f.mgr.SetPath(context.Background(), f.root); !apperr.Is(err, apperr.TaskConflict) {
		t.Fatalf("安装中手动指定路径应冲突: %v", err)
	}
	st, err := f.mgr.Recheck(context.Background())
	if err != nil || st.State != ffmpeg.StateInstalling {
		t.Fatalf("安装中重检应保持 installing: %+v %v", st, err)
	}
	f.release()
	waitFor(t, func() bool { return f.mgr.Status().State == ffmpeg.StateReady })
}

func TestInstallProgressEventsMonotonic(t *testing.T) {
	f := newInstFixture(t, "GOOD-ffmpeg")
	f.mgr.Install(context.Background(), "")
	f.release()
	waitFor(t, func() bool { return f.mgr.Status().State == ffmpeg.StateReady })
	f.em.mu.Lock()
	defer f.em.mu.Unlock()
	var lastV int64
	var lastP float64
	for _, x := range f.em.evts {
		if p, ok := x.payload.(taskProgressPayload); ok {
			if p.Version <= lastV || p.Progress < lastP {
				t.Fatalf("version / progress 应单调: %+v", p)
			}
			lastV, lastP = p.Version, p.Progress
		}
	}
	if lastV == 0 {
		t.Fatal("没有进度事件")
	}
}

// 契约 9.4：error / taskId 没有值时不出现在 JSON 里，前端拿到的是 optional 字段。
func TestFFmpegStatusJSON(t *testing.T) {
	raw, _ := json.Marshal(FFmpegStatus{State: "ready", Path: "/a/ffmpeg", Version: "6.1", Source: "bundled"})
	var m map[string]any
	json.Unmarshal(raw, &m)
	if _, ok := m["error"]; ok {
		t.Fatalf("error 为空时不应出现: %s", raw)
	}
	if _, ok := m["taskId"]; ok {
		t.Fatalf("taskId 为空时不应出现: %s", raw)
	}
	for _, k := range []string{"state", "path", "version", "source", "ffprobeMissing"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("缺字段 %s: %s", k, raw)
		}
	}
	raw, _ = json.Marshal(FFmpegStatus{State: "failed", TaskID: "T1", Error: apperr.New(apperr.Internal, "x")})
	m = nil
	json.Unmarshal(raw, &m)
	e, _ := m["error"].(map[string]any)
	if m["taskId"] != "T1" || e["code"] != "INTERNAL" {
		t.Fatalf("有值时应输出: %s", raw)
	}
}

func TestInstallBeforeStartAndNoInstaller(t *testing.T) {
	m := NewManager()
	if _, err := m.Install(context.Background(), ""); !apperr.Is(err, apperr.Internal) {
		t.Fatalf("%v", err)
	}
	f := newFixture(t)
	f.start(t)
	waitFor(t, func() bool { return f.mgr.Status().State == ffmpeg.StateMissing })
	if _, err := f.mgr.Install(context.Background(), ""); !apperr.Is(err, apperr.Internal) {
		t.Fatalf("无 Installer 应返回 INTERNAL: %v", err)
	}
}

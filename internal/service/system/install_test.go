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
	"FFmpegFree/internal/store"
	"FFmpegFree/internal/task"
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
	tasks *task.Manager
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
	st, err := store.Open(context.Background(), filepath.Join(root, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	tasks := task.NewManager(task.Config{Store: st, Emitter: f.em, LogDir: filepath.Join(root, "logs"), BatchConcurrency: 1,
		ProgressInterval: -1, Logf: func(string, ...any) {}})
	t.Cleanup(func() { tasks.Shutdown(2 * time.Second) })
	f.tasks = tasks
	// Start 会把设置里的 maxConcurrent 应用到任务管理器（0 = 自动 = NumCPU/2，限制在 1~3），
	// 会覆盖上面 BatchConcurrency: 1。这里把设置也钉成 1，让 batch 池在任何核数的机器上都只有 1 个名额，
	// "占满 batch 池让安装排队"的用例才有确定的前提（否则 4 核以上的机器上安装任务会直接开始运行）。
	settings := newMemSettings()
	if err := settings.SetSetting(context.Background(), SettingMaxConcurrent, 1); err != nil {
		t.Fatal(err)
	}
	f.mgr = NewManager()
	f.mgr.Start(context.Background(), Config{Locator: loc, Settings: settings, Emitter: f.em, Installer: inst, Tasks: tasks})
	waitFor(t, func() bool { return f.mgr.Status().State == ffmpeg.StateMissing })
	if n := tasks.BatchConcurrency(); n != 1 {
		t.Fatalf("测试夹具要求 batch 池并发数为 1, 实际 %d", n)
	}
	return f
}

func (f *instFixture) release() { close(f.gate) }

func TestInstallHappyPath(t *testing.T) {
	f := newInstFixture(t, "GOOD-ffmpeg")
	tk, err := f.mgr.Install(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if tk.ID == "" || tk.Type != task.TypeFFmpegInstall || tk.Status.Active() == false || tk.InputPaths == nil {
		t.Fatalf("%+v", tk)
	}
	st := f.mgr.Status()
	if st.State != ffmpeg.StateInstalling || st.TaskID != tk.ID {
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
	if f.em.count(task.EventCreated) != 1 || f.em.count(task.EventStatus) != 2 || f.em.count(task.EventProgress) == 0 {
		t.Fatalf("任务事件: %v", f.em.names())
	}
	// task:status 是 succeeded，且版本号大于 created
	f.em.mu.Lock()
	var last task.StatusEvent
	for _, x := range f.em.evts {
		if p, ok := x.payload.(task.StatusEvent); ok {
			last = p
		}
	}
	f.em.mu.Unlock()
	if last.Status != "succeeded" || last.ID != tk.ID || last.Version <= 1 {
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
	if f.em.count(task.EventCreated) != 1 {
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
	if f.em.count(task.EventStatus) != 2 {
		t.Fatal("应发 task:status(running) 和 task:status(failed)")
	}
	// failed 之后可以重新安装（此时不再是进行中）
	second, err := f.mgr.Install(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == st.TaskID {
		t.Fatal("失败后重新安装应生成新任务")
	}
	waitFor(t, func() bool { return f.em.count(task.EventCreated) == 2 })
	// 服务器的 gate 已经放开，第二次安装会很快再次失败；不能在它进行中取消（取消与失败谁先到不确定，
	// 曾导致偶发超时），而是等它以第二个任务的身份失败，再验证失败之后重新检测可用。
	waitFor(t, func() bool {
		s := f.mgr.Status()
		return s.State == ffmpeg.StateFailed && s.TaskID == second.ID
	})
	f.mgr.CancelInstall() // 已经结束：应当无副作用
	if _, err := f.mgr.Recheck(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return f.mgr.Status().State == ffmpeg.StateMissing })
}

func TestCancelInstall(t *testing.T) {
	f := newInstFixture(t, "GOOD-ffmpeg")
	if err := f.mgr.CancelInstall(); err != nil {
		t.Fatalf("没有安装时取消应无副作用: %v", err)
	}
	tk, err := f.mgr.Install(context.Background(), "")
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
	var statusEvt task.StatusEvent
	for _, x := range f.em.evts {
		if p, ok := x.payload.(task.StatusEvent); ok {
			statusEvt = p
		}
	}
	f.em.mu.Unlock()
	if statusEvt.ID != tk.ID || statusEvt.Status != "canceled" {
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
		if p, ok := x.payload.(task.ProgressEvent); ok {
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

func TestInstallTaskPersistedAndRetryable(t *testing.T) {
	f := newInstFixture(t, "BAD-not-runnable")
	// 给当前平台配上 cn 镜像（指向同一个地址），验证重试保留镜像参数
	inst := f.mgr.cfg.Installer
	pl := inst.Manifest.Platforms["linux-amd64"]
	for i := range pl.Archives {
		pl.Archives[i].Mirrors = map[string][]string{"cn": {pl.Archives[i].URL}}
	}
	inst.Manifest.Platforms["linux-amd64"] = pl
	tk, err := f.mgr.Install(context.Background(), "cn")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tk.Params, `"mirror":"cn"`) {
		t.Fatalf("Params 应记录 mirror 以便重试: %s", tk.Params)
	}
	f.release()
	waitFor(t, func() bool { return f.mgr.Status().State == ffmpeg.StateFailed })
	// 任务落库为 failed，错误与状态一致
	got, err := f.mgr.cfg.Tasks.Get(tk.ID)
	if err != nil || got.Status != task.StatusFailed || got.Error == nil || got.Type != task.TypeFFmpegInstall {
		t.Fatalf("%+v %v", got, err)
	}
	if st := f.mgr.Status(); st.Error == nil || st.Error.Code != got.Error.Code || st.TaskID != tk.ID {
		t.Fatalf("ffmpeg 状态应带同一个错误: %+v", st)
	}
	// 通过任务管理器 Retry（TaskService.Retry 走的就是这条路）会重新进入 installing
	nt, err := f.mgr.cfg.Tasks.Retry(tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { s := f.mgr.Status(); return s.State == ffmpeg.StateFailed && s.TaskID == nt.ID })
	if nt.ID == tk.ID {
		t.Fatal("Retry 应生成新任务")
	}
	// 安装日志可读
	if log, _ := f.mgr.cfg.Tasks.GetLog(nt.ID, 20); !strings.Contains(log, "开始安装") {
		t.Fatalf("日志: %q", log)
	}
}

func TestInstallCancelWhileQueued(t *testing.T) {
	// batch 池被占满时，安装任务排队；取消排队中的安装也应回到 missing。
	f := newInstFixture(t, "GOOD-ffmpeg")
	block := make(chan struct{})
	f.mgr.cfg.Tasks.Submit(task.Spec{Type: task.TypeConvert}, task.RunnerFunc(func(ctx context.Context, _ func(task.Progress)) (string, error) {
		<-block
		return "", nil
	}))
	tk, err := f.mgr.Install(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if tk.Status != task.StatusQueued || f.mgr.Status().State != ffmpeg.StateInstalling {
		t.Fatalf("%+v %+v", tk, f.mgr.Status())
	}
	// Submit 返回的是入队前的快照，恒为 queued，不能证明任务此刻真的在排队；
	// 以任务管理器里的实时状态为准：占位任务在运行，安装任务确实还在排队、没有开始。
	if cur, err := f.tasks.Get(tk.ID); err != nil || cur.Status != task.StatusQueued || cur.StartedAt != 0 {
		t.Fatalf("取消前安装任务应仍在排队: %+v %v", cur, err)
	}
	if err := f.mgr.CancelInstall(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return f.mgr.Status().State == ffmpeg.StateMissing })
	close(block)
	// 终态是 canceled，且从未进入 running（没有 Run，也就不会发起下载）。
	got, err := f.tasks.Wait(context.Background(), tk.ID)
	if err != nil || got.Status != task.StatusCanceled || got.StartedAt != 0 {
		t.Fatalf("排队中取消应直接 canceled 且从未开始: %+v %v", got, err)
	}
	f.hitMu.Lock()
	defer f.hitMu.Unlock()
	if f.hits != 0 {
		t.Fatal("排队中取消不应发起下载")
	}
}

func TestRetryClaimsInstallingWhileQueued(t *testing.T) {
	f := newInstFixture(t, "BAD-not-runnable")
	tk, err := f.mgr.Install(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	f.release()
	waitFor(t, func() bool { return f.mgr.Status().State == ffmpeg.StateFailed })

	// 占满 batch 池，让重试的安装任务排队
	block := make(chan struct{})
	f.mgr.cfg.Tasks.Submit(task.Spec{Type: task.TypeConvert}, task.RunnerFunc(func(ctx context.Context, _ func(task.Progress)) (string, error) {
		<-block
		return "", nil
	}))
	nt, err := f.mgr.cfg.Tasks.Retry(tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if nt.Status != task.StatusQueued {
		t.Fatalf("重试的任务应在排队: %+v", nt)
	}
	// 排队期间：已经处于 installing，第二个 Install 返回同一个任务，不会再提交
	if st := f.mgr.Status(); st.State != ffmpeg.StateInstalling || st.TaskID != nt.ID {
		t.Fatalf("Retry 应立刻占住 installing: %+v", st)
	}
	again, err := f.mgr.Install(context.Background(), "")
	if err != nil || again.ID != nt.ID {
		t.Fatalf("排队期间再次 Install 应返回同一个任务: %+v %v", again, err)
	}
	// 再 Retry 一次原任务：TASK_CONFLICT
	if _, err := f.mgr.cfg.Tasks.Retry(tk.ID); !apperr.Is(err, apperr.TaskConflict) {
		t.Fatalf("已有安装时 Retry 应 TASK_CONFLICT: %v", err)
	}
	if p, _ := f.mgr.cfg.Tasks.List(task.Filter{Types: []task.Type{task.TypeFFmpegInstall}}); p.Total != 2 {
		t.Fatalf("不应产生第三个安装任务: %d", p.Total)
	}
	// 取消排队中的重试 → 释放占位，可以重新安装
	if err := f.mgr.CancelInstall(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return f.mgr.Status().State != ffmpeg.StateInstalling })
	close(block)
	if _, err := f.mgr.Install(context.Background(), ""); err != nil {
		t.Fatalf("释放后应能再次安装: %v", err)
	}
}

func TestInstallConcurrentCallsShareOneTask(t *testing.T) {
	f := newInstFixture(t, "GOOD-ffmpeg")
	var wg sync.WaitGroup
	ids := make([]string, 12)
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tk, err := f.mgr.Install(context.Background(), "")
			if err != nil {
				t.Error(err)
				return
			}
			ids[i] = tk.ID
		}(i)
	}
	wg.Wait()
	for _, id := range ids {
		if id != ids[0] || id == "" {
			t.Fatalf("并发 Install 应返回同一个任务: %v", ids)
		}
	}
	f.release()
	waitFor(t, func() bool { return f.mgr.Status().State == ffmpeg.StateReady })
	if p, _ := f.mgr.cfg.Tasks.List(task.Filter{Types: []task.Type{task.TypeFFmpegInstall}}); p.Total != 1 {
		t.Fatalf("只应有一个安装任务: %d", p.Total)
	}
}

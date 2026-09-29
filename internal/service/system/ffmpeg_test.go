package system

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
)

const encodersOK = " V....D libx264  x\n A....D aac  x\n"

type memSettings struct {
	mu sync.Mutex
	m  map[string]string
}

func newMemSettings() *memSettings { return &memSettings{m: map[string]string{}} }

func (s *memSettings) GetSetting(_ context.Context, key string, dst any) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, ok := s.m[key]
	if !ok {
		return false, nil
	}
	return true, json.Unmarshal([]byte(raw), dst)
}

func (s *memSettings) SetSetting(_ context.Context, key string, v any) error {
	b, _ := json.Marshal(v)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = string(b)
	return nil
}

type recEmitter struct {
	mu     sync.Mutex
	events []FFmpegStatus
}

func (e *recEmitter) Emit(name string, payload any) {
	if name != EventFFmpegStatus {
		panic("意外的事件名 " + name)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, payload.(FFmpegStatus))
}

func (e *recEmitter) states() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []string
	for _, s := range e.events {
		out = append(out, s.State)
	}
	return out
}

// fakeRun：路径含 "good" → 6.1 合格；含 "old" → 4.4；其余无法执行。
func fakeRun(_ context.Context, exe string, args ...string) (string, error) {
	base := filepath.Base(exe)
	switch {
	case containsPath(exe, "good"):
	case containsPath(exe, "old"):
		return "ffmpeg version 4.4.2 x\n", nil
	default:
		return "", errors.New("无法执行")
	}
	for _, a := range args {
		if a == "-encoders" {
			return encodersOK, nil
		}
	}
	if base == "ffprobe" {
		return "ffprobe version 6.1 x\n", nil
	}
	return "ffmpeg version 6.1 x\n", nil
}

func containsPath(p, seg string) bool {
	return strings.Contains(filepath.ToSlash(p), "/"+seg+"/")
}

type fixture struct {
	root string
	mgr  *Manager
	em   *recEmitter
	set  *memSettings
	loc  *ffmpeg.Locator
	path map[string]string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	f := &fixture{root: root, mgr: NewManager(), em: &recEmitter{}, set: newMemSettings(), path: map[string]string{}}
	f.loc = &ffmpeg.Locator{
		Run:      fakeRun,
		LookPath: func(n string) (string, error) { return "", errors.New("no") },
		ExeDir:   func() (string, error) { return filepath.Join(root, "app"), nil },
		BinDir:   filepath.Join(root, "data", "bin"),
		GOOS:     "linux",
	}
	t.Cleanup(func() { ffmpeg.SetCurrent(nil) })
	return f
}

func (f *fixture) start(t *testing.T) {
	t.Helper()
	f.mgr.Start(context.Background(), Config{Locator: f.loc, Settings: f.set, Emitter: f.em})
}

func (f *fixture) install(t *testing.T, dir string) {
	t.Helper()
	for _, n := range []string{"ffmpeg", "ffprobe"} {
		p := filepath.Join(dir, n)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("等待超时")
}

func TestStartDetectsInBackgroundAndEmits(t *testing.T) {
	f := newFixture(t)
	// 假运行器按路径里的 "good" 段判定合格
	f.loc.BinDir = filepath.Join(f.root, "good", "bin")
	f.install(t, f.loc.BinDir)

	if st := f.mgr.Status(); st.State != ffmpeg.StateChecking {
		t.Fatalf("初始应为 checking: %+v", st)
	}
	f.start(t)
	waitFor(t, func() bool { return f.mgr.Status().State == ffmpeg.StateReady })

	st := f.mgr.Status()
	if st.Source != ffmpeg.SourceBundled || st.Version != "6.1" || st.Path == "" || st.Error != nil {
		t.Fatalf("%+v", st)
	}
	got := f.em.states()
	if len(got) != 2 || got[0] != "checking" || got[1] != "ready" {
		t.Fatalf("事件序列应为 checking→ready: %v", got)
	}
	// 门控已打开
	if b, err := ffmpeg.Require(); err != nil || b.FFmpeg != st.Path {
		t.Fatalf("Require: %+v %v", b, err)
	}
	// payload 是完整状态，JSON 字段名符合契约
	raw, _ := json.Marshal(st)
	var m map[string]any
	json.Unmarshal(raw, &m)
	for _, k := range []string{"state", "path", "version", "source", "taskId", "error"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("JSON 缺字段 %s: %s", k, raw)
		}
	}
}

func TestMissingStatusAndGate(t *testing.T) {
	f := newFixture(t)
	f.start(t)
	waitFor(t, func() bool { return f.mgr.Status().State == ffmpeg.StateMissing })
	st := f.mgr.Status()
	if st.Error == nil || st.Error.Code != apperr.FFmpegNotFound {
		t.Fatalf("missing 应带 FFMPEG_NOT_FOUND: %+v", st)
	}
	if _, err := ffmpeg.Require(); !apperr.Is(err, apperr.FFmpegNotFound) {
		t.Fatalf("门控应拒绝: %v", err)
	}
}

func TestOutdatedStatus(t *testing.T) {
	f := newFixture(t)
	f.loc.BinDir = filepath.Join(f.root, "old", "bin")
	f.install(t, f.loc.BinDir)
	f.start(t)
	waitFor(t, func() bool { return f.mgr.Status().State == ffmpeg.StateOutdated })
	st := f.mgr.Status()
	if st.Version != "4.4.2" || st.Path == "" || st.Error == nil {
		t.Fatalf("%+v", st)
	}
	if _, err := ffmpeg.Require(); err == nil {
		t.Fatal("outdated 不应放行")
	}
}

func TestSetPath(t *testing.T) {
	f := newFixture(t)
	f.start(t)
	waitFor(t, func() bool { return f.mgr.Status().State == ffmpeg.StateMissing })
	ctx := context.Background()

	// 校验失败：INVALID_ARGUMENT，不改状态、不写设置
	before := len(f.em.states())
	_, err := f.mgr.SetPath(ctx, filepath.Join(f.root, "nothing"))
	if !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("期望 INVALID_ARGUMENT: %v", err)
	}
	if f.mgr.Status().State != ffmpeg.StateMissing || len(f.em.states()) != before {
		t.Fatal("失败不应改状态或发事件")
	}
	if _, ok := f.set.m[SettingFFmpegPath]; ok {
		t.Fatal("失败不应写设置")
	}
	// 过低版本同样是 INVALID_ARGUMENT
	oldDir := filepath.Join(f.root, "old", "bin")
	f.install(t, oldDir)
	if _, err := f.mgr.SetPath(ctx, oldDir); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("旧版本应被拒绝: %v", err)
	}

	// 成功
	goodDir := filepath.Join(f.root, "good", "bin")
	f.install(t, goodDir)
	st, err := f.mgr.SetPath(ctx, goodDir)
	if err != nil || st.State != ffmpeg.StateReady || st.Source != ffmpeg.SourceCustom {
		t.Fatalf("%+v %v", st, err)
	}
	if f.mgr.Status() != st {
		t.Fatal("Status 应与返回值一致")
	}
	if got := f.em.states(); got[len(got)-1] != "ready" {
		t.Fatalf("成功应推送 ready: %v", got)
	}
	var saved string
	f.set.GetSetting(ctx, SettingFFmpegPath, &saved)
	if saved != goodDir {
		t.Fatalf("应持久化路径, got %q", saved)
	}
	if _, err := ffmpeg.Require(); err != nil {
		t.Fatal(err)
	}

	// 重新检测沿用已保存的自定义路径
	st, err = f.mgr.Recheck(ctx)
	if err != nil || st.Source != ffmpeg.SourceCustom {
		t.Fatalf("Recheck 应仍用自定义路径: %+v %v", st, err)
	}

	// 清除自定义路径 → 回到自动检测（这里没有其他来源 → missing）
	st, err = f.mgr.SetPath(ctx, "")
	if err != nil || st.State != ffmpeg.StateMissing {
		t.Fatalf("%+v %v", st, err)
	}
}

func TestRecheckEmitsCheckingThenResult(t *testing.T) {
	f := newFixture(t)
	f.start(t)
	waitFor(t, func() bool { return f.mgr.Status().State == ffmpeg.StateMissing })
	f.loc.BinDir = filepath.Join(f.root, "good", "bin")
	f.install(t, f.loc.BinDir)
	n := len(f.em.states())
	st, err := f.mgr.Recheck(context.Background())
	if err != nil || st.State != ffmpeg.StateReady {
		t.Fatalf("%+v %v", st, err)
	}
	got := f.em.states()[n:]
	if len(got) != 2 || got[0] != "checking" || got[1] != "ready" {
		t.Fatalf("Recheck 事件: %v", got)
	}
}

func TestPromptDismissedResetsWhenReady(t *testing.T) {
	f := newFixture(t)
	f.start(t)
	waitFor(t, func() bool { return f.mgr.Status().State == ffmpeg.StateMissing })
	ctx := context.Background()

	s, _ := f.mgr.GetSettings(ctx)
	if s.FFmpegPromptDismissed {
		t.Fatal("默认应为 false")
	}
	if err := f.mgr.UpdateSettings(ctx, Settings{FFmpegPromptDismissed: true}); err != nil {
		t.Fatal(err)
	}
	if s, _ = f.mgr.GetSettings(ctx); !s.FFmpegPromptDismissed {
		t.Fatal("应保存为 true")
	}
	// 仍 missing 时重新检测不重置
	f.mgr.Recheck(ctx)
	if !f.mgr.PromptDismissed(ctx) {
		t.Fatal("missing 时不应重置")
	}
	// 变为 ready 后重置
	f.loc.BinDir = filepath.Join(f.root, "good", "bin")
	f.install(t, f.loc.BinDir)
	f.mgr.Recheck(ctx)
	if f.mgr.PromptDismissed(ctx) {
		t.Fatal("ready 后应重置为 false")
	}
}

func TestUpdateSettingsInvalidPathIsAtomic(t *testing.T) {
	f := newFixture(t)
	f.start(t)
	waitFor(t, func() bool { return f.mgr.Status().State == ffmpeg.StateMissing })
	ctx := context.Background()
	err := f.mgr.UpdateSettings(ctx, Settings{FFmpegPath: filepath.Join(f.root, "x"), FFmpegPromptDismissed: true})
	if !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("%v", err)
	}
	if f.mgr.PromptDismissed(ctx) {
		t.Fatal("路径校验失败时整个更新不应生效")
	}
}

func TestNilSettingsFallsBackToMemory(t *testing.T) {
	f := newFixture(t)
	f.mgr.Start(context.Background(), Config{Locator: f.loc})
	waitFor(t, func() bool { return f.mgr.Status().State == ffmpeg.StateMissing })
	ctx := context.Background()
	goodDir := filepath.Join(f.root, "good", "bin")
	f.install(t, goodDir)
	if _, err := f.mgr.SetPath(ctx, goodDir); err != nil {
		t.Fatal(err)
	}
	if st, _ := f.mgr.Recheck(ctx); st.Source != ffmpeg.SourceCustom {
		t.Fatalf("内存兜底应生效: %+v", st)
	}
}

func TestBeforeStart(t *testing.T) {
	m := NewManager()
	if _, err := m.Recheck(context.Background()); !apperr.Is(err, apperr.Internal) {
		t.Fatalf("未 Start 应返回 INTERNAL: %v", err)
	}
	if m.Status().State != ffmpeg.StateChecking {
		t.Fatal("未 Start 状态应为 checking")
	}
}

func TestLegacyWithoutFFprobeStatus(t *testing.T) {
	f := newFixture(t)
	// 路径含 "/good/"，让假运行器认为 ffmpeg 合格；只创建 ffmpeg，不创建 ffprobe
	dir := filepath.Join(f.root, "good", "app")
	f.loc.ExeDir = func() (string, error) { return dir, nil }
	if err := os.MkdirAll(filepath.Join(dir, "ffmpeg"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "ffmpeg", "ffmpeg"), []byte("x"), 0o755)
	f.start(t)
	waitFor(t, func() bool { return f.mgr.Status().State == ffmpeg.StateReady })
	st := f.mgr.Status()
	if st.Source != ffmpeg.SourceLegacy || !st.FFprobeMissing || st.Error != nil {
		t.Fatalf("%+v", st)
	}
	if _, err := ffmpeg.Require(); err != nil {
		t.Fatalf("只需 ffmpeg 的功能应放行: %v", err)
	}
	if _, err := ffmpeg.RequireProbe(); !apperr.Is(err, apperr.FFmpegNotFound) {
		t.Fatalf("需要 ffprobe 的功能应被拦: %v", err)
	}
	raw, _ := json.Marshal(st)
	var m map[string]any
	json.Unmarshal(raw, &m)
	if m["ffprobeMissing"] != true {
		t.Fatalf("JSON 应带 ffprobeMissing: %s", raw)
	}
}

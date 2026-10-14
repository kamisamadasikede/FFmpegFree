package cat

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/store"
)

// fakeDetAdapter 模拟真适配器：检测时经 OnChange 通知，带 CheckedAt。
type fakeDetAdapter struct {
	kind     string
	onChange func(string)
	mu       sync.Mutex
	st       Status
	at       int64
	exe      string
	ready    bool
	rechecks atomic.Int32
	runs     atomic.Int32
}

func newFake(kind string, ready bool) *fakeDetAdapter {
	return &fakeDetAdapter{kind: kind, ready: ready, exe: "/bin/" + kind, st: Status{State: catagent.StateChecking}}
}

func (f *fakeDetAdapter) set(st Status, done bool) {
	f.mu.Lock()
	f.st = st
	if done {
		f.at = time.Now().UnixMilli() + int64(f.rechecks.Load())
	}
	f.mu.Unlock()
	if f.onChange != nil {
		f.onChange(f.kind)
	}
}
func (f *fakeDetAdapter) detect() {
	f.set(Status{State: catagent.StateChecking}, false)
	if f.ready {
		f.set(Status{State: catagent.StateReady, Version: "1.0"}, true)
	} else {
		f.set(Status{State: catagent.StateMissing}, true)
	}
}
func (f *fakeDetAdapter) Kind() string { return f.kind }
func (f *fakeDetAdapter) ExecutablePath() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.st.State != catagent.StateReady {
		return ""
	}
	return f.exe
}
func (f *fakeDetAdapter) ProtocolVersion() int { return catagent.ProtocolVersion }
func (f *fakeDetAdapter) Status() Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.st
}
func (f *fakeDetAdapter) CheckedAt() int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.at
}
func (f *fakeDetAdapter) Start()          { go f.detect() }
func (f *fakeDetAdapter) Recheck() Status { f.rechecks.Add(1); f.detect(); return f.Status() }
func (f *fakeDetAdapter) ListModels() ([]Model, error) {
	return []Model{}, nil
}
func (f *fakeDetAdapter) ListThinkLevels() ([]ThinkLevel, error) { return []ThinkLevel{}, nil }
func (f *fakeDetAdapter) RunTurn(catagent.TurnOptions) (catagent.TurnResponse, error) {
	f.runs.Add(1)
	return catagent.TurnResponse{}, errors.New("not used")
}

type statusRec struct {
	mu  sync.Mutex
	all []Status
}

func (r *statusRec) emit(ev string, p any) {
	if ev != catagent.EventStatus {
		return
	}
	r.mu.Lock()
	r.all = append(r.all, p.(Status))
	r.mu.Unlock()
}
func (r *statusRec) snap() []Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Status(nil), r.all...)
}

// 三家 + 一个额外注册的适配器。
func detSetup(t *testing.T, st *store.Store, debounce time.Duration) (*Service, *statusRec, []*fakeDetAdapter) {
	t.Helper()
	reg := catagent.NewRegistry()
	fakes := []*fakeDetAdapter{
		newFake(catagent.KindCatBuild, true),
		newFake(catagent.KindClaudeCLI, true),
		newFake(catagent.KindCodexCLI, false),
		newFake("extra_cli", true),
	}
	rec := &statusRec{}
	svc := New(Config{Registry: reg, Store: st, Emit: rec.emit, StatusDebounce: debounce})
	for _, f := range fakes {
		f.onChange = svc.AgentChanged
		reg.Register(f)
	}
	return svc, rec, fakes
}

func openDetStore(t *testing.T, path string) *store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

var wantOrder = []string{catagent.KindCatBuild, catagent.KindClaudeCLI, catagent.KindCodexCLI, "extra_cli"}

func assertFull(t *testing.T, all []Status) {
	t.Helper()
	if len(all) == 0 {
		t.Fatal("没有 cat:status")
	}
	for i, s := range all {
		if len(s.Agents) != len(wantOrder) {
			t.Fatalf("第 %d 条 agents 数 %d: %+v", i, len(s.Agents), s.Agents)
		}
		for j, k := range wantOrder {
			if s.Agents[j].AgentKind != k {
				t.Fatalf("第 %d 条第 %d 行 %s，应为 %s", i, j, s.Agents[j].AgentKind, k)
			}
		}
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("超时")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func row(s Status, kind string) catagent.AgentStatus {
	for _, a := range s.Agents {
		if a.AgentKind == kind {
			return a
		}
	}
	return catagent.AgentStatus{}
}

func TestComposeStatusEveryEmitHasAllRows(t *testing.T) {
	for _, d := range []time.Duration{-1, 0} { // 同步 / 去抖
		svc, rec, fakes := detSetup(t, nil, d)
		svc.StartDetection()
		waitFor(t, func() bool {
			last := rec.snap()
			if len(last) == 0 {
				return false
			}
			l := last[len(last)-1]
			for _, a := range l.Agents {
				if a.Checking {
					return false
				}
			}
			return true
		})
		// 单家变化
		fakes[2].set(Status{State: catagent.StateFailed}, true)
		waitFor(t, func() bool { l := rec.snap(); return row(l[len(l)-1], catagent.KindCodexCLI).CheckedAt != nil })
		st, err := svc.RecheckCat(context.Background(), "")
		if err != nil {
			t.Fatal(err)
		}
		assertFull(t, append(rec.snap(), st))
		last := rec.snap()[len(rec.snap())-1]
		if r := row(last, catagent.KindClaudeCLI); !r.Ready || r.ExePath != "/bin/claude_cli" || r.CheckedAt == nil || r.Stale || r.Checking {
			t.Fatalf("claude 行: %+v", r)
		}
		if r := row(last, catagent.KindCodexCLI); r.Ready || r.ExePath != "" {
			t.Fatalf("codex 行: %+v", r)
		}
		if last.State != catagent.StateReady { // 顶层仍以 cat_build 为主
			t.Fatalf("顶层: %+v", last.State)
		}
	}
}

func TestConcurrentOnChangeFinalReflectsLatest(t *testing.T) {
	svc, rec, fakes := detSetup(t, nil, 20*time.Millisecond)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		for _, f := range fakes {
			wg.Add(1)
			go func(f *fakeDetAdapter) { defer wg.Done(); f.detect() }(f)
		}
	}
	wg.Wait()
	fakes[1].set(Status{State: catagent.StateFailed, Version: "9"}, true) // 最后一次变化
	time.Sleep(80 * time.Millisecond)
	_ = svc
	all := rec.snap()
	assertFull(t, all)
	if r := row(all[len(all)-1], catagent.KindClaudeCLI); r.Ready || r.Version != "9" {
		t.Fatalf("最后一条未反映最新: %+v", r)
	}
	if len(all) > 60 {
		t.Fatalf("未去抖：%d 条", len(all))
	}
}

func TestRecheckSingleKind(t *testing.T) {
	svc, rec, fakes := detSetup(t, nil, -1)
	for _, f := range fakes {
		f.detect()
	}
	before := len(rec.snap())
	st, err := svc.RecheckCat(context.Background(), catagent.KindClaudeCLI)
	if err != nil {
		t.Fatal(err)
	}
	if fakes[1].rechecks.Load() != 1 || fakes[0].rechecks.Load() != 0 || fakes[2].rechecks.Load() != 0 {
		t.Fatal("只应重检 claude")
	}
	all := rec.snap()[before:]
	assertFull(t, append(all, st))
	for _, s := range all {
		for _, a := range s.Agents {
			if a.Checking && a.AgentKind != catagent.KindClaudeCLI {
				t.Fatalf("%s 不该进入检测中", a.AgentKind)
			}
		}
	}
	sawChecking := false
	for _, s := range all {
		sawChecking = sawChecking || row(s, catagent.KindClaudeCLI).Checking
	}
	if !sawChecking {
		t.Fatal("claude 行应出现 checking=true")
	}
	_, err = svc.RecheckCat(context.Background(), "nope_cli")
	var ae *apperr.AppError
	if !errors.As(err, &ae) || ae.Code != apperr.InvalidArgument {
		t.Fatalf("未知 agentKind: %v", err)
	}
}

func TestDetectPersistRoundTripAndStale(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")
	st1 := openDetStore(t, path)
	svc1, _, fakes1 := detSetup(t, st1, -1)
	for _, f := range fakes1 {
		f.detect()
	}
	var saved map[string]DetectRecord
	if ok, err := st1.GetSetting(context.Background(), DetectKey, &saved); !ok || err != nil {
		t.Fatalf("未落库: %v %v", ok, err)
	}
	if r := saved[catagent.KindClaudeCLI]; !r.Ready || r.ExePath != "/bin/claude_cli" || r.CheckedAt <= 0 || r.Version != "1.0" {
		t.Fatalf("claude 记录: %+v", r)
	}
	if r := saved[catagent.KindCodexCLI]; r.Ready || r.ExePath != "" {
		t.Fatalf("codex 记录: %+v", r)
	}
	_ = svc1
	// 加一项损坏 + 一项未知 kind
	var raw map[string]json.RawMessage
	_, _ = st1.GetSetting(context.Background(), DetectKey, &raw)
	raw["ghost_cli"] = json.RawMessage(`{"ready":true,"checkedAt":5}`)
	raw[catagent.KindCodexCLI] = json.RawMessage(`"broken"`)
	if err := st1.SetSetting(context.Background(), DetectKey, raw); err != nil {
		t.Fatal(err)
	}

	// 「重启」：新服务读落库，适配器还没检测完 → stale 行
	svc2, rec2, fakes2 := detSetup(t, st1, -1)
	fakes2[1].ready = false // 这次实时结果：claude 已卸载
	s0 := svc2.emitStatus()
	assertFull(t, []Status{s0})
	if r := row(s0, catagent.KindClaudeCLI); !r.Stale || !r.Checking || !r.Ready || r.CheckedAt == nil || *r.CheckedAt != saved[catagent.KindClaudeCLI].CheckedAt {
		t.Fatalf("stale 行: %+v", r)
	}
	if r := row(s0, catagent.KindCodexCLI); r.Stale || r.CheckedAt != nil {
		t.Fatalf("损坏项应忽略: %+v", r)
	}
	// stale 下发对话：只认实时（适配器仍在检测）→ 不就绪，不启动进程
	conv, err := svc2.CreateCatConversation(context.Background(), CreateConversationRequest{AgentKind: catagent.KindClaudeCLI})
	if err != nil {
		t.Fatal(err)
	}
	_, serr := svc2.SendCatMessage(context.Background(), SendMessageRequest{ConversationID: conv.ID, Content: "hi"})
	if serr == nil || fakes2[1].runs.Load() != 0 {
		t.Fatalf("stale 状态不得用落库结果发对话: %v runs=%d", serr, fakes2[1].runs.Load())
	}
	// 实时检测完 → 替换
	fakes2[1].detect()
	all := rec2.snap()
	if r := row(all[len(all)-1], catagent.KindClaudeCLI); r.Stale || r.Checking || r.Ready {
		t.Fatalf("实时应替换上次结果: %+v", r)
	}
	var again map[string]DetectRecord
	_, _ = st1.GetSetting(context.Background(), DetectKey, &again)
	if again[catagent.KindClaudeCLI].Ready {
		t.Fatal("实时结果应覆盖写回")
	}
	if _, ok := again["ghost_cli"]; ok {
		t.Fatal("未知 kind 不应写回")
	}
}

func TestDetectCorruptJSONIgnored(t *testing.T) {
	st := openDetStore(t, filepath.Join(t.TempDir(), "app.db"))
	if err := st.SetSetting(context.Background(), DetectKey, "not-a-map"); err != nil {
		t.Fatal(err)
	}
	svc, _, _ := detSetup(t, st, -1)
	s := svc.emitStatus()
	assertFull(t, []Status{s})
	for _, a := range s.Agents {
		if a.Stale {
			t.Fatalf("损坏 JSON 应忽略: %+v", a)
		}
	}
}

func TestCheckingRowKeepsPreviousResult(t *testing.T) {
	svc, rec, fakes := detSetup(t, nil, -1)
	fakes[1].detect() // claude 检测过一次：ready
	// 再次检测中：只发 checking
	fakes[1].set(Status{State: catagent.StateChecking}, false)
	fakes[3].set(Status{State: catagent.StateChecking}, false) // extra 从未检测过
	all := rec.snap()
	last := all[len(all)-1]
	r := row(last, catagent.KindClaudeCLI)
	if !r.Checking || !r.Ready || r.Version != "1.0" || r.ExePath != "/bin/claude_cli" || r.CheckedAt == nil || r.Stale {
		t.Fatalf("检测中应保留上次结果: %+v", r)
	}
	e := row(last, "extra_cli")
	if !e.Checking || e.Ready || e.Version != "" || e.ExePath != "" || e.CheckedAt != nil {
		t.Fatalf("从未检测过应为空: %+v", e)
	}
	_ = svc
}

package cat

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"FFmpegFree/internal/catagent"
)

// loginFake 在 fakeDetAdapter 上加 LiveLoggedIn（v0.47.1 loggedIn）。
type loginFake struct {
	*fakeDetAdapter
	logged *bool
}

func (f *loginFake) LiveLoggedIn() *bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.logged
}

func bptr(b bool) *bool { return &b }

func TestLoggedInOnlyFromLiveDetection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")
	st := openDetStore(t, path)
	reg := catagent.NewRegistry()
	rec := &statusRec{}
	svc := New(Config{Registry: reg, Store: st, Emit: rec.emit, StatusDebounce: -1})
	claude := &loginFake{fakeDetAdapter: newFake(catagent.KindClaudeCLI, true), logged: bptr(true)}
	codex := &loginFake{fakeDetAdapter: newFake(catagent.KindCodexCLI, true)} // 判断不了 = nil
	build := newFake(catagent.KindCatBuild, true)                             // 不实现 LiveLoggedIn
	for _, f := range []catagent.Adapter{build, claude, codex} {
		reg.Register(f)
	}
	claude.onChange, codex.onChange, build.onChange = svc.AgentChanged, svc.AgentChanged, svc.AgentChanged

	// 还没实时检测完：即便适配器手里有值也不带
	if r := row(svc.emitStatus(), catagent.KindClaudeCLI); r.LoggedIn != nil {
		t.Fatalf("未检测完不应带 loggedIn: %+v", r)
	}
	claude.detect()
	codex.detect()
	build.detect()
	s := svc.emitStatus()
	if r := row(s, catagent.KindClaudeCLI); r.LoggedIn == nil || !*r.LoggedIn {
		t.Fatalf("实时检测应带 loggedIn=true: %+v", r)
	}
	if r := row(s, catagent.KindCodexCLI); r.LoggedIn != nil {
		t.Fatalf("判断不了应省略: %+v", r)
	}
	if r := row(s, catagent.KindCatBuild); r.LoggedIn != nil {
		t.Fatalf("不支持的家应省略: %+v", r)
	}
	b, _ := json.Marshal(row(s, catagent.KindCodexCLI))
	if strings.Contains(string(b), "loggedIn") {
		t.Fatalf("nil 时 JSON 不应出现字段: %s", b)
	}

	// 检测中 / 超时：保留上次实时值
	claude.set(Status{State: catagent.StateChecking}, false)
	if r := row(svc.emitStatus(), catagent.KindClaudeCLI); !r.Checking || r.LoggedIn == nil || !*r.LoggedIn {
		t.Fatalf("检测中应保留上次实时值: %+v", r)
	}
	claude.timedOut.Store(true)
	if r := row(svc.emitStatus(), catagent.KindClaudeCLI); !r.Stale || r.LoggedIn == nil || !*r.LoggedIn {
		t.Fatalf("超时行应保留上次实时值: %+v", r)
	}

	// 不落库
	var raw map[string]map[string]any
	if ok, err := st.GetSetting(context.Background(), DetectKey, &raw); !ok || err != nil {
		t.Fatalf("未落库: %v %v", ok, err)
	}
	for k, v := range raw {
		if _, has := v["loggedIn"]; has {
			t.Fatalf("cat.detect 不得含 loggedIn: %s %+v", k, v)
		}
	}

	// 「重启」：落库行不带 loggedIn
	reg2 := catagent.NewRegistry()
	svc2 := New(Config{Registry: reg2, Store: st, StatusDebounce: -1})
	c2 := &loginFake{fakeDetAdapter: newFake(catagent.KindClaudeCLI, true), logged: bptr(false)}
	reg2.Register(c2)
	r := row(svc2.emitStatus(), catagent.KindClaudeCLI)
	if !r.Stale || r.LoggedIn != nil {
		t.Fatalf("落库行不应带 loggedIn: %+v", r)
	}
}

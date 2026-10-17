package cat_test

// v0.55 项目群聊端到端（契约 6.21.13–6.21.16）：真 Service + 真 store + 真适配器，
// 只把 CLI 换成假的可执行脚本（输出各家真实的 stream-json），不需要登录。

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/service/cat"
	"FFmpegFree/internal/store"
)

// fakeGrokScript：按群规里的「你是「名字（」认人，读 $F/<名字>.{pre,sleep,out}。
// 职责生成 / 派活判断各有固定入口（route.*）。
const fakeGrokScript = `#!/bin/sh
case "$1" in
  -v) echo "grok 1.0.40"; exit 0 ;;
  usage) echo "Error: Session not found." >&2; exit 1 ;;
  models) printf 'Default model: grok-4.6\n\nAvailable models:\n  * grok-4.6 (default)\n'; exit 0 ;;
esac
F='@@F@@'
cwd=""; sid=""; prev=""
for a in "$@"; do
  case "$prev" in --cwd) cwd="$a" ;; --session-id) sid="$a" ;; esac
  prev="$a"
done
if [ -n "$sid" ] && [ -n "$cwd" ]; then mkdir -p "$HOME/.grok/sessions/$(printf %s "$cwd" | sed 's|/|%2F|g')/$sid"; fi
ALL="$*"
case "$ALL" in
  *为项目群聊成员生成职责规范*)
    printf '%s\n' '{"type":"text","data":"{\"owns\":\"自己的活\",\"notTouch\":\"别人的活\",\"handoff\":\"\",\"dirs\":[]}"}'
    printf '%s\n' '{"type":"end","stopReason":"end_turn"}'
    exit 0 ;;
  *选出最合适的一个人*) N=route ;;
  *) N=$(printf '%s\n' "$ALL" | sed -n 's/.*你是「\([^（]*\)（.*/\1/p' | head -n 1) ;;
esac
printf '%s\n' "$N" >> "$F/calls.log"
printf '%s\n' "$ALL" > "$F/$N.argv"
if [ -f "$F/$N.pre" ]; then (cd "$cwd" && sh "$F/$N.pre"); fi
if [ -f "$F/$N.sleep" ]; then
  n=$(cat "$F/$N.sleep"); i=0
  while [ "$i" -lt "$n" ]; do sleep 0.1; i=$((i+1)); done
fi
if [ -f "$F/$N.exit" ]; then cat "$F/$N.out" 2>/dev/null; exit "$(cat "$F/$N.exit")"; fi
cat "$F/$N.out" 2>/dev/null
printf '%s\n' '{"type":"end","stopReason":"end_turn"}'
`

type groupEnv struct {
	t    *testing.T
	svc  *cat.Service
	st   *store.Store
	fake string // plan dir
	root string // project root (git repo)
	g    cat.Group
	mem  map[string]cat.GroupMember
	mu   sync.Mutex
	evs  []any
}

func newGroupEnv(t *testing.T, names ...string) *groupEnv {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("unix script fake")
	}
	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))
	fdir := filepath.Join(dir, "plan")
	_ = os.MkdirAll(fdir, 0o755)
	fake := filepath.Join(dir, "grok")
	if err := os.WriteFile(fake, []byte(strings.ReplaceAll(fakeGrokScript, "@@F@@", fdir)), 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(context.Background(), filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	temp := filepath.Join(dir, "tmp")
	_ = os.MkdirAll(temp, 0o755)
	reg := catagent.NewRegistry()
	build := catagent.NewBuildAdapter(catagent.BuildConfig{
		ComponentDir: filepath.Join(dir, "components"), DataTemp: temp,
		LookPath: func(file string) (string, error) {
			if file == "grok" || file == "grok.exe" {
				return fake, nil
			}
			return "", exec.ErrNotFound
		},
		Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, name, args...)
		},
	})
	reg.Register(build)
	build.Start()
	e := &groupEnv{t: t, st: st, fake: fdir, mem: map[string]cat.GroupMember{}}
	e.svc = cat.New(cat.Config{Store: st, Registry: reg, DataRoot: dir, Emit: func(name string, p any) {
		e.mu.Lock()
		e.evs = append(e.evs, p)
		e.mu.Unlock()
	}})
	t.Cleanup(func() { _ = e.svc.StopCatGroup(context.Background(), e.g.ID); e.svc.Wait() })
	ctx := context.Background()
	deadline := time.Now().Add(5 * time.Second)
	for {
		s, _ := e.svc.GetCatStatus(ctx)
		if s.State == catagent.StateReady {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("not ready: %+v", s)
		}
		time.Sleep(20 * time.Millisecond)
	}
	// 项目：带初始提交的 git 仓库，前后快照 / 改动文件都靠它。
	e.root = filepath.Join(dir, "proj")
	for _, d := range []string{"frontend", "backend"} {
		_ = os.MkdirAll(filepath.Join(e.root, d), 0o755)
	}
	_ = os.WriteFile(filepath.Join(e.root, "README.md"), []byte("demo\n"), 0o644)
	_ = os.WriteFile(filepath.Join(e.root, "backend", "api.go"), []byte("package api\n"), 0o644)
	_ = os.WriteFile(filepath.Join(e.root, "frontend", "app.ts"), []byte("export {}\n"), 0o644)
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}, {"-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "init"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = e.root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	p, err := e.svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: e.root})
	if err != nil {
		t.Fatal(err)
	}
	e.g, err = e.svc.CreateCatGroup(ctx, cat.CreateGroupRequest{ProjectID: p.Project.ID, Name: "群"})
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		m, err := e.svc.AddCatGroupMember(ctx, cat.AddGroupMemberRequest{GroupID: e.g.ID, Name: n, Title: n + "职位", AgentKind: catagent.KindCatBuild})
		if err != nil {
			t.Fatal(err)
		}
		e.mem[n] = m
	}
	e.svc.Wait() // 职责生成
	return e
}

func (e *groupEnv) setDirs(name string, dirs ...string) {
	e.t.Helper()
	m, err := e.svc.UpdateCatCharter(context.Background(), cat.UpdateCharterRequest{MemberID: e.mem[name].ID, Dirs: dirs})
	if err != nil {
		e.t.Fatal(err)
	}
	e.mem[name] = m
}

func (e *groupEnv) file(name, ext, body string) {
	e.t.Helper()
	if err := os.WriteFile(filepath.Join(e.fake, name+"."+ext), []byte(body), 0o644); err != nil {
		e.t.Fatal(err)
	}
}

// reply 写一轮假输出：先发「写文件」工具事件，再发正文。
func (e *groupEnv) reply(name, text string, writes ...string) {
	e.t.Helper()
	var b strings.Builder
	for i, w := range writes {
		ev := map[string]any{
			"type": "tool_call", "toolCallId": name + "-w" + string(rune('0'+i)), "toolName": "write_file", "kind": "edit",
			"status": "completed", "title": "write " + w, "rawInput": map[string]any{"path": w},
			"locations": []any{map[string]any{"path": w}},
		}
		raw, _ := json.Marshal(ev)
		b.Write(raw)
		b.WriteByte('\n')
	}
	if text != "" {
		raw, _ := json.Marshal(map[string]any{"type": "text", "data": text})
		b.Write(raw)
		b.WriteByte('\n')
	}
	e.file(name, "out", b.String())
}

// writeOnRun：假 CLI 运行时真的在项目里写文件。
func (e *groupEnv) writeOnRun(name string, files map[string]string) {
	var b strings.Builder
	for p, c := range files {
		b.WriteString("mkdir -p \"$(dirname '" + p + "')\" && printf '%s' '" + c + "' > '" + p + "'\n")
	}
	e.file(name, "pre", b.String())
}

func report(did, attn, give string) string {
	s := "## 完成了什么\n" + did + "\n"
	if attn != "" {
		s += "## 需要注意\n" + attn + "\n"
	}
	if give != "" {
		s += "## 交给谁\n" + give + "\n"
	}
	return s
}

func (e *groupEnv) send(text string, mentions ...string) string {
	e.t.Helper()
	var ids []string
	for _, n := range mentions {
		ids = append(ids, e.mem[n].ID)
	}
	res, err := e.svc.SendCatGroupMessage(context.Background(), cat.SendGroupMessageRequest{GroupID: e.g.ID, Text: text, Mentions: ids})
	if err != nil {
		e.t.Fatal(err)
	}
	return res.ChainID
}

func (e *groupEnv) msgs() []cat.GroupMessage {
	e.t.Helper()
	res, err := e.svc.ListCatGroupMessages(context.Background(), cat.ListGroupMessagesRequest{GroupID: e.g.ID, Limit: 200})
	if err != nil {
		e.t.Fatal(err)
	}
	return res.Messages
}

func (e *groupEnv) calls() []string {
	raw, _ := os.ReadFile(filepath.Join(e.fake, "calls.log"))
	return strings.Fields(string(raw))
}

func (e *groupEnv) dump() string {
	var b strings.Builder
	for _, m := range e.msgs() {
		who := m.MemberID
		for n, mm := range e.mem {
			if mm.ID == m.MemberID {
				who = n
			}
		}
		b.WriteString(m.Kind + "/" + m.SystemKind + " hop=" + itoa(m.Hop) + " who=" + who + " : " + strings.ReplaceAll(m.Text, "\n", "⏎") + "\n")
	}
	return b.String()
}

func itoa(i int) string { return strings.TrimSpace(strings.Repeat(" ", 0) + jsonNum(i)) }
func jsonNum(i int) string { raw, _ := json.Marshal(i); return string(raw) }

func (e *groupEnv) waitIdle() {
	e.t.Helper()
	done := make(chan struct{})
	go func() { e.svc.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		e.t.Fatalf("group did not settle:\n%s", e.dump())
	}
}

func (e *groupEnv) waitFor(cond func([]cat.GroupMessage) bool) {
	e.t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if cond(e.msgs()) {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	e.t.Fatalf("timeout:\n%s", e.dump())
}

func find(ms []cat.GroupMessage, f func(cat.GroupMessage) bool) []cat.GroupMessage {
	var out []cat.GroupMessage
	for _, m := range ms {
		if f(m) {
			out = append(out, m)
		}
	}
	return out
}

func sys(kind string) func(cat.GroupMessage) bool {
	return func(m cat.GroupMessage) bool { return m.Kind == "system" && m.SystemKind == kind }
}

// 1. @ 一人：只有他跑；汇报卡 did/attn/give；改了哪些文件由 Cat 填。
func TestGroupE2E_MentionOne(t *testing.T) {
	e := newGroupEnv(t, "前端", "后端")
	e.setDirs("后端", "backend")
	e.writeOnRun("后端", map[string]string{"backend/api.go": "package api // v2\n"})
	e.reply("后端", report("加了接口", "要重生 bindings", ""), "backend/api.go")
	e.send("加个接口", "后端")
	e.waitIdle()
	if c := e.calls(); len(c) != 1 || c[0] != "后端" {
		t.Fatalf("calls %v", c)
	}
	ms := e.msgs()
	cards := find(ms, func(m cat.GroupMessage) bool { return m.Kind == "member" })
	if len(cards) != 1 || cards[0].Report == nil {
		t.Fatalf("cards:\n%s", e.dump())
	}
	r := cards[0].Report
	if !r.Parsed || r.Did != "加了接口" || r.Attn != "要重生 bindings" || r.Give != "" {
		t.Fatalf("report %+v", r)
	}
	if len(r.Files) != 1 || r.Files[0].Path != "backend/api.go" || r.Files[0].Tag != "" {
		t.Fatalf("files %+v", r.Files)
	}
	if len(find(ms, sys("no_handoff"))) != 1 {
		t.Fatalf("no_handoff:\n%s", e.dump())
	}
}

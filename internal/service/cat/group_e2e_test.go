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
  case "$prev" in --cwd) cwd="$a" ;; --session-id) sid="$a" ;; --prompt-file) pf="$a" ;; esac
  prev="$a"
done
if [ -n "$sid" ] && [ -n "$cwd" ]; then mkdir -p "$HOME/.grok/sessions/$(printf %s "$cwd" | sed 's|/|%2F|g')/$sid"; fi
ALL="$*"
if [ -n "$pf" ] && [ -f "$pf" ]; then ALL="$ALL
$(cat "$pf")"; fi
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

func cardsOf(ms []cat.GroupMessage, memberID string) []cat.GroupMessage {
	return find(ms, func(m cat.GroupMessage) bool { return m.Kind == "member" && m.MemberID == memberID })
}

// 2. @ 两人并行；一条回复 @ 两人 hop 只 +1。
func TestGroupE2E_MentionTwoParallelAndHop(t *testing.T) {
	e := newGroupEnv(t, "甲", "乙", "丙")
	e.reply("乙", report("乙做完", "", ""))
	e.reply("丙", report("丙做完", "", ""))
	e.file("乙", "sleep", "10")
	e.file("丙", "sleep", "10")
	start := time.Now()
	e.send("一起做", "乙", "丙")
	e.waitIdle()
	if el := time.Since(start); el > 1800*time.Millisecond {
		t.Fatalf("not parallel: %v", el)
	}
	ms := e.msgs()
	for _, n := range []string{"乙", "丙"} {
		c := cardsOf(ms, e.mem[n].ID)
		if len(c) != 1 || c[0].Hop != 1 {
			t.Fatalf("%s cards:\n%s", n, e.dump())
		}
	}
	if len(cardsOf(ms, e.mem["甲"].ID)) != 0 {
		t.Fatal("甲 must not run")
	}

	// 甲 一条回复 @ 乙 和 丙：两句转交灰字都是「第 1/6 次」，乙丙的卡 hop=2。
	_ = os.Remove(filepath.Join(e.fake, "calls.log"))
	e.reply("甲", report("甲做完", "", "@乙 做接口，@丙 做页面"))
	chain := e.send("开工", "甲")
	e.waitIdle()
	ms = find(e.msgs(), func(m cat.GroupMessage) bool { return m.ChainID == chain })
	routes := find(ms, sys("route"))
	if len(routes) != 2 {
		t.Fatalf("routes:\n%s", e.dump())
	}
	for _, r := range routes {
		if !strings.Contains(r.Text, "（第 1/6 次）") || !strings.HasPrefix(r.Text, "已转交给 ") {
			t.Fatalf("route copy %q", r.Text)
		}
	}
	for _, n := range []string{"乙", "丙"} {
		c := cardsOf(ms, e.mem[n].ID)
		if len(c) != 1 || c[0].Hop != 2 {
			t.Fatalf("%s hop:\n%s", n, e.dump())
		}
	}
}

// 3. 不 @：派活判断（一次性、不留会话）；超时 / 无效 → 第一位 + 兜底文案；自动派活关 → 第一位 + 对应文案。
func TestGroupE2E_DispatchAI(t *testing.T) {
	e := newGroupEnv(t, "前端", "后端")
	e.reply("前端", report("前端做完", "", ""))
	e.reply("后端", report("后端做完", "", ""))
	ans, _ := json.Marshal(map[string]string{"memberId": e.mem["后端"].ID, "reason": "接口归后端"})
	e.reply("route", string(ans))
	chain := e.send("加个接口")
	e.waitIdle()
	ms := find(e.msgs(), func(m cat.GroupMessage) bool { return m.ChainID == chain })
	if len(find(ms, func(m cat.GroupMessage) bool { return m.Kind == "system" && m.Text == "交给 后端：接口归后端" })) != 1 {
		argv, _ := os.ReadFile(filepath.Join(e.fake, "route.argv")); out, _ := os.ReadFile(filepath.Join(e.fake, "route.out")); t.Logf("calls=%v argv=%s out=%s", e.calls(), argv, out)
		t.Fatalf("ai route copy:\n%s", e.dump())
	}
	if c := e.calls(); strings.Join(c, ",") != "route,后端" {
		t.Fatalf("calls %v", c)
	}
	argv, _ := os.ReadFile(filepath.Join(e.fake, "route.argv"))
	if strings.Contains(string(argv), "--session-id") || strings.Contains(string(argv), "--resume") {
		t.Fatalf("route turn must not keep a session: %s", argv)
	}

	fallback := "交给 前端：没判断出该谁接，先交给排第一的成员"
	// 无效输出（不存在的成员）
	_ = os.Remove(filepath.Join(e.fake, "calls.log"))
	e.reply("route", `{"memberId":"nobody","reason":"x"}`)
	chain = e.send("随便")
	e.waitIdle()
	ms = find(e.msgs(), func(m cat.GroupMessage) bool { return m.ChainID == chain })
	if len(find(ms, func(m cat.GroupMessage) bool { return m.Text == fallback })) != 1 || len(cardsOf(ms, e.mem["前端"].ID)) != 1 {
		t.Fatalf("invalid fallback:\n%s", e.dump())
	}
	// 非 JSON
	e.reply("route", "我觉得是后端")
	chain = e.send("随便2")
	e.waitIdle()
	ms = find(e.msgs(), func(m cat.GroupMessage) bool { return m.ChainID == chain })
	if len(find(ms, func(m cat.GroupMessage) bool { return m.Text == fallback })) != 1 {
		t.Fatalf("non-json fallback:\n%s", e.dump())
	}

	// 自动派活关
	_ = os.Remove(filepath.Join(e.fake, "calls.log"))
	if _, err := e.svc.SetCatGroupAutoRoute(context.Background(), cat.SetGroupAutoRouteRequest{GroupID: e.g.ID, On: false}); err != nil {
		t.Fatal(err)
	}
	chain = e.send("关了派活")
	e.waitIdle()
	ms = find(e.msgs(), func(m cat.GroupMessage) bool { return m.ChainID == chain })
	if len(find(ms, func(m cat.GroupMessage) bool { return m.Text == "交给 前端：没开自动派活，交给排第一的成员" })) != 1 {
		t.Fatalf("auto off copy:\n%s", e.dump())
	}
	if c := e.calls(); strings.Join(c, ",") != "前端" {
		t.Fatalf("auto off must not call route: %v", c)
	}
}

func TestGroupE2E_DispatchTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("20s route timeout")
	}
	e := newGroupEnv(t, "前端", "后端")
	e.reply("前端", report("前端做完", "", ""))
	e.reply("route", `{"memberId":"x","reason":"y"}`)
	e.file("route", "sleep", "250") // > 20s 派活超时
	chain := e.send("慢慢想")
	e.waitIdle()
	ms := find(e.msgs(), func(m cat.GroupMessage) bool { return m.ChainID == chain })
	if len(find(ms, func(m cat.GroupMessage) bool { return m.Text == "交给 前端：没判断出该谁接，先交给排第一的成员" })) != 1 || len(cardsOf(ms, e.mem["前端"].ID)) != 1 {
		t.Fatalf("timeout fallback:\n%s", e.dump())
	}
}

// 4. 回复里 @ 接力，6 次封顶 + 灰字；@ 自己忽略。
func TestGroupE2E_RelayLimit(t *testing.T) {
	e := newGroupEnv(t, "甲", "乙")
	e.reply("甲", report("甲做完", "", "@乙 继续"))
	e.reply("乙", report("乙做完", "", "@甲 继续，@乙 我自己也看看"))
	chain := e.send("开始", "甲")
	e.waitIdle()
	ms := find(e.msgs(), func(m cat.GroupMessage) bool { return m.ChainID == chain })
	cards := find(ms, func(m cat.GroupMessage) bool { return m.Kind == "member" })
	if len(cards) != 6 {
		t.Fatalf("cards %d:\n%s", len(cards), e.dump())
	}
	routes := find(ms, sys("route"))
	if len(routes) != 5 || !strings.Contains(routes[4].Text, "（第 5/6 次）") {
		t.Fatalf("routes:\n%s", e.dump())
	}
	lim := find(ms, sys("hop_limit"))
	if len(lim) != 1 || lim[0].Text != "已连续转交 6 次，先停在这里。需要继续请 @ 对应成员。" {
		t.Fatalf("limit:\n%s", e.dump())
	}
	for _, c := range cardsOf(ms, e.mem["乙"].ID) {
		for _, mid := range c.Mentions {
			if mid == e.mem["乙"].ID {
				t.Fatalf("self mention kept: %+v", c.Mentions)
			}
		}
	}
	if n := len(e.calls()); n != 6 {
		t.Fatalf("calls %v", e.calls())
	}
}

// 5. 换人：停掉原成员，灰字「已换人」「交给 X：你指定的」，不算接力。
func TestGroupE2E_Reassign(t *testing.T) {
	e := newGroupEnv(t, "甲", "乙")
	e.reply("甲", report("甲做完", "", ""))
	e.file("甲", "sleep", "100")
	e.reply("乙", report("乙做完", "", ""))
	chain := e.send("做", "甲")
	e.waitFor(func([]cat.GroupMessage) bool { return len(e.calls()) == 1 })
	time.Sleep(100 * time.Millisecond)
	if _, err := e.svc.ReassignCatGroupRoute(context.Background(), cat.ReassignGroupRouteRequest{GroupID: e.g.ID, ChainID: chain, ToMemberID: e.mem["乙"].ID}); err != nil {
		t.Fatal(err)
	}
	e.waitIdle()
	ms := find(e.msgs(), func(m cat.GroupMessage) bool { return m.ChainID == chain })
	want := []string{"已换人：甲 → 乙", "交给 乙：你指定的", "甲 已停止（换人时停下的）"}
	for _, w := range want {
		if len(find(ms, func(m cat.GroupMessage) bool { return m.Kind == "system" && m.Text == w })) != 1 {
			t.Fatalf("missing %q:\n%s", w, e.dump())
		}
	}
	if len(cardsOf(ms, e.mem["甲"].ID)) != 0 {
		t.Fatalf("甲 must not post a card:\n%s", e.dump())
	}
	c := cardsOf(ms, e.mem["乙"].ID)
	if len(c) != 1 || c[0].Hop != 1 {
		t.Fatalf("乙 card hop (reassign must not count):\n%s", e.dump())
	}
	if len(find(ms, func(m cat.GroupMessage) bool { return strings.HasPrefix(m.Text, "已转交给") })) != 0 {
		t.Fatalf("reassign counted as relay:\n%s", e.dump())
	}
}

// 6. 越界：标出来；撤销只在内容一致时恢复；之后又改过 → 报错；来源不明不能撤。
func TestGroupE2E_Overreach(t *testing.T) {
	e := newGroupEnv(t, "前端", "后端")
	e.setDirs("前端", "frontend")
	e.writeOnRun("前端", map[string]string{
		"frontend/app.ts": "export const a = 1\n",
		"backend/api.go":  "package api // 前端改的\n",
		"README.md":       "被谁改了\n", // 不在工具调用里：来源不明
	})
	e.reply("前端", report("改了页面", "", ""), "frontend/app.ts", "backend/api.go")
	chain := e.send("改页面", "前端")
	e.waitIdle()
	ms := find(e.msgs(), func(m cat.GroupMessage) bool { return m.ChainID == chain })
	c := cardsOf(ms, e.mem["前端"].ID)
	if len(c) != 1 {
		t.Fatalf("card:\n%s", e.dump())
	}
	tags := map[string]string{}
	for _, f := range c[0].Report.Files {
		tags[f.Path] = f.Tag
	}
	if tags["frontend/app.ts"] != "" || tags["backend/api.go"] != "overreach" || tags["README.md"] != "unknown" {
		t.Fatalf("tags %+v", c[0].Report.Files)
	}
	res, err := e.svc.RevertCatOverreach(context.Background(), cat.RevertOverreachRequest{MemberID: e.mem["前端"].ID, TurnID: c[0].TurnID, Files: []string{"backend/api.go", "README.md"}})
	if len(res.Reverted) != 1 || res.Reverted[0] != "backend/api.go" {
		t.Fatalf("revert %+v %v", res, err)
	}
	if len(res.Skipped) != 1 || res.Skipped[0].Path != "README.md" || res.Skipped[0].Reason != "来源不明，不能撤销" || err == nil {
		t.Fatalf("unknown must be skipped: %+v %v", res, err)
	}
	if b, _ := os.ReadFile(filepath.Join(e.root, "backend", "api.go")); string(b) != "package api\n" {
		t.Fatalf("not restored: %q", b)
	}

	// 第二轮越界，之后有人又改了 → 不能自动撤销。
	e.writeOnRun("前端", map[string]string{"backend/api.go": "package api // 又越界\n"})
	e.reply("前端", report("又改了", "", ""), "backend/api.go")
	chain = e.send("再改", "前端")
	e.waitIdle()
	c = cardsOf(find(e.msgs(), func(m cat.GroupMessage) bool { return m.ChainID == chain }), e.mem["前端"].ID)
	if len(c) != 1 {
		t.Fatalf("card2:\n%s", e.dump())
	}
	_ = os.WriteFile(filepath.Join(e.root, "backend", "api.go"), []byte("package api // 后端后来改的\n"), 0o644)
	res, err = e.svc.RevertCatOverreach(context.Background(), cat.RevertOverreachRequest{MemberID: e.mem["前端"].ID, TurnID: c[0].TurnID, Files: []string{"backend/api.go"}})
	if err == nil || len(res.Reverted) != 0 || len(res.Skipped) != 1 || !strings.Contains(res.Skipped[0].Reason, "之后又被改过") {
		t.Fatalf("changed-after must fail: %+v %v", res, err)
	}
	if b, _ := os.ReadFile(filepath.Join(e.root, "backend", "api.go")); string(b) != "package api // 后端后来改的\n" {
		t.Fatalf("must not touch: %q", b)
	}
}

// 7. 失败 / 停止：只发灰字，不发汇报、不接力；重试 hop 不加。
func TestGroupE2E_FailStopRetry(t *testing.T) {
	e := newGroupEnv(t, "甲", "乙")
	e.reply("甲", report("甲做完", "", "@乙 接着"))
	e.file("乙", "out", `{"type":"error","message":"额度用完了"}`+"\n")
	e.file("乙", "exit", "1")
	chain := e.send("做", "甲")
	e.waitIdle()
	ms := find(e.msgs(), func(m cat.GroupMessage) bool { return m.ChainID == chain })
	f := find(ms, sys("failed"))
	if len(f) != 1 || !strings.HasPrefix(f[0].Text, "乙 没完成：") || f[0].FailReason == "" || f[0].Hop != 1 {
		t.Fatalf("failed:\n%s", e.dump())
	}
	if len(cardsOf(ms, e.mem["乙"].ID)) != 0 || len(find(ms, sys("no_handoff"))) != 0 {
		t.Fatalf("failure must not post report:\n%s", e.dump())
	}

	// 重试：同一 hop 再跑一轮，卡 hop 跟原来一样（2），不再加。
	_ = os.Remove(filepath.Join(e.fake, "乙.exit"))
	e.reply("乙", report("乙重试做完", "", ""))
	if err := e.svc.RetryCatGroupMemberTurn(context.Background(), cat.RetryGroupMemberTurnRequest{GroupID: e.g.ID, MemberID: e.mem["乙"].ID, ChainID: chain}); err != nil {
		t.Fatal(err)
	}
	e.waitIdle()
	ms = find(e.msgs(), func(m cat.GroupMessage) bool { return m.ChainID == chain })
	c := cardsOf(ms, e.mem["乙"].ID)
	if len(c) != 1 || c[0].Hop != 2 || c[0].Report.Did != "乙重试做完" {
		t.Fatalf("retry card hop must equal original (2):\n%s", e.dump())
	}

	// 停止
	e.file("甲", "sleep", "100")
	chain = e.send("再做", "甲")
	e.waitFor(func([]cat.GroupMessage) bool { return strings.Count(strings.Join(e.calls(), ","), "甲") == 2 })
	time.Sleep(100 * time.Millisecond)
	if err := e.svc.StopCatGroupMember(context.Background(), e.mem["甲"].ID); err != nil {
		t.Fatal(err)
	}
	e.waitIdle()
	ms = find(e.msgs(), func(m cat.GroupMessage) bool { return m.ChainID == chain })
	if len(find(ms, func(m cat.GroupMessage) bool { return m.Kind == "system" && m.Text == "甲 已停止" })) != 1 ||
		len(find(ms, func(m cat.GroupMessage) bool { return m.Kind == "member" })) != 0 ||
		len(find(ms, sys("route"))) != 0 {
		t.Fatalf("stop:\n%s", e.dump())
	}
}

// 8. 没按汇报格式：整段当 did + systemKind=report_format 灰字。
func TestGroupE2E_UnparsedReport(t *testing.T) {
	e := newGroupEnv(t, "甲")
	e.reply("甲", "我把按钮改成蓝色了，顺便调了间距。")
	chain := e.send("改按钮", "甲")
	e.waitIdle()
	ms := find(e.msgs(), func(m cat.GroupMessage) bool { return m.ChainID == chain })
	c := cardsOf(ms, e.mem["甲"].ID)
	if len(c) != 1 || c[0].Report.Parsed || c[0].Report.Did != "我把按钮改成蓝色了，顺便调了间距。" {
		t.Fatalf("card:\n%s", e.dump())
	}
	g := find(ms, sys("report_format"))
	if len(g) != 1 || g[0].Text != "没按汇报格式写，只显示最后一段回复" {
		t.Fatalf("gray:\n%s", e.dump())
	}
}

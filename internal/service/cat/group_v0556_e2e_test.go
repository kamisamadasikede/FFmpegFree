package cat_test

// v0.55.6 群聊数据补全（契约 6.21.17）端到端：真 Service + 真 store + 假 CLI。

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"encoding/json"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/i18n"
	"FFmpegFree/internal/service/cat"
	"FFmpegFree/internal/store"
)

func (e *groupEnv) memberEvents(name string) []cat.GroupMemberEvent {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []cat.GroupMemberEvent
	for _, ev := range e.evs {
		if me, ok := ev.(cat.GroupMemberEvent); ok && me.Member.ID == e.mem[name].ID {
			out = append(out, me)
		}
	}
	return out
}

func (e *groupEnv) routeEvents() []cat.GroupRouteEvent {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []cat.GroupRouteEvent
	for _, ev := range e.evs {
		if re, ok := ev.(cat.GroupRouteEvent); ok {
			out = append(out, re)
		}
	}
	return out
}

func (e *groupEnv) member(name string) cat.GroupMember {
	e.t.Helper()
	list, err := e.svc.ListCatGroupMembers(context.Background(), e.g.ID)
	if err != nil {
		e.t.Fatal(err)
	}
	for _, m := range list {
		if m.ID == e.mem[name].ID {
			return m
		}
	}
	e.t.Fatalf("no member %s", name)
	return cat.GroupMember{}
}

func queuedLines(ms []cat.GroupMessage) []cat.GroupMessage {
	return find(ms, func(m cat.GroupMessage) bool {
		return m.Kind == "system" && m.SystemKind == "route" && m.Route != nil && m.Route.Queued
	})
}

// ② ListCatGroups / GetCatGroup 带成员摘要，avatarIndex = (order % 5) + 1，排序后跟着变。
func TestGroupV0556_ListGroupsMembersAvatar(t *testing.T) {
	e := newGroupEnv(t, "甲", "乙", "丙", "丁", "戊", "己")
	ctx := context.Background()
	empty, err := e.svc.CreateCatGroup(ctx, cat.CreateGroupRequest{ProjectID: e.g.ProjectID, Name: "空群"})
	if err != nil {
		t.Fatal(err)
	}
	if empty.Members == nil || len(empty.Members) != 0 {
		t.Fatalf("new group members must be []: %#v", empty.Members)
	}
	raw, _ := json.Marshal(empty)
	if !strings.Contains(string(raw), `"members":[]`) {
		t.Fatalf("json %s", raw)
	}
	list, err := e.svc.ListCatGroups(ctx, e.g.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	var g cat.Group
	for _, x := range list {
		if x.ID == e.g.ID {
			g = x
		}
		if x.ID == empty.ID && (x.Members == nil || len(x.Members) != 0) {
			t.Fatalf("empty group in list: %#v", x.Members)
		}
	}
	names := []string{"甲", "乙", "丙", "丁", "戊", "己"}
	if len(g.Members) != 6 {
		t.Fatalf("members %+v", g.Members)
	}
	for i, b := range g.Members {
		if b.Name != names[i] || b.Order != i || b.AvatarIndex != i%5+1 || b.Title != names[i]+"职位" || b.AgentKind == "" {
			t.Fatalf("brief %d %+v", i, b)
		}
	}
	raw, _ = json.Marshal(g.Members[0])
	if strings.Contains(string(raw), "charter") || strings.Contains(string(raw), "session") {
		t.Fatalf("brief leaks fields: %s", raw)
	}
	// 拖动排序：己 排第一 → avatarIndex 1
	ids := []string{e.mem["己"].ID, e.mem["甲"].ID, e.mem["乙"].ID, e.mem["丙"].ID, e.mem["丁"].ID, e.mem["戊"].ID}
	if _, err := e.svc.ReorderCatGroupMembers(ctx, cat.ReorderGroupMembersRequest{GroupID: e.g.ID, MemberIDs: ids}); err != nil {
		t.Fatal(err)
	}
	d, err := e.svc.GetCatGroup(ctx, e.g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if d.Group.Members[0].Name != "己" || d.Group.Members[0].AvatarIndex != 1 || d.Group.Members[1].AvatarIndex != 2 || d.Group.Members[5].AvatarIndex != 1 {
		t.Fatalf("after reorder %+v", d.Group.Members)
	}
	r, err := e.svc.RenameCatGroup(ctx, cat.RenameGroupRequest{GroupID: e.g.ID, Name: "新名"})
	if err != nil || len(r.Members) != 6 {
		t.Fatalf("rename members %+v %v", r.Members, err)
	}
	a, err := e.svc.SetCatGroupAutoRoute(ctx, cat.SetGroupAutoRouteRequest{GroupID: e.g.ID, On: false})
	if err != nil || len(a.Members) != 6 {
		t.Fatalf("autoroute members %+v %v", a.Members, err)
	}
}

// ① 「文件」页签：按 path 聚合汇报里的文件；changeCount / last* / overreach / tag / exists；按 updatedAt 降序。
func TestGroupV0556_ListGroupFiles(t *testing.T) {
	e := newGroupEnv(t, "前端", "后端")
	ctx := context.Background()
	e.setDirs("前端", "frontend")
	e.setDirs("后端", "backend")

	e.writeOnRun("后端", map[string]string{"backend/api.go": "package api // 1\n", "backend/gone.go": "package api\n"})
	e.reply("后端", report("加接口", "", ""), "backend/api.go", "backend/gone.go")
	e.send("加接口", "后端")
	e.waitIdle()
	time.Sleep(5 * time.Millisecond) // 两条汇报 createdAt 不同

	e.writeOnRun("前端", map[string]string{"frontend/app.ts": "export const a = 1\n", "backend/api.go": "package api // 前端改的\n"})
	e.reply("前端", report("改页面", "", ""), "frontend/app.ts", "backend/api.go")
	e.send("改页面", "前端")
	e.waitIdle()
	_ = os.Remove(filepath.Join(e.root, "backend", "gone.go"))

	files, err := e.svc.ListCatGroupFiles(ctx, e.g.ID)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]cat.GroupFile{}
	for _, f := range files {
		by[f.Path] = f
	}
	if len(files) != 3 {
		t.Fatalf("files %+v", files)
	}
	cards := find(e.msgs(), func(m cat.GroupMessage) bool { return m.Kind == "member" })
	if len(cards) != 2 {
		t.Fatalf("cards:\n%s", e.dump())
	}
	backCard, frontCard := cards[0], cards[1]
	api := by["backend/api.go"]
	if api.ChangeCount != 2 || api.LastMemberID != e.mem["前端"].ID || api.LastSeq != frontCard.Seq ||
		api.LastTurnID != frontCard.TurnID || !api.Overreach || api.Tag != "overreach" || !api.Exists || api.UpdatedAt != frontCard.CreatedAt {
		t.Fatalf("api.go %+v", api)
	}
	gone := by["backend/gone.go"]
	if gone.ChangeCount != 1 || gone.Exists || gone.Overreach || gone.Tag != "" || gone.LastMemberID != e.mem["后端"].ID || gone.LastSeq != backCard.Seq {
		t.Fatalf("gone.go %+v", gone)
	}
	app := by["frontend/app.ts"]
	if app.ChangeCount != 1 || !app.Exists || app.Overreach {
		t.Fatalf("app.ts %+v", app)
	}
	for i := 1; i < len(files); i++ {
		if files[i-1].UpdatedAt < files[i].UpdatedAt {
			t.Fatalf("not sorted desc: %+v", files)
		}
	}
	if files[len(files)-1].Path != "backend/gone.go" {
		t.Fatalf("oldest last: %+v", files)
	}
	// 成员删了 lastMemberId 照返
	if err := e.svc.RemoveCatGroupMember(ctx, e.mem["前端"].ID); err != nil {
		t.Fatal(err)
	}
	files, _ = e.svc.ListCatGroupFiles(ctx, e.g.ID)
	for _, f := range files {
		if f.Path == "backend/api.go" && f.LastMemberID != e.mem["前端"].ID {
			t.Fatalf("removed member id must stay: %+v", f)
		}
	}
	// 群不存在 → NOT_FOUND group_not_found
	_, err = e.svc.ListCatGroupFiles(ctx, "nope")
	var ae *apperr.AppError
	if !errors.As(err, &ae) || ae.Code != apperr.NotFound || !strings.Contains(ae.Detail, "group_not_found") {
		t.Fatalf("missing group err %v", err)
	}
	// 空群返回 [] 不是 null
	g2, _ := e.svc.CreateCatGroup(ctx, cat.CreateGroupRequest{ProjectID: e.g.ProjectID, Name: "空"})
	empty, err := e.svc.ListCatGroupFiles(ctx, g2.ID)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty %#v %v", empty, err)
	}
}

// ③ 派活判断耗时：GetCatRouteDetail startedAt/finishedAt/durationMs；routed 事件和灰字 route 带 durationMs；
// mention 不跑判断没有耗时；旧数据（两列 0）省略 durationMs。
func TestGroupV0556_DispatchTiming(t *testing.T) {
	e := newGroupEnv(t, "前端", "后端")
	ctx := context.Background()
	e.reply("前端", report("前端做完", "", ""))
	e.reply("后端", report("后端做完", "", ""))
	ans, _ := json.Marshal(map[string]string{"memberId": e.mem["后端"].ID, "reason": "接口归后端"})
	e.reply("route", string(ans))
	e.file("route", "sleep", "3") // ≥ 300ms
	before := time.Now().UnixMilli()
	chain := e.send("加个接口")
	e.waitIdle()
	after := time.Now().UnixMilli()
	ms := find(e.msgs(), func(m cat.GroupMessage) bool { return m.ChainID == chain })
	gl := find(ms, func(m cat.GroupMessage) bool { return m.Text == "交给 后端：接口归后端" })
	if len(gl) != 1 || gl[0].Route == nil || gl[0].Route.RouteSeq == 0 || gl[0].Route.DurationMs < 300 {
		t.Fatalf("ai gray line route %+v:\n%s", gl, e.dump())
	}
	d, err := e.svc.GetCatRouteDetail(ctx, cat.GetRouteDetailRequest{GroupID: e.g.ID, Seq: gl[0].Seq})
	if err != nil {
		t.Fatal(err)
	}
	if d.StartedAt < before || d.FinishedAt > after || d.FinishedAt <= d.StartedAt || d.DurationMs == nil ||
		*d.DurationMs != d.FinishedAt-d.StartedAt || *d.DurationMs < 300 || *d.DurationMs != gl[0].Route.DurationMs {
		t.Fatalf("detail timing %+v dur=%v", d, d.DurationMs)
	}
	var routed []cat.GroupRouteEvent
	for _, ev := range e.routeEvents() {
		if ev.ChainID == chain && ev.State == "routed" {
			routed = append(routed, ev)
		}
	}
	if len(routed) != 1 || routed[0].Route.DurationMs != *d.DurationMs {
		t.Fatalf("routed events %+v", routed)
	}
	for _, ev := range e.routeEvents() {
		if ev.State == "routing" && ev.Route.DurationMs != 0 {
			t.Fatalf("routing event must not carry duration %+v", ev)
		}
	}

	// 判断返回无效 → undecided 也有耗时，不起成员
	e.file("route", "sleep", "1")
	e.reply("route", `{"memberId":"nobody","reason":"x"}`)
	chain = e.send("随便")
	e.waitIdle()
	fb := find(find(e.msgs(), func(m cat.GroupMessage) bool { return m.ChainID == chain }), func(m cat.GroupMessage) bool {
		return m.Route != nil && m.Route.Mode == "undecided"
	})
	if len(fb) != 1 || fb[0].Route.DurationMs <= 0 {
		t.Fatalf("undecided timing %+v", fb)
	}
	if fd, err := e.svc.GetCatRouteDetail(ctx, cat.GetRouteDetailRequest{GroupID: e.g.ID, Seq: fb[0].Seq}); err != nil || fd.DurationMs == nil || fd.StartedAt == 0 {
		t.Fatalf("undecided detail %+v %v", fd, err)
	}

	// mention：不跑判断，没有耗时
	chain = e.send("直接", "前端")
	e.waitIdle()
	for _, m := range find(e.msgs(), func(m cat.GroupMessage) bool { return m.ChainID == chain }) {
		if m.Route != nil && (m.Route.DurationMs != 0 || m.Route.RouteSeq != 0) {
			t.Fatalf("mention must not have timing %+v", m.Route)
		}
	}

	// 旧数据：两列为 0 → durationMs 省略
	old, err := e.st.InsertCatGroupRoute(ctx, store.CatGroupRouteDetail{GroupID: e.g.ID, ChainID: "old", Mode: "ai", InputText: "i", OutputText: "o"})
	if err != nil {
		t.Fatal(err)
	}
	om, err := e.st.InsertCatGroupMessage(ctx, store.CatGroupMessage{GroupID: e.g.ID, Kind: "system", Text: "交给 前端：旧", ChainID: "old",
		SystemKind: "route", Route: &store.CatRouteInfo{Mode: "ai", MemberID: e.mem["前端"].ID, RouteSeq: old.RouteSeq}})
	if err != nil {
		t.Fatal(err)
	}
	od, err := e.svc.GetCatRouteDetail(ctx, cat.GetRouteDetailRequest{GroupID: e.g.ID, Seq: om.Seq})
	if err != nil || od.StartedAt != 0 || od.FinishedAt != 0 || od.DurationMs != nil {
		t.Fatalf("old detail %+v %v", od, err)
	}
	raw, _ := json.Marshal(od)
	if strings.Contains(string(raw), "durationMs") || !strings.Contains(string(raw), `"startedAt":0`) {
		t.Fatalf("old json %s", raw)
	}
}

// ④ 排队：同一成员在跑时再 @ 他 → 入队；灰字 route.queued=true 「交给 {成员}：正在干活，排在后面」；
// queue[].queuePosition 从 0 起；入队 / 出队推 cat:group-member；轮到开跑不发灰字；不同成员不排队（并行）。
func TestGroupV0556_QueueSameMember(t *testing.T) {
	e := newGroupEnv(t, "甲", "乙")
	ctx := context.Background()
	e.reply("甲", report("甲做完", "", ""))
	e.reply("乙", report("乙做完", "", ""))
	e.file("甲", "sleep", "8")

	c1 := e.send("第一件", "甲")
	c2 := e.send("第二件", "甲")
	c3 := e.send("第三件", "甲")
	c4 := e.send("乙的活", "乙") // 别的成员：并行，不排队

	m := e.member("甲")
	if m.Status != "running" || m.QueueLength != 2 || len(m.Queue) != 2 {
		t.Fatalf("甲 while busy %+v", m)
	}
	userSeq := map[string]int64{}
	for _, x := range e.msgs() {
		if x.Kind == "user" {
			userSeq[x.ChainID] = x.Seq
		}
	}
	if m.Queue[0].ChainID != c2 || m.Queue[0].QueuePosition != 0 || m.Queue[0].FromSeq != userSeq[c2] || m.Queue[0].Hop != 0 ||
		m.Queue[1].ChainID != c3 || m.Queue[1].QueuePosition != 1 || m.Queue[1].FromSeq != userSeq[c3] {
		t.Fatalf("queue %+v", m.Queue)
	}
	d, _ := e.svc.GetCatGroup(ctx, e.g.ID)
	for _, x := range d.Members {
		if x.ID == e.mem["甲"].ID && x.QueueLength != 2 {
			t.Fatalf("GetCatGroup member %+v", x)
		}
	}
	ql := queuedLines(e.msgs())
	if len(ql) != 2 {
		t.Fatalf("queued lines:\n%s", e.dump())
	}
	for i, q := range ql {
		if q.Text != "交给 甲：正在干活，排在后面" || q.Route.Mode != "mention" || q.Route.MemberID != e.mem["甲"].ID ||
			q.ToMemberID != e.mem["甲"].ID || q.ChainID != []string{c2, c3}[i] {
			t.Fatalf("queued line %d %+v route=%+v", i, q, q.Route)
		}
	}
	for _, x := range find(e.msgs(), func(m cat.GroupMessage) bool { return m.ChainID == c4 }) {
		if x.Route != nil && x.Route.Queued {
			t.Fatal("other member must not be queued")
		}
	}

	e.waitIdle()
	for _, c := range []string{c1, c2, c3} {
		if n := len(cardsOf(find(e.msgs(), func(m cat.GroupMessage) bool { return m.ChainID == c }), e.mem["甲"].ID)); n != 1 {
			t.Fatalf("chain %s cards %d:\n%s", c, n, e.dump())
		}
	}
	if n := strings.Count(strings.Join(e.calls(), ","), "甲"); n != 3 {
		t.Fatalf("甲 calls %d", n)
	}
	// 轮到开跑不发灰字：除了入队那两句，没有任何别的 route 灰字
	if r := find(e.msgs(), sys("route")); len(r) != 2 {
		t.Fatalf("extra route lines:\n%s", e.dump())
	}
	// 事件：入队推 queueLength 1、2；出队开跑推 running 且 queueLength 1、0
	evs := e.memberEvents("甲")
	var seen []string
	for _, ev := range evs {
		if ev.QueueLength != ev.Member.QueueLength || ev.QueueLength != len(ev.Queue) {
			t.Fatalf("event mismatch %+v", ev)
		}
		for i, q := range ev.Queue {
			if q.QueuePosition != i {
				t.Fatalf("event queue pos %+v", ev.Queue)
			}
		}
		tag := "q"
		if ev.Running != nil {
			tag = "r"
		}
		seen = append(seen, tag+itoa(ev.QueueLength))
	}
	joined := strings.Join(seen, ",")
	for _, want := range []string{"r1", "r2", "r0"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("member events %s missing %s", joined, want)
		}
	}
	m = e.member("甲")
	if m.Status != "idle" || m.QueueLength != 0 || m.Queue == nil || len(m.Queue) != 0 {
		t.Fatalf("甲 after %+v", m)
	}
}

// 停单人清空队列并推一次；排队的任务不再跑。
func TestGroupV0556_StopClearsQueue(t *testing.T) {
	e := newGroupEnv(t, "甲")
	ctx := context.Background()
	e.reply("甲", report("甲做完", "", ""))
	e.file("甲", "sleep", "100")
	e.send("一", "甲")
	e.send("二", "甲")
	e.send("三", "甲")
	if m := e.member("甲"); m.QueueLength != 2 {
		t.Fatalf("queue %+v", m)
	}
	e.waitFor(func([]cat.GroupMessage) bool { return strings.Contains(strings.Join(e.calls(), ","), "甲") })
	n0 := len(e.memberEvents("甲"))
	if err := e.svc.StopCatGroupMember(ctx, e.mem["甲"].ID); err != nil {
		t.Fatal(err)
	}
	evs := e.memberEvents("甲")[n0:]
	if len(evs) == 0 || evs[0].QueueLength != 0 || len(evs[0].Queue) != 0 || evs[0].Running == nil {
		t.Fatalf("clear event %+v", evs)
	}
	e.waitIdle()
	if n := strings.Count(strings.Join(e.calls(), ","), "甲"); n != 1 {
		t.Fatalf("queued tasks must be cancelled, calls %d", n)
	}
	if m := e.member("甲"); m.Status != "idle" || m.QueueLength != 0 {
		t.Fatalf("after stop %+v", m)
	}
	// 停完再 @ 他：直接开跑，不排队
	e.file("甲", "sleep", "0")
	chain := e.send("再来", "甲")
	e.waitIdle()
	ms := find(e.msgs(), func(m cat.GroupMessage) bool { return m.ChainID == chain })
	if len(queuedLines(ms)) != 0 || len(cardsOf(ms, e.mem["甲"].ID)) != 1 {
		t.Fatalf("after stop:\n%s", e.dump())
	}
}

// 接力 @ 到正在跑的人也排队（mode=mention，hop 入队时已定）；灰字文案跟随界面语言（en / ja）。
func TestGroupV0556_QueueRelayAndLocale(t *testing.T) {
	e := newGroupEnv(t, "甲", "乙")
	prev := i18n.Current()
	t.Cleanup(func() { i18n.Set(prev) })
	e.reply("甲", report("甲做完", "", "@乙 接着"))
	e.reply("乙", report("乙做完", "", ""))
	e.file("乙", "sleep", "8")
	busy := e.send("乙先忙", "乙")
	i18n.Set(i18n.LangEn)
	chain := e.send("甲做完给乙", "甲")
	e.waitIdle()
	ms := find(e.msgs(), func(m cat.GroupMessage) bool { return m.ChainID == chain })
	ql := queuedLines(ms)
	if len(ql) != 1 || ql[0].Text != "Assigned to 乙: still working, added to the queue" || ql[0].Route.Mode != "mention" || ql[0].Hop != 1 {
		t.Fatalf("relay queued:\n%s", e.dump())
	}
	c := cardsOf(ms, e.mem["乙"].ID)
	if len(c) != 1 || c[0].Hop != 2 {
		t.Fatalf("queued relay hop:\n%s", e.dump())
	}
	if len(cardsOf(find(e.msgs(), func(m cat.GroupMessage) bool { return m.ChainID == busy }), e.mem["乙"].ID)) != 1 {
		t.Fatal("first 乙 task")
	}
	for _, lang := range []string{i18n.LangZhCN, i18n.LangEn, i18n.LangJa} {
		s := i18n.Translate(lang, i18n.KeyCatGroupRouteQueued, map[string]string{"name": "X"})
		if !strings.Contains(s, "X") || strings.Contains(s, "{name}") {
			t.Fatalf("%s copy %q", lang, s)
		}
	}
}

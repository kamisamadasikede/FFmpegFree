package cat_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/service/cat"
	"FFmpegFree/internal/store"
)

type evRec struct {
	mu     sync.Mutex
	events []string
	proj   []catagent.ProjectEvent
}

func (r *evRec) emit(event string, payload any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
	if event == catagent.EventProject {
		r.proj = append(r.proj, payload.(catagent.ProjectEvent))
	}
}

func (r *evRec) snapshot() ([]string, []catagent.ProjectEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.events...), append([]catagent.ProjectEvent(nil), r.proj...)
}

type notReadyAdapter struct{ fakeAdapter }

func (notReadyAdapter) Status() catagent.Status { return catagent.Status{State: catagent.StateMissing} }

func newProjSvc(t *testing.T, st *store.Store, a catagent.Adapter, cfg cat.Config) (*cat.Service, *evRec) {
	t.Helper()
	reg := catagent.NewRegistry()
	if a != nil {
		reg.Register(a)
	}
	rec := &evRec{}
	cfg.Store, cfg.Registry, cfg.Emit = st, reg, rec.emit
	return cat.New(cfg), rec
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func reasonOf(err error) string {
	ae := apperr.From(err)
	if ae == nil {
		return ""
	}
	return strings.SplitN(ae.Detail, "\n", 2)[0]
}

func TestCreateCatProjectValidationAndDedup(t *testing.T) {
	ctx := context.Background()
	svc, _ := newProjSvc(t, openStore(t), nil, cat.Config{})
	dir := filepath.Join(t.TempDir(), "我的 项目(1)")
	_ = os.MkdirAll(dir, 0o755)
	file := filepath.Join(dir, "f.txt")
	_ = os.WriteFile(file, []byte("x"), 0o644)

	for _, p := range []string{"", "rel/dir", filepath.Join(dir, "nope"), file} {
		if _, err := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: p}); !apperr.Is(err, apperr.InvalidArgument) || reasonOf(err) != "reason=project_path" {
			t.Fatalf("%q: %v", p, err)
		}
	}
	if _, err := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: string(filepath.Separator)}); reasonOf(err) != "reason=project_root" {
		t.Fatalf("根: %v", err)
	}
	if _, err := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: dir, Name: strings.Repeat("名", 61)}); reasonOf(err) != "reason=project_name" {
		t.Fatalf("名字太长: %v", err)
	}
	if _, err := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: dir, Name: "a\nb"}); reasonOf(err) != "reason=project_name" {
		t.Fatalf("控制字符: %v", err)
	}
	res, err := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: "  " + dir + "  "})
	if err != nil || res.Existed || res.Project.Name != "我的 项目(1)" || res.Project.Path != dir || res.Project.Missing ||
		res.Project.CreatedAt == 0 || res.Project.CreatedAt != res.Project.UpdatedAt {
		t.Fatalf("%+v %v", res, err)
	}
	// 末尾带分隔符、给了别的（甚至非法的）名字：返回已有项目，不新建不改名
	again, err := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: dir + string(filepath.Separator), Name: strings.Repeat("x", 99)})
	if err != nil || !again.Existed || again.Project.ID != res.Project.ID || again.Project.Name != res.Project.Name {
		t.Fatalf("%+v %v", again, err)
	}
	other := filepath.Join(t.TempDir(), "我的 项目(1)")
	_ = os.MkdirAll(other, 0o755)
	o, err := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: other, Name: " 自定义 "})
	if err != nil || o.Existed || o.Project.Name != "自定义" {
		t.Fatalf("%+v %v", o, err)
	}
	list, err := svc.ListCatProjects(ctx)
	if err != nil || len(list) != 2 {
		t.Fatalf("%v %v", list, err)
	}
}

func TestListCatProjectsEmptyIsArray(t *testing.T) {
	svc, _ := newProjSvc(t, openStore(t), nil, cat.Config{})
	list, err := svc.ListCatProjects(context.Background())
	if err != nil || list == nil || len(list) != 0 {
		t.Fatalf("%#v %v", list, err)
	}
}

func TestRenameCatProject(t *testing.T) {
	ctx := context.Background()
	svc, _ := newProjSvc(t, openStore(t), nil, cat.Config{})
	dir := t.TempDir()
	res, _ := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: dir})
	for _, n := range []string{"", "   ", strings.Repeat("字", 61), "a\rb"} {
		if _, err := svc.RenameCatProject(ctx, cat.RenameProjectRequest{ID: res.Project.ID, Name: n}); reasonOf(err) != "reason=project_name" {
			t.Fatalf("%q: %v", n, err)
		}
	}
	if _, err := svc.RenameCatProject(ctx, cat.RenameProjectRequest{ID: "nope", Name: "x"}); !apperr.Is(err, apperr.NotFound) {
		t.Fatal(err)
	}
	if _, err := svc.RenameCatProject(ctx, cat.RenameProjectRequest{Name: "x"}); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	_ = os.RemoveAll(dir) // missing 的项目也能改名
	p, err := svc.RenameCatProject(ctx, cat.RenameProjectRequest{ID: res.Project.ID, Name: " " + strings.Repeat("字", 60) + " "})
	if err != nil || p.Name != strings.Repeat("字", 60) || !p.Missing || p.UpdatedAt <= res.Project.UpdatedAt || p.CreatedAt != res.Project.CreatedAt {
		t.Fatalf("%+v %v", p, err)
	}
}

func fileList(t *testing.T, root string) []string {
	var out []string
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err == nil {
			out = append(out, p)
		}
		return nil
	})
	sort.Strings(out)
	return out
}

func TestDeleteCatProjectKeepsFilesAndRemovesConversations(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)
	block := make(chan struct{})
	started := make(chan struct{}, 1)
	a := &fakeAdapter{run: func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		started <- struct{}{}
		o.OnTextDelta("半句")
		select {
		case <-o.Ctx.Done():
			return catagent.TurnResponse{}, o.Ctx.Err()
		case <-block:
			return catagent.TurnResponse{}, nil
		}
	}}
	svc, rec := newProjSvc(t, st, a, cat.Config{})
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "src"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "src", "main.go"), []byte("package main"), 0o644)
	before := fileList(t, dir)

	p, _ := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: dir})
	inConv, err := svc.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild, ProjectID: p.Project.ID})
	if err != nil || inConv.ProjectID != p.Project.ID {
		t.Fatalf("%+v %v", inConv, err)
	}
	loose, _ := svc.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild})
	if _, err := svc.SendCatMessage(ctx, cat.SendMessageRequest{ConversationID: inConv.ID, Content: "hi"}); err != nil {
		t.Fatal(err)
	}
	<-started
	if err := svc.DeleteCatProject(ctx, cat.DeleteProjectRequest{ID: p.Project.ID}); err != nil {
		t.Fatal(err)
	}
	close(block)
	svc.Wait()
	events, _ := rec.snapshot()
	if !strings.Contains(strings.Join(events, ","), catagent.EventTurn) {
		t.Fatalf("进行中的一轮应先被取消: %v", events)
	}
	if _, err := svc.GetCatConversation(ctx, inConv.ID); !apperr.Is(err, apperr.NotFound) {
		t.Fatalf("项目下对话应删除: %v", err)
	}
	if _, err := svc.GetCatConversation(ctx, loose.ID); err != nil {
		t.Fatalf("「对话」下的不受影响: %v", err)
	}
	if after := fileList(t, dir); strings.Join(after, "|") != strings.Join(before, "|") {
		t.Fatalf("删除项目不应动文件夹: %v → %v", before, after)
	}
	list, _ := svc.ListCatProjects(ctx)
	if len(list) != 0 {
		t.Fatal(list)
	}
	if err := svc.DeleteCatProject(ctx, cat.DeleteProjectRequest{ID: p.Project.ID}); err != nil {
		t.Fatalf("未知 id 幂等: %v", err)
	}
	if err := svc.DeleteCatProject(ctx, cat.DeleteProjectRequest{}); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatal(err)
	}
}

func TestProjectMissingEventsAndRecovery(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)
	dir := filepath.Join(t.TempDir(), "p")
	_ = os.MkdirAll(dir, 0o755)
	svc, rec := newProjSvc(t, st, nil, cat.Config{})
	p, _ := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: dir})

	list, _ := svc.ListCatProjects(ctx)
	if list[0].Missing {
		t.Fatal("应不缺")
	}
	if _, pe := rec.snapshot(); len(pe) != 0 {
		t.Fatalf("同一结果不发: %v", pe)
	}
	_ = os.RemoveAll(dir)
	list, _ = svc.ListCatProjects(ctx)
	_, _ = svc.ListCatProjects(ctx)
	if !list[0].Missing {
		t.Fatal("应缺")
	}
	_ = os.WriteFile(dir, []byte("not a dir"), 0o644) // 同路径是文件也算缺
	_, _ = svc.ListCatProjects(ctx)
	_ = os.Remove(dir)
	_ = os.MkdirAll(dir, 0o755)
	list, _ = svc.ListCatProjects(ctx)
	if list[0].Missing {
		t.Fatal("文件夹回来应自动恢复")
	}
	_, pe := rec.snapshot()
	want := []catagent.ProjectEvent{{ID: p.Project.ID, Missing: true}, {ID: p.Project.ID, Missing: false}}
	if len(pe) != 2 || pe[0] != want[0] || pe[1] != want[1] {
		t.Fatalf("事件不对: %+v", pe)
	}

	// 进程重启（新服务实例）后第一次计算只记录、不发事件
	_ = os.RemoveAll(dir)
	svc2, rec2 := newProjSvc(t, st, nil, cat.Config{})
	list, _ = svc2.ListCatProjects(ctx)
	if !list[0].Missing {
		t.Fatal("应缺")
	}
	if _, pe := rec2.snapshot(); len(pe) != 0 {
		t.Fatalf("首次计算不发事件: %v", pe)
	}
}

func TestProjectStatTimeoutCountsAsMissingAndParallel(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)
	var slow sync.Map
	stat := func(p string) (os.FileInfo, error) {
		if _, ok := slow.Load(p); ok {
			time.Sleep(400 * time.Millisecond)
		}
		return os.Stat(p)
	}
	svc, _ := newProjSvc(t, st, nil, cat.Config{Stat: stat, StatTimeout: 100 * time.Millisecond})
	var dirs []string
	for i := 0; i < 4; i++ {
		d := t.TempDir()
		if _, err := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: d}); err != nil {
			t.Fatal(err)
		}
		dirs = append(dirs, d)
	}
	for _, d := range dirs {
		slow.Store(d, true)
	}
	start := time.Now()
	list, err := svc.ListCatProjects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if el := time.Since(start); el > 350*time.Millisecond {
		t.Fatalf("各项目应并行、单个超时即止: %v", el)
	}
	for _, p := range list {
		if !p.Missing {
			t.Fatalf("超时应按缺: %+v", p)
		}
	}
}

func TestCreateConversationWithProject(t *testing.T) {
	ctx := context.Background()
	svc, _ := newProjSvc(t, openStore(t), nil, cat.Config{})
	dir := t.TempDir()
	p, _ := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: dir})
	if _, err := svc.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: "cat_cli", ProjectID: "nope"}); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("agentKind 校验先于项目: %v", err)
	}
	if _, err := svc.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild, ProjectID: "nope"}); !apperr.Is(err, apperr.NotFound) {
		t.Fatal(err)
	}
	c, err := svc.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild, ProjectID: p.Project.ID, ProjectPath: "/ignored"})
	if err != nil || c.ProjectID != p.Project.ID || c.ProjectPath != "" {
		t.Fatalf("%+v %v", c, err)
	}
	list, _ := svc.ListCatConversations(ctx)
	if len(list) != 1 || list[0].ProjectID != p.Project.ID {
		t.Fatalf("%+v", list)
	}
	_ = os.RemoveAll(dir)
	_, err = svc.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild, ProjectID: p.Project.ID})
	if !apperr.Is(err, apperr.CatProjectMissing) || apperr.From(err).Message != "项目文件夹不见了。" {
		t.Fatal(err)
	}
	if list, _ := svc.ListCatConversations(ctx); len(list) != 1 {
		t.Fatal("缺失时不应创建对话")
	}
	// 缺失项目下的对话照常能看
	if _, err := svc.GetCatConversation(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
}

func TestSendCheckOrderAndSandboxRoot(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)
	dir := t.TempDir()

	// 未就绪优先于项目缺失
	nr, _ := newProjSvc(t, st, &notReadyAdapter{}, cat.Config{})
	p, _ := nr.CreateCatProject(ctx, cat.CreateProjectRequest{Path: dir})
	conv, _ := nr.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild, ProjectID: p.Project.ID})
	loose, _ := nr.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild})
	if _, err := nr.SendCatMessage(ctx, cat.SendMessageRequest{ConversationID: "nope", Content: "hi"}); !apperr.Is(err, apperr.NotFound) {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var roots []string
	a := &fakeAdapter{run: func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		mu.Lock()
		roots = append(roots, o.ProjectPath)
		mu.Unlock()
		return catagent.TurnResponse{Message: catagent.WireMessage{Role: "assistant", Content: "好"}}, nil
	}}
	svc, rec := newProjSvc(t, st, a, cat.Config{})
	if _, err := svc.ListCatProjects(ctx); err != nil { // 先记录一次 missing=false
		t.Fatal(err)
	}
	if _, err := svc.SendCatMessage(ctx, cat.SendMessageRequest{ConversationID: conv.ID, Content: "1", ProjectPath: "/evil"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SendCatMessage(ctx, cat.SendMessageRequest{ConversationID: loose.ID, Content: "2", ProjectPath: "/evil"}); err != nil {
		t.Fatal(err)
	}
	svc.Wait()
	sort.Strings(roots)
	if len(roots) != 2 || roots[0] != "" || roots[1] != dir {
		t.Fatalf("沙箱根只来自所属项目（req.projectPath 忽略）: %q", roots)
	}

	_ = os.RemoveAll(dir)
	if _, err := nr.SendCatMessage(ctx, cat.SendMessageRequest{ConversationID: conv.ID, Content: "hi"}); !apperr.Is(err, apperr.CatNotReady) {
		t.Fatalf("CAT_NOT_READY 优先: %v", err)
	}
	before, _ := svc.GetCatConversation(ctx, conv.ID)
	evBefore, _ := rec.snapshot()
	_, err := svc.SendCatMessage(ctx, cat.SendMessageRequest{ConversationID: conv.ID, Content: "hi"})
	if !apperr.Is(err, apperr.CatProjectMissing) {
		t.Fatal(err)
	}
	after, _ := svc.GetCatConversation(ctx, conv.ID)
	if len(after.Messages) != len(before.Messages) {
		t.Fatal("缺失时不应存用户消息")
	}
	evAfter, pe := rec.snapshot()
	newEv := evAfter[len(evBefore):]
	if len(newEv) != 1 || newEv[0] != catagent.EventProject || len(pe) != 1 || !pe[0].Missing {
		t.Fatalf("只应发 cat:project 变缺，不发 cat:message / cat:turn: %v %+v", newEv, pe)
	}
	_, err = svc.SendCatMessage(ctx, cat.SendMessageRequest{ConversationID: conv.ID, Content: "hi"})
	if !apperr.Is(err, apperr.CatProjectMissing) {
		t.Fatal(err)
	}
	if _, pe := rec.snapshot(); len(pe) != 1 {
		t.Fatal("同一结果不重复发")
	}
}

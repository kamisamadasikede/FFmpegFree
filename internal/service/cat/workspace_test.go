package cat_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/service/cat"
	"FFmpegFree/internal/store"
)

type workspaceHistory struct{ transcriptFake }

func (*workspaceHistory) SessionLabel(string, string) string                   { return "Native title" }
func (*workspaceHistory) ReadContextUsage(string, string) (int64, int64, bool) { return 0, 0, false }

func TestWorkspaceConversationLifecycleAndExport(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)
	a := &workspaceHistory{transcriptFake: transcriptFake{wires: []catagent.WireMessage{{Role: "user", Content: "你好\n第二行"}, {Role: "assistant", Content: "## 内容\n\n```go\nfmt.Println(1)\n```"}}}}
	svc, _ := newProjSvc(t, st, a, cat.Config{})
	c, e := st.InsertCatConversation(ctx, store.CatConversation{ID: "metadata", Title: "Stored title", AgentKind: catagent.KindCatBuild})
	if e != nil {
		t.Fatal(e)
	}
	u, e := svc.UpdateCatConversation(ctx, cat.ConversationUpdate{ConvID: c.ID, Action: "rename", Title: "  我的对话  "})
	if e != nil || u.Title != "我的对话" {
		t.Fatalf("%+v %v", u, e)
	}
	var wg sync.WaitGroup
	for _, action := range []string{"pin", "archive"} {
		wg.Add(1)
		go func(action string) {
			defer wg.Done()
			if _, err := svc.UpdateCatConversation(ctx, cat.ConversationUpdate{ConvID: c.ID, Action: action, Enabled: true}); err != nil {
				t.Error(err)
			}
		}(action)
	}
	wg.Wait()
	// A fresh Service must retain metadata, despite the provider's different title.
	restarted, _ := newProjSvc(t, st, a, cat.Config{})
	list, e := restarted.ListCatConversations(ctx)
	if e != nil || len(list) != 1 || list[0].Title != "我的对话" || !list[0].Pinned || !list[0].Archived {
		t.Fatalf("%+v %v", list, e)
	}
	d, e := restarted.GetCatConversation(ctx, c.ID)
	if e != nil || d.Title != "我的对话" || len(d.Messages) != 2 {
		t.Fatalf("%+v %v", d, e)
	}
	md, e := restarted.PrepareCatConversationExport(ctx, c.ID, "md")
	if e != nil || !strings.Contains(md.Content, a.wires[0].Content) || !strings.Contains(md.Content, a.wires[1].Content) {
		t.Fatalf("%+v %v", md, e)
	}
	js, e := restarted.PrepareCatConversationExport(ctx, c.ID, "json")
	var exported cat.ConversationDetail
	if e != nil || json.Unmarshal([]byte(js.Content), &exported) != nil || exported.Messages[1].Content != a.wires[1].Content {
		t.Fatalf("%+v %v", js, e)
	}
	if _, e = restarted.PrepareCatConversationExport(ctx, c.ID, "html"); !apperr.Is(e, apperr.InvalidArgument) {
		t.Fatalf("%v", e)
	}
	if _, e = restarted.UpdateCatConversation(ctx, cat.ConversationUpdate{ConvID: c.ID, Action: "rename", Title: "bad\nname"}); !apperr.Is(e, apperr.InvalidArgument) {
		t.Fatalf("%v", e)
	}
	u, e = restarted.UpdateCatConversation(ctx, cat.ConversationUpdate{ConvID: c.ID, Action: "reset_title"})
	if e != nil || u.Title != "Native title" {
		t.Fatalf("%+v %v", u, e)
	}
	if _, e = restarted.UpdateCatConversation(ctx, cat.ConversationUpdate{ConvID: c.ID, Action: "archive", Enabled: false}); e != nil {
		t.Fatal(e)
	}
	u, _ = st.GetCatConversation(ctx, c.ID)
	if u.Archived || !u.Pinned {
		t.Fatal("restore overwrote pin")
	}
	a.wires = nil
	// SQLite message rows are not a substitute for a missing CLI transcript.
	_, _ = st.InsertCatMessage(ctx, c.ID, store.CatMessage{Role: "user", Content: "cached only"})
	if _, e = restarted.PrepareCatConversationExport(ctx, c.ID, "md"); !apperr.Is(e, apperr.NotFound) {
		t.Fatalf("%v", e)
	}
}

func TestWorkspacePromptsConcurrentPersistence(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)
	svc, _ := newProjSvc(t, st, nil, cat.Config{})
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, e := svc.SaveCatPrompt(ctx, cat.CatPrompt{Name: fmt.Sprint("Prompt ", i), Content: "Text\nwith `code`"}); e != nil {
				t.Error(e)
			}
		}(i)
	}
	wg.Wait()
	w, e := svc.GetCatWorkspace(ctx, "")
	if e != nil || len(w.Prompts) != 12 {
		t.Fatalf("%+v %v", w, e)
	}
	p := w.Prompts[0]
	p.Content = "updated"
	if _, e = svc.SaveCatPrompt(ctx, p); e != nil {
		t.Fatal(e)
	}
	if e = svc.DeleteCatPrompt(ctx, p.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = svc.SaveCatPrompt(ctx, p); !apperr.Is(e, apperr.NotFound) {
		t.Fatalf("resurrected deleted prompt: %v", e)
	}
	other, _ := newProjSvc(t, st, nil, cat.Config{})
	w, e = other.GetCatWorkspace(ctx, "")
	if e != nil || len(w.Prompts) != 11 {
		t.Fatalf("%+v %v", w, e)
	}
	if _, e = svc.SaveCatPrompt(ctx, cat.CatPrompt{Name: "", Content: "text"}); !apperr.Is(e, apperr.InvalidArgument) {
		t.Fatalf("%v", e)
	}
}

func TestWorkspaceRulesScopedToEveryTurn(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)
	root := t.TempDir()
	home := t.TempDir()
	var got []string
	a := &fakeAdapter{run: func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		got = append(got, o.Rules)
		return catagent.TurnResponse{}, nil
	}}
	svc, _ := newProjSvc(t, st, a, cat.Config{DataRoot: root, CLIHome: home})
	p, e := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: t.TempDir()})
	if e != nil {
		t.Fatal(e)
	}
	q, e := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: t.TempDir()})
	if e != nil {
		t.Fatal(e)
	}
	_, e = svc.SaveCatSettings(ctx, cat.Settings{CustomInstruction: "GLOBAL"})
	if e != nil {
		t.Fatal(e)
	}
	if e = svc.SaveCatProjectRules(ctx, cat.ProjectRulesRequest{ProjectID: p.Project.ID, Rules: "  ONLY P  "}); e != nil {
		t.Fatal(e)
	}
	if e = svc.SaveCatProjectRules(ctx, cat.ProjectRulesRequest{ProjectID: "missing", Rules: "bad"}); e == nil {
		t.Fatal("missing project accepted")
	}
	for _, pid := range []string{p.Project.ID, q.Project.ID, "", p.Project.ID} {
		c, e := svc.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild, ProjectID: pid})
		if e != nil {
			t.Fatal(e)
		}
		if _, e = svc.SendCatMessage(ctx, cat.SendMessageRequest{ConversationID: c.ID, Content: "hi"}); e != nil {
			t.Fatal(e)
		}
		svc.Wait()
	}
	if len(got) != 4 || !strings.Contains(got[0], "GLOBAL") || !strings.Contains(got[0], "ONLY P") || got[1] != "GLOBAL" || got[2] != "GLOBAL" || got[3] != got[0] {
		t.Fatalf("%q", got)
	}
	w, e := svc.GetCatWorkspace(ctx, p.Project.ID)
	if e != nil || w.Rules != "ONLY P" {
		t.Fatalf("%+v %v", w, e)
	}
}

func TestWorkspaceContextSearchScopeLimitsAndCancellation(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)
	svc, _ := newProjSvc(t, st, nil, cat.Config{})
	root := t.TempDir()
	outside := t.TempDir()
	sharedFile(t, filepath.Join(root, "src", "hello.ts"), "text")
	sharedFile(t, filepath.Join(root, "node_modules", "hello.ts"), "skip")
	sharedFile(t, filepath.Join(root, ".git", "hello.ts"), "skip")
	sharedFile(t, filepath.Join(root, ".env"), "private")
	sharedFile(t, filepath.Join(root, "token.pem"), "private")
	sharedFile(t, filepath.Join(outside, "outside.ts"), "skip")
	// Windows junctions require no elevated symlink permission.
	target := catagent.NativeShareTarget{AgentKind: catagent.KindClaudeCLI, SkillsPath: filepath.Join(root, "linked"), ConfigPath: filepath.Join(t.TempDir(), "config.json"), Format: "json"}
	manifest := catagent.NativeShareManifest{}
	linked, _ := catagent.SyncNativeShare(target, map[string]string{"outside": outside}, nil, manifest["fixture"], true)
	if linked.SkillCount != 1 {
		t.Fatalf("junction fixture: %+v", linked)
	}
	p, e := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: root})
	if e != nil {
		t.Fatal(e)
	}
	r, e := svc.SearchCatContextFiles(ctx, cat.SearchContextRequest{ProjectID: p.Project.ID, Query: "hello"})
	if e != nil || len(r.Files) != 1 || r.Files[0].RelPath != "src/hello.ts" || !filepath.IsAbs(r.Files[0].Path) {
		t.Fatalf("%+v %v", r, e)
	}
	r, e = svc.SearchCatContextFiles(ctx, cat.SearchContextRequest{ProjectID: p.Project.ID})
	if e != nil || len(r.Files) != 1 {
		t.Fatalf("%+v %v", r, e)
	}
	r, e = svc.SearchCatContextFiles(ctx, cat.SearchContextRequest{ProjectID: p.Project.ID, Query: "../"})
	if e != nil || len(r.Files) != 0 {
		t.Fatalf("%+v %v", r, e)
	}
	for i := 0; i < 70; i++ {
		sharedFile(t, filepath.Join(root, fmt.Sprintf("file-%03d.ts", i)), "file")
	}
	r, e = svc.SearchCatContextFiles(ctx, cat.SearchContextRequest{ProjectID: p.Project.ID})
	if e != nil || len(r.Files) != 50 || !r.Truncated {
		t.Fatalf("%+v %v", r, e)
	}
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = svc.SearchCatContextFiles(cancelCtx, cat.SearchContextRequest{ProjectID: p.Project.ID}); e == nil {
		t.Fatal("cancellation ignored")
	}
	if _, e = svc.SearchCatContextFiles(ctx, cat.SearchContextRequest{ConvID: "unknown", ProjectID: p.Project.ID}); !apperr.Is(e, apperr.NotFound) {
		t.Fatalf("invalid conversation fell back to project: %v", e)
	}
	_ = os.Remove(filepath.Join(root, "src", "hello.ts"))
}

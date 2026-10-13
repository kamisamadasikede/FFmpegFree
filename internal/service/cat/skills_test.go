package cat_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/service/cat"
)

func TestInspectNoProject(t *testing.T) {
	svc, _ := newProjSvc(t, openStore(t), &fakeAdapter{}, cat.Config{})
	res, err := svc.InspectCatConfig(context.Background(), cat.InspectRequest{})
	if err != nil || res.Reason != catagent.InspectNoProject {
		t.Fatalf("%+v %v", res, err)
	}
	if res.SkillsRoot != "" || res.McpConfigPath != "" || res.SkillsRootExists {
		t.Fatalf("paths should omit: %+v", res)
	}
	if res.Skills == nil || res.MCPServers == nil {
		t.Fatal("lists must be non-nil")
	}
}

func TestInspectFillsPathFieldsEvenWhenUnsupported(t *testing.T) {
	ctx := context.Background()
	proj := filepath.Join(t.TempDir(), "proj")
	_ = os.MkdirAll(proj, 0o755)
	reg := catagent.NewRegistry()
	reg.Register(&fakeAdapter{})
	reg.Register(catagent.NewStubAdapter(catagent.KindClaudeCLI))
	svc := cat.New(cat.Config{Store: openStore(t), Registry: reg})
	pr, err := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: proj})
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.InspectCatConfig(ctx, cat.InspectRequest{ProjectID: pr.Project.ID, AgentKind: catagent.KindClaudeCLI})
	if err != nil {
		t.Fatal(err)
	}
	wantRoot, wantMCP := catagent.ProjectCatPaths(proj)
	if res.SkillsRoot != wantRoot || res.McpConfigPath != wantMCP {
		t.Fatalf("paths: %+v", res)
	}
	if res.SkillsRootExists {
		t.Fatal("skills dir should not exist yet")
	}
	if res.Reason != catagent.InspectUnsupported && res.Reason != catagent.InspectNotReady {
		// stub returns unsupported via Inspect; fakeAdapter without Inspect → not_ready for build only
		t.Fatalf("reason=%q", res.Reason)
	}
}

func TestInspectListsProjectSkillsFromDisk(t *testing.T) {
	ctx := context.Background()
	proj := filepath.Join(t.TempDir(), "proj")
	skills := filepath.Join(proj, ".ffmpegfree", "cat", "skills")
	_ = os.MkdirAll(filepath.Join(skills, "from-disk"), 0o755)
	svc, _ := newProjSvc(t, openStore(t), &fakeAdapter{}, cat.Config{})
	pr, err := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: proj})
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.InspectCatConfig(ctx, cat.InspectRequest{ProjectID: pr.Project.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !res.SkillsRootExists {
		t.Fatal("expected exists")
	}
	found := false
	for _, it := range res.Skills {
		if it.Name == "from-disk" && it.Source == "project" && it.Enabled {
			found = true
		}
	}
	if !found {
		t.Fatalf("skills=%+v", res.Skills)
	}
}

func TestRevealSkillsDirMissing(t *testing.T) {
	ctx := context.Background()
	proj := filepath.Join(t.TempDir(), "proj")
	_ = os.MkdirAll(proj, 0o755)
	var opened []string
	svc, _ := newProjSvc(t, openStore(t), nil, cat.Config{OpenFolder: func(dir string) error {
		opened = append(opened, dir)
		return nil
	}})
	pr, _ := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: proj})
	res, err := svc.RevealCatSkillsDir(ctx, cat.SkillsDirRequest{ProjectID: pr.Project.ID})
	if err != nil || res.SkillsRootExists || res.Opened {
		t.Fatalf("%+v %v", res, err)
	}
	if len(opened) != 0 {
		t.Fatalf("should not open: %v", opened)
	}
	if _, err := svc.RevealCatSkillsDir(ctx, cat.SkillsDirRequest{}); !apperr.Is(err, apperr.InvalidArgument) || reasonOf(err) != "reason=project" {
		t.Fatalf("%v", err)
	}
}

func TestEnsureCreateSkillsDirMkdirAndOpen(t *testing.T) {
	ctx := context.Background()
	proj := filepath.Join(t.TempDir(), "proj")
	_ = os.MkdirAll(proj, 0o755)
	gitignore := filepath.Join(proj, ".gitignore")
	_ = os.WriteFile(gitignore, []byte("node_modules/\n"), 0o644)
	before, _ := os.ReadFile(gitignore)
	var opened []string
	svc, _ := newProjSvc(t, openStore(t), nil, cat.Config{OpenFolder: func(dir string) error {
		opened = append(opened, dir)
		return nil
	}})
	pr, _ := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: proj})
	res, err := svc.EnsureCreateCatSkillsDir(ctx, cat.SkillsDirRequest{ProjectID: pr.Project.ID})
	if err != nil || !res.Opened || !res.SkillsRootExists {
		t.Fatalf("%+v %v", res, err)
	}
	want, _ := catagent.ProjectCatPaths(proj)
	if res.SkillsRoot != want || len(opened) != 1 || opened[0] != want {
		t.Fatalf("opened=%v res=%+v", opened, res)
	}
	// 不写 mcp.json
	if _, err := os.Stat(res.McpConfigPath); !os.IsNotExist(err) {
		t.Fatalf("mcp.json should not be created: %v", err)
	}
	after, _ := os.ReadFile(gitignore)
	if string(after) != string(before) {
		t.Fatalf("gitignore mutated: %q → %q", before, after)
	}
	// 路径必须在项目下
	if !strings.HasPrefix(res.SkillsRoot, proj) {
		t.Fatalf("escape? %s", res.SkillsRoot)
	}
}

func TestSetInspectItemEnabledValidationAndPersist(t *testing.T) {
	ctx := context.Background()
	proj := filepath.Join(t.TempDir(), "proj")
	skills := filepath.Join(proj, ".ffmpegfree", "cat", "skills")
	_ = os.MkdirAll(filepath.Join(skills, "alpha"), 0o755)
	svc, _ := newProjSvc(t, openStore(t), &fakeAdapter{}, cat.Config{})
	pr, _ := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: proj})

	if _, err := svc.SetCatInspectItemEnabled(ctx, cat.SetInspectItemEnabledRequest{}); !apperr.Is(err, apperr.InvalidArgument) || reasonOf(err) != "reason=project" {
		t.Fatalf("empty project: %v", err)
	}
	if _, err := svc.SetCatInspectItemEnabled(ctx, cat.SetInspectItemEnabledRequest{ProjectID: pr.Project.ID, Category: "nope", Name: "alpha"}); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("bad category: %v", err)
	}
	if _, err := svc.SetCatInspectItemEnabled(ctx, cat.SetInspectItemEnabledRequest{ProjectID: pr.Project.ID, Category: "skill", Name: ""}); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("empty name: %v", err)
	}
	if _, err := svc.SetCatInspectItemEnabled(ctx, cat.SetInspectItemEnabledRequest{ProjectID: pr.Project.ID, Category: "skill", Name: "missing"}); !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("unknown: %v", err)
	}

	item, err := svc.SetCatInspectItemEnabled(ctx, cat.SetInspectItemEnabledRequest{
		ProjectID: pr.Project.ID, Category: "skill", Name: "alpha", Enabled: false,
	})
	if err != nil || item.Name != "alpha" || item.Enabled {
		t.Fatalf("%+v %v", item, err)
	}
	res, err := svc.InspectCatConfig(ctx, cat.InspectRequest{ProjectID: pr.Project.ID})
	if err != nil {
		t.Fatal(err)
	}
	var enabled *bool
	for _, it := range res.Skills {
		if it.Name == "alpha" {
			v := it.Enabled
			enabled = &v
		}
	}
	if enabled == nil || *enabled {
		t.Fatalf("overlay not applied: %+v", res.Skills)
	}
	// 再开回来
	item, err = svc.SetCatInspectItemEnabled(ctx, cat.SetInspectItemEnabledRequest{
		ProjectID: pr.Project.ID, Category: "skill", Name: "alpha", Enabled: true,
	})
	if err != nil || !item.Enabled {
		t.Fatalf("%+v %v", item, err)
	}
}

func TestInspectProjectMissing(t *testing.T) {
	ctx := context.Background()
	proj := filepath.Join(t.TempDir(), "gone")
	_ = os.MkdirAll(proj, 0o755)
	svc, _ := newProjSvc(t, openStore(t), &fakeAdapter{}, cat.Config{})
	pr, _ := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: proj})
	_ = os.RemoveAll(proj)
	_, err := svc.InspectCatConfig(ctx, cat.InspectRequest{ProjectID: pr.Project.ID})
	if !apperr.Is(err, apperr.CatProjectMissing) {
		t.Fatalf("%v", err)
	}
}

func TestSendInjectsSkillsOverlay(t *testing.T) {
	ctx := context.Background()
	proj := filepath.Join(t.TempDir(), "proj")
	_ = os.MkdirAll(proj, 0o755)
	var got catagent.SkillsOverlay
	a := &fakeAdapter{run: func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		got = o.Skills
		return catagent.TurnResponse{Message: catagent.WireMessage{Role: "assistant", Content: "ok"}}, nil
	}}
	svc, rec := newProjSvc(t, openStore(t), a, cat.Config{})
	pr, _ := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: proj})
	c, _ := svc.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild, ProjectID: pr.Project.ID})
	if _, err := svc.SendCatMessage(ctx, cat.SendMessageRequest{ConversationID: c.ID, Content: "hi"}); err != nil {
		t.Fatal(err)
	}
	svc.Wait()
	_ = rec
	wantRoot, wantMCP := catagent.ProjectCatPaths(proj)
	if got.SkillsRoot != wantRoot || got.McpConfigPath != wantMCP || got.AgentKind != catagent.KindCatBuild {
		t.Fatalf("%+v", got)
	}
}

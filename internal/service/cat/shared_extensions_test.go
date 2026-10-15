package cat_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/service/cat"
)

func sharedFile(t *testing.T, path, text string) {
	t.Helper()
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(path, []byte(text), 0600); e != nil {
		t.Fatal(e)
	}
}
func checkSharedSynced(t *testing.T, r cat.SharedExtensions, skills, mcp int) {
	t.Helper()
	if len(r.Targets) != 3 {
		t.Fatalf("targets=%+v", r)
	}
	for _, target := range r.Targets {
		if target.State != "synced" || target.SkillCount != skills || target.MCPCount != mcp {
			t.Fatalf("%+v", target)
		}
	}
}
func sharedSource(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "source")
	sharedFile(t, filepath.Join(p, "SKILL.md"), "---\nname: "+name+"\ndescription: A reusable demo skill\n---\nInstructions.\n")
	sharedFile(t, filepath.Join(p, "scripts", "run.js"), "console.log('asset')")
	return p
}
func TestSharedExtensionsLifecycle(t *testing.T) {
	ctx := context.Background()
	home, root := t.TempDir(), t.TempDir()
	svc, _ := newProjSvc(t, openStore(t), nil, cat.Config{DataRoot: root, CLIHome: home})
	req := cat.SkillsDirRequest{}
	r, e := svc.GetCatSharedExtensions(ctx, req)
	if e != nil {
		t.Fatal(e)
	}
	checkSharedSynced(t, r, 0, 0)
	if _, e = os.Stat(filepath.Join(home, ".codex")); !os.IsNotExist(e) {
		t.Fatal("inspection touched native home")
	}
	r, e = svc.ImportCatSharedSkill(ctx, cat.SharedImportRequest{SourcePath: sharedSource(t, "demo")})
	if e != nil {
		t.Fatal(e)
	}
	checkSharedSynced(t, r, 1, 0)
	b, e := os.ReadFile(filepath.Join(r.SkillsRoot, "demo", "scripts", "run.js"))
	if e != nil || !strings.Contains(string(b), "asset") {
		t.Fatal("missing imported assets")
	}
	raw := `{"mcpServers":{"local":{"command":"test-only-no-launch","args":["one two"],"env":{"TOKEN":"test-secret-only"}},"remote":{"url":"https://example.invalid/mcp","type":"http","headers":{"Authorization":"test-secret-only"}}}}`
	r, e = svc.SaveCatSharedMCP(ctx, cat.SharedMCPRequest{JSON: raw})
	if e != nil {
		t.Fatal(e)
	}
	checkSharedSynced(t, r, 1, 2)
	output, _ := json.Marshal(r)
	if strings.Contains(string(output), "test-secret-only") {
		t.Fatal("secrets in ordinary status")
	}
	r, e = svc.SetCatSharedItemEnabled(ctx, cat.SetInspectItemEnabledRequest{Category: "skill", Name: "demo", Enabled: false})
	if e != nil {
		t.Fatal(e)
	}
	checkSharedSynced(t, r, 0, 2)
	r, e = svc.SetCatSharedItemEnabled(ctx, cat.SetInspectItemEnabledRequest{Category: "mcp", Name: "local", Enabled: false})
	if e != nil {
		t.Fatal(e)
	}
	checkSharedSynced(t, r, 0, 1)
	editor, e := svc.ReadCatSharedMCP(ctx, req)
	if e != nil {
		t.Fatal(e)
	}
	var parsed struct {
		Servers map[string]catagent.SharedMCPServer `json:"mcpServers"`
	}
	if json.Unmarshal([]byte(editor), &parsed) != nil || !parsed.Servers["local"].Disabled {
		t.Fatal("editor ignored toggle")
	}
	r, e = svc.SyncCatSharedExtensions(ctx, req)
	if e != nil {
		t.Fatal(e)
	}
	checkSharedSynced(t, r, 0, 1)
	if _, e = svc.ImportCatSharedSkill(ctx, cat.SharedImportRequest{SourcePath: sharedSource(t, "demo")}); e == nil {
		t.Fatal("overwrote duplicate skill")
	}
}
func TestSharedExtensionsProjectIsolationAndInvalidInput(t *testing.T) {
	ctx := context.Background()
	home, root, project := t.TempDir(), t.TempDir(), t.TempDir()
	svc, _ := newProjSvc(t, openStore(t), nil, cat.Config{DataRoot: root, CLIHome: home})
	pr, e := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: project})
	if e != nil {
		t.Fatal(e)
	}
	pid := pr.Project.ID
	r, e := svc.ImportCatSharedSkill(ctx, cat.SharedImportRequest{ProjectID: pid, SourcePath: sharedSource(t, "project-demo")})
	if e != nil {
		t.Fatal(e)
	}
	checkSharedSynced(t, r, 1, 0)
	for _, target := range r.Targets {
		rel, e := filepath.Rel(project, target.SkillsPath)
		if e != nil || strings.HasPrefix(rel, "..") {
			t.Fatal("project escaped")
		}
	}
	global, e := svc.GetCatSharedExtensions(ctx, cat.SkillsDirRequest{})
	if e != nil || len(global.Skills) != 0 {
		t.Fatal("scope leaked")
	}
	if _, e = os.Stat(filepath.Join(home, ".agents")); !os.IsNotExist(e) {
		t.Fatal("project sync touched global home")
	}
	for _, raw := range []string{
		`{"mcpServers":{"../escape":{"command":"bad"}}}`,
		`{"mcpServers":{"test":{"command":"x","url":"https://example.invalid"}}}`,
		`{"mcpServers":{"test":{"url":"file:///x"}}}`,
		`{"mcpServers":{"test":{"command":"x","unsupported":"silent-drop"}}}`,
		`{"mcpServers":null}`,
		`{"mcpServers":{}} {}`,
	} {
		if _, e := svc.SaveCatSharedMCP(ctx, cat.SharedMCPRequest{JSON: raw}); e == nil {
			t.Fatalf("accepted invalid config: %s", raw)
		}
	}
	if _, e := svc.SetCatSharedItemEnabled(ctx, cat.SetInspectItemEnabledRequest{Category: "skill", Name: "unknown", Enabled: false}); e == nil {
		t.Fatal("unknown toggle accepted")
	}
	if _, e := svc.GetCatSharedExtensions(ctx, cat.SkillsDirRequest{ProjectID: "missing-project"}); e == nil {
		t.Fatal("unknown project accepted")
	}
}
func TestSharedExtensionsRejectsLinkedImport(t *testing.T) {
	ctx := context.Background()
	home, root := t.TempDir(), t.TempDir()
	svc, _ := newProjSvc(t, openStore(t), nil, cat.Config{DataRoot: root, CLIHome: home})
	source := sharedSource(t, "linked-demo")
	external := t.TempDir()
	target := catagent.NativeShareTarget{AgentKind: catagent.KindCodexCLI, SkillsPath: source, ConfigPath: filepath.Join(t.TempDir(), "unused.toml"), Format: "toml"}
	empty := catagent.NativeShareManifest{}
	r, _ := catagent.SyncNativeShare(target, map[string]string{"external": external}, nil, empty[catagent.KindCodexCLI], true)
	if r.SkillCount != 1 {
		t.Fatalf("setup junction failed: %+v", r)
	}
	if _, e := svc.ImportCatSharedSkill(ctx, cat.SharedImportRequest{SourcePath: source}); e == nil {
		t.Fatal("linked files imported")
	}
	if _, e := os.Stat(filepath.Join(root, "cat", "shared", "skills", "linked-demo")); !os.IsNotExist(e) {
		t.Fatal("partial import left behind")
	}
}
func TestSharedExtensionsSyncBeforeTurnAndPreserveMalformedSource(t *testing.T) {
	ctx := context.Background()
	home, root := t.TempDir(), t.TempDir()
	target := catagent.NativeShareTargets(home, "", false)[2]
	var observed bool
	adapter := &fakeAdapter{run: func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		_, err := os.Stat(filepath.Join(target.SkillsPath, "demo", "SKILL.md"))
		observed = err == nil
		return catagent.TurnResponse{Message: catagent.WireMessage{Role: "assistant", Content: "ok"}}, nil
	}}
	svc, _ := newProjSvc(t, openStore(t), adapter, cat.Config{DataRoot: root, CLIHome: home})
	canonical := filepath.Join(root, "cat", "shared")
	sharedFile(t, filepath.Join(canonical, "skills", "demo", "SKILL.md"), "---\nname: demo\ndescription: Demonstration\n---\nHello\n")
	c, e := svc.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = svc.SendCatMessage(ctx, cat.SendMessageRequest{ConversationID: c.ID, Content: "hi"}); e != nil {
		t.Fatal(e)
	}
	svc.Wait()
	if !observed {
		t.Fatal("CLI launched before sync")
	}
	sharedFile(t, filepath.Join(canonical, "mcp.json"), "{broken")
	r, e := svc.SyncCatSharedExtensions(ctx, cat.SkillsDirRequest{})
	if e != nil || len(r.Warnings) == 0 {
		t.Fatal("malformed source not reported")
	}
	if _, e = os.Stat(filepath.Join(target.SkillsPath, "demo", "SKILL.md")); e != nil {
		t.Fatal("malformed source deleted owned links")
	}
	sharedFile(t, filepath.Join(canonical, "mcp.json"), `{"mcpServers":{}}`)
	sharedFile(t, filepath.Join(canonical, "skills", "demo", "SKILL.md"), "invalid frontmatter")
	r, e = svc.SyncCatSharedExtensions(ctx, cat.SkillsDirRequest{})
	if e != nil || len(r.Warnings) == 0 {
		t.Fatal("invalid skill not reported")
	}
	if _, e = os.Stat(filepath.Join(target.SkillsPath, "demo")); e != nil {
		t.Fatal("invalid skill removed link")
	}
	// Invalid skills must not prevent independent MCP updates.
	r, e = svc.SaveCatSharedMCP(ctx, cat.SharedMCPRequest{JSON: `{"mcpServers":{"independent":{"command":"no-launch"}}}`})
	if e != nil {
		t.Fatal(e)
	}
	for _, target := range r.Targets {
		if target.MCPCount != 1 {
			t.Fatal("invalid skill blocked MCP sync")
		}
	}
}
func TestSharedExtensionsConcurrentToggles(t *testing.T) {
	ctx := context.Background()
	svc, _ := newProjSvc(t, openStore(t), nil, cat.Config{DataRoot: t.TempDir(), CLIHome: t.TempDir()})
	_, e := svc.SaveCatSharedMCP(ctx, cat.SharedMCPRequest{JSON: `{"mcpServers":{"a":{"command":"no-launch"},"b":{"command":"no-launch"}}}`})
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, name := range []string{"a", "b"} {
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			_, e := svc.SetCatSharedItemEnabled(ctx, cat.SetInspectItemEnabledRequest{Category: "mcp", Name: n, Enabled: false})
			errs <- e
		}(name)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	r, e := svc.GetCatSharedExtensions(ctx, cat.SkillsDirRequest{})
	if e != nil {
		t.Fatal(e)
	}
	checkSharedSynced(t, r, 0, 0)
	for _, item := range r.MCPServers {
		if item.Enabled {
			t.Fatal("lost concurrent toggle")
		}
	}
}

func TestSharedExtensionsSaveFailureRestoresEnabledState(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "cat", "shared")
	if e := os.MkdirAll(filepath.Join(base, "mcp.json"), 0700); e != nil {
		t.Fatal(e)
	}
	sharedFile(t, filepath.Join(base, "enabled.json"), `{"mcp":{"existing":false}}`)
	svc, _ := newProjSvc(t, openStore(t), nil, cat.Config{DataRoot: root, CLIHome: t.TempDir()})
	_, e := svc.SaveCatSharedMCP(context.Background(), cat.SharedMCPRequest{JSON: `{"mcpServers":{"existing":{"command":"no-launch"}}}`})
	if e == nil {
		t.Fatal("save should fail")
	}
	raw, e := os.ReadFile(filepath.Join(base, "enabled.json"))
	if e != nil {
		t.Fatal(e)
	}
	var state map[string]map[string]bool
	if json.Unmarshal(raw, &state) != nil || state["mcp"]["existing"] {
		t.Fatal("failed save changed enablement")
	}
}

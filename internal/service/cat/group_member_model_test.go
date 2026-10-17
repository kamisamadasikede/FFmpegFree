package cat_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/service/cat"
	"FFmpegFree/internal/store"
)

func TestAddCatGroupMember_RejectsUnknownModel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix script fake")
	}
	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))
	fake := filepath.Join(dir, "grok")
	script := `#!/bin/sh
case "$1" in
  -v) echo "grok 1.0.40"; exit 0 ;;
  models) printf 'Default model: grok-4.6\n\nAvailable models:\n  * grok-4.6 (default)\n'; exit 0 ;;
esac
exit 0
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(context.Background(), filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	reg := catagent.NewRegistry()
	build := catagent.NewBuildAdapter(catagent.BuildConfig{
		ComponentDir: filepath.Join(dir, "components"), DataTemp: filepath.Join(dir, "tmp"),
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
	svc := cat.New(cat.Config{Store: st, Registry: reg, DataRoot: dir})
	ctx := context.Background()
	for {
		s, _ := svc.GetCatStatus(ctx)
		if s.State == catagent.StateReady {
			break
		}
	}
	proj := filepath.Join(dir, "proj")
	_ = os.MkdirAll(proj, 0o755)
	_ = os.WriteFile(filepath.Join(proj, "README.md"), []byte("x\n"), 0o644)
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}, {"-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "i"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = proj
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	p, err := svc.CreateCatProject(ctx, cat.CreateProjectRequest{Path: proj})
	if err != nil {
		t.Fatal(err)
	}
	g, err := svc.CreateCatGroup(ctx, cat.CreateGroupRequest{ProjectID: p.Project.ID, Name: "群"})
	if err != nil {
		t.Fatal(err)
	}
	// empty model = OK
	if _, err := svc.AddCatGroupMember(ctx, cat.AddGroupMemberRequest{
		GroupID: g.ID, Name: "甲", Title: "工程师", AgentKind: catagent.KindCatBuild,
	}); err != nil {
		t.Fatal(err)
	}
	// known model = OK
	if _, err := svc.AddCatGroupMember(ctx, cat.AddGroupMemberRequest{
		GroupID: g.ID, Name: "乙", Title: "工程师2", AgentKind: catagent.KindCatBuild, ModelID: "grok-4.6",
	}); err != nil {
		t.Fatal(err)
	}
	// unknown model = reject
	_, err = svc.AddCatGroupMember(ctx, cat.AddGroupMemberRequest{
		GroupID: g.ID, Name: "丙", Title: "工程师3", AgentKind: catagent.KindCatBuild, ModelID: "not-a-real-model",
	})
	if !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("want InvalidArgument, got %v", err)
	}
	if err == nil || !strings.Contains(err.Error(), "模型") {
		t.Fatalf("message %v", err)
	}
}

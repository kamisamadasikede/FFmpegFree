package cat_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/service/cat"
	"FFmpegFree/internal/store"
)

func charterTestSvc(t *testing.T, grokScript string) (*cat.Service, string, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("unix script fake")
	}
	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))
	fake := filepath.Join(dir, "grok")
	if err := os.WriteFile(fake, []byte(grokScript), 0o755); err != nil {
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
	deadline := time.Now().Add(10 * time.Second)
	for {
		s, _ := svc.GetCatStatus(ctx)
		if s.State == catagent.StateReady {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("not ready: %+v", s)
		}
		time.Sleep(20 * time.Millisecond)
	}
	proj := filepath.Join(dir, "proj")
	_ = os.MkdirAll(proj, 0o755)
	_ = os.WriteFile(filepath.Join(proj, "README.md"), []byte("demo\n"), 0o644)
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
	return svc, g.ID, dir
}

func TestAddCatGroupMember_GeneratesFullCharter(t *testing.T) {
	script := `#!/bin/sh
case "$1" in
  -v) echo "grok 1.0.40"; exit 0 ;;
  models) printf 'Default model: grok-4.6\n\nAvailable models:\n  * grok-4.6 (default)\n'; exit 0 ;;
esac
# Real grok --json-schema: one JSON document
printf '%s\n' '{"owns":"后端接口","notTouch":"前端页面","handoff":"多数时候只向老板汇报","dirs":["internal/"]}'
exit 0
`
	svc, gid, _ := charterTestSvc(t, script)
	ctx := context.Background()
	m, err := svc.AddCatGroupMember(ctx, cat.AddGroupMemberRequest{
		GroupID: gid, Name: "后端", Title: "后端工程师", AgentKind: catagent.KindCatBuild, ModelID: "grok-4.6",
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	var got cat.GroupMember
	for {
		svc.Wait()
		detail, err := svc.GetCatGroup(ctx, gid)
		if err != nil {
			t.Fatal(err)
		}
		for _, x := range detail.Members {
			if x.ID == m.ID {
				got = x
			}
		}
		if got.Charter.Status == "ok" || got.Charter.Status == "failed" || time.Now().After(deadline) {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if got.Charter.Status != "ok" {
		t.Fatalf("status=%q failReason=%q", got.Charter.Status, got.Charter.FailReason)
	}
	if got.Charter.Owns != "后端接口" || got.Charter.NotTouch != "前端页面" {
		t.Fatalf("full charter not written: owns=%q notTouch=%q dirs=%v (Add must call generateCharter full=true)", got.Charter.Owns, got.Charter.NotTouch, got.Charter.Dirs)
	}
	if len(got.Charter.Dirs) != 1 || got.Charter.Dirs[0] != "internal/" {
		t.Fatalf("dirs=%v", got.Charter.Dirs)
	}
	if got.Charter.FailReason != "" {
		t.Fatalf("failReason should be empty on success, got %q", got.Charter.FailReason)
	}
}

func TestGenerateCharter_FailReasonOnEmptyBody(t *testing.T) {
	script := `#!/bin/sh
case "$1" in
  -v) echo "grok 1.0.40"; exit 0 ;;
  models) printf 'Default model: grok-4.6\n\nAvailable models:\n  * grok-4.6 (default)\n'; exit 0 ;;
esac
# success exit but empty stdout → empty_reply / 模型没有返回内容
exit 0
`
	svc, gid, _ := charterTestSvc(t, script)
	ctx := context.Background()
	m, err := svc.AddCatGroupMember(ctx, cat.AddGroupMemberRequest{
		GroupID: gid, Name: "空返回", Title: "测试", AgentKind: catagent.KindCatBuild,
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	var got cat.GroupMember
	for {
		svc.Wait()
		detail, err := svc.GetCatGroup(ctx, gid)
		if err != nil {
			t.Fatal(err)
		}
		for _, x := range detail.Members {
			if x.ID == m.ID {
				got = x
			}
		}
		if got.Charter.Status == "failed" || got.Charter.Status == "ok" || time.Now().After(deadline) {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if got.Charter.Status != "failed" {
		t.Fatalf("want failed, got %q", got.Charter.Status)
	}
	if got.Charter.FailReason == "" {
		t.Fatal("failReason empty")
	}
	// AppError.Message from empty_reply is typically 「回复没生成出来，请重试。」
	if strings.Contains(got.Charter.FailReason, "{") {
		t.Fatalf("failReason looks like raw JSON: %q", got.Charter.FailReason)
	}
}

func TestRegenerateCatCharter_FailReasonOnBadJSON(t *testing.T) {
	script := `#!/bin/sh
case "$1" in
  -v) echo "grok 1.0.40"; exit 0 ;;
  models) printf 'Default model: grok-4.6\n\nAvailable models:\n  * grok-4.6 (default)\n'; exit 0 ;;
esac
printf '%s\n' '这不是 JSON'
exit 0
`
	svc, gid, _ := charterTestSvc(t, script)
	ctx := context.Background()
	m, err := svc.AddCatGroupMember(ctx, cat.AddGroupMemberRequest{
		GroupID: gid, Name: "坏JSON", Title: "测试", AgentKind: catagent.KindCatBuild,
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		svc.Wait()
		detail, _ := svc.GetCatGroup(ctx, gid)
		var st string
		for _, x := range detail.Members {
			if x.ID == m.ID {
				st = x.Charter.Status
			}
		}
		if st == "failed" || st == "ok" || time.Now().After(deadline) {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	_, err = svc.RegenerateCatCharter(ctx, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(15 * time.Second)
	var got cat.GroupMember
	for {
		svc.Wait()
		detail, err := svc.GetCatGroup(ctx, gid)
		if err != nil {
			t.Fatal(err)
		}
		for _, x := range detail.Members {
			if x.ID == m.ID {
				got = x
			}
		}
		if got.Charter.Status == "failed" || got.Charter.Status == "ok" || time.Now().After(deadline) {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if got.Charter.Status != "failed" {
		t.Fatalf("want failed, got %q failReason=%q", got.Charter.Status, got.Charter.FailReason)
	}
	if got.Charter.FailReason == "" {
		t.Fatal("failReason empty")
	}
}

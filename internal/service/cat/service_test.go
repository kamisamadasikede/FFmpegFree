package cat_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/service/cat"
	"FFmpegFree/internal/store"
)

func TestCreateRejectsOtherKinds(t *testing.T) {
	svc, _, cleanup := setupSvc(t, false)
	defer cleanup()
	_, err := svc.CreateCatConversation(context.Background(), cat.CreateConversationRequest{AgentKind: "cat_cli"})
	if !apperr.Is(err, apperr.InvalidArgument) {
		t.Fatalf("%v", err)
	}
}

func TestCreateAndPersist(t *testing.T) {
	svc, _, cleanup := setupSvc(t, false)
	defer cleanup()
	c, err := svc.CreateCatConversation(context.Background(), cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild, Title: "测"})
	if err != nil {
		t.Fatal(err)
	}
	if c.AgentKind != catagent.KindCatBuild {
		t.Fatalf("%+v", c)
	}
	list, err := svc.ListCatConversations(context.Background())
	if err != nil || len(list) != 1 {
		t.Fatalf("%v %v", list, err)
	}
	detail, err := svc.GetCatConversation(context.Background(), c.ID)
	if err != nil || detail.AgentKind != catagent.KindCatBuild || len(detail.Messages) != 0 {
		t.Fatalf("%+v %v", detail, err)
	}
}

func TestSendWhenNotReady(t *testing.T) {
	svc, _, cleanup := setupSvc(t, false)
	defer cleanup()
	c, err := svc.CreateCatConversation(context.Background(), cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.SendCatMessage(context.Background(), cat.SendMessageRequest{ConversationID: c.ID, Content: "你好"})
	if !apperr.Is(err, apperr.CatNotReady) {
		t.Fatalf("%v", err)
	}
}

func TestSendWithFakeBinary(t *testing.T) {
	svc, root, cleanup := setupSvc(t, true)
	defer cleanup()
	_ = root
	// wait ready
	deadline := time.Now().Add(2 * time.Second)
	for {
		st, _ := svc.GetCatStatus(context.Background())
		if st.State == catagent.StateReady {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("not ready: %+v", st)
		}
		time.Sleep(20 * time.Millisecond)
	}
	c, err := svc.CreateCatConversation(context.Background(), cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild})
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.SendCatMessage(context.Background(), cat.SendMessageRequest{
		ConversationID: c.ID, Content: "你好", ModelID: "m1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.TurnID == "" || res.AssistantMessage != nil || res.UserMessage.Content != "你好" {
		t.Fatalf("%+v", res)
	}
	svc.Wait()
	detail, err := svc.GetCatConversation(context.Background(), c.ID)
	if err != nil || len(detail.Messages) != 2 || detail.Messages[1].Content != "收到" {
		t.Fatalf("%+v %v", detail, err)
	}
}

func setupSvc(t *testing.T, withBinary bool) (*cat.Service, string, func()) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "app.db")
	st, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	temp := filepath.Join(dir, "tmp")
	_ = os.MkdirAll(temp, 0o755)
	reg := catagent.NewRegistry()
	cfg := catagent.BuildConfig{
		ComponentDir: filepath.Join(dir, "components"),
		DataTemp:     temp,
		LookPath:     func(string) (string, error) { return "", exec.ErrNotFound },
	}
	var fake string
	if withBinary {
		fake = filepath.Join(dir, "grok-fake")
		if runtime.GOOS == "windows" {
			// Windows：用 cmd 打出一行 streaming-json。
			bat := fake + ".bat"
			_ = os.WriteFile(bat, []byte("@echo {\"type\":\"text\",\"data\":\"收到\"}\r\n@echo {\"type\":\"end\",\"stopReason\":\"end_turn\"}\r\n"), 0o755)
			fake = bat
		} else {
			script := "#!/bin/sh\nprintf '%s\\n' '{\"type\":\"text\",\"data\":\"收到\"}'\nprintf '%s\\n' '{\"type\":\"end\",\"stopReason\":\"end_turn\"}'\n"
			_ = os.WriteFile(fake, []byte(script), 0o755)
		}
		cfg.LookPath = func(file string) (string, error) {
			if file == "grok" || file == "grok.exe" {
				return fake, nil
			}
			return "", exec.ErrNotFound
		}
		cfg.Exec = func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, fake)
		}
	}
	build := catagent.NewBuildAdapter(cfg)
	reg.Register(build)
	if withBinary {
		build.Start()
	} else {
		build.Recheck()
	}
	svc := cat.New(cat.Config{Store: st, Registry: reg})
	return svc, fake, func() { _ = st.Close() }
}

package cat_test

import (
	"context"
	"encoding/json"
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
	if res.AssistantMessage == nil || res.AssistantMessage.Content != "收到" {
		t.Fatalf("%+v", res)
	}
	detail, err := svc.GetCatConversation(context.Background(), c.ID)
	if err != nil || len(detail.Messages) != 2 {
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
	comp := filepath.Join(dir, "components")
	temp := filepath.Join(dir, "tmp")
	_ = os.MkdirAll(temp, 0o755)
	var root string
	if withBinary {
		root = filepath.Join(comp, "cat", "build", "0.0.0-test")
		_ = os.MkdirAll(root, 0o755)
		name := "cat-build"
		if runtime.GOOS == "windows" {
			name = "cat-build.exe"
		}
		_ = os.WriteFile(filepath.Join(root, name), []byte("x"), 0o755)
	}
	reg := catagent.NewRegistry()
	build := catagent.NewBuildAdapter(catagent.BuildConfig{
		ComponentDir: comp,
		DataTemp:     temp,
		Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			mode, outPath, reqPath := "turn", "", ""
			for i := 0; i+1 < len(args); i++ {
				switch args[i] {
				case "--capabilities":
					mode, outPath = "cap", args[i+1]
				case "--response":
					outPath = args[i+1]
				case "--request":
					reqPath = args[i+1]
				}
			}
			if mode == "cap" {
				b, _ := json.Marshal(catagent.Capabilities{Version: 1, Models: []catagent.Model{{ID: "m1", DisplayName: "Cat 助手 1.0"}}})
				_ = os.WriteFile(outPath, b, 0o644)
			} else {
				var req catagent.TurnRequest
				if raw, e := os.ReadFile(reqPath); e == nil {
					_ = json.Unmarshal(raw, &req)
				}
				b, _ := json.Marshal(catagent.TurnResponse{Version: 1, Message: catagent.WireMessage{Role: "assistant", Content: "收到"}})
				_ = os.WriteFile(outPath, b, 0o644)
			}
			if runtime.GOOS == "windows" {
				return exec.CommandContext(ctx, "cmd", "/C", "exit", "0")
			}
			return exec.CommandContext(ctx, "sh", "-c", "exit 0")
		},
	})
	reg.Register(build)
	if withBinary {
		build.Start()
	} else {
		build.Recheck()
	}
	svc := cat.New(cat.Config{Store: st, Registry: reg})
	return svc, root, func() { _ = st.Close() }
}

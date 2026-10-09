package catagent

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
)

func TestRunTurn_FakeBinarySuccess(t *testing.T) {
	root, temp := setupBuildDirs(t)
	touchEntry(t, root)
	a := NewBuildAdapter(BuildConfig{
		ComponentDir: filepath.Dir(filepath.Dir(root)), // …/components — wait: root is …/cat/build/ver
		DataTemp:     temp,
		Exec: fakeBuild(t, func(mode string, req TurnRequest, outPath string) int {
			if mode == "cap" {
				writeJSON(t, outPath, Capabilities{
					Version: 1,
					Models:  []Model{{ID: "m1", DisplayName: "Cat 助手 1.0"}},
				})
				return 0
			}
			if req.Version != 1 || len(req.Messages) == 0 {
				t.Fatalf("bad req %+v", req)
			}
			writeJSON(t, outPath, TurnResponse{
				Version: 1,
				Message: WireMessage{Role: "assistant", Content: "你好"},
			})
			return 0
		}),
	})
	// ComponentDir should be parent of cat/: root = ComponentDir/cat/build/ver
	comp := filepath.Dir(filepath.Dir(filepath.Dir(root)))
	a.cfg.ComponentDir = comp
	a.detect()
	if a.Status().State != StateReady {
		t.Fatalf("status %+v", a.Status())
	}
	models, err := a.ListModels()
	if err != nil || len(models) != 1 || models[0].DisplayName != "Cat 助手 1.0" {
		t.Fatalf("models %v %v", models, err)
	}
	thinks, err := a.ListThinkLevels()
	if err != nil || len(thinks) != 0 {
		t.Fatalf("thinks should be empty: %v %v", thinks, err)
	}
	resp, err := a.RunTurn(TurnOptions{
		Ctx: context.Background(), ConversationID: "c1",
		Messages: []WireMessage{{Role: "user", Content: "hi"}},
	})
	if err != nil || resp.Message.Content != "你好" {
		t.Fatalf("%+v %v", resp, err)
	}
}

func TestRunTurn_MissingEntry(t *testing.T) {
	a := NewBuildAdapter(BuildConfig{ComponentDir: t.TempDir(), DataTemp: t.TempDir()})
	a.detect()
	st := a.Status()
	if st.State != StateMissing || st.CanDownload || st.Error == nil || st.Error.Message != MsgNotReady {
		t.Fatalf("%+v", st)
	}
	_, err := a.RunTurn(TurnOptions{Ctx: context.Background(), Messages: []WireMessage{{Role: "user", Content: "x"}}})
	if !apperr.Is(err, apperr.CatNotReady) {
		t.Fatalf("%v", err)
	}
}

func TestRunTurn_NonZeroExit(t *testing.T) {
	root, temp := setupBuildDirs(t)
	touchEntry(t, root)
	comp := filepath.Dir(filepath.Dir(filepath.Dir(root)))
	a := NewBuildAdapter(BuildConfig{
		ComponentDir: comp, DataTemp: temp,
		Exec: fakeBuild(t, func(mode string, _ TurnRequest, outPath string) int {
			if mode == "cap" {
				writeJSON(t, outPath, Capabilities{Version: 1, Models: []Model{{ID: "m1", DisplayName: "Cat 助手 1.0"}}})
				return 0
			}
			return 3
		}),
	})
	a.detect()
	_, err := a.RunTurn(TurnOptions{
		Ctx: context.Background(), ConversationID: "c1", Timeout: 3 * time.Second,
		Messages: []WireMessage{{Role: "user", Content: "x"}},
	})
	if !apperr.Is(err, apperr.CatReplyFailed) || apperr.From(err).Message != MsgReplyFailed {
		t.Fatalf("%v", err)
	}
}

func TestHandleToolRequests_RefuseWrite(t *testing.T) {
	res := HandleToolRequests("/tmp", []ToolRequest{{ID: "1", Kind: "write_file", Path: "a.txt", Content: "x"}})
	if len(res) != 1 || res[0].OK || res[0].Content != MsgToolWriteRef {
		t.Fatalf("%+v", res)
	}
}

func TestHandleToolRequests_ListDir(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi"), 0o644)
	res := HandleToolRequests(dir, []ToolRequest{{ID: "1", Kind: "list_dir", Path: "."}})
	if len(res) != 1 || !res[0].OK || res[0].Content == "" {
		t.Fatalf("%+v", res)
	}
}

func setupBuildDirs(t *testing.T) (root, temp string) {
	t.Helper()
	comp := t.TempDir()
	root = filepath.Join(comp, "cat", "build", "0.0.0-test")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	return root, t.TempDir()
}

func touchEntry(t *testing.T, root string) {
	t.Helper()
	name := entryNameUnix
	if runtime.GOOS == "windows" {
		name = entryNameWindows
	}
	if err := os.WriteFile(filepath.Join(root, name), []byte("placeholder"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, _ := json.Marshal(v)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func fakeBuild(t *testing.T, handle func(mode string, req TurnRequest, outPath string) int) func(ctx context.Context, name string, args ...string) *exec.Cmd {
	t.Helper()
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		mode, reqPath, outPath := "turn", "", ""
		for i := 0; i+1 < len(args); i++ {
			switch args[i] {
			case "--request":
				reqPath = args[i+1]
			case "--response":
				outPath = args[i+1]
			case "--capabilities":
				mode = "cap"
				outPath = args[i+1]
			}
		}
		var req TurnRequest
		if reqPath != "" {
			if raw, err := os.ReadFile(reqPath); err == nil {
				_ = json.Unmarshal(raw, &req)
			}
		}
		code := handle(mode, req, outPath)
		if runtime.GOOS == "windows" {
			return exec.CommandContext(ctx, "cmd", "/C", "exit", itoa(code))
		}
		return exec.CommandContext(ctx, "sh", "-c", "exit "+itoa(code))
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

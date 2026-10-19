package cat

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/store"
)

func groupTestImage(t *testing.T) (string, []byte) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 240, 120))
	for y := 0; y < 120; y++ {
		for x := 0; x < 240; x++ {
			c := color.RGBA{R: 240, G: 20, B: 20, A: 255}
			if x >= 120 {
				c = color.RGBA{R: 20, G: 30, B: 240, A: 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "reference.png")
	if err := os.WriteFile(path, b.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return path, b.Bytes()
}

func TestGroupImages_RouteWorkerPersistenceAndValidation(t *testing.T) {
	for _, mode := range []string{"mention", "auto", "off"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			path, raw := groupTestImage(t)
			var mu sync.Mutex
			var calls []catagent.TurnOptions
			var member string
			s, g, ms, _ := groupWorkTestService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
				mu.Lock()
				calls = append(calls, o)
				mu.Unlock()
				text := "图片左侧为红色，右侧为蓝色。"
				if o.JSONSchema != "" {
					body, _ := json.Marshal(map[string]any{"memberIds": []string{member}, "everyone": false, "reason": "解读图片", "tasks": []any{}})
					text = string(body)
				}
				return catagent.TurnResponse{Message: catagent.WireMessage{Content: text}}, nil
			})
			member = ms[0].ID
			req := SendGroupMessageRequest{GroupID: g.ID, Files: []SendAttachment{{Path: path}, {Path: path}}}
			if mode == "mention" {
				req.Mentions = []string{member}
			}
			if mode == "off" {
				if _, err := s.SetCatGroupAutoRoute(ctx, SetGroupAutoRouteRequest{GroupID: g.ID, On: false}); err != nil {
					t.Fatal(err)
				}
			}
			sent, err := s.SendCatGroupMessage(ctx, req)
			if err != nil {
				t.Fatal(err)
			}
			s.Wait()
			if len(sent.UserMessage.Files) != 1 {
				t.Fatalf("expected deduplicated image: %+v", sent.UserMessage)
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			back, err := s.cfg.Store.GetCatGroupChainOrigin(ctx, g.ID, sent.ChainID)
			if err != nil || len(back.Files) != 1 {
				t.Fatalf("image not persisted: %+v, %v", back, err)
			}
			mu.Lock()
			captured := append([]catagent.TurnOptions{}, calls...)
			mu.Unlock()
			want := 1
			if mode == "auto" {
				want = 2
			}
			if len(captured) != want {
				t.Fatalf("calls=%d want=%d", len(captured), want)
			}
			for _, o := range captured {
				if len(o.Files) != 1 || o.Files[0].Path == path {
					t.Fatalf("missing managed image in CLI call: %+v", o.Files)
				}
				got, err := os.ReadFile(o.Files[0].Path)
				if err != nil || !bytes.Equal(got, raw) {
					t.Fatalf("CLI image bytes changed: %v", err)
				}
			}
			// Retry/reassign uses the persisted origin even without a trigger seq.
			files, err := s.groupTurnFiles(ctx, g.ID, sent.ChainID, 0)
			if err != nil || len(files) != 1 {
				t.Fatalf("retry images lost: %+v %v", files, err)
			}
			before, _ := s.cfg.Store.ListCatGroupMessages(ctx, g.ID, 0, 200)
			bad := filepath.Join(t.TempDir(), "fake.png")
			_ = os.WriteFile(bad, []byte("not an image"), 0600)
			if _, err := s.SendCatGroupMessage(ctx, SendGroupMessageRequest{GroupID: g.ID, Files: []SendAttachment{{Path: bad}}}); err == nil {
				t.Fatal("invalid image accepted")
			}
			after, _ := s.cfg.Store.ListCatGroupMessages(ctx, g.ID, 0, 200)
			if len(before) != len(after) {
				t.Fatal("invalid attachment created a user message")
			}
		})
	}
}

func TestGroupImages_WorkflowDependentTasksReceiveOriginal(t *testing.T) {
	ctx := context.Background()
	path, raw := groupTestImage(t)
	gate := make(chan struct{})
	var tasks []store.CatGroupWorkTask
	var mu sync.Mutex
	var calls []catagent.TurnOptions
	s, g, ms, _ := groupWorkTestService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		mu.Lock()
		calls = append(calls, o)
		mu.Unlock()
		if o.JSONSchema != "" {
			body, _ := json.Marshal(map[string]any{"memberIds": []string{tasks[0].MemberID}, "everyone": false, "reason": "按图开发验收", "tasks": tasks})
			return catagent.TurnResponse{Message: catagent.WireMessage{Content: string(body)}}, nil
		}
		select {
		case <-gate:
		case <-o.Ctx.Done():
			return catagent.TurnResponse{}, o.Ctx.Err()
		}
		o.OnActivity("tool", "check", "检查图片", false)
		return catagent.TurnResponse{Message: catagent.WireMessage{Content: `已按图片完成检查。<cat_task_result>{"status":"done","summary":"已完成","evidence":"实际检查通过"}</cat_task_result>`}}, nil
	})
	tasks = []store.CatGroupWorkTask{
		{ID: "draw", Title: "按图实现", MemberID: ms[0].ID, Instruction: "按图片实现", Kind: "develop", Status: "pending"},
		{ID: "check", Title: "对照原图验收", MemberID: ms[1].ID, Instruction: "对照图片实际检查", Kind: "verify", Status: "pending", DependsOn: []string{"draw"}},
	}
	sent, err := s.SendCatGroupMessage(ctx, SendGroupMessageRequest{GroupID: g.ID, Text: "按图实现并验收", Files: []SendAttachment{{Path: path}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	close(gate)
	s.Wait()
	w, err := s.cfg.Store.GetCatGroupWorkflow(ctx, sent.ChainID)
	if err != nil || w.Status != "completed" {
		t.Fatalf("workflow failed: %+v %v", w, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 3 {
		t.Fatalf("calls=%d want route+2 tasks", len(calls))
	}
	for _, o := range calls {
		if len(o.Files) != 1 {
			t.Fatal("dependent task did not receive image")
		}
		got, err := os.ReadFile(o.Files[0].Path)
		if err != nil || !bytes.Equal(got, raw) {
			t.Fatal("dependent task image lost")
		}
	}
}

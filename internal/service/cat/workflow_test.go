package cat_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"testing"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/service/cat"
)

func TestWorkflowQueueKeepsRunningTaskAndStartsAfterExit(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)
	root := t.TempDir()
	reg := catagent.NewRegistry()
	started := make(chan catagent.TurnOptions, 8)
	release := make(chan struct{}, 8)
	reg.Register(&fakeAdapter{run: func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		started <- o
		select {
		case <-release:
			return catagent.TurnResponse{Message: catagent.WireMessage{Role: "assistant", Content: "ok"}}, nil
		case <-o.Ctx.Done():
			return catagent.TurnResponse{}, o.Ctx.Err()
		}
	}})
	s := cat.New(cat.Config{Store: st, Registry: reg, DataRoot: root, CLIHome: t.TempDir()})
	c, e := s.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild})
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.SendCatMessage(ctx, cat.SendMessageRequest{ConversationID: c.ID, Content: "first"})
	if e != nil {
		t.Fatal(e)
	}
	first := <-started
	q, e := s.QueueCatMessage(ctx, cat.SendMessageRequest{ConversationID: c.ID, Content: "second", ModelID: "grok-4.5", PermissionMode: "plan"})
	if e != nil || len(q.Items) != 1 {
		t.Fatalf("%+v %v", q, e)
	}
	q, e = s.QueueCatMessage(ctx, cat.SendMessageRequest{ConversationID: c.ID, Content: "third"})
	if e != nil {
		t.Fatal(e)
	}
	thirdID := q.Items[1].ID
	q, e = s.UpdateCatMessageQueue(ctx, cat.QueueAction{ConvID: c.ID, ID: thirdID, Action: "edit", Content: "guide edited"})
	if e != nil {
		t.Fatal(e)
	}
	q, e = s.UpdateCatMessageQueue(ctx, cat.QueueAction{ConvID: c.ID, ID: thirdID, Action: "guide"})
	if e != nil || q.Items[0].Request.Content != "guide edited" {
		t.Fatalf("%+v %v", q, e)
	}
	if first.Ctx.Err() != nil {
		t.Fatal("queue interrupted current task")
	}
	if _, e = s.SendCatMessage(ctx, cat.SendMessageRequest{ConversationID: c.ID, Content: "replace"}); !apperr.Is(e, apperr.TaskConflict) {
		t.Fatalf("expected conflict: %v", e)
	}
	select {
	case <-started:
		t.Fatal("another process started too early")
	default:
	}
	release <- struct{}{}
	select {
	case next := <-started:
		if next.Messages[0].Content != "guide edited" {
			t.Fatal(next.Messages)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("queue did not start")
	}
	release <- struct{}{}
	select {
	case next := <-started:
		if next.ModelID != "grok-4.5" || next.PermissionMode != catagent.ModeGrokDefault {
			t.Fatalf("choices changed: %+v", next)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second item did not start")
	}
	release <- struct{}{}
	s.Wait()
	q, e = s.GetCatMessageQueue(ctx, c.ID)
	if e != nil || len(q.Items) != 0 {
		t.Fatalf("%+v %v", q, e)
	}
}

func TestWorkflowQueueFailureAndRestartPreserveDrafts(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)
	reg := catagent.NewRegistry()
	gate := make(chan struct{})
	started := make(chan struct{})
	reg.Register(&fakeAdapter{run: func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		close(started)
		<-gate
		return catagent.TurnResponse{}, errors.New("failure")
	}})
	cfg := cat.Config{Store: st, Registry: reg, DataRoot: t.TempDir(), CLIHome: t.TempDir()}
	s := cat.New(cfg)
	c, _ := s.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild})
	if _, e := s.SendCatMessage(ctx, cat.SendMessageRequest{ConversationID: c.ID, Content: "first"}); e != nil {
		t.Fatal(e)
	}
	<-started
	q, e := s.QueueCatMessage(ctx, cat.SendMessageRequest{ConversationID: c.ID, Content: "saved draft"})
	if e != nil {
		t.Fatal(e)
	}
	restarted := cat.New(cfg)
	restored, e := restarted.GetCatMessageQueue(ctx, c.ID)
	if e != nil || !restored.Paused || len(restored.Items) != 1 {
		t.Fatalf("restart: %+v %v", restored, e)
	}
	close(gate)
	s.Wait()
	q, e = s.GetCatMessageQueue(ctx, c.ID)
	if e != nil || !q.Paused || len(q.Items) != 1 {
		t.Fatalf("failure: %+v %v", q, e)
	}
	q, e = s.UpdateCatMessageQueue(ctx, cat.QueueAction{ConvID: c.ID, ID: q.Items[0].ID, Action: "delete"})
	if e != nil || len(q.Items) != 0 {
		t.Fatalf("delete: %+v %v", q, e)
	}
}

func TestWorkflowQueueStopPreservesWaitingMessage(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)
	reg := catagent.NewRegistry()
	started := make(chan struct{}, 1)
	reg.Register(&fakeAdapter{run: func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		started <- struct{}{}
		<-o.Ctx.Done()
		return catagent.TurnResponse{}, o.Ctx.Err()
	}})
	s := cat.New(cat.Config{Store: st, Registry: reg, DataRoot: t.TempDir(), CLIHome: t.TempDir()})
	c, _ := s.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild})
	res, e := s.SendCatMessage(ctx, cat.SendMessageRequest{ConversationID: c.ID, Content: "running"})
	if e != nil {
		t.Fatal(e)
	}
	<-started
	if _, e = s.QueueCatMessage(ctx, cat.SendMessageRequest{ConversationID: c.ID, Content: "preserved"}); e != nil {
		t.Fatal(e)
	}
	if e = s.CancelCatTurn(ctx, cat.CancelTurnRequest{ConvID: c.ID, TurnID: res.TurnID}); e != nil {
		t.Fatal(e)
	}
	s.Wait()
	q, e := s.GetCatMessageQueue(ctx, c.ID)
	if e != nil || !q.Paused || len(q.Items) != 1 || q.Items[0].Request.Content != "preserved" {
		t.Fatalf("%+v %v", q, e)
	}
	select {
	case <-started:
		t.Fatal("cancel automatically sent queued message")
	default:
	}
}

func TestWorkflowOpenBridgeAuthorizationScopeAndExpiry(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)
	reg := catagent.NewRegistry()
	started := make(chan catagent.TurnOptions, 1)
	gate := make(chan struct{})
	reg.Register(&fakeAdapter{run: func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		started <- o
		<-gate
		return catagent.TurnResponse{}, nil
	}})
	root := t.TempDir()
	var mu sync.Mutex
	opens := []cat.OpenFileEvent{}
	s := cat.New(cat.Config{Store: st, Registry: reg, DataRoot: root, CLIHome: t.TempDir(), FileBridge: true, Emit: func(event string, payload any) {
		if event == "cat:open-file" {
			mu.Lock()
			opens = append(opens, payload.(cat.OpenFileEvent))
			mu.Unlock()
		}
	}})
	c, _ := s.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild})
	files, e := s.ListCatFiles(ctx, cat.ListFilesRequest{ConvID: c.ID})
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(files.Root, "hello.txt"), []byte("hello"), 0600); e != nil {
		t.Fatal(e)
	}
	_, e = s.SendCatMessage(ctx, cat.SendMessageRequest{ConversationID: c.ID, Content: "show file"})
	if e != nil {
		t.Fatal(e)
	}
	o := <-started
	endpoint := regexp.MustCompile(`http://127\.0\.0\.1:\d+/open-file`).FindString(o.Rules)
	token := regexp.MustCompile(`Bearer ([a-f0-9]{64})`).FindStringSubmatch(o.Rules)
	if endpoint == "" || len(token) != 2 {
		t.Fatal("bridge capability missing")
	}
	client := &http.Client{Timeout: time.Second}
	post := func(path, auth, origin string) int {
		b, _ := json.Marshal(map[string]string{"path": path})
		req, _ := http.NewRequest("POST", endpoint, bytes.NewReader(b))
		req.Header.Set("Authorization", auth)
		req.Header.Set("Content-Type", "application/json")
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		r, e := client.Do(req)
		if e != nil {
			return 0
		}
		defer r.Body.Close()
		return r.StatusCode
	}
	if post("hello.txt", "Bearer wrong", "") != 401 {
		t.Fatal("bad token accepted")
	}
	if post("../outside.txt", "Bearer "+token[1], "") != 400 {
		t.Fatal("escape accepted")
	}
	if post("hello.txt", "Bearer "+token[1], "https://example.com") != 403 {
		t.Fatal("browser origin accepted")
	}
	if post("hello.txt", "Bearer "+token[1], "") != 200 {
		t.Fatal("valid preview not opened")
	}
	mu.Lock()
	if len(opens) != 1 || opens[0].ConvID != c.ID || opens[0].RelPath != "hello.txt" {
		t.Fatal(opens)
	}
	mu.Unlock()
	close(gate)
	s.Wait()
	if post("hello.txt", "Bearer "+token[1], "") != 0 {
		t.Fatal("endpoint survived turn")
	}
}

func TestWorkflowRevisionRejectsExternalOverwrite(t *testing.T) {
	ctx := context.Background()
	s, _ := newProjSvc(t, openStore(t), nil, cat.Config{DataRoot: t.TempDir()})
	c, _ := s.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild})
	files, e := s.ListCatFiles(ctx, cat.ListFilesRequest{ConvID: c.ID})
	if e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(files.Root, "draft.txt")
	_ = os.WriteFile(p, []byte("original"), 0600)
	b, e := s.ReadCatFile(ctx, cat.ReadFileRequest{ConvID: c.ID, RelPath: "draft.txt"})
	if e != nil {
		t.Fatal(e)
	}
	_ = os.WriteFile(p, []byte("external update"), 0600)
	_, e = s.WriteCatFile(ctx, cat.WriteFileRequest{ConvID: c.ID, RelPath: "draft.txt", Content: "user draft", ExpectedRevision: b.Revision})
	if !apperr.Is(e, apperr.TaskConflict) {
		t.Fatal(e)
	}
	actual, _ := os.ReadFile(p)
	if string(actual) != "external update" {
		t.Fatal("external file overwritten")
	}
}

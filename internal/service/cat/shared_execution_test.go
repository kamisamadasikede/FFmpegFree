package cat_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"FFmpegFree/internal/agentexec"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/paths"
	"FFmpegFree/internal/service/cat"
	"FFmpegFree/internal/store"
)

func TestSingleChatSharedExecutionAndRemovedFolder(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	st, err := store.Open(ctx, filepath.Join(root, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	runtime := agentexec.New()
	t.Cleanup(runtime.Close)
	recorder := &recorder{turnCh: make(chan catagent.TurnEvent, 16)}
	observed := make(chan catagent.TurnOptions, 2)
	registry := catagent.NewRegistry()
	registry.Register(&fakeAdapter{run: func(opts catagent.TurnOptions) (catagent.TurnResponse, error) {
		observed <- opts
		opts.OnTextDelta("单聊回复正常")
		return catagent.TurnResponse{Message: catagent.WireMessage{Role: "assistant", Content: "单聊回复正常"}}, nil
	}})
	svc := cat.New(cat.Config{Store: st, Registry: registry, Execution: runtime, DataRoot: root, Emit: recorder.emit})
	t.Cleanup(svc.Wait)
	conversation, err := svc.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild})
	if err != nil {
		t.Fatal(err)
	}
	dir := paths.ResolveCatConvDir(root, conversation.ID)
	for turn := 0; turn < 2; turn++ {
		if turn == 1 {
			// Exercise the adapter's existing recovery behavior with an app-owned
			// empty folder removed between turns, without touching a project.
			if err = os.Remove(dir); err != nil {
				t.Fatal(err)
			}
		}
		if _, err = svc.SendCatMessage(ctx, cat.SendMessageRequest{ConversationID: conversation.ID, Content: "继续"}); err != nil {
			t.Fatal(err)
		}
		if event := waitTerminal(t, recorder); event.Status != catagent.TurnCompleted {
			t.Fatalf("single chat failed: %+v", event)
		}
		svc.Wait()
		opts := <-observed
		physical, err := agentexec.CanonicalRoot(dir)
		if err != nil {
			t.Fatal(err)
		}
		if !paths.SamePath(opts.ProjectPath, physical) || opts.ConversationID != conversation.ID {
			t.Fatalf("wrong execution identity: cwd=%s id=%s", opts.ProjectPath, opts.ConversationID)
		}
		if _, err = os.Stat(dir); err != nil {
			t.Fatal(err)
		}
	}
}

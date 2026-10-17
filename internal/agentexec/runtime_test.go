package agentexec

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"FFmpegFree/internal/catagent"
)

type fake struct {
	catagent.Adapter
	fn func(catagent.TurnOptions) (catagent.TurnResponse, error)
}

func (*fake) Kind() string                                                    { return "test" }
func (f *fake) RunTurn(o catagent.TurnOptions) (catagent.TurnResponse, error) { return f.fn(o) }
func TestWorkspaceOverlapAndCancellation(t *testing.T) {
	r := New()
	defer r.Close()
	root := t.TempDir()
	child := filepath.Join(root, "nested")
	if e := os.Mkdir(child, 0700); e != nil {
		t.Fatal(e)
	}
	var count atomic.Int32
	started := make(chan struct{}, 2)
	gate := make(chan struct{})
	a := &fake{fn: func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		count.Add(1)
		started <- struct{}{}
		select {
		case <-gate:
		case <-o.Ctx.Done():
		}
		return catagent.TurnResponse{}, o.Ctx.Err()
	}}
	go r.Run(a, Scope{OwnerID: "a", Workspace: root}, catagent.TurnOptions{ConversationID: "a"})
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, e := r.Run(a, Scope{OwnerID: "b", Workspace: child}, catagent.TurnOptions{Ctx: ctx, ConversationID: "b"})
		done <- e
	}()
	cancel()
	select {
	case e := <-done:
		if e == nil {
			t.Fatal("cancelled waiter ran")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("waiter blocked")
	}
	if count.Load() != 1 {
		t.Fatal("overlapping writers ran concurrently")
	}
	close(gate)
}

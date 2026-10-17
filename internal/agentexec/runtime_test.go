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

func TestLeaseKeyBypassesFolderLock(t *testing.T) {
	r := New()
	defer r.Close()
	root := t.TempDir()
	started := make(chan string, 4)
	gate := make(chan struct{})
	a := &fake{fn: func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		started <- o.ConversationID
		select {
		case <-gate:
		case <-o.Ctx.Done():
		}
		return catagent.TurnResponse{}, nil
	}}
	go r.Run(a, Scope{OwnerID: "plain", Workspace: root}, catagent.TurnOptions{ConversationID: "plain"})
	go r.Run(a, Scope{OwnerID: "m1", Workspace: root, LeaseKey: "p|g|m1"}, catagent.TurnOptions{ConversationID: "m1"})
	go r.Run(a, Scope{OwnerID: "m2", Workspace: root, LeaseKey: "p|g|m2"}, catagent.TurnOptions{ConversationID: "m2"})
	for i := 0; i < 3; i++ {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("lease-keyed runs blocked on the folder lock")
		}
	}
	// same LeaseKey stays exclusive
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, e := r.Run(a, Scope{OwnerID: "m1b", Workspace: root, LeaseKey: "p|g|m1"}, catagent.TurnOptions{Ctx: ctx, ConversationID: "m1b"}); e == nil {
		t.Fatal("same lease key ran concurrently")
	}
	// a second plain run still waits for the folder
	ctx2, cancel2 := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel2()
	if _, e := r.Run(a, Scope{OwnerID: "plain2", Workspace: root}, catagent.TurnOptions{Ctx: ctx2, ConversationID: "plain2"}); e == nil {
		t.Fatal("plain runs on one folder ran concurrently")
	}
	close(gate)
}

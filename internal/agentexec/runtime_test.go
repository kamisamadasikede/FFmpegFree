package agentexec

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
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

func TestDifferentWorkspacesRunConcurrently(t *testing.T) {
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
	go r.Run(a, Scope{OwnerID: "m1", Workspace: t.TempDir()}, catagent.TurnOptions{ConversationID: "m1"})
	go r.Run(a, Scope{OwnerID: "m2", Workspace: t.TempDir()}, catagent.TurnOptions{ConversationID: "m2"})
	for i := 0; i < 3; i++ {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("independent workspaces blocked each other")
		}
	}
	// a second plain run still waits for the folder
	ctx2, cancel2 := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel2()
	if _, e := r.Run(a, Scope{OwnerID: "plain2", Workspace: root}, catagent.TurnOptions{Ctx: ctx2, ConversationID: "plain2"}); e == nil {
		t.Fatal("plain runs on one folder ran concurrently")
	}
	close(gate)
}

func TestWorkspaceRootOverlap(t *testing.T) {
	child := t.TempDir()
	root := filepath.VolumeName(child) + string(filepath.Separator)
	if !overlaps(root, child) || !overlaps(child, root) {
		t.Fatal("filesystem root did not overlap its child")
	}
	if overlaps(child, child+"-sibling") {
		t.Fatal("sibling path was treated as nested")
	}
}

func TestCancelledPreparationDoesNotStartAdapter(t *testing.T) {
	r := New()
	defer r.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &fake{fn: func(catagent.TurnOptions) (catagent.TurnResponse, error) {
		t.Fatal("adapter started after preparation was cancelled")
		return catagent.TurnResponse{}, nil
	}}
	_, err := r.Run(a, Scope{OwnerID: "prepare", Workspace: t.TempDir(), Prepare: func(context.Context) error {
		cancel()
		return nil
	}}, catagent.TurnOptions{Ctx: ctx, ConversationID: "prepare"})
	if err != context.Canceled {
		t.Fatalf("got %v, want cancellation", err)
	}
}

func TestSameSessionReplacementWaitsForCancelledCleanup(t *testing.T) {
	r := New()
	defer r.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	oldEntered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	var calls atomic.Int32
	a := &fake{fn: func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		if calls.Add(1) == 1 {
			close(oldEntered)
			<-o.Ctx.Done()
			<-release
		}
		return catagent.TurnResponse{}, o.Ctx.Err()
	}}
	root := t.TempDir()
	oldDone := make(chan error, 1)
	go func() {
		_, err := r.Run(a, Scope{OwnerID: "same", Workspace: root}, catagent.TurnOptions{Ctx: ctx, ConversationID: "same"})
		oldDone <- err
	}()
	<-oldEntered
	cancel()
	// Cancellation of a replacement waiting on the same session must be prompt.
	waitingCtx, stopWait := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer stopWait()
	_, err := r.Run(a, Scope{OwnerID: "same", Workspace: root}, catagent.TurnOptions{Ctx: waitingCtx, ConversationID: "same"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("replacement did not wait cancellably: %v", err)
	}
	freshDone := make(chan error, 1)
	go func() {
		_, err := r.Run(a, Scope{OwnerID: "same", Workspace: root}, catagent.TurnOptions{ConversationID: "same"})
		freshDone <- err
	}()
	select {
	case err := <-freshDone:
		t.Fatalf("replacement exited before cleanup: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if calls.Load() != 1 {
		t.Fatal("native sessions overlapped")
	}
	releaseOnce.Do(func() { close(release) })
	select {
	case err := <-freshDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("replacement did not wake after cleanup")
	}
	<-oldDone
	if calls.Load() != 2 {
		t.Fatalf("calls=%d", calls.Load())
	}
}

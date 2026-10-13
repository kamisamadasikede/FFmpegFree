//go:build windows

package cat

import (
	"strings"
	"testing"
	"time"
)

func TestConsoleEcho(t *testing.T) {
	events := make(chan TermEvent, 32)
	h := newTermHub(func(ev string, payload any) {
		if ev == termEvent {
			events <- payload.(TermEvent)
		}
	}, nil)
	if !h.pty {
		t.Fatal("应该直接打开控制台")
	}
	id, _, err := h.start(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.stop(id) })
	if err := h.write(id, "echo hello-pty\r\n"); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(8 * time.Second)
	got := ""
	for !strings.Contains(got, "hello-pty") {
		select {
		case ev := <-events:
			if ev.Exit != nil {
				t.Fatalf("终端退出 %d，已收到 %q", *ev.Exit, got)
			}
			got += ev.Data
		case <-deadline:
			t.Fatalf("没有回显，已收到 %q", got)
		}
	}
}

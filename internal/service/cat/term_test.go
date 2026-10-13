package cat

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestTermHelper(t *testing.T) {
	if os.Getenv("CAT_TERM_HELPER") != "1" {
		return
	}
	in := bufio.NewScanner(os.Stdin)
	out := bufio.NewWriter(os.Stdout)
	for in.Scan() {
		line := in.Text()
		if line == "quit" {
			_ = out.Flush()
			os.Exit(0)
		}
		fmt.Fprintln(out, "got:"+line)
		_ = out.Flush()
	}
	_ = out.Flush()
}

func TestTermEchoAndStop(t *testing.T) {
	events := make(chan TermEvent, 8)
	h := newTermHub(func(ev string, payload any) {
		if ev != termEvent {
			t.Errorf("事件名 %s", ev)
			return
		}
		events <- payload.(TermEvent)
	}, func(cwd string) *exec.Cmd {
		cmd := exec.Command(os.Args[0], "-test.run=^TestTermHelper$")
		cmd.Env = append(os.Environ(), "CAT_TERM_HELPER=1")
		cmd.Dir = cwd
		return cmd
	})
	id, _, err := h.start(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.stop(id) })
	if err := h.write(id, "hi\n"); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(5 * time.Second)
	got := ""
	for !strings.Contains(got, "got:hi") {
		select {
		case ev := <-events:
			got += ev.Data
		case <-deadline:
			t.Fatalf("没有回显，已收到 %q", got)
		}
	}
	if err := h.write(id, "quit\n"); err != nil {
		t.Fatal(err)
	}
	deadline = time.After(5 * time.Second)
	for {
		select {
		case ev := <-events:
			if ev.Exit != nil {
				return
			}
		case <-deadline:
			t.Fatal("命令行没有结束")
		}
	}
}

package catagent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestParseExportMarkdown_KeepsAssistantHeadings(t *testing.T) {
	in := "preamble\n" +
		"## User\n\n你好\n\n" +
		"## Assistant\n\n第一段\n\n## 它实际怎么跑\n\n仍是助手正文\n\n```\n## User\n```\n\n" +
		"## Tools\n\n- Read: a.go\n" +
		"## Thinking\n\n先看目录\n" +
		"## Assistant\n\n第二段\n"
	got := ParseExportMarkdown(in)
	if len(got) != 5 {
		t.Fatalf("%d %+v", len(got), got)
	}
	want := []WireMessage{
		{Role: "user", Content: "你好"},
		{Role: "assistant", Content: "第一段\n\n## 它实际怎么跑\n\n仍是助手正文\n\n```\n## User\n```"},
		{Role: "tool", Content: "- Read: a.go"},
		{Role: "thought", Content: "先看目录"},
		{Role: "assistant", Content: "第二段"},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("[%d] got %+v want %+v", i, got[i], want[i])
		}
	}
}

func TestSessionLabelIn_FirstUserLine(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "work")
	convID := "conv-label"
	dir := filepath.Dir(contextSignalsPath(root, cwd, convID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "{\"type\":\"system\",\"content\":\"sys\"}\n" +
		"{\"type\":\"user\",\"content\":\"帮我看一下这个很长很长的文件夹名字问题\"}\n" +
		"{\"type\":\"assistant\",\"content\":\"好\"}\n"
	if err := os.WriteFile(filepath.Join(dir, "chat_history.jsonl"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got := sessionLabelIn(root, cwd, convID)
	if got != "帮我看一下这个很长很长的文件夹名字问…" {
		t.Fatalf("%q", got)
	}
	if sessionLabelIn(root, cwd, "missing") != "" {
		t.Fatal("missing session should have no label")
	}
}

func TestSessionStartedIn(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "work")
	if sessionStartedIn(root, cwd, "conv") {
		t.Fatal("missing session dir reported started")
	}
	dir := filepath.Dir(contextSignalsPath(root, cwd, "conv"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if !sessionStartedIn(root, cwd, "conv") {
		t.Fatal("session dir not seen")
	}
}

func TestLoadTranscript_ReadsExport(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "grok-export")
	var script string
	if runtime.GOOS == "windows" {
		fake += ".bat"
		script = "@echo off\r\n" +
			"if \"%~1\"==\"export\" (\r\n" +
			"  echo ## User\r\n" +
			"  echo hello\r\n" +
			"  echo ## Assistant\r\n" +
			"  echo world\r\n" +
			"  exit /b 0\r\n" +
			")\r\n" +
			"echo 1.0.0\r\n"
	} else {
		script = "#!/bin/sh\n" +
			"if [ \"$1\" = export ]; then\n" +
			"  printf '%s\\n' '## User' 'hello' '## Assistant' 'world'\n" +
			"  exit 0\n" +
			"fi\n" +
			"echo 1.0.0\n"
	}
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	sessions := t.TempDir()
	cwd := filepath.Join(dir, "work")
	convID := "conv-export"
	if err := os.MkdirAll(filepath.Dir(contextSignalsPath(sessions, cwd, convID)), 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewBuildAdapter(BuildConfig{
		DataTemp:     t.TempDir(),
		SessionsRoot: sessions,
		LookPath:     func(string) (string, error) { return fake, nil },
		Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, name, args...)
		},
	})
	a.detect()
	if a.Status().State != StateReady {
		t.Fatalf("not ready: %+v", a.Status())
	}
	empty, err := a.LoadTranscript(context.Background(), "no-such", cwd)
	if err != nil || len(empty) != 0 {
		t.Fatalf("missing session: %+v %v", empty, err)
	}
	got, err := a.LoadTranscript(context.Background(), convID, cwd)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Content != "hello" || got[1].Role != "assistant" || got[1].Content != "world" {
		t.Fatalf("%+v", got)
	}
}

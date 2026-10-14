package catagent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// v0.48.1：本轮有最终回复且 turn.completed → 成功；重连提示 / 标准错误告警 / 非零退出只进日志。
func fakeCodex(t *testing.T, body string) *CLIAgentAdapter {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("unix script fake")
	}
	dir := t.TempDir()
	fake := filepath.Join(dir, "codex")
	script := "#!/bin/sh\n[ \"$1\" = \"--version\" ] && { echo codex-cli 0.162.1; exit 0; }\n[ \"$1\" = \"exec\" ] || exit 0\n" + body
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewCodexAdapter(CLIAgentConfig{
		DataTemp: t.TempDir(), DataRoot: t.TempDir(),
		LookPath: func(string) (string, error) { return fake, nil },
		Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, fake, args...)
		},
	})
	a.detect()
	return a
}

const okTail = `printf '%s\n' '{"type":"item.completed","item":{"id":"i1","type":"agent_message","text":"你好！有什么我可以帮你的吗？"}}'
printf '%s\n' '{"type":"turn.completed","usage":{"input_tokens":11655,"cached_input_tokens":0,"output_tokens":13}}'
`

func TestCodexTurn_ReconnectThenReplyIsSuccess(t *testing.T) {
	a := fakeCodex(t, `printf '%s\n' '{"type":"thread.started","thread_id":"t1"}'
printf '%s\n' '{"type":"error","message":"Reconnecting... 1/5 (stream disconnected before completion: error sending request)"}'
printf '%s\n' '{"type":"error","message":"Reconnecting... 2/5 (stream disconnected before completion: timeout)"}'
echo "ERROR codex_core: stream error: connection reset; retrying 2/5 in 400ms" >&2
`+okTail+`exit 1
`)
	var errs, retries []string
	resp, err := a.RunTurn(TurnOptions{
		Ctx: context.Background(), ConversationID: "c1",
		Messages: []WireMessage{{Role: "user", Content: "hi"}},
		OnError:  func(m string) { errs = append(errs, m) },
		OnActivity: func(kind, id, text string, _ bool) {
			if kind == "retry" {
				retries = append(retries, text)
			}
		},
	})
	if err != nil {
		t.Fatalf("successful turn reported failed: %v", err)
	}
	if !strings.Contains(resp.Message.Content, "你好") || resp.OutputTokens != 13 {
		t.Fatalf("resp %+v", resp)
	}
	if len(errs) != 0 {
		t.Fatalf("reconnect surfaced as error rows: %q", errs)
	}
	if len(retries) == 0 || retries[0] != "正在重试（1/5）" {
		t.Fatalf("retries %q", retries)
	}
}

func TestCodexTurn_CompletedWithStderrWarningIsSuccess(t *testing.T) {
	a := fakeCodex(t, `echo "warning: request timed out talking to telemetry" >&2
`+okTail+`exit 0
`)
	if _, err := a.RunTurn(TurnOptions{Ctx: context.Background(), ConversationID: "c1",
		Messages: []WireMessage{{Role: "user", Content: "hi"}}}); err != nil {
		t.Fatalf("want success, got %v", err)
	}
}

func TestCodexTurn_FailedWithoutCompletionStillFails(t *testing.T) {
	a := fakeCodex(t, `printf '%s\n' '{"type":"error","message":"Reconnecting... 5/5 (stream disconnected)"}'
printf '%s\n' '{"type":"turn.failed","error":{"message":"stream disconnected"}}'
exit 1
`)
	if _, err := a.RunTurn(TurnOptions{Ctx: context.Background(), ConversationID: "c1",
		Messages: []WireMessage{{Role: "user", Content: "hi"}}}); err == nil {
		t.Fatal("want failure")
	}
}

func TestCodexRetryPiece(t *testing.T) {
	if _, ok := codexRetryPiece("unexpected status 401 Unauthorized"); ok {
		t.Fatal("401 is not a retry")
	}
	if p, ok := codexRetryPiece("Re-connecting... 3/5"); !ok || p.Text != "正在重试（3/5）" {
		t.Fatalf("%+v %v", p, ok)
	}
}

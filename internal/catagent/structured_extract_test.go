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

func TestExtractStructuredJSONContent(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string // substring that must appear / empty means want empty
		has  string
	}{
		{"bare route", `{"memberId":"m1","reason":"接口"}`, "", `{"memberId":"m1"`},
		{"grok result wrapper", `{"result":{"memberId":"m1","reason":"x"}}`, "", `"memberId":"m1"`},
		{"result string", `{"type":"result","result":"{\"memberId\":\"m1\",\"reason\":\"x\"}"}`, "", `"memberId":"m1"`},
		{"streaming text wrap", "{\"type\":\"text\",\"data\":\"{\\\"memberId\\\":\\\"m1\\\",\\\"reason\\\":\\\"x\\\"}\"}\n{\"type\":\"end\"}\n", "", `"memberId":"m1"`},
		{"charter owns", `{"owns":"前端","notTouch":"后端","handoff":"","dirs":["frontend/"]}`, "", `"owns":"前端"`},
		{"grok structured_output only", `{"text":"hello","structured_output":{"owns":"前端","notTouch":"后端","handoff":"汇报","dirs":[]}}`, "", `"owns":"前端"`},
		{"grok text JSON string", `{"text":"{\"memberId\":\"m1\",\"reason\":\"接口\"}"}`, "", `"memberId":"m1"`},
		{"grok text object", `{"text":{"memberId":"m2","reason":"职责"}}`, "", `"memberId":"m2"`},
		{"prefer structured_output over text", `{"text":{"memberId":"from-text","reason":"x"},"structured_output":{"memberId":"from-so","reason":"y"}}`, "", `"memberId":"from-so"`},
		{"empty", `   `, "", ""},
		{"plain prose", "我觉得是后端", "", ""},
	}
	for _, c := range cases {
		got := ExtractStructuredJSONContent([]byte(c.raw))
		if c.has == "" {
			if got != "" {
				t.Fatalf("%s: want empty, got %q", c.name, got)
			}
			continue
		}
		if !strings.Contains(got, c.has) {
			t.Fatalf("%s: got %q, want contain %q", c.name, got, c.has)
		}
	}
}

func TestBuildAdapter_RunTurn_JSONSchemaReadsDocument(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix script fake")
	}
	fake := filepath.Join(t.TempDir(), "grok-fake")
	// Real grok --output-format json: one JSON object, not streaming-json lines.
	script := `#!/bin/sh
printf '%s\n' '{"memberId":"mem-42","reason":"接口归他"}'
exit 0
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewBuildAdapter(BuildConfig{
		DataTemp: t.TempDir(),
		LookPath: func(string) (string, error) { return fake, nil },
		Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, fake)
		},
	})
	a.mu.Lock()
	a.exe = fake
	a.st.State = StateReady
	a.mu.Unlock()
	resp, err := a.RunTurn(TurnOptions{
		Ctx: context.Background(), ConversationID: "route-test",
		JSONSchema: RouteSchema, Ephemeral: true, NoTools: true,
		Messages: []WireMessage{{Role: "user", Content: "选出最合适的一个人"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Message.Content, `"memberId":"mem-42"`) {
		t.Fatalf("content %q", resp.Message.Content)
	}
}

func TestBuildAdapter_RunTurn_JSONSchemaResultWrapper(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix script fake")
	}
	fake := filepath.Join(t.TempDir(), "grok-fake")
	script := `#!/bin/sh
printf '%s\n' '{"result":"{\"memberId\":\"m9\",\"reason\":\"职责\"}"}'
exit 0
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewBuildAdapter(BuildConfig{
		DataTemp: t.TempDir(),
		LookPath: func(string) (string, error) { return fake, nil },
		Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, fake)
		},
	})
	a.mu.Lock()
	a.exe = fake
	a.st.State = StateReady
	a.mu.Unlock()
	resp, err := a.RunTurn(TurnOptions{
		Ctx: context.Background(), ConversationID: "route-wrap",
		JSONSchema: RouteSchema, Ephemeral: true,
		Messages: []WireMessage{{Role: "user", Content: "x"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Message.Content, `"memberId":"m9"`) {
		t.Fatalf("content %q", resp.Message.Content)
	}
}

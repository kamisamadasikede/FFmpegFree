package catagent

import (
	"os"
	"strings"
	"testing"
)

func scanFixture(t *testing.T, a *CLIAgentAdapter, path string) []streamPiece {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []streamPiece
	if err := a.scanCLIStream(f, func(p streamPiece) {
		switch p.Kind {
		case "text", "tool", "thought":
			out = append(out, p)
		}
	}, nil, nil); err != nil {
		t.Fatal(err)
	}
	return out
}

func seq(ps []streamPiece) string {
	var b []string
	for _, p := range ps {
		b = append(b, p.Kind+"|"+p.ID+"|"+p.Text)
	}
	return strings.Join(b, "\n")
}

func TestClaudeStreamLiveToolsInterleaved(t *testing.T) {
	a := &CLIAgentAdapter{kind: cliKindClaude}
	got := seq(scanFixture(t, a, "testdata/stream/claude_tools.jsonl"))
	want := strings.Join([]string{
		"text||先写文件。",
		`tool|toolu_1|Write {"file_path":"a.txt","content":"hi"}`,
		`tool|toolu_2|Bash {"command":"false"}`,
		"tool|toolu_2:error|error: exit 1",
		"text||完成。",
	}, "\n")
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

// 实时工具行与刷新后从会话记录解析出的工具行文字一致（刷新不会多出 / 变样）。
func TestClaudeLiveToolMatchesTranscript(t *testing.T) {
	line := `{"type":"assistant","message":{"id":"m1","role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"Write","input":{"file_path":"a.txt","content":"hi"}}]}}`
	ps, _, ok := claudeActivityPieces([]byte(line))
	if !ok || len(ps) != 1 {
		t.Fatalf("pieces=%v ok=%v", ps, ok)
	}
	hist := ParseClaudeTranscript(strings.NewReader(line + "\n"))
	if len(hist) != 1 || hist[0].Role != "tool" || hist[0].Content != ps[0].Text {
		t.Fatalf("hist=%v live=%q", hist, ps[0].Text)
	}
}

func TestCodexStreamLiveTools(t *testing.T) {
	a := &CLIAgentAdapter{kind: cliKindCodex}
	got := seq(scanFixture(t, a, "testdata/stream/codex_tools.jsonl"))
	want := strings.Join([]string{
		"text||看一下目录。",
		"tool|item_1|shell bash -lc ls",
		"tool|item_1|shell bash -lc ls",
		"tool|item_2|apply_patch\nupdate a.txt",
		`tool|item_3|fs.read {"p":"x"}`,
		"tool|item_4|web_search wails notify",
		"tool|item_5|shell false · 失败",
		"text||好了。",
	}, "\n")
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	for _, p := range scanFixture(t, a, "testdata/stream/codex_tools.jsonl") {
		if p.Kind == "tool" && !p.Replace {
			t.Fatalf("tool piece must be replace (same id updates in place): %+v", p)
		}
	}
}

package catagent

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func rawLine(t *testing.T, s string) map[string]json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// v0.47 §6.19.19.5 Codex：turn.completed.usage。字段名取自 openai/codex exec_events.rs `struct Usage`
// （仓库 testdata 里没有带 usage 的真机 turn.completed：codex 样本是未登录 401 运行）。
// reasoning_output_tokens 是 output 的子项、cached_input_tokens 是 input 的子项，都不得再相加。
func TestParseCodexUsage_NoDoubleCount(t *testing.T) {
	raw := rawLine(t, `{"type":"turn.completed","usage":{"input_tokens":1200,"cached_input_tokens":800,"cache_write_input_tokens":0,"output_tokens":300,"reasoning_output_tokens":120}}`)
	p, ok := parseCodexUsage(raw)
	if !ok || p.Kind != "usage" {
		t.Fatalf("%+v %v", p, ok)
	}
	if p.ID != "1500" {
		t.Fatalf("total=%s want 1500 (input+output only)", p.ID)
	}
	if p.Text != "300" {
		t.Fatalf("output=%s want 300 (reasoning not added)", p.Text)
	}
	// 只有 total_tokens → 合计取它，输出省略
	p, ok = parseCodexUsage(rawLine(t, `{"type":"turn.completed","usage":{"total_tokens":42}}`))
	if !ok || p.ID != "42" || p.Text != "" {
		t.Fatalf("%+v", p)
	}
	if _, ok := parseCodexUsage(rawLine(t, `{"type":"turn.completed"}`)); ok {
		t.Fatal("no usage → not ok")
	}
}

// 仓库里 codex 真机 stdout 样本（未登录）没有 turn.completed，解析后不得冒出 usage。
func TestCodexSample_NoTurnCompletedUsage(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "codex", "exec-json-unauth-0.162.1.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	a := NewCodexAdapter(CLIAgentConfig{DataTemp: t.TempDir()})
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		p, _, _, _ := a.parseCLIStreamLine(sc.Bytes())
		if p.Kind == "usage" {
			t.Fatalf("unexpected usage from unauth sample: %+v", p)
		}
	}
}

// v0.47 Claude：result.usage.output_tokens。usage 对象形状取自仓库真机 transcript
// testdata/claude/061a686f-….jsonl（claude 2.1.296，值全为 0）；仓库没有真机 type=result 行，
// 故把该 usage 对象嵌进 result 事件，并另以非 0 值验证取数。
func TestClaudeResultUsage_OutputTokens(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "claude", "061a686f-48a9-43ac-91f6-cc737232921d.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	i := strings.Index(string(b), `"usage":{`)
	if i < 0 {
		t.Fatal("sample lacks usage")
	}
	dec := json.NewDecoder(strings.NewReader(string(b[i+len(`"usage":`):])))
	var usage json.RawMessage
	if err := dec.Decode(&usage); err != nil {
		t.Fatal(err)
	}
	var u map[string]json.RawMessage
	_ = json.Unmarshal(usage, &u)
	if _, ok := u["output_tokens"]; !ok {
		t.Fatal("sample usage lacks output_tokens")
	}
	// 真机样本值为 0 → 无合计，不产出 usage（不估算）
	raw := rawLine(t, `{"type":"result","session_id":"s","usage":`+string(usage)+`}`)
	if p, ok := parseClaudeResult(raw, "s"); ok && p.Kind == "usage" {
		t.Fatalf("zero sample must not yield usage: %+v", p)
	}
	raw = rawLine(t, `{"type":"result","session_id":"s","duration_ms":999999,"usage":{"input_tokens":10,"cache_creation_input_tokens":5,"cache_read_input_tokens":100,"output_tokens":42}}`)
	p, ok := parseClaudeResult(raw, "s")
	if !ok || p.ID != "157" || p.Text != "42" {
		t.Fatalf("%+v", p)
	}
	// 没有 output_tokens → 输出省略
	p, _ = parseClaudeResult(rawLine(t, `{"type":"result","usage":{"input_tokens":10}}`), "")
	if p.ID != "10" || p.Text != "" {
		t.Fatalf("%+v", p)
	}
}

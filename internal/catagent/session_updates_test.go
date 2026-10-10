package catagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectSessionUpdates_MatchesLiveShape(t *testing.T) {
	in := strings.Join([]string{
		`{"method":"session/update","params":{"update":{"sessionUpdate":"user_message_chunk","content":{"type":"text","text":"你好"}}}}`,
		`{"method":"session/update","params":{"update":{"sessionUpdate":"agent_thought_chunk","content":{"text":"先"}}}}`,
		`{"method":"session/update","params":{"update":{"sessionUpdate":"agent_thought_chunk","content":{"text":"看"}}}}`,
		`{"method":"session/update","params":{"update":{"sessionUpdate":"tool_call","toolCallId":"c1","toolName":"read_file","rawInput":{"path":"a.go"}}}}`,
		`{"method":"session/update","params":{"update":{"sessionUpdate":"tool_call_update","toolCallId":"c1","status":"completed","locations":[{"path":"a.go"},{"path":"b.go"}]}}}`,
		`{"method":"session/update","params":{"update":{"sessionUpdate":"agent_message_chunk","content":{"text":"甲"}}}}`,
		`{"method":"session/update","params":{"update":{"sessionUpdate":"agent_message_chunk","content":{"text":"乙"}}}}`,
		`{"method":"session/update","params":{"update":{"sessionUpdate":"turn_completed"}}}`,
		`{"method":"session/update","params":{"update":{"sessionUpdate":"user_message_chunk","content":{"type":"text","text":"继续"}}}}`,
		`{"method":"session/update","params":{"update":{"sessionUpdate":"tool_call","toolCallId":"c1","toolName":"grep","rawInput":{"pattern":"foo","path":"src"}}}}`,
		`{"method":"session/update","params":{"update":{"sessionUpdate":"agent_message_chunk","content":{"text":"好"}}}}`,
		``,
	}, "\n")
	got, err := ProjectSessionUpdates([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	want := []WireMessage{
		{Role: "user", Content: "你好"},
		{Role: "thought", Content: "先看"},
		{Role: "tool", Content: "read_file\na.go\nb.go"},
		{Role: "assistant", Content: "甲乙"},
		{Role: "user", Content: "继续"},
		{Role: "tool", Content: "grep · foo\nsrc"},
		{Role: "assistant", Content: "好"},
	}
	if len(got) != len(want) {
		t.Fatalf("%d %+v", len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("[%d] got %+v want %+v", i, got[i], want[i])
		}
	}
}

func TestProjectSessionUpdates_TextStaysBetweenTools(t *testing.T) {
	in := strings.Join([]string{
		`{"method":"session/update","params":{"update":{"sessionUpdate":"user_message_chunk","content":{"type":"text","text":"做一份脚本"}}}}`,
		`{"method":"session/update","params":{"update":{"sessionUpdate":"agent_thought_chunk","content":{"text":"先看目录"}}}}`,
		`{"method":"session/update","params":{"update":{"sessionUpdate":"agent_message_chunk","content":{"text":"我先看一下。"}}}}`,
		`{"method":"session/update","params":{"update":{"sessionUpdate":"tool_call","toolCallId":"c1","toolName":"list_dir","rawInput":{"path":"D:\\GoProject\\test"}}}}`,
		`{"method":"session/update","params":{"update":{"sessionUpdate":"agent_message_chunk","content":{"text":"再写配置。"}}}}`,
		`{"method":"session/update","params":{"update":{"sessionUpdate":"tool_call","toolCallId":"c2","toolName":"write","rawInput":{"path":"docker-compose.yml"}}}}`,
		`{"method":"session/update","params":{"update":{"sessionUpdate":"agent_message_chunk","content":{"text":"用法在下面。"}}}}`,
		``,
	}, "\n")
	got, err := ProjectSessionUpdates([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	want := []WireMessage{
		{Role: "user", Content: "做一份脚本"},
		{Role: "thought", Content: "先看目录"},
		{Role: "assistant", Content: "我先看一下。"},
		{Role: "tool", Content: "list_dir\nD:\\GoProject\\test"},
		{Role: "assistant", Content: "再写配置。"},
		{Role: "tool", Content: "write\ndocker-compose.yml"},
		{Role: "assistant", Content: "用法在下面。"},
	}
	if len(got) != len(want) {
		t.Fatalf("%d %+v", len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("[%d] got %+v want %+v", i, got[i], want[i])
		}
	}
}

func TestProjectSessionUpdates_KeepsResultError(t *testing.T) {
	rows, err := ProjectSessionUpdates([]byte("{\"type\":\"result\",\"subtype\":\"error_during_execution\",\"is_error\":true,\"errors\":[\"network timeout\"]}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Role != "system" || rows[0].Content != "network timeout" {
		t.Fatalf("%+v", rows)
	}
}

func TestFollowSessionUpdates_ReadsNewLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "updates.jsonl")
	body := "{\"method\":\"session/update\",\"params\":{\"update\":{\"sessionUpdate\":\"agent_message_chunk\",\"content\":{\"text\":\"流\"}}}}\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	close(stop)
	var mode int32
	var got []string
	followSessionUpdates(path, 0, &mode, stop, func(p streamPiece) {
		got = append(got, p.Kind+":"+p.Text)
	}, nil)
	if strings.Join(got, "") != "text:流" {
		t.Fatalf("%v", got)
	}
	if mode != followFile {
		t.Fatalf("mode %d", mode)
	}
}

func TestLoadTranscript_PrefersUpdates(t *testing.T) {
	dir := t.TempDir()
	sessions := t.TempDir()
	cwd := filepath.Join(dir, "work")
	convID := "conv-updates"
	sessionDir := filepath.Dir(contextSignalsPath(sessions, cwd, convID))
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "{\"method\":\"session/update\",\"params\":{\"update\":{\"sessionUpdate\":\"user_message_chunk\",\"content\":{\"type\":\"text\",\"text\":\"问\"}}}}\n" +
		"{\"method\":\"session/update\",\"params\":{\"update\":{\"sessionUpdate\":\"agent_message_chunk\",\"content\":{\"text\":\"答\"}}}}\n"
	if err := os.WriteFile(filepath.Join(sessionDir, "updates.jsonl"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	a := NewBuildAdapter(BuildConfig{DataTemp: t.TempDir(), SessionsRoot: sessions})
	got, err := a.LoadTranscript(t.Context(), convID, cwd)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Content != "问" || got[1].Role != "assistant" || got[1].Content != "答" {
		t.Fatalf("%+v", got)
	}
}

package catagent

import (
	"encoding/json"
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
		if !wireEqual(got[i], want[i]) {
			t.Fatalf("[%d] got %+v want %+v", i, got[i], want[i])
		}
	}
}

func wireEqual(a, b WireMessage) bool {
	if a.Role != b.Role || a.Content != b.Content || len(a.Media) != len(b.Media) {
		return false
	}
	for i := range a.Media {
		if a.Media[i] != b.Media[i] {
			return false
		}
	}
	return true
}

func TestProjectSessionUpdates_ImageDoesNotDumpData(t *testing.T) {
	in := strings.Join([]string{
		`{"method":"session/update","params":{"update":{"sessionUpdate":"user_message_chunk","content":{"type":"text","text":"看这张"}}}}`,
		`{"method":"session/update","params":{"update":{"sessionUpdate":"user_message_chunk","content":{"type":"image","data":"AAAA","mimeType":"image/png"}}}}`,
		`{"method":"session/update","params":{"update":{"sessionUpdate":"agent_message_chunk","content":{"text":"看到了"}}}}`,
		`{"method":"session/update","params":{"update":{"sessionUpdate":"turn_completed"}}}`,
		`{"method":"session/update","params":{"update":{"sessionUpdate":"user_message_chunk","content":{"type":"image","data":"BBBB","mimeType":"image/png"}}}}`,
		``,
	}, "\n")
	got, err := ProjectSessionUpdates([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("%d %+v", len(got), got)
	}
	if got[0].Role != "user" || got[0].Content != "看这张\n图片" {
		t.Fatalf("user %+v", got[0])
	}
	if strings.Contains(got[0].Content, "AAAA") || strings.Contains(got[2].Content, "BBBB") {
		t.Fatalf("image data leaked: %+v", got)
	}
	if len(got[0].Media) != 1 || got[0].Media[0].Kind != "image" || got[0].Media[0].Mime != "image/png" || got[0].Media[0].Data != "AAAA" {
		t.Fatalf("media %+v", got[0].Media)
	}
	if got[2].Role != "user" || got[2].Content != "图片" {
		t.Fatalf("second %+v", got[2])
	}
	if len(got[2].Media) != 1 || got[2].Media[0].Data != "BBBB" {
		t.Fatalf("second media %+v", got[2].Media)
	}
}

func TestProjectSessionUpdates_PreviewPathStaysInText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.mp4")
	text := "看这个\n" + path + "\nnotes.txt"
	raw, err := json.Marshal(map[string]any{
		"method": "session/update",
		"params": map[string]any{
			"update": map[string]any{
				"sessionUpdate": "user_message_chunk",
				"content":       map[string]any{"type": "text", "text": text},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := ProjectSessionUpdates(append(raw, '\n'))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Content != text {
		t.Fatalf("content %+v", got)
	}
	if len(got[0].Media) != 1 || got[0].Media[0].Kind != "video" || got[0].Media[0].Path != path || got[0].Media[0].Name != "a.mp4" {
		t.Fatalf("media %+v", got[0].Media)
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
		if !wireEqual(got[i], want[i]) {
			t.Fatalf("[%d] got %+v want %+v", i, got[i], want[i])
		}
	}
}

func TestProjectSessionUpdates_RetryStaysOnlyWhileCurrent(t *testing.T) {
	retry := `{"method":"_x.ai/session/update","params":{"update":{"sessionUpdate":"retry_state","type":"retrying","attempt":1,"max_retries":15,"reason":"transport"}}}`
	msg := `{"method":"session/update","params":{"update":{"sessionUpdate":"agent_message_chunk","content":{"text":"好了"}}}}`
	recovered, err := ProjectSessionUpdates([]byte(retry + "\n" + msg + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered) != 1 || recovered[0].Role != "assistant" || recovered[0].Content != "好了" {
		t.Fatalf("recovered %+v", recovered)
	}
	live, err := ProjectSessionUpdates([]byte(msg + "\n" + retry + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(live) != 2 || live[1].Role != "status" || live[1].Content != "正在重试（1/15）" {
		t.Fatalf("live %+v", live)
	}
}

func TestRetryStatusText(t *testing.T) {
	if retryStatusText(2, 15) != "正在重试（2/15）" || retryStatusText(3, 0) != "正在重试（第 3 次）" || retryStatusText(0, 0) != "正在重试" {
		t.Fatalf("%q %q %q", retryStatusText(2, 15), retryStatusText(3, 0), retryStatusText(0, 0))
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

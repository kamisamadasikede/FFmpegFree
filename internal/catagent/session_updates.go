package catagent

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

const (
	followAuto int32 = iota
	followFile
	followStdout
)

// sessionUpdatesPath 是 CLI 自己追加的会话事件。当场输出和刷新后的记录都读它，避免和 export 的 Markdown 各画一套。
func sessionUpdatesPath(root, cwd, convID string) string {
	p := contextSignalsPath(root, cwd, convID)
	if p == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(p), "updates.jsonl")
}

// ProjectSessionUpdates 把 updates.jsonl 收成和这一轮流式结束时相同的消息顺序：
// 用户原句按原样。同一段思考、同一次工具调用留在第一次出现的位置，后面只更新内容。
// 助手正文接在当时的位置；中间新出现思考或工具后，再来的正文另起一段，不再抽回上面。
func ProjectSessionUpdates(data []byte) ([]WireMessage, error) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	var rows []*WireMessage
	cur := newUpdateTurn()
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		if name, content, ok := peekSessionUpdate(line); ok {
			switch name {
			case "user_message_chunk":
				text := strings.TrimSpace(extractContentDelta(content))
				if text == "" {
					continue
				}
				cur = newUpdateTurn()
				row := &WireMessage{Role: "user", Content: text}
				rows = append(rows, row)
				continue
			case "turn_completed":
				cur = newUpdateTurn()
				continue
			}
		}
		piece, errMsg, isErr := parseStreamLine(line, cur.snaps)
		if isErr {
			if msg := strings.TrimSpace(errMsg); msg != "" {
				rows = append(rows, &WireMessage{Role: "system", Content: msg})
			}
			continue
		}
		if piece.Text == "" {
			continue
		}
		switch piece.Kind {
		case "thought":
			key := piece.ID
			if key == "" {
				key = "thought"
			}
			row := cur.thought[key]
			if row == nil {
				row = &WireMessage{Role: "thought"}
				cur.thought[key] = row
				rows = append(rows, row)
				cur.assistant = nil
			}
			if piece.Replace {
				row.Content = piece.Text
			} else {
				row.Content += piece.Text
			}
		case "tool":
			id := piece.ID
			if id == "" {
				id = "tool"
			}
			row := cur.tools[id]
			if row == nil {
				row = &WireMessage{Role: "tool"}
				cur.tools[id] = row
				rows = append(rows, row)
				cur.assistant = nil
			}
			row.Content = piece.Text
		default:
			if cur.assistant == nil {
				cur.assistant = &WireMessage{Role: "assistant"}
				rows = append(rows, cur.assistant)
			}
			cur.assistant.Content += piece.Text
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	out := make([]WireMessage, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.Content) == "" {
			continue
		}
		out = append(out, *row)
	}
	if out == nil {
		out = []WireMessage{}
	}
	return out, nil
}

type updateTurn struct {
	thought   map[string]*WireMessage
	assistant *WireMessage
	tools     map[string]*WireMessage
	snaps     map[string]*toolSnap
}

func newUpdateTurn() *updateTurn {
	return &updateTurn{
		thought: map[string]*WireMessage{},
		tools:   map[string]*WireMessage{},
		snaps:   map[string]*toolSnap{},
	}
}

func peekSessionUpdate(line []byte) (name string, content json.RawMessage, ok bool) {
	var raw map[string]json.RawMessage
	if json.Unmarshal(line, &raw) != nil {
		return "", nil, false
	}
	method, okm := rawStr(raw, "method")
	if !okm || method != "session/update" {
		return "", nil, false
	}
	params, okp := raw["params"]
	if !okp {
		return "", nil, false
	}
	var p struct {
		Update struct {
			SessionUpdate string          `json:"sessionUpdate"`
			Content       json.RawMessage `json:"content"`
		} `json:"update"`
	}
	if json.Unmarshal(params, &p) != nil || p.Update.SessionUpdate == "" {
		return "", nil, false
	}
	return p.Update.SessionUpdate, p.Update.Content, true
}

// followSessionUpdates 从 offset 之后跟着读新行。文件先出字就用文件；标准输出先出字就用标准输出。只走一条，避免同一句画两遍。
func followSessionUpdates(path string, offset int64, mode *int32, stop <-chan struct{}, onPiece func(streamPiece), onErr func(string)) {
	if path == "" || onPiece == nil || mode == nil {
		return
	}
	var pending []byte
	tools := map[string]*toolSnap{}
	var textBuf strings.Builder
	stopFollow := false
	flushText := func() {
		if textBuf.Len() == 0 {
			return
		}
		text := textBuf.String()
		textBuf.Reset()
		if !claimFollow(mode, followFile) {
			stopFollow = true
			return
		}
		onPiece(streamPiece{Kind: "text", Text: text})
	}
	emit := func(p streamPiece) bool {
		if p.Text == "" {
			return false
		}
		if p.Kind == "text" || p.Kind == "" {
			textBuf.WriteString(p.Text)
			return false
		}
		flushText()
		if stopFollow {
			return true
		}
		if !claimFollow(mode, followFile) {
			return true
		}
		onPiece(p)
		return false
	}
	readMore := func(final bool) {
		f, err := os.Open(path)
		if err != nil {
			return
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil {
			return
		}
		// 文件被截短（压缩会话）时从新的末尾接着看，不能把旧记录再播一遍。
		if st.Size() < offset {
			offset = st.Size()
			pending = nil
			return
		}
		if st.Size() > offset {
			buf := make([]byte, st.Size()-offset)
			n, readErr := f.ReadAt(buf, offset)
			if n > 0 {
				offset += int64(n)
				pending = append(pending, buf[:n]...)
			}
			if readErr != nil && readErr != io.EOF && n == 0 {
				return
			}
		}
		for !stopFollow {
			i := bytes.IndexByte(pending, '\n')
			var line []byte
			if i >= 0 {
				line = pending[:i]
				pending = pending[i+1:]
			} else if final && len(bytes.TrimSpace(pending)) > 0 {
				line = pending
				pending = nil
			} else {
				break
			}
			line = bytes.TrimSpace(line)
			if len(line) == 0 {
				continue
			}
			piece, errMsg, isErr := parseStreamLine(line, tools)
			if isErr {
				if onErr != nil {
					if msg := strings.TrimSpace(errMsg); msg != "" {
						onErr(msg)
					}
				}
				continue
			}
			if piece.Text == "" {
				continue
			}
			if emit(piece) {
				return
			}
		}
		flushText()
	}
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			readMore(true)
			return
		case <-ticker.C:
			readMore(false)
			if stopFollow {
				return
			}
		}
	}
}

func claimFollow(mode *int32, want int32) bool {
	for {
		v := atomic.LoadInt32(mode)
		if v == want {
			return true
		}
		if v != followAuto {
			return false
		}
		if atomic.CompareAndSwapInt32(mode, followAuto, want) {
			return true
		}
	}
}

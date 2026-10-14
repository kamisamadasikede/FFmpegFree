package cat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"FFmpegFree/internal/store"
)

// messageFooter 是应用侧按对话持久化的消息尾元数据（契约 v0.37.1）。
// 无新迁移：落在 <DataRoot>/cat/footers/<convId>.json。
type messageFooter struct {
	TurnID       string   `json:"turnId"`
	MessageID    string   `json:"messageId"`
	FinishedAt   int64    `json:"finishedAt"`
	TokenTotal   *int64   `json:"tokenTotal,omitempty"`
	TokensPerSec *float64 `json:"tokensPerSec,omitempty"`
	FirstTokenMs *int64   `json:"firstTokenMs,omitempty"` // v0.45
	TotalMs      *int64   `json:"totalMs,omitempty"`      // v0.45
}

var footerMu sync.Mutex

func footerPath(dataRoot, convID string) string {
	root := strings.TrimSpace(dataRoot)
	convID = strings.TrimSpace(convID)
	if root == "" || convID == "" {
		return ""
	}
	return filepath.Join(root, "cat", "footers", convID+".json")
}

func loadFooters(dataRoot, convID string) []messageFooter {
	p := footerPath(dataRoot, convID)
	if p == "" {
		return nil
	}
	b, err := os.ReadFile(p)
	if err != nil || len(b) == 0 {
		return nil
	}
	var list []messageFooter
	if json.Unmarshal(b, &list) != nil {
		return nil
	}
	return list
}

func appendFooter(dataRoot, convID string, f messageFooter) {
	p := footerPath(dataRoot, convID)
	if p == "" || f.FinishedAt <= 0 {
		return
	}
	footerMu.Lock()
	defer footerMu.Unlock()
	list := loadFooters(dataRoot, convID)
	// 同 turn 覆盖（终态后 context 补发不应重复堆积）。
	replaced := false
	for i := range list {
		if list[i].TurnID == f.TurnID {
			list[i] = f
			replaced = true
			break
		}
	}
	if !replaced {
		list = append(list, f)
	}
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	raw, err := json.Marshal(list)
	if err != nil {
		return
	}
	_ = os.WriteFile(p, raw, 0o600)
}

// applyFooters 把已持久化的消息尾按「assistant 出现顺序」合并进导出正文。
// 旧消息无对应记录 → 不补假数据。
func applyFooters(dataRoot, convID string, msgs []store.CatMessage) []store.CatMessage {
	list := loadFooters(dataRoot, convID)
	if len(list) == 0 || len(msgs) == 0 {
		return msgs
	}
	ai := 0
	for i := range msgs {
		if msgs[i].Role != "assistant" {
			continue
		}
		if ai >= len(list) {
			break
		}
		f := list[ai]
		ai++
		msgs[i].FinishedAt = f.FinishedAt
		msgs[i].TokenTotal = f.TokenTotal
		msgs[i].TokensPerSec = f.TokensPerSec
		msgs[i].FirstTokenMs = f.FirstTokenMs
		msgs[i].TotalMs = f.TotalMs
	}
	return msgs
}

// computeTokensPerSec 按契约：tokenTotal / (durationMs/1000)；durationMs≤0 → nil。
func computeTokensPerSec(tokenTotal, durationMs int64) *float64 {
	if tokenTotal <= 0 || durationMs <= 0 {
		return nil
	}
	v := float64(tokenTotal) / (float64(durationMs) / 1000.0)
	return &v
}

// msPtr 是计时字段：0 毫秒也是有效值（同一毫秒内出字），负数 / 未知 → nil。
func msPtr(from, to int64) *int64 {
	if from <= 0 || to <= 0 || to < from {
		return nil
	}
	d := to - from
	return &d
}

func int64Ptr(v int64) *int64 {
	if v <= 0 {
		return nil
	}
	x := v
	return &x
}

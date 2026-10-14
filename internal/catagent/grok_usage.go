package catagent

import (
	"bytes"
	"context"
	"encoding/json"
	"time"
)

// 契约 v0.45 §6.19.19.2：Grok 流里没有 token，终态后跑 `grok usage <SESSION_ID>` 取本轮合计。
// H testdata/help/grok-usage.txt（grok 1.0.40）：`grok usage [OPTIONS] <SESSION_ID> [TURN]`，
// 不传 TURN = 会话合计 + 每个已记录的 turn。样本 testdata/grok/usage-1.0.40.json 为 box 真机输出；
// 未知会话 → stderr「Error: Session '<id>' not found.」、退出码 1（真机）。

const grokUsageTimeout = 5 * time.Second

type grokUsageOut struct {
	Turns []struct {
		TurnNumber  int64  `json:"turnNumber"`
		EndedAt     string `json:"endedAt"`
		TotalTokens int64  `json:"totalTokens"`
		// v0.47：grok 1.0.40 每个 turn 有独立 outputTokens（样本 reasoningTokens < outputTokens，按子项处理，不另加）。
		OutputTokens int64 `json:"outputTokens"`
	} `json:"turns"`
}

// grokTurnUsage 是选中那项 turn 的用量；0 = 没有。
type grokTurnUsage struct {
	Total  int64
	Output int64
}

// parseGrokUsage 取 endedAt 晚于本轮开始、turnNumber 最大的一项的 totalTokens / outputTokens；拿不到返回零值。
func parseGrokUsage(raw []byte, startedAt time.Time) grokTurnUsage {
	var u grokUsageOut
	if json.Unmarshal(bytes.TrimSpace(raw), &u) != nil {
		return grokTurnUsage{}
	}
	var bestN int64 = -1
	var got grokTurnUsage
	for _, t := range u.Turns {
		end, err := time.Parse(time.RFC3339Nano, t.EndedAt)
		if err != nil || !end.After(startedAt) {
			continue
		}
		if t.TurnNumber > bestN {
			bestN = t.TurnNumber
			got = grokTurnUsage{Total: t.TotalTokens, Output: t.OutputTokens}
		}
	}
	if got.Total < 0 {
		got.Total = 0
	}
	if got.Output < 0 {
		got.Output = 0
	}
	return got
}

func grokUsageArgs(sessionID string) []string {
	return []string{"usage", sessionID}
}

// grokTurnTokens 在与本轮同样的环境下跑 grok usage；超时、非 0 退出、无匹配 turn 都返回零值（不估算）。
// 用独立 ctx：取消 / 失败的轮也尝试取（§6.19.19.2 第 4 条）。
func (a *BuildAdapter) grokTurnTokens(exe string, env []string, dir, sessionID string, startedAt time.Time) grokTurnUsage {
	if exe == "" || sessionID == "" {
		return grokTurnUsage{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), grokUsageTimeout)
	defer cancel()
	cmd := a.runCmd(ctx, exe, grokUsageArgs(sessionID)...)
	cmd.Dir = dir
	cmd.Env = env
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		if a.cfg.Logf != nil {
			a.cfg.Logf("cat build usage: %v %s", err, truncate(errb.String(), 300))
		}
		return grokTurnUsage{}
	}
	return parseGrokUsage(out.Bytes(), startedAt)
}

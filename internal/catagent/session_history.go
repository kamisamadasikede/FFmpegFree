package catagent

import (
	"context"
	"errors"
)

// SessionHistory 是打开对话时从 CLI 会话文件读正文的统一入口（老板 / 架构师：多 CLI 聚合壳；契约 v0.45.1 §6.19.19.0）。
// 服务层只按 agentKind 取适配器，再调这里；库里只存元数据（含我们记下的 sessionId）。
// 读不到 → 空列表，从不拼接本地消息缓存、不捏造历史。续聊也走同一份 sessionId。
// 形状照 Grok（BuildAdapter.LoadTranscript）现有路径：三家都实现本接口，不按家各自特判。
type SessionHistory interface {
	LoadTranscript(ctx context.Context, convID, cwd string) ([]WireMessage, error)
	// SessionLabel 侧栏短标题（首句用户话）；只当次展示，不回写库。没有就空串。
	SessionLabel(convID, cwd string) string
	// SessionResumable：是否已有可续会话（下一句带 --resume）。cwd 给 Grok 定位会话目录；Claude / Codex 看存下的 id，可忽略 cwd。
	SessionResumable(convID, cwd string) bool
	// ReadContextUsage 直接读 CLI 自己的会话文件取上下文占用（v0.48.3）：Grok signals.json、Codex rollout token_count、
	// Claude transcript usage。拿不到 ok=false，前端不画圆环；应用不落盘任何上下文文件。
	ReadContextUsage(convID, cwd string) (used, window int64, ok bool)
}

var (
	_ SessionHistory = (*BuildAdapter)(nil)
	_ SessionHistory = (*CLIAgentAdapter)(nil)
)

// 读历史的两种「没有正文」（v0.45.1 historyStatus）。LoadTranscript 返回空列表 + 其中之一；服务层据此填状态、不报错。
var (
	// ErrHistoryMissing：存过 sessionId，但 CLI 会话文件已不在 / grok export 报 not found。
	// 从没存过 sessionId（新对话）不返回错误（v0.45.2）。
	ErrHistoryMissing = errors.New("cat history missing")
	// ErrHistoryReadFailed：文件在但打不开 / 解析失败，或 export 其它错误。
	ErrHistoryReadFailed = errors.New("cat history read failed")
)

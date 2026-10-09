// Package catagent 实现 Cat 助手适配器注册表与 Build 适配器（契约 6.19）。
// 界面与 message 只出现「Cat 助手」；永不出现 grok / CLI / 命令行 / 可执行文件名。
package catagent

import "FFmpegFree/internal/apperr"

// AgentKind 是会话锁定的适配器种类。
const (
	KindCatBuild = "cat_build"
	// 二期预留（一期不注册）：
	KindCatCLI   = "cat_cli"
	KindCatCode  = "cat_code"
	KindCodexCLI = "codex_cli"
)

// 访问模式（一期只允许 ask）。
const (
	AccessAsk  = "ask"  // 请求批准
	AccessFull = "full" // 完全访问（置灰不可选）
)

// 组件状态（契约 6.19.4）。
const (
	StateChecking = "checking"
	StateReady    = "ready"
	StateMissing  = "missing"
	StateFailed   = "failed"
)

// 事件名（契约 6.19.5）。
const (
	EventStatus  = "cat:status"
	EventMessage = "cat:message"
	EventTurn    = "cat:turn"
)

// 用户可见文案（契约 6.19.7）。
const (
	MsgNotReady     = "Cat 助手还没准备好，发布后即可使用。"
	MsgReplyFailed  = "回复没生成出来，请重试。"
	MsgToolWriteRef = "当前只能查看项目文件，改文件和运行命令下一期开放。"
)

// Status 是 CatStatus。
type Status struct {
	State       string           `json:"state"` // checking | ready | missing | failed
	Version     string           `json:"version"`
	CanDownload bool             `json:"canDownload"`
	Error       *apperr.AppError `json:"error,omitempty"`
}

// Model 是 CatModel。
type Model struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}

// ThinkLevel 是 CatThinkLevel。
type ThinkLevel struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}

// MessageEvent 是 cat:message 载荷（可含流式增量；一期先整段）。
type MessageEvent struct {
	ConversationID string `json:"conversationId"`
	MessageID      string `json:"messageId"`
	Role           string `json:"role"`
	Content        string `json:"content"`
	Delta          bool   `json:"delta,omitempty"` // true = 流式增量追加到同 messageId
	CreatedAt      int64  `json:"createdAt"`
}

// TurnEvent 是 cat:turn 载荷。
type TurnEvent struct {
	ConversationID string           `json:"conversationId"`
	Status         string           `json:"status"` // succeeded | failed | canceled
	Error          *apperr.AppError `json:"error,omitempty"`
}

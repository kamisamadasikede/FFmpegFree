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
	EventProject = "cat:project" // v0.31，6.19.10.3
)

// 用户可见文案（契约 6.19.7）。
const (
	MsgNotReady     = "Cat 助手还没准备好，发布后即可使用。"
	MsgReplyFailed  = "回复没生成出来，请重试。"
	MsgToolWriteRef = "当前只能查看项目文件，改文件和运行命令下一期开放。"

	// v0.31 项目（6.19.10.8 / 6.19.10.9）。后三句是架构师建议文案，产品改了只改这里。
	MsgProjectMissing  = "项目文件夹不见了。"
	MsgProjectNotFound = "找不到这个项目。"
	MsgProjectDelete   = "删除项目失败，请重试。"
	MsgProjectPath     = "请选择一个文件夹。"
	MsgProjectRoot     = "不能把整个磁盘作为项目，请选择里面的文件夹。"
	MsgProjectName     = "名字需要 1~60 个字。"
	// v0.31.1（6.19.10.2 第 8 条 / 6.19.10.8）。
	MsgProjectDuplicate   = "这个文件夹已经建过项目了。"
	MsgProjectTurnRunning = "有对话正在回复，请先停止再换文件夹。"
	MsgProjectRevealFail  = "无法打开文件管理器。"
	// 工具失败结果（回给适配器，不是界面文案）：对话不属于项目时没有项目根。
	MsgToolNoProject = "这个对话没有项目文件夹，不能查看文件。"
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

// cat:message 增量操作（契约 6.19.9）。
const (
	OpAppend  = "append"
	OpReplace = "replace"
	OpDone    = "done"
)

// cat:turn 状态（契约 6.19.9）。
const (
	TurnRunning   = "running"
	TurnCancelled = "cancelled"
	TurnFailed    = "failed"
	TurnCompleted = "completed"
)

// BlockText 是正文块类型。
const BlockText = "text"

// MessageBlock 是 replace 时携带的整块内容。
type MessageBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// MessageEvent 是 cat:message 增量载荷（契约 6.19.9）。
// 同一 messageId 内 seq 从 1 严格递增；done 之后不再有同 messageId 事件。
type MessageEvent struct {
	ConvID    string        `json:"convId"`
	TurnID    string        `json:"turnId"`
	MessageID string        `json:"messageId"`
	Seq       int           `json:"seq"`
	Op        string        `json:"op"` // append | replace | done
	Block     *MessageBlock `json:"block,omitempty"`
	TextDelta string        `json:"textDelta,omitempty"`
}

// TurnEvent 是 cat:turn 载荷（契约 6.19.9）。
type TurnEvent struct {
	ConvID string `json:"convId"`
	TurnID string `json:"turnId"`
	Status string `json:"status"` // running | cancelled | failed | completed
}

// CancelCatTurnRequest 是 CancelCatTurn 入参（契约 6.19.9）。
type CancelCatTurnRequest struct {
	ConvID string `json:"convId"`
	TurnID string `json:"turnId"`
}

// ProjectEvent 是 cat:project 载荷（契约 6.19.10.3）：某个项目的 missing 与上一次计算结果不同时发。
type ProjectEvent struct {
	ID      string `json:"id"`
	Missing bool   `json:"missing"`
}

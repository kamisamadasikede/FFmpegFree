// Package catagent 实现 Cat 助手适配器注册表与 Build 适配器（契约 6.19）。
// 界面文案与 message 只出现「Cat 助手」；永不出现 grok / CLI / 命令行 / 可执行文件名。
// 例外（老板 10-09 定）：模型选择器直接显示 CLI 返回的真实模型名（如 grok-4.6），不做映射。
package catagent

import "FFmpegFree/internal/apperr"

// AgentKind 是会话锁定的适配器种类。
// ReasonSessionNotFound 是续聊会话不存在的失败 detail（v0.45）；服务层据此置 retryable=false。
const ReasonSessionNotFound = "reason=session_not_found"

const (
	KindCatBuild  = "cat_build"
	KindClaudeCLI = "claude_cli" // v0.36
	KindCodexCLI  = "codex_cli"
	// 仍延后（胶囊置灰「下一期开放」）：
	KindCatCLI  = "cat_cli"
	KindCatCode = "cat_code"
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
	// MsgNotReady：PATH 上找不到入口（v0.31.3）。
	MsgNotReady = "未检测到 Cat 助手，请先安装并确保可在终端直接运行。"
	// MsgAuthInvalid：入口在但登录 / API Key 失效（仍用 CAT_NOT_READY，detail reason=auth）。
	MsgAuthInvalid = "登录失效，请重新登录后再试。"
	MsgReplyFailed = "回复没生成出来，请重试。"
	// MsgSessionGone（v0.45）：续聊时之前的会话已找不到；本轮不可重试，下一句开新对话。
	MsgSessionGone  = "之前的对话记录已找不到，请重新发送，将开始新的对话。"
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
	// 刷新后旧进程还占着这场会话时，不再开第二个。
	MsgTurnStillRunning  = "这条对话还在回复，请先停止再发。"
	MsgProjectRevealFail = "无法打开文件管理器。"
	// 工具失败结果（回给适配器，不是界面文案）：对话不属于项目时没有项目根。
	MsgToolNoProject = "这个对话没有项目文件夹，不能查看文件。"
)

// AgentStatus 是某一家的就绪概况（契约 v0.36 CatAgentStatus）。
type AgentStatus struct {
	AgentKind string `json:"agentKind"` // cat_build | claude_cli | codex_cli
	Ready     bool   `json:"ready"`
	Version   string `json:"version,omitempty"`
	LoggedIn  *bool  `json:"loggedIn,omitempty"` // 该家不需要登录时可省略
	Detail    string `json:"detail,omitempty"`
	// v0.44（6.19.18.2）：该家模式列表，保守→放开，前端不得重排。
	Modes []AgentMode `json:"modes"`
}

// Status 是 CatStatus。
type Status struct {
	State       string           `json:"state"` // checking | ready | missing | failed；兼容：整体态仍以 cat_build 为主
	Version     string           `json:"version"`
	CanDownload bool             `json:"canDownload"`
	Error       *apperr.AppError `json:"error,omitempty"`
	Agents      []AgentStatus    `json:"agents"` // v0.36：固定顺序 cat_build, claude_cli, codex_cli
}

// Model 是 CatModel。
type Model struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	// v0.44（6.19.18.2）：该模型支持的档位 id；未知 = []（前端隐藏强度行）。
	Efforts []string `json:"efforts"`
	// v0.44：该模型默认档；未知 = ""。
	DefaultEffort string `json:"defaultEffort"`
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
// ContextUsed / ContextWindow 是这一轮结束后的上下文占用；没有真实数字时省略，界面不画 0%。
// Error 只在失败时带上，message 是这一轮看到的原文。
type TurnEvent struct {
	ConvID string `json:"convId"`
	TurnID string `json:"turnId"`
	Status string `json:"status"` // running | cancelled | failed | completed
	// Retryable（v0.44.5，6.19.18.7 第 7 条）：始终输出；仅最后一轮失败且后端内存里仍有失败记录时为 true。
	Retryable     bool             `json:"retryable"`
	ContextUsed   int64            `json:"contextUsed,omitempty"`
	ContextWindow int64            `json:"contextWindow,omitempty"`
	Error         *apperr.AppError `json:"error,omitempty"`
	// v0.37.1 消息尾：仅终态可带；running 禁止。
	MessageID    string   `json:"messageId,omitempty"`
	FinishedAt   int64    `json:"finishedAt,omitempty"`
	TokenTotal   *int64   `json:"tokenTotal,omitempty"`
	TokensPerSec *float64 `json:"tokensPerSec,omitempty"` // v0.47：outputTokens / 生成秒数
	OutputTokens *int64   `json:"outputTokens,omitempty"` // v0.47 §6.19.19.5：无则省略
	// v0.45 §6.19.19.3：应用侧计时（毫秒，起点 = 本轮 running）。无正文 / 思考增量 → 省略 firstTokenMs；
	// totalMs 有 finishedAt 就带（完成、失败、取消都带）。
	FirstTokenMs *int64 `json:"firstTokenMs,omitempty"`
	TotalMs      *int64 `json:"totalMs,omitempty"`
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

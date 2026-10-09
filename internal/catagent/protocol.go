package catagent

// ProtocolVersion 是与 Build 入口约定的协议版本。
// v2：PATH 上的 grok headless（streaming-json）；路径 / 可执行名仍不回前端。
const ProtocolVersion = 2

/*
协议（cat_build / Grok Build CLI，docs.x.ai/build）：

就绪：exec.LookPath("grok")（Windows 亦可 grok.exe）。认证继承用户环境
XAI_API_KEY 或 ~/.grok/auth.json（grok login），无 URL/Key UI。

调用（一期）：
  grok -p "<最新一条用户消息>" --output-format streaming-json \
       --cwd <projectAbsPath|应用临时目录> -s <conversationId> \
       --no-auto-update --no-alt-screen
  可选：-m <modelId>（非 default 时）
  环境：GROK_SANDBOX=read-only GROK_WRITE_FILE=0 GROK_DISABLE_AUTOUPDATER=1
  不传 --always-approve（一期只读）。

streaming-json（NDJSON，捕获自 CLI 0.2.x；官方未正式发布 schema）：
  {"type":"text","data":"..."}      → OnTextDelta / cat:message append
  {"type":"thought","data":"..."}   → 忽略
  {"type":"end","stopReason":"...","sessionId":"..."} → 回合结束
  {"type":"error","message":"..."}  → CAT_REPLY_FAILED

取消：context 取消 → proc.Kill 进程树。

会话：-s 使用 conversationId，多轮由 CLI 会话续写；应用只传本轮最新用户句。
无项目：--cwd 落到 <DataTemp>/cat/cwd-<id>/，不把项目工具上下文传给 CLI。
*/

// WireMessage 是协议里的一条消息。
type WireMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ToolRequest 是入口二进制请求的工具调用。
type ToolRequest struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"` // list_dir | read_text | write_file | exec
	Path    string `json:"path,omitempty"`
	Content string `json:"content,omitempty"`
	Command string `json:"command,omitempty"`
}

// ToolResult 是应用回填的工具结果。
type ToolResult struct {
	ID      string `json:"id"`
	OK      bool   `json:"ok"`
	Content string `json:"content"`
}

// TurnRequest 是一轮对话请求（旧文件协议保留类型；v2 不再写盘）。
type TurnRequest struct {
	Version        int           `json:"version"`
	ConversationID string        `json:"conversationId"`
	TurnID         string        `json:"turnId"`
	ModelID        string        `json:"modelId"`
	ThinkLevelID   string        `json:"thinkLevelId,omitempty"`
	ProjectPath    string        `json:"projectPath,omitempty"`
	Messages       []WireMessage `json:"messages"`
	ToolResults    []ToolResult  `json:"toolResults,omitempty"`
}

// TurnResponse 是一轮对话响应。
type TurnResponse struct {
	Version      int           `json:"version"`
	Message      WireMessage   `json:"message"`
	ToolRequests []ToolRequest `json:"toolRequests,omitempty"`
}

// Capabilities 是能力探测结果。
type Capabilities struct {
	Version     int          `json:"version"`
	Models      []Model      `json:"models"`
	ThinkLevels []ThinkLevel `json:"thinkLevels"`
}

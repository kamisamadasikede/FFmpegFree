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
  批准方式只带一个开关：全部批准 / 空 → --always-approve；
  普通模式 → --permission-mode default；plan → --permission-mode plan；
  自动审查 → --permission-mode auto。不与 --always-approve 同时传。
  环境：只强制 GROK_DISABLE_AUTOUPDATER=1。不设置 GROK_SANDBOX、GROK_WRITE_FILE。
  不传 --tools、--disallowed-tools、--deny、--no-subagents、--disable-web-search。
  工具集是本机 grok 自己的完整集合（写、改、跑命令、联网、MCP、子代理等，以 CLI 当前版本为准）。

streaming-json（NDJSON，ACP session/update，或旧的 type/data）：
  正文增量                         → OnTextDelta / cat:message append（助手正文）
  agent_thought_chunk / thought    → OnActivity thought（界面单独一行，不写入助手正文）
  tool_call / tool_call_update     → OnActivity tool（界面单独一行）
  end                              → 回合结束
  error / result.is_error / 超时类标准错误 → CAT_REPLY_FAILED（message 用原文）

取消：context 取消 → proc.Kill 进程树。

会话：-s 使用 conversationId，多轮由 CLI 会话续写；应用只传本轮最新用户句。
无项目：--cwd 用 paths.ResolveCatConvDir（新对话 <数据目录>/cat/conversations/<id>，
已有 <数据目录>/tmp/cat/cwd-<id> 的旧对话继续用旧目录）。DataRoot 未配置时回退到 DataTemp。
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

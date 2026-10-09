package catagent

// ProtocolVersion 是与 Build 入口二进制约定的协议版本。
// 老板发布包后若字段变更，只升版本并在适配器里兼容；路径 / 可执行名仍不回前端。
const ProtocolVersion = 1

/*
协议草图（TBD，稳定后回写契约 6.19.3）：

调用方式（一期占位）：
  <entry> --component-root <dir> --request <request.json> --response <response.json>
  <entry> --component-root <dir> --capabilities <capabilities.json>

request.json（version=1）：
  {
    "version": 1,
    "conversationId": "...",
    "turnId": "...",
    "modelId": "...",
    "thinkLevelId": "...",   // 可空
    "projectPath": "...",    // 可选只读根
    "messages": [ {"role":"user|assistant|system","content":"..."} ],
    "toolResults": [ {"id":"...","ok":true,"content":"..."} ]
  }

response.json（version=1）：
  {
    "version": 1,
    "message": {"role":"assistant","content":"..."},
    "toolRequests": [
      {"id":"...","kind":"list_dir|read_text|write_file|exec","path":"...","content":"...","command":"..."}
    ]
  }

capabilities.json（version=1）：
  {
    "version": 1,
    "models": [ {"id":"...","displayName":"Cat 助手 1.0"} ],
    "thinkLevels": [ {"id":"high","displayName":"高"} ]  // 可空数组 → 前端隐藏强度
  }

流式：后续可改为 stdout NDJSON 行事件；一期先整段 response，事件形状已预留 delta。
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

// TurnRequest 是一轮对话请求文件。
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

// TurnResponse 是一轮对话响应文件。
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

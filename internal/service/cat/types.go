package cat

import (
	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/store"
)

// Status 复用适配器状态。
type Status = catagent.Status

// Model / ThinkLevel 复用。
type Model = catagent.Model
type ThinkLevel = catagent.ThinkLevel

// Conversation 是列表项。
type Conversation = store.CatConversation

// ConversationDetail 含消息。
// ContextUsed / ContextWindow 是打开会话时读到的上下文占用；没有则省略。
type ConversationDetail struct {
	store.CatConversation
	Messages      []store.CatMessage `json:"messages"`
	ContextUsed   int64              `json:"contextUsed,omitempty"`
	ContextWindow int64              `json:"contextWindow,omitempty"`
}

// CreateConversationRequest 创建会话。
type CreateConversationRequest struct {
	AgentKind string `json:"agentKind"`
	Title     string `json:"title"`
	// ProjectPath 自 v0.31 作废：后端忽略，只为绑定兼容保留（6.19.10.1 第 3 条）。
	ProjectPath string `json:"projectPath"`
	AccessMode  string `json:"accessMode"` // 一期只接受 ask / 空
	// ProjectID 是所属项目（v0.31，可选；空 = 不属于任何项目）。创建后不可变。
	ProjectID string `json:"projectId,omitempty"`
}

// SendMessageRequest 发送消息。
type SendMessageRequest struct {
	ConversationID string `json:"conversationId"`
	Content        string `json:"content"`
	ModelID        string `json:"modelId"`
	ThinkLevelID   string `json:"thinkLevelId"`
	// ProjectPath 自 v0.31 作废：后端忽略，只读工具的根只来自对话所属项目（6.19.10.5）。
	ProjectPath string `json:"projectPath"`
}

// SendMessageResult 是 SendCatMessage 返回值。
// v0.30.1：同步只返回已落库的用户消息与本轮 turnId；助手回复走 cat:message / cat:turn 流式事件。
// AssistantMessage 保留字段以兼容，流式路径下恒为空。
type SendMessageResult struct {
	UserMessage      store.CatMessage  `json:"userMessage"`
	TurnID           string            `json:"turnId"`
	AssistantMessage *store.CatMessage `json:"assistantMessage,omitempty"`
	Error            *apperr.AppError  `json:"error,omitempty"`
}

// CancelTurnRequest 是 CancelCatTurn 入参（契约 6.19.9）。
type CancelTurnRequest = catagent.CancelCatTurnRequest

// Project 是 CatProject（契约 6.19.10.1）。missing 由后端实时计算，不落库，始终输出。
type Project struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
	Missing   bool   `json:"missing"`
}

// CreateProjectRequest 是 CreateCatProjectRequest。
type CreateProjectRequest struct {
	Path string `json:"path"`
	Name string `json:"name,omitempty"`
}

// CreateProjectResult 是 CreateCatProjectResult：existed=true 表示这个路径已经建过项目，返回的是已有的那个。
type CreateProjectResult struct {
	Project Project `json:"project"`
	Existed bool    `json:"existed"`
}

// RenameProjectRequest 是 RenameCatProjectRequest。
type RenameProjectRequest struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// DeleteProjectRequest 是 DeleteCatProjectRequest。
type DeleteProjectRequest struct {
	ID string `json:"id"`
}

// RelocateProjectRequest 是 RelocateCatProjectRequest（v0.31.1）。
type RelocateProjectRequest struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

// RevealProjectRequest 是 RevealCatProjectRequest（v0.31.1）。
type RevealProjectRequest struct {
	ID string `json:"id"`
}

// ProjectEvent 是 cat:project 载荷。
type ProjectEvent = catagent.ProjectEvent

// ListFilesRequest 是 ListCatFiles 入参（契约 6.19.11.1）。RelPath 空 = 根。
type ListFilesRequest struct {
	ConvID  string `json:"convId"`
	RelPath string `json:"relPath"`
}

// FileEntry 是文件面板的一层条目。符号链接 / junction 的 IsDir 恒为 false。
type FileEntry struct {
	Name    string `json:"name"`
	RelPath string `json:"relPath"`
	IsDir   bool   `json:"isDir"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"modTime"`
}

// ListFilesResult 是 ListCatFiles 返回值。Entries 始终是数组（空为 []）。
type ListFilesResult struct {
	Root      string      `json:"root"`
	Entries   []FileEntry `json:"entries"`
	Truncated bool        `json:"truncated"`
}

// RevealConversationFolderRequest 是 RevealCatConversationFolder 入参。
type RevealConversationFolderRequest struct {
	ConvID string `json:"convId"`
}

// ReadFileRequest 是 ReadCatFile 入参。RelPath 指向根下的一个文件。
type ReadFileRequest struct {
	ConvID  string `json:"convId"`
	RelPath string `json:"relPath"`
}

// ReadFileResult 是侧边预览的文件内容。文本放 Content，有界的图片和文档放 DataBase64。
// Kind：text、image、media、pdf、docx、xlsx、pptx、doc、binary、tooLarge。
type ReadFileResult struct {
	RelPath    string `json:"relPath"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Size       int64  `json:"size"`
	Content    string `json:"content"`
	DataBase64 string `json:"dataBase64"`
	Mime       string `json:"mime"`
	Language   string `json:"language"`
	Editable   bool   `json:"editable"`
	ModTime    int64  `json:"modTime"`
}

// WriteFileRequest 是 WriteCatFile 入参。只覆盖已有文本，Content 为完整新内容。
type WriteFileRequest struct {
	ConvID  string `json:"convId"`
	RelPath string `json:"relPath"`
	Content string `json:"content"`
}

// WriteFileResult 是保存后的大小和修改时间。
type WriteFileResult struct {
	RelPath string `json:"relPath"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"modTime"`
}

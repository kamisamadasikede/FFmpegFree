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
type ConversationDetail struct {
	store.CatConversation
	Messages []store.CatMessage `json:"messages"`
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

// ProjectEvent 是 cat:project 载荷。
type ProjectEvent = catagent.ProjectEvent

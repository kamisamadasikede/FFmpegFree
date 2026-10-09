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
	AgentKind   string `json:"agentKind"`
	Title       string `json:"title"`
	ProjectPath string `json:"projectPath"`
	AccessMode  string `json:"accessMode"` // 一期只接受 ask / 空
}

// SendMessageRequest 发送消息。
type SendMessageRequest struct {
	ConversationID string `json:"conversationId"`
	Content        string `json:"content"`
	ModelID        string `json:"modelId"`
	ThinkLevelID   string `json:"thinkLevelId"`
	ProjectPath    string `json:"projectPath"`
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

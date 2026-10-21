package cat

import "FFmpegFree/internal/store"

type (
	Group        = store.CatGroup
	MemberBrief  = store.CatGroupMemberBrief
	QueuedTask   = store.CatGroupQueuedTask
	GroupMember  = store.CatGroupMember
	GroupMessage = store.CatGroupMessage
	GroupReport  = store.CatGroupReport
	RouteInfo    = store.CatRouteInfo
	RouteDetail  = store.CatGroupRouteDetail
	Charter      = store.CatCharter
)

type CreateGroupRequest struct {
	ProjectID string `json:"projectId"`
	Name      string `json:"name"`
}

type RenameGroupRequest struct {
	GroupID string `json:"groupId"`
	Name    string `json:"name"`
}

type GroupDetail struct {
	Group     Group                    `json:"group"`
	Members   []GroupMember            `json:"members"`
	Workflows []store.CatGroupWorkflow `json:"workflows"`
}

type SetGroupAutoRouteRequest struct {
	GroupID string `json:"groupId"`
	On      bool   `json:"on"`
}

type AddGroupMemberRequest struct {
	GroupID      string `json:"groupId"`
	Name         string `json:"name"`
	Title        string `json:"title"`
	AgentKind    string `json:"agentKind"`
	ModelID      string `json:"modelId"`
	ThinkLevelID string `json:"thinkLevelId"`
}

type UpdateGroupMemberRequest struct {
	MemberID     string  `json:"memberId"`
	Name         *string `json:"name"`
	Title        *string `json:"title"`
	AgentKind    *string `json:"agentKind"`
	ModelID      *string `json:"modelId"`
	ThinkLevelID *string `json:"thinkLevelId"`
}

type ReorderGroupMembersRequest struct {
	GroupID   string   `json:"groupId"`
	MemberIDs []string `json:"memberIds"`
}

type UpdateCharterRequest struct {
	MemberID string   `json:"memberId"`
	Owns     *string  `json:"owns"`
	NotTouch *string  `json:"notTouch"`
	Dirs     []string `json:"dirs"`
}

type SendGroupMessageRequest struct {
	ClientMessageID string           `json:"clientMessageId,omitempty"`
	GroupID         string           `json:"groupId"`
	Text            string           `json:"text"`
	Mentions        []string         `json:"mentions"`
	Files           []SendAttachment `json:"files,omitempty"`
}

type SendGroupMessageResult struct {
	UserMessage GroupMessage `json:"userMessage"`
	ChainID     string       `json:"chainId"`
}

type ReassignGroupRouteRequest struct {
	TaskID     string `json:"taskId,omitempty"`
	GroupID    string `json:"groupId"`
	ChainID    string `json:"chainId"`
	ToMemberID string `json:"toMemberId"`
}

type ReassignGroupRouteResult struct {
	System GroupMessage `json:"system"`
}

type ListGroupMessagesRequest struct {
	GroupID   string `json:"groupId"`
	BeforeSeq int64  `json:"beforeSeq"`
	Limit     int    `json:"limit"`
}

type ListGroupMessagesResult struct {
	Messages []GroupMessage `json:"messages"`
	HasMore  bool           `json:"hasMore"`
}

type GetRouteDetailRequest struct {
	GroupID string `json:"groupId"`
	Seq     int64  `json:"seq"`
}

type GetRouteDetailResult struct {
	Input    string `json:"input"`
	Output   string `json:"output"`
	Mode     string `json:"mode"`
	MemberID string `json:"memberId"`
	Reason   string `json:"reason"`
	// 6.21.17 ③：派活判断耗时；旧数据两列 0；任一为 0 时省略 durationMs。
	StartedAt  int64  `json:"startedAt"`
	FinishedAt int64  `json:"finishedAt"`
	DurationMs *int64 `json:"durationMs,omitempty"`
}

// GroupFile 是「文件」页签的一行（契约 6.21.17 ①）。
type GroupFile struct {
	Path         string `json:"path"`
	LastMemberID string `json:"lastMemberId"`
	LastSeq      int64  `json:"lastSeq"`
	LastTurnID   string `json:"lastTurnId"`
	ChangeCount  int    `json:"changeCount"`
	Overreach    bool   `json:"overreach"`
	Tag          string `json:"tag,omitempty"`
	Exists       bool   `json:"exists"`
	UpdatedAt    int64  `json:"updatedAt"`
}

type RevertOverreachRequest struct {
	MemberID string   `json:"memberId"`
	TurnID   string   `json:"turnId"`
	Files    []string `json:"files"`
}

type RevertSkippedFile struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type RevertOverreachResult struct {
	Reverted []string            `json:"reverted"`
	Skipped  []RevertSkippedFile `json:"skipped"`
}

type RetryGroupMemberTurnRequest struct {
	TaskID   string `json:"taskId,omitempty"`
	GroupID  string `json:"groupId"`
	MemberID string `json:"memberId"`
	ChainID  string `json:"chainId"`
}

// GroupMemberEvent is cat:group-member payload (契约 6.21.14 ④).
type GroupMemberEvent struct {
	GroupID string        `json:"groupId"`
	Member  GroupMember   `json:"member"`
	Running *GroupRunning `json:"running,omitempty"`
	// 6.21.17 ④：与 member.queueLength / member.queue 相同，方便状态条直接用。
	QueueLength int          `json:"queueLength"`
	Queue       []QueuedTask `json:"queue"`
}

type GroupRunning struct {
	TurnID    string `json:"turnId"`
	StartedAt int64  `json:"startedAt"`
	ToolCount int    `json:"toolCount"`
	ElapsedMs int64  `json:"elapsedMs"`
}

// GroupRouteEvent is cat:group-route payload.
type GroupRouteEvent struct {
	GroupID string    `json:"groupId"`
	ChainID string    `json:"chainId"`
	State   string    `json:"state"` // routing|routed
	Route   RouteInfo `json:"route"`
}

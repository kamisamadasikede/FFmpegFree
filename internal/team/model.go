package team

import "FFmpegFree/internal/catagent"

type Human struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Role           string `json:"role"`
	Avatar         string `json:"avatar"`
	NotifyMentions bool   `json:"notifyMentions"`
}
type Employee struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Avatar         string `json:"avatar"`
	Role           string `json:"role"`
	Specialties    string `json:"specialties"`
	Focus          string `json:"focus"`
	Standards      string `json:"standards"`
	Knowledge      string `json:"knowledge"`
	AgentKind      string `json:"agentKind"`
	Model          string `json:"model"`
	Effort         string `json:"effort"`
	PermissionMode string `json:"permissionMode"`
	Transport      string `json:"transport"`
	Enabled        bool   `json:"enabled"`
	Version        int64  `json:"version"`
}
type Room struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ProjectID     string `json:"projectId"`
	Workspace     string `json:"workspace"`
	CoordinatorID string `json:"coordinatorId"`
	Paused        bool   `json:"paused"`
	Archived      bool   `json:"archived"`
	RosterVersion int64  `json:"rosterVersion"`
	UpdatedAt     int64  `json:"updatedAt"`
}
type Member struct {
	ID         string `json:"id"`
	RoomID     string `json:"roomId"`
	Kind       string `json:"kind"`
	ProfileID  string `json:"profileId"`
	Name       string `json:"name"`
	Role       string `json:"role"`
	Active     bool   `json:"active"`
	Generation int64  `json:"generation"`
}
type Message struct {
	ID           string   `json:"id"`
	RoomID       string   `json:"roomId"`
	RoomSeq      int64    `json:"roomSeq"`
	Revision     int64    `json:"revision"`
	FromMemberID string   `json:"fromMemberId"`
	TaskID       string   `json:"taskId,omitempty"`
	RunID        string   `json:"runId,omitempty"`
	ReplyToID    string   `json:"replyToId,omitempty"`
	Kind         string   `json:"kind"`
	Content      string   `json:"content"`
	Mentions     []string `json:"mentions"`
	Status       string   `json:"status"`
	CreatedAt    int64    `json:"createdAt"`
}
type Task struct {
	ID              string   `json:"id"`
	RoomID          string   `json:"roomId"`
	MemberID        string   `json:"memberId"`
	RequesterID     string   `json:"requesterId"`
	ParentID        string   `json:"parentId,omitempty"`
	MessageID       string   `json:"messageId"`
	SessionID       string   `json:"sessionId"`
	ConfigKey       string   `json:"executionConfigKey,omitempty"`
	Title           string   `json:"title"`
	Instruction     string   `json:"instruction"`
	Acceptance      []string `json:"acceptance"`
	Status          string   `json:"status"`
	Revision        int64    `json:"revision"`
	RunID           string   `json:"runId"`
	CanSteer        bool     `json:"canSteer"`
	InputMode       string   `json:"inputMode"`
	Activity        string   `json:"activity"`
	Result          string   `json:"result"`
	Error           string   `json:"error"`
	Required        bool     `json:"required"`
	DependsOn       []string `json:"dependsOn"`
	WaitFor         []string `json:"waitFor"`
	FinishRequested bool     `json:"finishRequested"`
	ResumeRequested bool     `json:"resumeRequested"`
	Validation      string   `json:"validation"`
	CreatedAt       int64    `json:"createdAt"`
	UpdatedAt       int64    `json:"updatedAt"`
}
type InboxItem struct {
	ID                 string `json:"id"`
	RoomID             string `json:"roomId"`
	MessageID          string `json:"messageId"`
	TaskID             string `json:"taskId"`
	RecipientID        string `json:"recipientId"`
	NeedsReply         bool   `json:"needsReply"`
	ReadAt             int64  `json:"readAt"`
	AnsweredAt         int64  `json:"answeredAt"`
	NotificationStatus string `json:"notificationStatus"`
	CreatedAt          int64  `json:"createdAt"`
}
type Event struct {
	RoomID   string `json:"roomId"`
	EventSeq int64  `json:"eventSeq"`
	Kind     string `json:"kind"`
	Data     any    `json:"data"`
}
type Snapshot struct {
	Room       Room        `json:"room"`
	Members    []Member    `json:"members"`
	Messages   []Message   `json:"messages"`
	Tasks      []Task      `json:"tasks"`
	Inbox      []InboxItem `json:"inbox"`
	Artifacts  []Artifact  `json:"artifacts"`
	EventSeq   int64       `json:"eventSeq"`
	HasEarlier bool        `json:"hasEarlier"`
}
type Bootstrap struct {
	Human     Human                           `json:"human"`
	Employees []Employee                      `json:"employees"`
	Rooms     []Room                          `json:"rooms"`
	Inbox     []InboxItem                     `json:"inbox"`
	Modes     map[string][]catagent.AgentMode `json:"modes"`
}
type CreateRoom struct {
	Name          string   `json:"name"`
	ProjectID     string   `json:"projectId"`
	EmployeeIDs   []string `json:"employeeIds"`
	CoordinatorID string   `json:"coordinatorId"`
}

// Command is shared by human bindings and authenticated employee tools.
type Command struct {
	APIVersion       int      `json:"apiVersion"`
	RequestID        string   `json:"requestId"`
	RoomID           string   `json:"roomId"`
	Intent           string   `json:"intent"`
	Content          string   `json:"content"`
	Title            string   `json:"title"`
	TargetMemberIDs  []string `json:"targetMemberIds"`
	ReplyToID        string   `json:"replyToMessageId"`
	TaskID           string   `json:"targetTaskId"`
	ExpectedRunID    string   `json:"expectedRunId"`
	ExpectedRevision int64    `json:"expectedTaskRevision"`
	Acceptance       []string `json:"acceptance"`
	DependsOn        []string `json:"dependsOn"`
	Required         bool     `json:"requiredForCompletion"`
}
type Delivery struct {
	MemberID string `json:"memberId"`
	TaskID   string `json:"taskId,omitempty"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
}
type CommandResult struct {
	Message    Message    `json:"message"`
	Deliveries []Delivery `json:"deliveries"`
}
type Notification struct{ ID, RoomID, MessageID, TaskID, Title, Body string }

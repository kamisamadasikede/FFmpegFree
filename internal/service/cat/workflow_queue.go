package cat

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/id"
	"FFmpegFree/internal/store"
)

type QueuedMessage struct {
	ID        string             `json:"id"`
	Request   SendMessageRequest `json:"request"`
	CreatedAt int64              `json:"createdAt"`
	Held      bool               `json:"held"`
}
type MessageQueue struct {
	Revision         int64           `json:"revision"`
	ConvID           string          `json:"convId"`
	Items            []QueuedMessage `json:"items"`
	Paused           bool            `json:"paused"`
	Error            string          `json:"error,omitempty"`
	QueueingDisabled bool            `json:"queueingDisabled"`
	CanSteer         bool            `json:"canSteer"`
}
type QueueAction struct {
	ConvID   string              `json:"convId"`
	ID       string              `json:"id"`
	Action   string              `json:"action"`
	Content  string              `json:"content"`
	BeforeID string              `json:"beforeId"`
	Request  *SendMessageRequest `json:"request,omitempty"`
}

func (s *Service) readQueue(ctx context.Context, convID string) (MessageQueue, error) {
	q := MessageQueue{ConvID: convID, Items: []QueuedMessage{}}
	_, err := s.cfg.Store.GetSetting(ctx, "cat.queue."+convID, &q)
	if q.Items == nil {
		q.Items = []QueuedMessage{}
	}
	if !s.queueArmed[convID] && len(q.Items) > 0 {
		q.Paused = true
	}
	s.mu.Lock()
	t := s.turns[convID]
	s.mu.Unlock()
	q.CanSteer = false
	if t != nil {
		t.mu.Lock()
		q.CanSteer = t.steer != nil && !t.finished
		t.mu.Unlock()
	}
	return q, err
}
func (s *Service) saveQueue(ctx context.Context, q *MessageQueue) error {
	now := time.Now().UnixMilli()
	if now <= q.Revision {
		q.Revision++
	} else {
		q.Revision = now
	}
	if err := s.cfg.Store.SetSetting(ctx, "cat.queue."+q.ConvID, q); err != nil {
		return err
	}
	s.emit("cat:queue", *q)
	return nil
}
func (s *Service) GetCatMessageQueue(ctx context.Context, convID string) (MessageQueue, error) {
	if _, err := s.requireConv(ctx, convID); err != nil {
		return MessageQueue{}, err
	}
	s.workflowMu.Lock()
	defer s.workflowMu.Unlock()
	return s.readQueue(ctx, convID)
}
func (s *Service) QueueCatMessage(ctx context.Context, req SendMessageRequest) (MessageQueue, error) {
	if _, err := s.requireConv(ctx, req.ConversationID); err != nil {
		return MessageQueue{}, err
	}
	if (strings.TrimSpace(req.Content) == "" && len(req.Attachments) == 0) || utf8.RuneCountInString(req.Content) > 100000 {
		return MessageQueue{}, apperr.New(apperr.InvalidArgument, "消息为空或过长。")
	}
	files := make([]catagent.PromptFile, 0, len(req.Attachments))
	for _, f := range req.Attachments {
		files = append(files, catagent.PromptFile{Name: f.Name, Path: f.Path})
	}
	if _, err := catagent.NormalizePromptFiles(files); err != nil {
		return MessageQueue{}, err
	}
	s.workflowMu.Lock()
	defer s.workflowMu.Unlock()
	q, err := s.readQueue(ctx, req.ConversationID)
	if err != nil {
		return q, err
	}
	if len(q.Items) >= 20 {
		return q, apperr.New(apperr.InvalidArgument, "待发消息最多 20 条。")
	}
	if s.queueArmed == nil {
		s.queueArmed = map[string]bool{}
	}
	s.queueArmed[q.ConvID] = true
	if len(q.Items) == 0 && !q.QueueingDisabled {
		q.Paused = false
		q.Error = ""
	}
	q.Items = append(q.Items, QueuedMessage{ID: id.New(), Request: req, CreatedAt: time.Now().UnixMilli()})
	if err = s.saveQueue(ctx, &q); err != nil {
		return q, err
	}
	return s.dispatchQueue(ctx, q)
}
func (s *Service) UpdateCatMessageQueue(ctx context.Context, req QueueAction) (MessageQueue, error) {
	if _, err := s.requireConv(ctx, req.ConvID); err != nil {
		return MessageQueue{}, err
	}
	s.workflowMu.Lock()
	defer s.workflowMu.Unlock()
	q, err := s.readQueue(ctx, req.ConvID)
	if err != nil {
		return q, err
	}
	if s.queueArmed == nil {
		s.queueArmed = map[string]bool{}
	}
	s.queueArmed[q.ConvID] = true
	switch req.Action {
	case "steer-new":
		if req.Request == nil || req.Request.ConversationID != req.ConvID {
			return q, apperr.New(apperr.InvalidArgument, "引导消息不正确。")
		}
		if err := s.steerWorkflowMessage(ctx, *req.Request); err != nil {
			return q, err
		}
		return q, nil
	case "pause":
		q.Paused = true
	case "resume":
		q.QueueingDisabled = false
		q.Paused = false
		q.Error = ""
	case "queue-off":
		q.QueueingDisabled, q.Paused = true, true
	case "queue-on":
		q.QueueingDisabled, q.Paused, q.Error = false, false, ""
	case "edit", "edit-start", "edit-cancel", "edit-send", "edit-steer", "delete", "guide", "send", "move", "steer":
		found := false
		for i, item := range q.Items {
			if item.ID != req.ID {
				continue
			}
			found = true
			if req.Action == "edit" || req.Action == "edit-send" || req.Action == "edit-steer" {
				if utf8.RuneCountInString(req.Content) > 100000 || (strings.TrimSpace(req.Content) == "" && len(item.Request.Attachments) == 0) {
					return q, apperr.New(apperr.InvalidArgument, "消息为空或过长。")
				}
				item.Request.Content, item.Held = req.Content, false
			}
			switch req.Action {
			case "delete":
				q.Items = append(q.Items[:i], q.Items[i+1:]...)
			case "edit":
				q.Items[i] = item
			case "edit-start":
				q.Items[i].Held = true
			case "edit-cancel":
				q.Items[i].Held = false
			case "guide":
				q.Items = append([]QueuedMessage{item}, append(q.Items[:i], q.Items[i+1:]...)...)
			case "send", "edit-send":
				q.QueueingDisabled = false
				item.Held = false
				q.Items = append([]QueuedMessage{item}, append(q.Items[:i], q.Items[i+1:]...)...)
				q.Paused, q.Error = false, ""
			case "steer", "edit-steer":
				if err := s.steerWorkflowMessage(ctx, item.Request); err != nil {
					return q, err
				}
				q.Items = append(q.Items[:i], q.Items[i+1:]...)
			case "move":
				if req.BeforeID == item.ID {
					break
				}
				remaining := append(append([]QueuedMessage{}, q.Items[:i]...), q.Items[i+1:]...)
				at := len(remaining)
				if req.BeforeID != "" {
					at = -1
					for j, other := range remaining {
						if other.ID == req.BeforeID {
							at = j
							break
						}
					}
					if at < 0 {
						return q, apperr.New(apperr.NotFound, "待发顺序已经变化，请重试。")
					}
				}
				q.Items = append(remaining[:at], append([]QueuedMessage{item}, remaining[at:]...)...)
			}
			break
		}
		if !found {
			return q, apperr.New(apperr.NotFound, "这条消息已经发送或删除。")
		}
	default:
		return q, apperr.New(apperr.InvalidArgument, "未知队列操作。")
	}
	if err = s.saveQueue(ctx, &q); err != nil {
		return q, err
	}
	return s.dispatchQueue(ctx, q)
}

// Queue removal follows native acceptance. A rejected/stale steer keeps its draft.
// The current process, permission mode and model stay in effect during steering.
func (s *Service) steerWorkflowMessage(ctx context.Context, req SendMessageRequest) error {
	if (strings.TrimSpace(req.Content) == "" && len(req.Attachments) == 0) || utf8.RuneCountInString(req.Content) > 100000 {
		return apperr.New(apperr.InvalidArgument, "消息为空或过长。")
	}
	files := make([]catagent.PromptFile, 0, len(req.Attachments))
	for _, f := range req.Attachments {
		files = append(files, catagent.PromptFile{Name: f.Name, Path: f.Path})
	}
	var err error
	files, err = catagent.NormalizePromptFiles(files)
	if err != nil {
		return err
	}
	s.mu.Lock()
	t := s.turns[req.ConversationID]
	s.mu.Unlock()
	if t == nil {
		return apperr.New(apperr.TaskConflict, "当前任务已经结束，请发送为下一轮。")
	}
	t.mu.Lock()
	steer, finished := t.steer, t.finished
	t.mu.Unlock()
	if steer == nil || finished {
		return apperr.New(apperr.TaskConflict, "当前任务尚不能接收实时引导，可以选择下一轮发送。")
	}
	if err = steer(ctx, req.Content, files); err != nil {
		return err
	}
	s.emit("cat:steered", struct {
		ConvID      string            `json:"convId"`
		Result      SendMessageResult `json:"result"`
		Attachments []SendAttachment  `json:"attachments"`
	}{req.ConversationID, SendMessageResult{UserMessage: store.CatMessage{ID: id.New(), Role: "user", Content: req.Content, CreatedAt: time.Now().UnixMilli()}, TurnID: t.turnID}, req.Attachments})
	return nil
}

// Called only while workflowMu is held. startTurn reserves the conversation
// atomically; it never replaces an existing process.
func (s *Service) dispatchQueue(ctx context.Context, q MessageQueue) (MessageQueue, error) {
	s.mu.Lock()
	busy := s.turns[q.ConvID] != nil || s.deleting[q.ConvID]
	s.mu.Unlock()
	if busy || q.Paused || len(q.Items) == 0 {
		return q, nil
	}
	if q.Items[0].Held {
		return q, nil
	}
	item := q.Items[0]
	req := item.Request
	req.queueID = item.ID
	_, err := s.startTurn(ctx, req, nil, nil)
	if err != nil {
		q.Paused = true
		q.Error = apperr.From(err).Message
		_ = s.saveQueue(ctx, &q)
		return q, nil
	}
	q.Items = q.Items[1:]
	q.Error = ""
	return q, s.saveQueue(ctx, &q)
}

// runTurn has returned and removed its process before another message starts.
func (s *Service) afterWorkflowTurn(t *turnState) {
	s.workflowMu.Lock()
	defer s.workflowMu.Unlock()
	ctx := context.Background()
	if _, err := s.requireConv(ctx, t.convID); err != nil {
		return
	}
	q, err := s.readQueue(ctx, t.convID)
	if err != nil || len(q.Items) == 0 {
		return
	}
	s.lastMu.Lock()
	lt := s.lastTurns[t.convID]
	success := lt != nil && lt.turnID == t.turnID && lt.status == catagent.TurnCompleted
	s.lastMu.Unlock()
	if !success {
		q.Paused = true
		q.Error = "上一轮停止或失败，待发消息已保留。"
		_ = s.saveQueue(ctx, &q)
		return
	}
	_, _ = s.dispatchQueue(ctx, q)
}

func (s *Service) markWorkflowDeleting(convID string) {
	s.workflowMu.Lock()
	defer s.workflowMu.Unlock()
	s.launchMu.Lock()
	defer s.launchMu.Unlock()
	s.mu.Lock()
	if s.deleting == nil {
		s.deleting = map[string]bool{}
	}
	s.deleting[convID] = true
	s.mu.Unlock()
	q, err := s.readQueue(context.Background(), convID)
	if err == nil {
		q.Paused = true
		_ = s.saveQueue(context.Background(), &q)
	}
}
func (s *Service) unmarkWorkflowDeleting(convID string) {
	s.mu.Lock()
	delete(s.deleting, convID)
	s.mu.Unlock()
}
func (s *Service) clearWorkflowStorage(ctx context.Context, convID string) {
	s.workflowMu.Lock()
	defer s.workflowMu.Unlock()
	delete(s.queueArmed, convID)
	_ = s.cfg.Store.DeleteSetting(ctx, "cat.queue."+convID)
	_ = s.cfg.Store.DeleteSetting(ctx, "cat.turnChanges."+convID)
}

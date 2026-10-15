package cat

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/id"
)

type QueuedMessage struct {
	ID        string             `json:"id"`
	Request   SendMessageRequest `json:"request"`
	CreatedAt int64              `json:"createdAt"`
}
type MessageQueue struct {
	Revision int64           `json:"revision"`
	ConvID   string          `json:"convId"`
	Items    []QueuedMessage `json:"items"`
	Paused   bool            `json:"paused"`
	Error    string          `json:"error,omitempty"`
}
type QueueAction struct {
	ConvID  string `json:"convId"`
	ID      string `json:"id"`
	Action  string `json:"action"`
	Content string `json:"content"`
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
	if len(q.Items) == 0 {
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
	case "pause":
		q.Paused = true
	case "resume":
		q.Paused = false
		q.Error = ""
	case "edit", "delete", "guide":
		found := false
		for i, item := range q.Items {
			if item.ID != req.ID {
				continue
			}
			found = true
			switch req.Action {
			case "delete":
				q.Items = append(q.Items[:i], q.Items[i+1:]...)
			case "edit":
				if utf8.RuneCountInString(req.Content) > 100000 || (strings.TrimSpace(req.Content) == "" && len(item.Request.Attachments) == 0) {
					return q, apperr.New(apperr.InvalidArgument, "消息为空或过长。")
				}
				q.Items[i].Request.Content = req.Content
			case "guide":
				q.Items = append([]QueuedMessage{item}, append(q.Items[:i], q.Items[i+1:]...)...)
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

// Called only while workflowMu is held. startTurn reserves the conversation
// atomically; it never replaces an existing process.
func (s *Service) dispatchQueue(ctx context.Context, q MessageQueue) (MessageQueue, error) {
	s.mu.Lock()
	busy := s.turns[q.ConvID] != nil || s.deleting[q.ConvID]
	s.mu.Unlock()
	if busy || q.Paused || len(q.Items) == 0 {
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

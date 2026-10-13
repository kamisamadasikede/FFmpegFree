package cat

import (
	"context"
	"strings"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/id"
)

const maxNotifyBodyRunes = 80

// notifyOrdinaryTurn 在普通对话一轮终态后发完成通知（契约 6.19.12.10）。
// cancelled 不发；开关关不发；前台不发系统通知（应用内 toast 由前端按 cat:turn 处理）；
// 后台发系统通知，点击走 OpenFromNotification → cat:notify。
func (s *Service) notifyOrdinaryTurn(ctx context.Context, convID, status string, turnErr *apperr.AppError) {
	if status != catagent.TurnCompleted && status != catagent.TurnFailed {
		return
	}
	if s.cfg.Notify == nil {
		return
	}
	if st, err := s.GetCatSettings(ctx); err == nil && !st.NotifyEnabled() {
		return
	}
	if s.cfg.Foreground != nil && s.cfg.Foreground() {
		return
	}
	title := "Cat 已回复"
	body := s.convNotifyBody(ctx, convID)
	if status == catagent.TurnFailed {
		title = "Cat 回复失败"
		if turnErr != nil {
			if msg := strings.TrimSpace(turnErr.Message); msg != "" {
				body = clipRunes(msg, maxNotifyBodyRunes)
			}
		}
	}
	s.cfg.Notify(Notification{
		ID:     id.New(),
		Title:  title,
		Body:   body,
		ConvID: convID,
	})
}

func (s *Service) convNotifyBody(ctx context.Context, convID string) string {
	if s.cfg.Store == nil || strings.TrimSpace(convID) == "" {
		return "新对话"
	}
	c, err := s.cfg.Store.GetCatConversation(ctx, convID)
	if err != nil {
		return "新对话"
	}
	s.applyCLITitle(ctx, &c)
	title := strings.TrimSpace(c.Title)
	if title == "" {
		return "新对话"
	}
	return clipRunes(title, maxNotifyBodyRunes)
}

func clipRunes(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n < 1 {
		return ""
	}
	return string(r[:n]) + "…"
}

package cat

import (
	"context"
	"strings"
	"sync/atomic"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/id"
)

const maxNotifyBodyRunes = 80

// 普通 turn 系统通知文案（契约 v0.48 6.19.12.11-2；中点 U+00B7 前后各一个半角空格）。
const (
	NotifyTitleTurnDone   = "Cat \u00b7 对话已完成"
	NotifyTitleTurnFailed = "Cat \u00b7 回复失败"
)

// NotifyStatus 是 GetCatNotifyStatus 的返回（v0.48 6.19.12.11-5）。
type NotifyStatus struct {
	Available  bool `json:"available"`
	Authorized bool `json:"authorized"`
}

// windowFocus 记前端最近一次上报的键盘焦点；从未上报 = 未聚焦（宁可多发一条）。
type windowFocus struct {
	reported atomic.Bool
	focused  atomic.Bool
}

// SetWindowFocused 由前端在 focus / blur / visibilitychange 时上报（绑定 SetCatWindowFocused）。
func (s *Service) SetWindowFocused(focused bool) {
	s.win.focused.Store(focused)
	s.win.reported.Store(true)
}

// WindowFocused = 最近一次上报为聚焦 且 窗口未隐藏、未最小化（Config.Foreground；nil 时只看上报值）。
func (s *Service) WindowFocused() bool {
	if !s.win.reported.Load() || !s.win.focused.Load() {
		return false
	}
	if s.cfg.Foreground != nil && !s.cfg.Foreground() {
		return false
	}
	return true
}

// GetCatNotifyStatus 报告系统通知是否可用 / 已授权；没接平台实现 → 都是 false。
func (s *Service) GetCatNotifyStatus() NotifyStatus {
	if s.cfg.NotifyStatus == nil {
		return NotifyStatus{}
	}
	return s.cfg.NotifyStatus()
}

// notifyOrdinaryTurn 在普通对话一轮终态后发 OS 系统通知（契约 v0.48 6.19.12.11）。
// cancelled 不发；开关关不发；窗口有键盘焦点不发（也不再有应用内 toast）；
// 否则发系统通知，点击走 OpenFromNotification → cat:notify。
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
	if s.WindowFocused() {
		return
	}
	title := NotifyTitleTurnDone
	body := s.convNotifyBody(ctx, convID)
	if status == catagent.TurnFailed {
		title = NotifyTitleTurnFailed
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

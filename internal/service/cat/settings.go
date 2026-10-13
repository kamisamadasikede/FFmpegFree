package cat

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
)

// SettingsKey 是 settings 表里 Cat 设置的键（契约 6.19.12.1 第 1 条）。
const SettingsKey = "cat.settings"

// MaxCustomInstructionRunes 是全局自定义指令的字数上限。
const MaxCustomInstructionRunes = 4000

// Settings 是 Cat 设置（契约 6.19.12.1 CatSettings + v0.35.1 notifyOnFinish）。
type Settings struct {
	DefaultModel        string `json:"defaultModel"`
	DefaultEffort       string `json:"defaultEffort"`
	DefaultApprovalMode string `json:"defaultApprovalMode"`
	CustomInstruction   string `json:"customInstruction"`
	// NotifyOnFinish 为 nil 视为 true；Get / Save 的返回值总是填好。
	NotifyOnFinish *bool `json:"notifyOnFinish"`
}

// NotifyEnabled 报告是否要发完成通知（nil = 开）。
func (s Settings) NotifyEnabled() bool { return s.NotifyOnFinish == nil || *s.NotifyOnFinish }

func (s Settings) normalized() Settings {
	s.DefaultModel = strings.TrimSpace(s.DefaultModel)
	s.DefaultEffort = strings.TrimSpace(s.DefaultEffort)
	s.DefaultApprovalMode = strings.TrimSpace(s.DefaultApprovalMode)
	if s.NotifyOnFinish == nil {
		t := true
		s.NotifyOnFinish = &t
	}
	return s
}

// ValidEffort 报告思考强度是否合法（空 = 默认）。
func ValidEffort(v string) bool {
	switch v {
	case "", "low", "medium", "high", "xhigh":
		return true
	}
	return false
}

// ValidApprovalMode 报告批准方式是否合法（空 = 全部批准）。
func ValidApprovalMode(v string) bool {
	switch v {
	case "", "normal", "plan", "auto", "approve":
		return true
	}
	return false
}

// GetCatSettings 读 Cat 设置；没有记录 = 全部空值（通知默认开）。
func (s *Service) GetCatSettings(ctx context.Context) (Settings, error) {
	if s.cfg.Store == nil {
		return Settings{}, apperr.New(apperr.Internal, "本地存储尚未初始化")
	}
	var v Settings
	if _, err := s.cfg.Store.GetSetting(ctx, SettingsKey, &v); err != nil {
		return Settings{}.normalized(), apperr.Wrap(apperr.IOError, "读取设置失败", err)
	}
	return v.normalized(), nil
}

// SaveCatSettings 整体保存并返回保存后的值。校验失败 INVALID_ARGUMENT；写入失败 IO_ERROR，库里仍是旧值（前端回滚，6.19.12.7）。
func (s *Service) SaveCatSettings(ctx context.Context, req Settings) (Settings, error) {
	if s.cfg.Store == nil {
		return Settings{}, apperr.New(apperr.Internal, "本地存储尚未初始化")
	}
	v := req.normalized()
	if !ValidEffort(v.DefaultEffort) {
		return Settings{}, apperr.New(apperr.InvalidArgument, "思考强度不对").WithDetail("reason=effort")
	}
	if !ValidApprovalMode(v.DefaultApprovalMode) {
		return Settings{}, apperr.New(apperr.InvalidArgument, "批准方式不对").WithDetail("reason=approval_mode")
	}
	if utf8.RuneCountInString(v.CustomInstruction) > MaxCustomInstructionRunes {
		return Settings{}, apperr.New(apperr.InvalidArgument, "最多 4000 字。").WithDetail("reason=too_long")
	}
	if err := s.cfg.Store.SetSetting(ctx, SettingsKey, v); err != nil {
		return Settings{}, apperr.Wrap(apperr.IOError, "保存失败，请重试。", err)
	}
	return v, nil
}

// rulesForTurn 是这一轮要带的全局自定义指令；读不到设置就不带（不让发送失败）。
func (s *Service) rulesForTurn(ctx context.Context) string {
	if s.cfg.Store == nil {
		return ""
	}
	v, err := s.GetCatSettings(ctx)
	if err != nil {
		s.cfg.Logf("cat settings read: %v", err)
		return ""
	}
	return strings.TrimSpace(v.CustomInstruction)
}

// InspectRequest 是 InspectCatConfig 的参数。
type InspectRequest struct {
	ProjectID string `json:"projectId"`
}

// InspectResult / InspectItem 复用适配器的类型（契约 CatInspectResult / CatInspectItem）。
type InspectResult = catagent.InspectResult
type InspectItem = catagent.InspectItem

// InspectCatConfig 只读列出技能、插件、钩子、MCP、子代理（契约 6.19.12.5）。失败返回空列表 + reason，error 为 nil。
func (s *Service) InspectCatConfig(ctx context.Context, req InspectRequest) (InspectResult, error) {
	a, err := s.adapter(catagent.KindCatBuild)
	if err != nil {
		return catagent.EmptyInspect(catagent.InspectNotReady), nil
	}
	insp, ok := a.(interface {
		Inspect(context.Context, string) catagent.InspectResult
	})
	if !ok || a.Status().State != catagent.StateReady {
		return catagent.EmptyInspect(catagent.InspectNotReady), nil
	}
	cwd := ""
	if pid := strings.TrimSpace(req.ProjectID); pid != "" && s.cfg.Store != nil {
		row, err := s.cfg.Store.GetCatProject(ctx, pid)
		if err == nil && !s.checkProject(row).Missing {
			cwd = row.Path
		} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
			s.cfg.Logf("cat inspect project: %v", err)
		}
	}
	if cwd == "" && s.cfg.DataRoot != "" {
		cwd = filepath.Join(s.cfg.DataRoot, "cat")
	}
	return insp.Inspect(ctx, cwd), nil
}

// ClearAllCatConversations 删掉所有对话（含定时任务产生的），先取消进行中的回复；
// 普通对话在数据目录里的自有文件夹尽力删（失败只记日志）；项目文件夹、定时任务和运行记录不动（6.19.13.7-4 / 6.19.13.10-5）。
func (s *Service) ClearAllCatConversations(ctx context.Context) error {
	if s.cfg.Store == nil {
		return apperr.New(apperr.Internal, "本地存储尚未初始化")
	}
	list, err := s.cfg.Store.ListCatConversations(ctx)
	if err != nil {
		return apperr.Wrap(apperr.IOError, "清空失败，请重试。", err)
	}
	for _, c := range list {
		s.cancelTurnAndWait(c.ID)
	}
	var firstErr error
	for _, c := range list {
		if err := s.cfg.Store.DeleteCatConversation(ctx, c.ID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		// 删除与取消之间若又起了一轮，再停一次。
		s.cancelTurn(c.ID, "")
		if c.ProjectID == "" {
			s.removeConvDir(c.ID)
		}
	}
	if firstErr != nil {
		return apperr.Wrap(apperr.IOError, "清空失败，请重试。", firstErr)
	}
	return nil
}

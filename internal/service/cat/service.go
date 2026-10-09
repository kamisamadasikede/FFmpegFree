// Package cat 实现 Cat 助手一期（契约 6.19）：仅 cat_build，会话锁定 agentKind。
package cat

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"unicode/utf8"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/id"
	"FFmpegFree/internal/store"
)

// Config 依赖。
type Config struct {
	Store    *store.Store
	Registry *catagent.Registry
	Emit     func(event string, payload any)
	Logf     func(format string, args ...any)
	// DefaultAgentKind 读取设置里的新建会话预填（默认 cat_build）。
	DefaultAgentKind func(ctx context.Context) string
}

// Service 是 CatService。
type Service struct {
	cfg Config

	mu    sync.Mutex
	turns map[string]context.CancelFunc // conversationId → cancel
}

// New 创建服务。
func New(cfg Config) *Service {
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	if cfg.DefaultAgentKind == nil {
		cfg.DefaultAgentKind = func(context.Context) string { return catagent.KindCatBuild }
	}
	if cfg.Registry == nil {
		cfg.Registry = catagent.NewRegistry()
	}
	return &Service{cfg: cfg, turns: map[string]context.CancelFunc{}}
}

func (s *Service) emit(event string, payload any) {
	if s.cfg.Emit != nil {
		s.cfg.Emit(event, payload)
	}
}

func (s *Service) adapter(kind string) (catagent.Adapter, error) {
	a, ok := s.cfg.Registry.Get(kind)
	if !ok || a == nil {
		return nil, apperr.New(apperr.CatNotReady, catagent.MsgNotReady)
	}
	return a, nil
}

// ---------- 状态 ----------

func (s *Service) GetCatStatus(ctx context.Context) (Status, error) {
	_ = ctx
	a, err := s.adapter(catagent.KindCatBuild)
	if err != nil {
		return Status{State: catagent.StateMissing, CanDownload: false, Error: apperr.From(err)}, nil
	}
	return a.Status(), nil
}

func (s *Service) RecheckCat(ctx context.Context) (Status, error) {
	_ = ctx
	a, err := s.adapter(catagent.KindCatBuild)
	if err != nil {
		st := Status{State: catagent.StateMissing, CanDownload: false, Error: apperr.From(err)}
		s.emit(catagent.EventStatus, st)
		return st, nil
	}
	return a.Recheck(), nil
}

func (s *Service) ListCatModels(ctx context.Context, agentKind string) ([]Model, error) {
	_ = ctx
	if !catagent.Phase1Creatable(agentKind) {
		return nil, apperr.New(apperr.InvalidArgument, "不支持这种助手模式")
	}
	a, err := s.adapter(agentKind)
	if err != nil {
		return nil, err
	}
	return a.ListModels()
}

func (s *Service) ListCatThinkLevels(ctx context.Context, agentKind string) ([]ThinkLevel, error) {
	_ = ctx
	if !catagent.Phase1Creatable(agentKind) {
		return nil, apperr.New(apperr.InvalidArgument, "不支持这种助手模式")
	}
	a, err := s.adapter(agentKind)
	if err != nil {
		return nil, err
	}
	return a.ListThinkLevels()
}

// ---------- 会话 ----------

func (s *Service) ListCatConversations(ctx context.Context) ([]Conversation, error) {
	if s.cfg.Store == nil {
		return nil, apperr.New(apperr.Internal, "本地存储尚未初始化")
	}
	return s.cfg.Store.ListCatConversations(ctx)
}

func (s *Service) GetCatConversation(ctx context.Context, convID string) (ConversationDetail, error) {
	if s.cfg.Store == nil {
		return ConversationDetail{}, apperr.New(apperr.Internal, "本地存储尚未初始化")
	}
	if strings.TrimSpace(convID) == "" {
		return ConversationDetail{}, apperr.New(apperr.InvalidArgument, "会话编号不能为空")
	}
	c, err := s.cfg.Store.GetCatConversation(ctx, convID)
	if errors.Is(err, sql.ErrNoRows) {
		return ConversationDetail{}, apperr.New(apperr.NotFound, "找不到这个会话")
	}
	if err != nil {
		return ConversationDetail{}, apperr.Wrap(apperr.IOError, "读取会话失败", err)
	}
	msgs, err := s.cfg.Store.ListCatMessages(ctx, convID)
	if err != nil {
		return ConversationDetail{}, apperr.Wrap(apperr.IOError, "读取消息失败", err)
	}
	return ConversationDetail{CatConversation: c, Messages: msgs}, nil
}

func (s *Service) CreateCatConversation(ctx context.Context, req CreateConversationRequest) (Conversation, error) {
	if s.cfg.Store == nil {
		return Conversation{}, apperr.New(apperr.Internal, "本地存储尚未初始化")
	}
	kind := strings.TrimSpace(req.AgentKind)
	if kind == "" {
		kind = s.cfg.DefaultAgentKind(ctx)
	}
	if !catagent.Phase1Creatable(kind) {
		return Conversation{}, apperr.New(apperr.InvalidArgument, "不支持这种助手模式")
	}
	if _, err := s.adapter(kind); err != nil {
		// 允许未就绪时仍创建空会话？契约：可创建；发送时再报未就绪。
		// 注册表一期必有 cat_build；这里主要挡脏 kind。
	}
	access := strings.TrimSpace(req.AccessMode)
	if access == "" {
		access = catagent.AccessAsk
	}
	if access != catagent.AccessAsk {
		return Conversation{}, apperr.New(apperr.InvalidArgument, "当前只能使用请求批准")
	}
	title := strings.TrimSpace(req.Title)
	c, err := s.cfg.Store.InsertCatConversation(ctx, store.CatConversation{
		Title:       title,
		AgentKind:   kind,
		AccessMode:  access,
		ProjectPath: strings.TrimSpace(req.ProjectPath),
	})
	if err != nil {
		return Conversation{}, apperr.Wrap(apperr.IOError, "创建会话失败", err)
	}
	return c, nil
}

func (s *Service) DeleteCatConversation(ctx context.Context, convID string) error {
	if s.cfg.Store == nil {
		return apperr.New(apperr.Internal, "本地存储尚未初始化")
	}
	if strings.TrimSpace(convID) == "" {
		return apperr.New(apperr.InvalidArgument, "会话编号不能为空")
	}
	s.cancelTurn(convID)
	err := s.cfg.Store.DeleteCatConversation(ctx, convID)
	if errors.Is(err, sql.ErrNoRows) {
		return apperr.New(apperr.NotFound, "找不到这个会话")
	}
	if err != nil {
		return apperr.Wrap(apperr.IOError, "删除会话失败", err)
	}
	return nil
}

// ---------- 发送 / 取消 ----------

func (s *Service) SendCatMessage(ctx context.Context, req SendMessageRequest) (SendMessageResult, error) {
	if s.cfg.Store == nil {
		return SendMessageResult{}, apperr.New(apperr.Internal, "本地存储尚未初始化")
	}
	content := strings.TrimSpace(req.Content)
	if req.ConversationID == "" || content == "" {
		return SendMessageResult{}, apperr.New(apperr.InvalidArgument, "会话或内容不能为空")
	}
	if utf8.RuneCountInString(content) > 100_000 {
		return SendMessageResult{}, apperr.New(apperr.InvalidArgument, "内容太长")
	}
	c, err := s.cfg.Store.GetCatConversation(ctx, req.ConversationID)
	if errors.Is(err, sql.ErrNoRows) {
		return SendMessageResult{}, apperr.New(apperr.NotFound, "找不到这个会话")
	}
	if err != nil {
		return SendMessageResult{}, apperr.Wrap(apperr.IOError, "读取会话失败", err)
	}
	// 路由：只用会话已存 agentKind。
	a, err := s.adapter(c.AgentKind)
	if err != nil {
		return SendMessageResult{}, err
	}
	if a.Status().State != catagent.StateReady {
		return SendMessageResult{}, apperr.New(apperr.CatNotReady, catagent.MsgNotReady)
	}

	userMsg, err := s.cfg.Store.InsertCatMessage(ctx, c.ID, store.CatMessage{Role: "user", Content: content})
	if err != nil {
		return SendMessageResult{}, apperr.Wrap(apperr.IOError, "保存消息失败", err)
	}
	s.emit(catagent.EventMessage, catagent.MessageEvent{
		ConversationID: c.ID, MessageID: userMsg.ID, Role: userMsg.Role,
		Content: userMsg.Content, CreatedAt: userMsg.CreatedAt,
	})
	if c.Title == "" {
		_ = s.cfg.Store.TouchCatConversation(ctx, c.ID, titleFrom(content), req.ProjectPath)
	} else if req.ProjectPath != "" {
		_ = s.cfg.Store.TouchCatConversation(ctx, c.ID, "", req.ProjectPath)
	}

	hist, err := s.cfg.Store.ListCatMessages(ctx, c.ID)
	if err != nil {
		return SendMessageResult{}, apperr.Wrap(apperr.IOError, "读取消息失败", err)
	}
	wires := make([]catagent.WireMessage, 0, len(hist))
	for _, m := range hist {
		wires = append(wires, catagent.WireMessage{Role: m.Role, Content: m.Content})
	}
	projectPath := req.ProjectPath
	if projectPath == "" {
		projectPath = c.ProjectPath
	}

	turnCtx, cancel := context.WithCancel(ctx)
	s.setTurn(c.ID, cancel)
	defer s.clearTurn(c.ID, cancel)

	turnID := id.New()
	resp, runErr := a.RunTurn(catagent.TurnOptions{
		Ctx: turnCtx, ConversationID: c.ID, TurnID: turnID,
		ModelID: req.ModelID, ThinkLevelID: req.ThinkLevelID,
		ProjectPath: projectPath, Messages: wires,
	})
	if runErr != nil {
		ae := apperr.From(runErr)
		status := "failed"
		if apperr.Is(runErr, apperr.Canceled) {
			status = "canceled"
		}
		s.emit(catagent.EventTurn, catagent.TurnEvent{ConversationID: c.ID, Status: status, Error: ae})
		return SendMessageResult{UserMessage: userMsg, Error: ae}, runErr
	}

	// 工具：写/跑拒绝；只读尽量执行。一期若有 toolRequests，把拒绝说明并入助手回复。
	if len(resp.ToolRequests) > 0 {
		results := catagent.HandleToolRequests(projectPath, resp.ToolRequests)
		var refused []string
		for _, r := range results {
			if !r.OK && r.Content == catagent.MsgToolWriteRef {
				refused = append(refused, r.Content)
				break
			}
		}
		if len(refused) > 0 {
			if strings.TrimSpace(resp.Message.Content) == "" {
				resp.Message.Content = refused[0]
			} else if !strings.Contains(resp.Message.Content, refused[0]) {
				resp.Message.Content = resp.Message.Content + "\n\n" + refused[0]
			}
		}
	}

	asstContent := strings.TrimSpace(resp.Message.Content)
	if asstContent == "" {
		ae := apperr.New(apperr.CatReplyFailed, catagent.MsgReplyFailed)
		s.emit(catagent.EventTurn, catagent.TurnEvent{ConversationID: c.ID, Status: "failed", Error: ae})
		return SendMessageResult{UserMessage: userMsg, Error: ae}, ae
	}
	asst, err := s.cfg.Store.InsertCatMessage(ctx, c.ID, store.CatMessage{Role: "assistant", Content: asstContent})
	if err != nil {
		return SendMessageResult{UserMessage: userMsg}, apperr.Wrap(apperr.IOError, "保存消息失败", err)
	}
	s.emit(catagent.EventMessage, catagent.MessageEvent{
		ConversationID: c.ID, MessageID: asst.ID, Role: asst.Role,
		Content: asst.Content, CreatedAt: asst.CreatedAt,
	})
	s.emit(catagent.EventTurn, catagent.TurnEvent{ConversationID: c.ID, Status: "succeeded"})
	return SendMessageResult{UserMessage: userMsg, AssistantMessage: &asst}, nil
}

func (s *Service) CancelCatTurn(ctx context.Context, conversationID string) error {
	_ = ctx
	if strings.TrimSpace(conversationID) == "" {
		return apperr.New(apperr.InvalidArgument, "会话编号不能为空")
	}
	if !s.cancelTurn(conversationID) {
		return nil // 幂等：没有进行中的一轮
	}
	return nil
}

func (s *Service) setTurn(id string, cancel context.CancelFunc) {
	s.mu.Lock()
	if old, ok := s.turns[id]; ok && old != nil {
		old()
	}
	s.turns[id] = cancel
	s.mu.Unlock()
}

func (s *Service) clearTurn(id string, mine context.CancelFunc) {
	s.mu.Lock()
	if cur, ok := s.turns[id]; ok {
		// 函数值不可比；若仍登记则删掉（同会话同时只会有一轮）
		_ = cur
		delete(s.turns, id)
	}
	s.mu.Unlock()
	if mine != nil {
		mine()
	}
}

func (s *Service) cancelTurn(id string) bool {
	s.mu.Lock()
	cancel, ok := s.turns[id]
	if ok {
		delete(s.turns, id)
	}
	s.mu.Unlock()
	if ok && cancel != nil {
		cancel()
		return true
	}
	return false
}

func titleFrom(text string) string {
	t := strings.Join(strings.Fields(text), " ")
	r := []rune(t)
	if len(r) > 18 {
		return string(r[:18]) + "…"
	}
	if t == "" {
		return "新对话"
	}
	return t
}

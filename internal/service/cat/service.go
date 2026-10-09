// Package cat 实现 Cat 助手一期（契约 6.19）：仅 cat_build，会话锁定 agentKind。
package cat

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"sync"
	"time"
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
	// Stat 判断项目文件夹是否还在（6.19.10.4）；nil = os.Stat。测试可注入慢 / 失败的 stat。
	Stat func(path string) (os.FileInfo, error)
	// StatTimeout 是每个项目路径 stat 的最长等待；0 = DefaultProjectStatTimeout（2 秒）。
	StatTimeout time.Duration
	// OpenFolder 在系统文件管理器里打开文件夹本身（v0.31.1 RevealCatProject）；app 里接 system.Manager.OpenFolder，测试注入。
	OpenFolder func(dir string) error
	// PathKey 是项目去重比较键；nil = paths.Key（Windows / macOS 小写，Linux 区分大小写）。测试注入其他平台口径。
	PathKey func(p string) string
}

// Service 是 CatService。
type Service struct {
	cfg Config

	mu    sync.Mutex
	turns map[string]*turnState // convId → 进行中的一轮（同会话同时只有一轮）
	wg    sync.WaitGroup        // 后台回合，测试 / 关闭时可等待

	statTimeout time.Duration
	projMu      sync.Mutex
	projMissing map[string]bool // 项目 id → 上一次计算的 missing（6.19.10.3）
	rekeyMu     sync.Mutex
	rekeyed     bool // ensureProjectKeys 已成功跑过
}

// turnState 是一轮进行中的回复。终态事件只发一次；终态后丢弃迟到增量。
type turnState struct {
	convID string
	turnID string
	cancel context.CancelFunc
	done   chan struct{} // 后台 runTurn 返回后关闭（含已出文字落库）

	mu       sync.Mutex
	msgID    string          // 本轮助手正文 messageId（落库同 id）
	seq      int             // 同 messageId 内严格递增，从 1 起
	text     strings.Builder // 已流出文字
	finished bool
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
	st := cfg.StatTimeout
	if st <= 0 {
		st = DefaultProjectStatTimeout
	}
	return &Service{cfg: cfg, turns: map[string]*turnState{}, statTimeout: st, projMissing: map[string]bool{}}
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
	// v0.31：projectId 校验在 agentKind 之后（6.19.10.2 第 5 条）；req.ProjectPath 作废、忽略。
	projectID := strings.TrimSpace(req.ProjectID)
	if projectID != "" {
		row, err := s.cfg.Store.GetCatProject(ctx, projectID)
		if errors.Is(err, sql.ErrNoRows) {
			return Conversation{}, errProjectNotFound()
		}
		if err != nil {
			return Conversation{}, apperr.Wrap(apperr.IOError, "读取项目失败", err)
		}
		if s.checkProject(row).Missing {
			return Conversation{}, errProjectMissing()
		}
	}
	title := strings.TrimSpace(req.Title)
	c, err := s.cfg.Store.InsertCatConversation(ctx, store.CatConversation{
		Title:      title,
		AgentKind:  kind,
		AccessMode: access,
		ProjectID:  projectID,
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
	s.cancelTurnAndWait(convID)
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

// SendCatMessage 同步校验并落库用户消息，返回 { userMessage, turnId }；
// 助手回复在后台跑，经 cat:message（append/replace/done）与 cat:turn 推送（契约 6.19.9）。
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
	// v0.31（6.19.10.2 第 6 条）：会话属于项目时实时检查文件夹；missing 同步返回，不启 turn、不存用户消息、不发事件。
	// 只读工具的根只来自所属项目（6.19.10.5）；不属于项目 → 根为空，项目工具一律拒绝。req.ProjectPath 作废、忽略。
	projectRoot := ""
	if c.ProjectID != "" {
		p, err := s.projectForConversation(ctx, c.ProjectID)
		if err != nil {
			return SendMessageResult{}, err
		}
		projectRoot = p.Path
	}

	userMsg, err := s.cfg.Store.InsertCatMessage(ctx, c.ID, store.CatMessage{Role: "user", Content: content})
	if err != nil {
		return SendMessageResult{}, apperr.Wrap(apperr.IOError, "保存消息失败", err)
	}
	if c.Title == "" {
		_ = s.cfg.Store.TouchCatConversation(ctx, c.ID, titleFrom(content))
	}

	hist, err := s.cfg.Store.ListCatMessages(ctx, c.ID)
	if err != nil {
		return SendMessageResult{}, apperr.Wrap(apperr.IOError, "读取消息失败", err)
	}
	wires := make([]catagent.WireMessage, 0, len(hist))
	for _, m := range hist {
		wires = append(wires, catagent.WireMessage{Role: m.Role, Content: m.Content})
	}

	// 回合生命周期与调用方 ctx 解耦（绑定层传入的是应用根 ctx，取消只走 CancelCatTurn）。
	turnCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	t := &turnState{convID: c.ID, turnID: id.New(), cancel: cancel, msgID: id.New(), done: make(chan struct{})}
	s.setTurn(t)
	s.emit(catagent.EventTurn, catagent.TurnEvent{ConvID: t.convID, TurnID: t.turnID, Status: catagent.TurnRunning})

	opts := catagent.TurnOptions{
		Ctx: turnCtx, ConversationID: c.ID, TurnID: t.turnID,
		ModelID: req.ModelID, ThinkLevelID: req.ThinkLevelID,
		ProjectPath: projectRoot, Messages: wires,
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer close(t.done)
		s.runTurn(turnCtx, t, a, opts)
	}()
	return SendMessageResult{UserMessage: userMsg, TurnID: t.turnID}, nil
}

// streamChunkRunes 是整段回复拆 append 时每段的字数（CLI 真流式协议到位前使用）。
const streamChunkRunes = 24

// runTurn 在后台跑一轮并推送增量；终态只发一次。
func (s *Service) runTurn(ctx context.Context, t *turnState, a catagent.Adapter, opts catagent.TurnOptions) {
	defer s.clearTurn(t)
	streamed := false
	opts.OnTextDelta = func(delta string) {
		streamed = true
		s.appendText(ctx, t, delta)
	}

	resp, runErr := a.RunTurn(opts)
	if runErr != nil {
		if ctx.Err() != nil || apperr.Is(runErr, apperr.Canceled) {
			s.finishCancelled(t)
			return
		}
		s.cfg.Logf("cat turn failed: %v", runErr)
		s.finish(t, catagent.TurnFailed)
		return
	}

	// 工具：写/跑拒绝；只读尽量执行。一期若有 toolRequests，把拒绝说明并入助手回复。
	extra := ""
	if len(resp.ToolRequests) > 0 {
		results := catagent.HandleToolRequests(opts.ProjectPath, resp.ToolRequests)
		for _, r := range results {
			if !r.OK && r.Content == catagent.MsgToolWriteRef {
				extra = r.Content
				break
			}
		}
	}

	if !streamed {
		// CLI 真流式协议到位前：整段回复拆成若干 append。
		body := strings.TrimSpace(resp.Message.Content)
		if extra != "" && !strings.Contains(body, extra) {
			if body == "" {
				body = extra
			} else {
				body = body + "\n\n" + extra
			}
		}
		for _, chunk := range splitRunes(body, streamChunkRunes) {
			if !s.appendText(ctx, t, chunk) {
				break
			}
		}
	} else if extra != "" {
		cur := t.currentText()
		if !strings.Contains(cur, extra) {
			if strings.TrimSpace(cur) == "" {
				s.appendText(ctx, t, extra)
			} else {
				s.appendText(ctx, t, "\n\n"+extra)
			}
		}
	}

	if ctx.Err() != nil {
		s.finishCancelled(t)
		return
	}
	text := strings.TrimSpace(t.currentText())
	if text == "" {
		s.finish(t, catagent.TurnFailed)
		return
	}
	if _, err := s.cfg.Store.InsertCatMessage(context.WithoutCancel(ctx), t.convID, store.CatMessage{
		ID: t.msgID, Role: "assistant", Content: text,
	}); err != nil {
		s.cfg.Logf("cat save assistant message: %v", err)
		s.finish(t, catagent.TurnFailed)
		return
	}
	s.finish(t, catagent.TurnCompleted)
}

func (t *turnState) currentText() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.text.String()
}

// appendText 发一条 append；回合已终态或已取消时丢弃并返回 false。
func (s *Service) appendText(ctx context.Context, t *turnState, delta string) bool {
	if delta == "" {
		return true
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.finished || ctx.Err() != nil {
		return false
	}
	t.seq++
	t.text.WriteString(delta)
	s.emit(catagent.EventMessage, catagent.MessageEvent{
		ConvID: t.convID, TurnID: t.turnID, MessageID: t.msgID,
		Seq: t.seq, Op: catagent.OpAppend, TextDelta: delta,
	})
	return true
}

// finish 发该消息 done（若已出过字）与终态 cat:turn；只生效一次。
func (s *Service) finish(t *turnState, status string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return s.finishLocked(t, status)
}

func (s *Service) finishLocked(t *turnState, status string) bool {
	if t.finished {
		return false
	}
	t.finished = true
	if t.seq > 0 {
		t.seq++
		s.emit(catagent.EventMessage, catagent.MessageEvent{
			ConvID: t.convID, TurnID: t.turnID, MessageID: t.msgID, Seq: t.seq, Op: catagent.OpDone,
		})
	}
	s.emit(catagent.EventTurn, catagent.TurnEvent{ConvID: t.convID, TurnID: t.turnID, Status: status})
	return true
}

// finishCancelled 收尾被取消的一轮：已流出文字落库保留（不加后缀），并确保发过 cancelled。
func (s *Service) finishCancelled(t *turnState) {
	s.finish(t, catagent.TurnCancelled) // 通常 stopTurn 已发过，这里幂等
	text := strings.TrimSpace(t.currentText())
	if text == "" {
		return
	}
	if _, err := s.cfg.Store.InsertCatMessage(context.Background(), t.convID, store.CatMessage{
		ID: t.msgID, Role: "assistant", Content: text,
	}); err != nil {
		s.cfg.Logf("cat save partial assistant message: %v", err)
	}
}

// CancelCatTurn 立即停止 { convId, turnId } 这一轮；不匹配或已结束则幂等返回 nil。
// turnId 为空时停止该会话当前这一轮（兼容）。
func (s *Service) CancelCatTurn(ctx context.Context, req CancelTurnRequest) error {
	_ = ctx
	if strings.TrimSpace(req.ConvID) == "" {
		return apperr.New(apperr.InvalidArgument, "会话编号不能为空")
	}
	s.cancelTurn(req.ConvID, strings.TrimSpace(req.TurnID))
	return nil
}

// Wait 等待所有后台回合结束（测试 / 关闭用）。
func (s *Service) Wait() { s.wg.Wait() }

func (s *Service) setTurn(t *turnState) {
	s.mu.Lock()
	old := s.turns[t.convID]
	s.turns[t.convID] = t
	s.mu.Unlock()
	if old != nil {
		s.stopTurn(old)
	}
}

func (s *Service) clearTurn(t *turnState) {
	s.mu.Lock()
	if cur, ok := s.turns[t.convID]; ok && cur == t {
		delete(s.turns, t.convID)
	}
	s.mu.Unlock()
	t.cancel()
}

// turnWaitLimit 是删除会话 / 项目时等后台一轮收尾的上限。
const turnWaitLimit = 5 * time.Second

// cancelTurnAndWait 取消会话当前一轮，并等后台收尾（已出文字落库）后再返回，避免删除后又写回消息。
func (s *Service) cancelTurnAndWait(convID string) {
	t := s.cancelTurnState(convID, "")
	if t == nil || t.done == nil {
		return
	}
	select {
	case <-t.done:
	case <-time.After(turnWaitLimit):
		s.cfg.Logf("cat turn %s did not finish within %s after cancel", t.turnID, turnWaitLimit)
	}
}

// cancelTurn 取消会话当前一轮；turnID 非空时须匹配。
func (s *Service) cancelTurn(convID, turnID string) bool {
	return s.cancelTurnState(convID, turnID) != nil
}

func (s *Service) cancelTurnState(convID, turnID string) *turnState {
	s.mu.Lock()
	t, ok := s.turns[convID]
	if ok && (turnID == "" || t.turnID == turnID) {
		delete(s.turns, convID)
	} else {
		ok = false
	}
	s.mu.Unlock()
	if !ok {
		return nil
	}
	s.stopTurn(t)
	return t
}

// stopTurn 立即打断适配器并发 done（若已出字）+ cancelled；之后的迟到增量会被丢弃。
// 已出文字的落库由后台 goroutine 的 finishCancelled 完成。
func (s *Service) stopTurn(t *turnState) {
	t.cancel()
	s.finish(t, catagent.TurnCancelled)
}

func splitRunes(s string, n int) []string {
	if s == "" {
		return nil
	}
	r := []rune(s)
	out := make([]string, 0, len(r)/n+1)
	for i := 0; i < len(r); i += n {
		j := i + n
		if j > len(r) {
			j = len(r)
		}
		out = append(out, string(r[i:j]))
	}
	return out
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

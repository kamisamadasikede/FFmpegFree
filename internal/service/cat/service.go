// Package cat 实现 Cat 助手（契约 6.19）：v0.36 可创建 cat_build|claude_cli|codex_cli，会话锁定 agentKind。
package cat

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/genoffice"
	"FFmpegFree/internal/id"
	"FFmpegFree/internal/localassets"
	"FFmpegFree/internal/paths"
	"FFmpegFree/internal/service/doc"
	"FFmpegFree/internal/store"
)

// Config 依赖。
type Config struct {
	Store    *store.Store
	Registry *catagent.Registry
	Emit     func(event string, payload any)
	Logf     func(format string, args ...any)
	// DefaultAgentKind 读取新建会话预填（优先 CatSettings.defaultAgentKind，再系统旧键，默认 cat_build）。
	DefaultAgentKind func(ctx context.Context) string
	// SystemDefaultAgentKind 读系统旧键 catDefaultAgentKind；GetCatSettings 在 cat.settings 缺 defaultAgentKind 时回落。
	SystemDefaultAgentKind func(ctx context.Context) string
	// Stat 判断项目文件夹是否还在（6.19.10.4）；nil = os.Stat。测试可注入慢 / 失败的 stat。
	Stat func(path string) (os.FileInfo, error)
	// StatTimeout 是每个项目路径 stat 的最长等待；0 = DefaultProjectStatTimeout（2 秒）。
	StatTimeout time.Duration
	// OpenFolder 在系统文件管理器里打开文件夹本身（v0.31.1 RevealCatProject）；app 里接 system.Manager.OpenFolder，测试注入。
	OpenFolder func(dir string) error
	// PathKey 是项目去重比较键；nil = paths.Key（Windows / macOS 小写，Linux 区分大小写）。测试注入其他平台口径。
	PathKey func(p string) string
	// DataRoot 是应用数据目录。普通对话的文件夹（paths.CatConvDir / 旧 cwd）从这里算。空则不建、不删对话文件夹。
	DataRoot string
	// CLIHome isolates native sharing in tests; production uses the user's home.
	CLIHome string
	// Preview 把本机图片、视频和文档登记成页面能加载的地址。空则不能预览。
	Preview                *localassets.Registry
	DocumentPreview        func(context.Context, string) (doc.DocPreview, error)
	ReleaseDocumentPreview func(string) error
	FileBridge             bool
	// Now 是可注入的时钟（定时任务测试用）；nil = time.Now。
	Now func() time.Time
	// Location 是计算定时任务的时区；nil = time.Local。
	Location *time.Location
	// Notify 发系统通知（定时任务结束）；nil 不发。
	Notify func(n Notification)
	// Foreground 报告窗口是否可见且未最小化（定时任务用；普通 turn 另要求前端上报的键盘焦点，v0.48）。nil = 不在前台。
	Foreground func() bool
	// NotifyStatus 报告系统通知是否可用 / 已授权（GetCatNotifyStatus，v0.48）；nil = 不可用。
	NotifyStatus func() NotifyStatus
	// ScheduleTick 是调度器检查间隔；0 = 30 秒。
	ScheduleTick time.Duration
	// ScheduleRunTimeout 是一次定时运行的上限；0 = 30 分钟（v0.35.3）。
	ScheduleRunTimeout time.Duration
	// StatusDebounce 合并状态变化后发 cat:status 的等待（v0.47.1）；0 = 50ms，<0 = 同步发（测试）。
	StatusDebounce time.Duration
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
	sched       scheduler
	win         windowFocus // v0.48：前端上报的窗口焦点（SetCatWindowFocused）
	terms       *termHub

	// v0.40 克隆：全局同时最多 1 个
	cloneMu     sync.Mutex
	clone       *cloneJob
	cloneEmitMu sync.Mutex
	cloneEmitAt time.Time

	// v0.44 RetryCatTurn：每个会话最后一轮的原请求与终态（只在内存；重启后不能重试）。
	lastMu    sync.Mutex
	lastTurns map[string]*lastTurn

	// v0.46 服务商模型列表缓存（6.19.20.2 ⑤）
	pmOnce sync.Once
	pm     *providerModelsCache

	// v0.47.1 检测状态 / cat.detect
	det detectState

	// Only duplicate installers for the same CLI are excluded.
	cliUpdateMu sync.Mutex
	cliUpdating map[string]bool
	sharingMu   sync.Mutex
	workspaceMu sync.Mutex
	workflowMu  sync.Mutex
	launchMu    sync.Mutex
	queueArmed  map[string]bool
	deleting    map[string]bool
	pptMu       sync.Mutex
	ppt         *genoffice.Manager

	// v0.51 分支树：fetch 节流状态与 cat.branchPrefs 读写锁
	btree branchTreeState
}

// lastTurn 是某会话最后一轮（6.19.18.7）。
type lastTurn struct {
	turnID  string
	req     SendMessageRequest
	userMsg store.CatMessage
	hook    *turnHook
	status  string
	// noRetry（v0.45）：续聊会话已不存在等失败，原样重发也不会好 → 不可重试。
	noRetry bool
}

// actRec 是一条思考或一次工具调用。界面单独画，不写进助手正文。
type actRec struct {
	kind string
	id   string
	seq  int
	text string
}

// turnState 是一轮进行中的回复。终态事件只发一次；终态后丢弃迟到增量。
type turnState struct {
	convID string
	turnID string
	cancel context.CancelFunc
	done   chan struct{} // 后台 runTurn 返回后关闭（含已出文字落库）

	mu            sync.Mutex
	msgID         string          // 本轮助手正文 messageId（落库同 id）
	seq           int             // 同 messageId 内严格递增，从 1 起
	text          strings.Builder // 已流出文字
	acts          []*actRec       // 思考 / 工具，按出现顺序；不并入正文
	actBy         map[string]*actRec
	finished      bool
	failMsg       string
	failReason    string // v0.53.1：超时类失败的 reason 行（reason=idle_timeout / reason=timeout），随失败事件带给前端
	errSeen       map[string]struct{}
	retryID       string
	retrySeq      int
	retryText     string
	usageCwd      string // 这一轮的工作目录，用来读上下文占用
	agentKind     string // 这一轮的助手，按它取 SessionHistory.ReadContextUsage
	contextUsed   int64
	contextWindow int64
	emittedUsed   int64
	emittedWindow int64
	hook          *turnHook // 定时任务的收尾回调；终态时调一次
	// v0.37.1 消息尾
	startedAt    int64 // 发 running / 启 turn 的墙钟（Unix 毫秒）
	tokenTotal   int64 // CLI 上报合计；0 = 无 usage
	outputTokens int64 // v0.47：CLI 上报输出 token；0 = 无
	finishedAt   int64
	steer        catagent.SteerFunc // protected by mu; cleared when the native turn closes
	tokensPerSec *float64
	firstTokenAt int64 // v0.45：第一个正文 / 思考增量的墙钟；0 = 还没有
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
	return &Service{cfg: cfg, turns: map[string]*turnState{}, statTimeout: st, projMissing: map[string]bool{}, lastTurns: map[string]*lastTurn{}}
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

// agentStatusOf 把适配器 Status 收成 AgentStatus。
func agentStatusOf(kind string, st Status) catagent.AgentStatus {
	as := catagent.AgentStatus{
		AgentKind: kind,
		Ready:     st.State == catagent.StateReady,
		Version:   st.Version,
	}
	if st.Error != nil {
		as.Detail = st.Error.Message
	}
	as.Modes = catagent.ModesFor(kind) // v0.44 6.19.18.3
	return as
}

// composeStatus 组装顶层 Status（以 cat_build 为主）+ agents[]：注册表里所有已注册适配器各一行，按注册顺序
// （v0.47.1 6.19.21.1）；v0.36 三家里未注册的补在后面（missing）。recheck=true 时对已注册适配器调用 Recheck。
func (s *Service) composeStatus(recheck bool) Status {
	kinds := s.cfg.Registry.Kinds()
	seen := map[string]bool{}
	for _, k := range kinds {
		seen[k] = true
	}
	for _, k := range catagent.CreatableKinds() {
		if !seen[k] {
			kinds = append(kinds, k)
		}
	}
	agents := make([]catagent.AgentStatus, 0, len(kinds))
	var top Status
	topSet := false
	for _, kind := range kinds {
		var st Status
		a, ok := s.cfg.Registry.Get(kind)
		if ok && a != nil {
			if recheck {
				st = a.Recheck()
			} else {
				st = a.Status()
			}
		} else {
			a = nil
			st = Status{State: catagent.StateMissing, CanDownload: false, Error: apperr.New(apperr.CatNotReady, catagent.MsgNotReady)}
		}
		if kind == catagent.KindCatBuild {
			top = st
			top.Agents = nil
			topSet = true
		}
		agents = append(agents, s.agentRow(kind, a, st))
	}
	if !topSet {
		top = Status{State: catagent.StateMissing, CanDownload: false, Error: apperr.New(apperr.CatNotReady, catagent.MsgNotReady)}
	}
	top.Agents = agents
	return top
}

func (s *Service) GetCatStatus(ctx context.Context) (Status, error) {
	s.applyAllProviderOverlays(ctx)
	return s.composeStatus(false), nil
}

func (s *Service) ListCatModels(ctx context.Context, agentKind string) ([]Model, error) {
	// v0.36：未知 / 未注册 → 空切片、nil error（不报 InvalidArgument）。
	// v0.38：启用自定义时列表探测注入同一套 URL/Key（C6）。
	// v0.46（6.19.20.2 ⑦）：自定义开 → 服务商列表；失败 = 空切片、nil error。
	if catagent.Phase1Creatable(strings.TrimSpace(agentKind)) && s.customEnabled(ctx, agentKind) {
		r, err := s.ListCatProviderModels(ctx, ListCatProviderModelsRequest{AgentKind: agentKind})
		if err != nil || r.Models == nil {
			return []Model{}, nil
		}
		return r.Models, nil
	}
	a, ok := s.cfg.Registry.Get(agentKind)
	if !ok || a == nil {
		return []Model{}, nil
	}
	s.applyProviderOverlay(ctx, a, agentKind)
	list, err := a.ListModels()
	if err != nil {
		return nil, err
	}
	if list == nil {
		return []Model{}, nil
	}
	return list, nil
}

// ListCatThinkLevels 保留兼容（6.19.18.2）：返回该家「当前所选模型」（设置里 defaultsByAgent[kind].model）的档位；
// 模型为空 / 不在列表 → 空（前端隐藏强度行）。新前端以 CatModel.efforts 为准。
func (s *Service) ListCatThinkLevels(ctx context.Context, agentKind string) ([]ThinkLevel, error) {
	out := []ThinkLevel{}
	a, ok := s.cfg.Registry.Get(agentKind)
	if !ok || a == nil {
		return out, nil
	}
	model := ""
	if s.cfg.Store != nil {
		if v, err := s.GetCatSettings(ctx); err == nil {
			model = v.DefaultsByAgent[agentKind].Model
		}
	}
	if model == "" {
		return out, nil
	}
	list, err := s.ListCatModels(ctx, agentKind)
	if err != nil {
		return out, nil
	}
	if m, ok := catagent.FindModel(list, model); ok {
		for _, e := range m.Efforts {
			out = append(out, ThinkLevel{ID: e, DisplayName: catagent.EffortDisplayName(e)})
		}
	}
	return out, nil
}

// effortForTurn 按当前模型的 efforts 过滤这一轮的档位（6.19.18.6 末句）：不在列表 → 不传（不报错）。
func (s *Service) effortForTurn(ctx context.Context, kind, model, effort string) string {
	effort = strings.TrimSpace(effort)
	model = strings.TrimSpace(model)
	if effort == "" || model == "" || model == "default" {
		return ""
	}
	list, err := s.ListCatModels(ctx, kind)
	if err != nil {
		return ""
	}
	m, ok := catagent.FindModel(list, model)
	if !ok || !catagent.ModelSupportsEffort(m, effort) {
		return ""
	}
	return effort
}

// ---------- 会话 ----------

func (s *Service) ListCatConversations(ctx context.Context) ([]Conversation, error) {
	if s.cfg.Store == nil {
		return nil, apperr.New(apperr.Internal, "本地存储尚未初始化")
	}
	list, err := s.cfg.Store.ListCatConversations(ctx)
	if err != nil {
		return nil, err
	}
	hidden, _ := s.cfg.Store.ListCatGroupMemberIDs(ctx)
	out := make([]Conversation, 0, len(list))
	for i := range list {
		if hidden[list[i].ID] {
			continue
		}
		s.applyCLITitle(ctx, &list[i])
		out = append(out, list[i])
	}
	return out, nil
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
	msgs, hist, err := s.loadTranscript(ctx, c)
	if err != nil {
		return ConversationDetail{}, err
	}
	s.applyCLITitle(ctx, &c)
	used, window := s.lookupContext(ctx, c)
	return ConversationDetail{CatConversation: c, Messages: msgs, HistoryStatus: hist, LastTurn: s.lastTurnInfo(convID), ContextUsed: used, ContextWindow: window}, nil
}

// applyCLITitle 用 CLI 会话里的第一句当展示名。库里的 title 不更新。
// 统一走 SessionHistory（Grok / Claude / Codex 同一接口），不按家特判。
func (s *Service) applyCLITitle(ctx context.Context, c *store.CatConversation) {
	if c.CustomTitle != "" {
		c.Title = c.CustomTitle
		return
	}
	if h := s.sessionHistory(c.AgentKind); h != nil {
		if label := h.SessionLabel(c.ID, s.cwdFor(ctx, *c)); label != "" {
			c.Title = label
		}
	}
}

// sessionHistory 按 agentKind 取 SessionHistory；假适配器没有就返回 nil。
func (s *Service) sessionHistory(agentKind string) catagent.SessionHistory {
	a, err := s.adapter(agentKind)
	if err != nil {
		return nil
	}
	h, _ := a.(catagent.SessionHistory)
	return h
}

func (s *Service) sessionResumable(agentKind, convID, cwd string) bool {
	// 只听适配器（架构笔记阶段 A）：服务层不再用 Grok 包级 SessionStarted 兜底。
	if h := s.sessionHistory(agentKind); h != nil {
		return h.SessionResumable(convID, cwd)
	}
	return false
}

// cwdFor 是这条对话在 CLI 里的工作目录：项目用项目文件夹，否则用会话目录。
func (s *Service) cwdFor(ctx context.Context, c store.CatConversation) string {
	if c.ProjectID != "" {
		if row, err := s.cfg.Store.GetCatProject(ctx, c.ProjectID); err == nil {
			if p := strings.TrimSpace(row.Path); p != "" {
				return p
			}
		}
	}
	return catagent.TurnCwd(s.cfg.DataRoot, "", c.ID)
}

// loadTranscript 打开会话时从 CLI 自己的会话文件读正文（SessionHistory，照 Grok 路径）：
// 按 agentKind 取适配器 → LoadTranscript(convID, cwd)；Claude / Codex / Grok 同一接口。
// 读不到就空列表，从不拼接本地消息缓存、不捏造历史。库只留元数据。
func (s *Service) loadTranscript(ctx context.Context, c store.CatConversation) ([]store.CatMessage, string, error) {
	cwd := s.cwdFor(ctx, c)
	h := s.sessionHistory(c.AgentKind)
	var (
		wires []catagent.WireMessage
		err   error
	)
	if h != nil {
		wires, err = h.LoadTranscript(ctx, c.ID, cwd)
	} else {
		// 测试假适配器可能只挂了 LoadTranscript；生产三家都实现完整 SessionHistory。
		a, aerr := s.adapter(c.AgentKind)
		if aerr != nil {
			return []store.CatMessage{}, "", nil
		}
		loader, ok := a.(interface {
			LoadTranscript(context.Context, string, string) ([]catagent.WireMessage, error)
		})
		if !ok {
			return []store.CatMessage{}, "", nil
		}
		wires, err = loader.LoadTranscript(ctx, c.ID, cwd)
	}
	// 读不到正文：空列表 + historyStatus，不报错、不捏造（6.19.19.0 第 5 条）。
	switch {
	case errors.Is(err, catagent.ErrHistoryMissing):
		return []store.CatMessage{}, HistoryMissing, nil
	case errors.Is(err, catagent.ErrHistoryReadFailed):
		s.cfg.Logf("cat history read failed: conv=%s kind=%s", c.ID, c.AgentKind)
		return []store.CatMessage{}, HistoryReadFailed, nil
	case err != nil:
		return nil, "", err
	}
	return applyFooters(s.cfg.DataRoot, c.ID, wiresToMessages(c.ID, wires)), "", nil
}

func wiresToMessages(convID string, wires []catagent.WireMessage) []store.CatMessage {
	out := make([]store.CatMessage, 0, len(wires))
	for i, w := range wires {
		if w.Role == "user" {
			w.Content = catagent.StripInternalInstructions(w.Content)
			if w.Content == "" && len(w.Media) == 0 {
				continue
			}
		}
		role := w.Role
		switch role {
		case "user", "assistant", "system", "thought", "tool", "status":
		default:
			role = "assistant"
		}
		out = append(out, store.CatMessage{
			ID: convID + ":" + strconv.Itoa(i), Role: role, Content: w.Content, CreatedAt: int64(i + 1),
			Media: mediaOf(w.Media),
		})
	}
	return out
}

func mediaOf(list []catagent.WireMedia) []store.CatMedia {
	if len(list) == 0 {
		return nil
	}
	out := make([]store.CatMedia, 0, len(list))
	for _, m := range list {
		out = append(out, store.CatMedia{Kind: m.Kind, Name: m.Name, Path: m.Path, Mime: m.Mime, Data: m.Data})
	}
	return out
}

// lookupContext 读取这个对话最近一次的上下文占用。读不到就返回 0，不让会话详情失败。
func (s *Service) lookupContext(ctx context.Context, c store.CatConversation) (int64, int64) {
	cwd := ""
	if c.ProjectID != "" {
		p, err := s.projectForConversation(ctx, c.ProjectID)
		if p.Path != "" {
			cwd = p.Path
		} else if err != nil {
			return 0, 0
		}
	}
	if cwd == "" {
		cwd = paths.ResolveCatConvDir(s.cfg.DataRoot, c.ID)
	}
	h := s.sessionHistory(c.AgentKind)
	if h == nil {
		return 0, 0
	}
	used, window, ok := h.ReadContextUsage(c.ID, cwd)
	if !ok {
		return 0, 0
	}
	return used, window
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
		// 允许未就绪 / 未注册桩时仍创建空会话；发送时再报未就绪（v0.36）。
		_ = err
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
	// 普通对话创建时就建自己的文件夹（6.19.11.4）。失败不让创建失败，下次用到时再建。
	if projectID == "" {
		s.ensureConvDir(c.ID)
	}
	return c, nil
}

// DeleteCatConversation 删除会话及其消息（先取消进行中的一轮，最多等 5 秒）；空 id INVALID_ARGUMENT，不存在的 id 返回 nil（契约 v0.31.2）。
func (s *Service) DeleteCatConversation(ctx context.Context, convID string) error {
	if s.cfg.Store == nil {
		return apperr.New(apperr.Internal, "本地存储尚未初始化")
	}
	convID = strings.TrimSpace(convID)
	if convID == "" {
		return apperr.New(apperr.InvalidArgument, "会话编号不能为空")
	}
	c, getErr := s.cfg.Store.GetCatConversation(ctx, convID)
	if errors.Is(getErr, sql.ErrNoRows) {
		return nil
	}
	if getErr != nil {
		return apperr.Wrap(apperr.IOError, "读取会话失败", getErr)
	}
	s.markWorkflowDeleting(convID)
	defer s.unmarkWorkflowDeleting(convID)
	s.cancelTurnAndWait(convID)
	err := s.cfg.Store.DeleteCatConversation(ctx, convID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // v0.31.2：不存在的 id 幂等返回 nil，可重复删
	}
	if err != nil {
		return apperr.Wrap(apperr.IOError, "删除会话失败", err)
	}
	s.clearWorkflowStorage(ctx, convID)
	// 只删普通对话自己的文件夹。项目文件夹永远不动（6.19.11.4）。
	if c.ProjectID == "" {
		s.removeConvDir(convID)
	}
	return nil
}

// ---------- 发送 / 取消 ----------

// SendCatMessage 同步校验并落库用户消息，返回 { userMessage, turnId }；
// 助手回复在后台跑，经 cat:message（append/replace/done）与 cat:turn 推送（契约 6.19.9）。
func (s *Service) SendCatMessage(ctx context.Context, req SendMessageRequest) (SendMessageResult, error) {
	return s.sendMessage(ctx, req, nil)
}

// turnHook 是定时任务挂在一轮上的收尾回调和超时（手动对话为 nil）。
type turnHook struct {
	timeout  time.Duration
	onFinish func(status string, err *apperr.AppError)
}

func (s *Service) sendMessage(ctx context.Context, req SendMessageRequest, hook *turnHook) (SendMessageResult, error) {
	return s.startTurn(ctx, req, hook, nil)
}

// startTurn 开一轮。retryOf 非空时是原地重试（6.19.18.7）：沿用原用户消息，不新建。
func (s *Service) startTurn(ctx context.Context, req SendMessageRequest, hook *turnHook, retryOf *store.CatMessage) (SendMessageResult, error) {
	s.launchMu.Lock()
	defer s.launchMu.Unlock()
	if s.cfg.Store == nil {
		return SendMessageResult{}, apperr.New(apperr.Internal, "本地存储尚未初始化")
	}
	content := strings.TrimSpace(req.Content)
	files := make([]catagent.PromptFile, 0, len(req.Attachments))
	for _, a := range req.Attachments {
		files = append(files, catagent.PromptFile{Name: a.Name, Path: a.Path})
	}
	files, ferr := catagent.NormalizePromptFiles(files)
	if ferr != nil {
		return SendMessageResult{}, ferr
	}
	if req.ConversationID == "" || (content == "" && len(files) == 0) {
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
	// v0.46（6.19.20.2 ⑧）：自定义开时模型 id 可手填，按格式校验（防止被当成参数）。
	if m := strings.TrimSpace(req.ModelID); m != "" && m != "default" && s.customEnabled(ctx, c.AgentKind) && !ValidCustomModelID(m) {
		return SendMessageResult{}, apperr.New(apperr.InvalidArgument, "模型 ID 不正确").WithDetail("reason=model")
	}
	projectRoot := ""
	if c.ProjectID != "" {
		p, err := s.projectForConversation(ctx, c.ProjectID)
		if err != nil {
			return SendMessageResult{}, err
		}
		projectRoot = p.Path
	}

	// 刷新后旧进程可能还占着这场会话。再开一个会抢同一份记录。
	// Existing turns are never replaced; additional messages use the queue.
	s.mu.Lock()
	_, busy := s.turns[c.ID]
	busy = busy || s.deleting[c.ID]
	s.mu.Unlock()
	if busy || len(catagent.LiveConvIDs([]string{c.ID})) > 0 {
		return SendMessageResult{}, apperr.New(apperr.TaskConflict, catagent.MsgTurnStillRunning).WithDetail("reason=turn_running")
	}

	// 只刷新这条标签的排序时间。句子本身不写入数据库，历史在 CLI。
	if err := s.cfg.Store.TouchCatConversation(ctx, c.ID, ""); err != nil {
		s.cfg.Logf("cat touch conversation: %v", err)
	}
	userMsg := store.CatMessage{ID: id.New(), Role: "user", Content: content, CreatedAt: time.Now().UnixMilli()}
	if retryOf != nil {
		userMsg = *retryOf
	}
	cwd := catagent.TurnCwd(s.cfg.DataRoot, projectRoot, c.ID)

	// 回合生命周期与调用方 ctx 解耦（绑定层传入的是应用根 ctx，取消只走 CancelCatTurn）。
	turnCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	nowMs := time.Now().UnixMilli()
	t := &turnState{
		convID: c.ID, turnID: id.New(), cancel: cancel, msgID: id.New(), done: make(chan struct{}),
		usageCwd:  catagent.TurnCwd(s.cfg.DataRoot, projectRoot, c.ID),
		agentKind: c.AgentKind,
		hook:      hook,
		startedAt: nowMs,
	}
	s.setTurn(t)
	s.lastMu.Lock()
	s.lastTurns[c.ID] = &lastTurn{turnID: t.turnID, req: req, userMsg: userMsg, hook: hook, status: catagent.TurnRunning}
	s.lastMu.Unlock()
	if req.queueID != "" {
		s.emit("cat:queued-start", struct {
			ConvID      string            `json:"convId"`
			Result      SendMessageResult `json:"result"`
			Attachments []SendAttachment  `json:"attachments"`
		}{c.ID, SendMessageResult{UserMessage: userMsg, TurnID: t.turnID}, req.Attachments})
	}
	s.emit(catagent.EventTurn, catagent.TurnEvent{ConvID: t.convID, TurnID: t.turnID, Status: catagent.TurnRunning})

	provider := s.providerOverlayFor(ctx, c.AgentKind)
	s.applyProviderOverlay(ctx, a, c.AgentKind)
	opts := catagent.TurnOptions{
		Ctx: turnCtx, ConversationID: c.ID, TurnID: t.turnID,
		ModelID: req.ModelID, ThinkLevelID: s.effortForTurn(ctx, c.AgentKind, req.ModelID, req.ThinkLevelID),
		// v0.44：旧值读时映射；空 / 不认识 → 该家默认（最保守）模式（6.19.18.3）。
		PermissionMode: catagent.NormalizeMode(c.AgentKind, req.PermissionMode),
		ProjectPath:    projectRoot,
		Resume:         s.sessionResumable(c.AgentKind, c.ID, cwd),
		Messages:       []catagent.WireMessage{{Role: "user", Content: content}},
		Files:          files,
		Rules:          s.workspaceRulesForTurn(ctx, c.ProjectID),
		Provider:       provider,
		Skills:         skillsOverlayForProject(c.AgentKind, projectRoot),
		LiveSteer:      c.AgentKind == catagent.KindCodexCLI,
	}
	opts.OnSteerReady = func(steer catagent.SteerFunc) {
		t.mu.Lock()
		t.steer = steer
		t.mu.Unlock()
		s.emit("cat:steer-status", struct {
			ConvID string `json:"convId"`
			TurnID string `json:"turnId"`
			Ready  bool   `json:"ready"`
		}{t.convID, t.turnID, steer != nil})
	}
	if hook != nil && hook.timeout > 0 {
		opts.Timeout = hook.timeout
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer close(t.done)
		defer s.afterWorkflowTurn(t)
		s.syncSharingBeforeTurn(turnCtx, projectRoot)
		stopWorkspace := s.watchTurnWorkspace(turnCtx, t, cwd, &opts)
		s.runTurn(turnCtx, t, a, opts)
		stopWorkspace()
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
	// 群聊成员回合（6.21.14）自带 OnActivity：记工具数和写过的文件（越界 / 撤销靠它），
	// 这里只接住 retry，其余交回调用方的回调，不能覆盖掉。
	prevActivity := opts.OnActivity
	opts.OnActivity = func(kind, id, text string, replace bool) {
		if kind == "retry" {
			s.showRetry(ctx, t, text)
			return
		}
		if prevActivity != nil {
			prevActivity(kind, id, text, replace)
			return
		}
		s.appendActivity(ctx, t, kind, id, text, replace)
	}
	opts.OnError = func(msg string) {
		s.noteTurnError(ctx, t, msg)
	}

	resp, runErr := a.RunTurn(opts)
	s.noteTurnUsage(t, resp)
	if runErr != nil {
		if ctx.Err() != nil || apperr.Is(runErr, apperr.Canceled) {
			s.finishCancelled(t)
			return
		}
		s.cfg.Logf("cat turn failed: %v", runErr)
		if ae := apperr.From(runErr); ae != nil {
			if strings.Contains(ae.Detail, catagent.ReasonSessionNotFound) || strings.Contains(ae.Detail, catagent.ReasonProviderNoResponses) {
				s.markNoRetry(t.convID, t.turnID)
			}
			s.noteTurnError(ctx, t, ae.Message)
			// v0.53.1：超时类失败以适配器文案为准，不被之前 CLI 吐过的报错（重连提示等）盖住，
			// 否则界面只剩通用的「回复没生成出来，请重试。」。
			if r := strings.SplitN(ae.Detail, "\n", 2)[0]; r == catagent.ReasonIdleTimeout || r == "reason=timeout" {
				t.mu.Lock()
				t.failMsg, t.failReason = ae.Message, r
				t.mu.Unlock()
			}
		}
		s.finish(t, catagent.TurnFailed)
		return
	}

	// grok 在自己的进程里执行工具，不把 ToolRequests 交回来。
	// 下面只覆盖仍把工具请求交回应用的适配器。
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
	if text == "" && !t.hasActivity() {
		s.finish(t, catagent.TurnFailed)
		return
	}
	// 回复留在 CLI 的会话里。点开对话时再 grok export，不写入 cat_messages。
	s.finish(t, catagent.TurnCompleted)
}

func (t *turnState) currentText() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.text.String()
}

func (t *turnState) hasActivity() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.acts) > 0
}

// noteTurnError 记下失败原文，并立刻在对话里放一行。同一句只放一次。
func (s *Service) noteTurnError(ctx context.Context, t *turnState, msg string) {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return
	}
	t.mu.Lock()
	if t.finished || ctx.Err() != nil {
		t.mu.Unlock()
		return
	}
	if t.failMsg == "" || (t.failMsg == catagent.MsgReplyFailed && msg != catagent.MsgReplyFailed) {
		t.failMsg = msg
	}
	if msg == catagent.MsgReplyFailed {
		t.mu.Unlock()
		return
	}
	if t.errSeen == nil {
		t.errSeen = map[string]struct{}{}
	}
	if _, ok := t.errSeen[msg]; ok {
		t.mu.Unlock()
		return
	}
	t.errSeen[msg] = struct{}{}
	n := len(t.errSeen)
	s.clearRetryLocked(t)
	t.mu.Unlock()
	s.emit(catagent.EventMessage, catagent.MessageEvent{
		ConvID: t.convID, TurnID: t.turnID, MessageID: t.turnID + ":err:" + strconv.Itoa(n),
		Seq: 1, Op: catagent.OpReplace, TextDelta: msg,
		Block: &catagent.MessageBlock{Type: "error", Text: msg},
	})
}

// showRetry 把超时重试状态推到界面。同一句不重复发；空文字表示这一轮重试已经过去。
func (s *Service) showRetry(ctx context.Context, t *turnState, text string) {
	text = strings.TrimSpace(text)
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.finished || ctx.Err() != nil {
		return
	}
	if text == t.retryText {
		return
	}
	t.retryText = text
	if t.retryID == "" {
		t.retryID = t.turnID + ":retry"
	}
	t.retrySeq++
	s.emit(catagent.EventMessage, catagent.MessageEvent{
		ConvID: t.convID, TurnID: t.turnID, MessageID: t.retryID,
		Seq: t.retrySeq, Op: catagent.OpReplace, TextDelta: text,
		Block: &catagent.MessageBlock{Type: "retry", Text: text},
	})
}

// clearRetryLocked 收起重试行。调用时已持有 t.mu。
func (s *Service) clearRetryLocked(t *turnState) {
	if t.retryText == "" {
		return
	}
	t.retryText = ""
	if t.retryID == "" {
		return
	}
	t.retrySeq++
	s.emit(catagent.EventMessage, catagent.MessageEvent{
		ConvID: t.convID, TurnID: t.turnID, MessageID: t.retryID,
		Seq: t.retrySeq, Op: catagent.OpReplace,
		Block: &catagent.MessageBlock{Type: "retry"},
	})
}

// appendActivity 把思考增量或工具标签推到界面。思考按段追加；工具按 id 整段替换。
func (s *Service) appendActivity(ctx context.Context, t *turnState, kind, key, text string, replace bool) {
	if text == "" || (kind != "thought" && kind != "tool") {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.finished || ctx.Err() != nil {
		return
	}
	s.clearRetryLocked(t)
	if key == "" {
		key = kind
	}
	if t.actBy == nil {
		t.actBy = map[string]*actRec{}
	}
	rec := t.actBy[kind+":"+key]
	if rec == nil {
		// 正文已经开始之后又出现新的思考或工具：先收起这一段，后面的句子另起一段，留在工具下面
		if t.seq > 0 {
			s.rotateTextLocked(t)
		}
		rec = &actRec{kind: kind, id: t.msgID + ":" + kind + ":" + sanitizeActKey(key)}
		t.actBy[kind+":"+key] = rec
		t.acts = append(t.acts, rec)
	} else if replace && rec.text == text {
		return
	}
	if kind == "thought" {
		t.markFirstTokenLocked()
	}
	if replace {
		rec.text = text
	} else {
		rec.text += text
	}
	rec.seq++
	s.emit(catagent.EventMessage, catagent.MessageEvent{
		ConvID: t.convID, TurnID: t.turnID, MessageID: rec.id,
		Seq: rec.seq, Op: catagent.OpReplace, TextDelta: rec.text,
		Block: &catagent.MessageBlock{Type: kind, Text: rec.text},
	})
}

func sanitizeActKey(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "0"
	}
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" {
		return "0"
	}
	if len(out) > 40 {
		out = out[:40]
	}
	return out
}

// rotateTextLocked 结束当前正文段。下一段用新的 messageId，从 seq 1 重新计。
// 调用时已持有 t.mu。
func (s *Service) rotateTextLocked(t *turnState) {
	if t.seq == 0 {
		return
	}
	t.seq++
	prev, seq := t.msgID, t.seq
	t.msgID = id.New()
	t.seq = 0
	s.emit(catagent.EventMessage, catagent.MessageEvent{
		ConvID: t.convID, TurnID: t.turnID, MessageID: prev, Seq: seq, Op: catagent.OpDone,
	})
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
	s.clearRetryLocked(t)
	t.markFirstTokenLocked()
	t.seq++
	t.text.WriteString(delta)
	s.emit(catagent.EventMessage, catagent.MessageEvent{
		ConvID: t.convID, TurnID: t.turnID, MessageID: t.msgID,
		Seq: t.seq, Op: catagent.OpAppend, TextDelta: delta,
	})
	return true
}

// finish 发该消息 done（若已出过字）与终态 cat:turn；只生效一次。
// 进程退出后如果用量文件才写完，已收尾的一轮会再补发一次带数字的终态，供圆环更新。
func (s *Service) finish(t *turnState, status string) bool {
	s.stampContext(t)
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.finished {
		s.emitContextUpdateLocked(t, status)
		return false
	}
	return s.finishLocked(t, status)
}

func (s *Service) stampContext(t *turnState) {
	if t.usageCwd == "" || t.convID == "" {
		return
	}
	h := s.sessionHistory(t.agentKind)
	if h == nil {
		return
	}
	used, window, ok := h.ReadContextUsage(t.convID, t.usageCwd)
	if !ok {
		return
	}
	t.mu.Lock()
	t.contextUsed, t.contextWindow = used, window
	t.mu.Unlock()
}

func (s *Service) emitContextUpdateLocked(t *turnState, status string) {
	if t.contextUsed <= 0 || t.contextWindow <= 0 {
		return
	}
	if t.contextUsed == t.emittedUsed && t.contextWindow == t.emittedWindow {
		return
	}
	t.emittedUsed, t.emittedWindow = t.contextUsed, t.contextWindow
	ev := s.turnEventLocked(t, status)
	ev.ContextUsed, ev.ContextWindow = t.contextUsed, t.contextWindow
	s.emit(catagent.EventTurn, ev)
}

func (s *Service) finishLocked(t *turnState, status string) bool {
	if t.finished {
		return false
	}
	t.finished = true
	s.noteLastTurnStatus(t.convID, t.turnID, status)
	if t.seq > 0 {
		t.seq++
		s.emit(catagent.EventMessage, catagent.MessageEvent{
			ConvID: t.convID, TurnID: t.turnID, MessageID: t.msgID, Seq: t.seq, Op: catagent.OpDone,
		})
	}
	s.fillFooterLocked(t)
	ev := s.turnEventLocked(t, status)
	s.emit(catagent.EventTurn, ev)
	s.persistFooterLocked(t)
	schedHook := t.hook
	if h := schedHook; h != nil && h.onFinish != nil {
		go h.onFinish(status, ev.Error) // 持有 t.mu，回调另起 goroutine
	}
	// v0.40：普通 turn 完成/失败通知（定时回合走 hook，不在此发）
	if schedHook == nil {
		convID, turnErr := t.convID, ev.Error
		go s.notifyOrdinaryTurn(context.Background(), convID, status, turnErr)
	}
	return true
}

// noteTurnUsage 记下适配器上报的合计 / 输出 token（契约 v0.37.1、v0.47）。
func (s *Service) noteTurnUsage(t *turnState, resp catagent.TurnResponse) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if resp.TokenTotal > 0 {
		t.tokenTotal = resp.TokenTotal
	}
	if resp.OutputTokens > 0 {
		t.outputTokens = resp.OutputTokens
	}
	if resp.ContextUsed > 0 && resp.ContextWindow > 0 {
		// v0.48.3：Codex / Claude 由适配器在本轮结束时从 CLI 会话文件读出；不落盘，重开对话时再读一次。
		t.contextUsed, t.contextWindow = resp.ContextUsed, resp.ContextWindow
	}
}

func (s *Service) fillFooterLocked(t *turnState) {
	if t.finishedAt <= 0 {
		t.finishedAt = time.Now().UnixMilli()
	}
	t.tokensPerSec = computeTokensPerSec(t.outputTokens, msPtr(t.startedAt, t.finishedAt), msPtr(t.startedAt, t.firstTokenAt))
}

func (s *Service) turnEventLocked(t *turnState, status string) catagent.TurnEvent {
	ev := catagent.TurnEvent{ConvID: t.convID, TurnID: t.turnID, Status: status}
	if status == catagent.TurnFailed {
		msg := strings.TrimSpace(t.failMsg)
		if msg == "" {
			msg = catagent.MsgReplyFailed
		}
		ev.Error = apperr.New(apperr.CatReplyFailed, msg)
		if t.failReason != "" {
			ev.Error = ev.Error.WithDetail(t.failReason)
		}
		ev.Retryable = s.retryableNow(t.convID, t.turnID)
	}
	if t.contextUsed > 0 && t.contextWindow > 0 {
		ev.ContextUsed, ev.ContextWindow = t.contextUsed, t.contextWindow
		t.emittedUsed, t.emittedWindow = t.contextUsed, t.contextWindow
	}
	// v0.37.1：终态才带消息尾；running 不会走到这里。
	if status == catagent.TurnRunning {
		return ev
	}
	ev.MessageID = t.msgID
	if t.finishedAt > 0 {
		ev.FinishedAt = t.finishedAt
	}
	ev.FirstTokenMs = msPtr(t.startedAt, t.firstTokenAt)
	ev.TotalMs = msPtr(t.startedAt, t.finishedAt)
	ev.TokenTotal = int64Ptr(t.tokenTotal)
	ev.TokensPerSec = t.tokensPerSec
	ev.OutputTokens = int64Ptr(t.outputTokens)
	return ev
}

func (s *Service) persistFooterLocked(t *turnState) {
	if t.finishedAt <= 0 {
		return
	}
	// 只在本轮出过助手正文时落盘，避免空失败回合打乱「assistant 顺序 ↔ footer」对齐。
	// 终态 cat:turn 仍始终带 finishedAt，供进行中气泡挂尾。
	if t.text.Len() == 0 && t.seq == 0 {
		return
	}
	appendFooter(s.cfg.DataRoot, t.convID, messageFooter{
		TurnID: t.turnID, MessageID: t.msgID, FinishedAt: t.finishedAt,
		TokenTotal: int64Ptr(t.tokenTotal), TokensPerSec: t.tokensPerSec,
		OutputTokens: int64Ptr(t.outputTokens),
		FirstTokenMs: msPtr(t.startedAt, t.firstTokenAt), TotalMs: msPtr(t.startedAt, t.finishedAt),
	})
}

// finishCancelled 收尾被取消的一轮。已流出的文字只留在当前界面和 CLI 会话里，不写入 SQLite。
func (s *Service) finishCancelled(t *turnState) {
	s.finish(t, catagent.TurnCancelled) // 通常 stopTurn 已发过，这里幂等
}

// CancelCatTurn 立即停止 { convId, turnId } 这一轮；不匹配或已结束则幂等返回 nil。
// turnId 为空时停止该会话当前这一轮（兼容）。
func (s *Service) CancelCatTurn(ctx context.Context, req CancelTurnRequest) error {
	s.workflowMu.Lock()
	defer s.workflowMu.Unlock()
	convID := strings.TrimSpace(req.ConvID)
	if convID == "" {
		return apperr.New(apperr.InvalidArgument, "会话编号不能为空")
	}
	s.mu.Lock()
	current, busy := s.turns[convID]
	s.mu.Unlock()
	if busy && req.TurnID != "" && current.turnID != req.TurnID {
		return nil
	}
	if !busy && req.TurnID != "" {
		s.lastMu.Lock()
		lt := s.lastTurns[convID]
		stale := lt == nil || lt.turnID != req.TurnID
		s.lastMu.Unlock()
		if stale {
			return nil
		}
	}
	if s.cfg.Store != nil {
		if q, e := s.readQueue(ctx, convID); e == nil && len(q.Items) > 0 {
			q.Paused = true
			q.Error = "任务已停止，待发消息已保留。"
			_ = s.saveQueue(ctx, &q)
		}
	}
	if s.cancelTurn(convID, strings.TrimSpace(req.TurnID)) {
		return nil
	}
	// 刷新之后界面已经不记这一轮，进程可能还在。没有本进程的回合时才去结束它。
	if !busy {
		catagent.StopLiveConv(convID)
	}
	return nil
}

// LiveTurn 是刷新后仍在回复的一场。TurnID 为空表示进程还在，但已经不是这次打开的应用拉起来的。
type LiveTurn struct {
	ConvID string `json:"convId"`
	TurnID string `json:"turnId"`
}

// ListLiveCatTurns 告诉界面哪些对话还在回复。本进程记着的回合优先；其余看命令行里是否还挂着这场会话。
func (s *Service) ListLiveCatTurns(ctx context.Context) ([]LiveTurn, error) {
	s.mu.Lock()
	out := make([]LiveTurn, 0, len(s.turns))
	have := make(map[string]bool, len(s.turns))
	for id, t := range s.turns {
		have[id] = true
		out = append(out, LiveTurn{ConvID: id, TurnID: t.turnID})
	}
	s.mu.Unlock()
	if s.cfg.Store == nil || ctx == nil {
		return out, nil
	}
	list, err := s.cfg.Store.ListCatConversations(ctx)
	if err != nil {
		return out, nil
	}
	ids := make([]string, 0, len(list))
	for _, c := range list {
		if !have[c.ID] {
			ids = append(ids, c.ID)
		}
	}
	for _, id := range catagent.LiveConvIDs(ids) {
		out = append(out, LiveTurn{ConvID: id})
	}
	return out, nil
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

// lastTurnInfo 是 GetCatConversation 的 lastTurn（v0.44.6）：内存里没有 → nil；进行中 / noRetry 时不可重试。
func (s *Service) lastTurnInfo(convID string) *CatTurnState {
	s.mu.Lock()
	_, busy := s.turns[convID]
	s.mu.Unlock()
	s.lastMu.Lock()
	defer s.lastMu.Unlock()
	lt := s.lastTurns[convID]
	if lt == nil {
		return nil
	}
	return &CatTurnState{TurnID: lt.turnID, Status: lt.status, Retryable: !busy && !lt.noRetry && lt.status == catagent.TurnFailed}
}

// retryableNow 报告 turnID 是否是该会话内存里记下的最后一轮且失败（失败事件用）。
func (s *Service) retryableNow(convID, turnID string) bool {
	s.lastMu.Lock()
	defer s.lastMu.Unlock()
	lt := s.lastTurns[convID]
	return lt != nil && lt.turnID == turnID && lt.status == catagent.TurnFailed && !lt.noRetry
}

func (s *Service) markNoRetry(convID, turnID string) {
	s.lastMu.Lock()
	if lt := s.lastTurns[convID]; lt != nil && lt.turnID == turnID {
		lt.noRetry = true
	}
	s.lastMu.Unlock()
}

// markFirstTokenLocked 记下第一个正文 / 思考增量的时刻（§6.19.19.3 第 2 条）。调用时已持有 t.mu。
func (t *turnState) markFirstTokenLocked() {
	if t.firstTokenAt == 0 {
		t.firstTokenAt = time.Now().UnixMilli()
	}
}

func (s *Service) noteLastTurnStatus(convID, turnID, status string) {
	s.lastMu.Lock()
	if lt := s.lastTurns[convID]; lt != nil && lt.turnID == turnID {
		lt.status = status
	}
	s.lastMu.Unlock()
}

// RetryCatTurnRequest 是 RetryCatTurn 入参（契约 6.19.18.7）。
type RetryCatTurnRequest struct {
	ConversationID string `json:"conversationId"`
	TurnID         string `json:"turnId"`
}

// RetryCatTurn 原地重试会话最后一轮失败的 turn（6.19.18.7）：重发原用户消息（附件、model / effort / mode 不变），
// 不新增用户消息；返回新 turnId，事件沿用 cat:message / cat:turn。参数按当前 CLI 版本 / 模型重新生成，
// 不再支持的档位按 effortForTurn 回落为不传。
// 只认本进程记下的最后一轮（应用重启后没有记录 → retry_not_allowed）。
func (s *Service) RetryCatTurn(ctx context.Context, req RetryCatTurnRequest) (SendMessageResult, error) {
	convID := strings.TrimSpace(req.ConversationID)
	turnID := strings.TrimSpace(req.TurnID)
	notAllowed := apperr.New(apperr.InvalidArgument, "这条回复已无法重试，请重新发送。").WithDetail("reason=retry_not_allowed")
	if convID == "" || turnID == "" {
		return SendMessageResult{}, notAllowed
	}
	s.mu.Lock()
	_, busy := s.turns[convID]
	s.mu.Unlock()
	s.lastMu.Lock()
	lt := s.lastTurns[convID]
	var orig lastTurn
	if lt != nil {
		orig = *lt
	}
	s.lastMu.Unlock()
	if busy || lt == nil || orig.turnID != turnID || orig.status != catagent.TurnFailed || orig.noRetry {
		return SendMessageResult{}, notAllowed
	}
	um := orig.userMsg
	// 定时任务的收尾回调已在失败时调过一次，重试按手动对话处理（hook=nil）。
	return s.startTurn(ctx, orig.req, nil, &um)
}

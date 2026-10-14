package catagent

import (
	"sync"
)

// Adapter 是某一 agentKind 的运行时适配器。
type Adapter interface {
	Kind() string
	// ExecutablePath 仅内部使用，绝不回前端。
	ExecutablePath() string
	ProtocolVersion() int
	Status() Status
	Recheck() Status
	ListModels() ([]Model, error)
	ListThinkLevels() ([]ThinkLevel, error)
	// RunTurn 执行一轮；未就绪返回 CAT_NOT_READY。
	RunTurn(opts TurnOptions) (TurnResponse, error)
}

// Registry 按 agentKind 查找适配器。v0.36 注册 cat_build / claude_cli / codex_cli。
type Registry struct {
	mu  sync.RWMutex
	by  map[string]Adapter
	ord []string // 注册顺序（v0.47.1：cat:status agents[] 按此顺序）
	def string   // 新建会话默认 kind
}

// NewRegistry 创建空注册表，默认 kind=cat_build。
func NewRegistry() *Registry {
	return &Registry{by: map[string]Adapter{}, def: KindCatBuild}
}

// Register 注册适配器（同 kind 覆盖）。
func (r *Registry) Register(a Adapter) {
	if a == nil {
		return
	}
	r.mu.Lock()
	if _, ok := r.by[a.Kind()]; !ok {
		r.ord = append(r.ord, a.Kind())
	}
	r.by[a.Kind()] = a
	r.mu.Unlock()
}

// Kinds 返回已注册的 agentKind，按注册顺序（v0.47.1 6.19.21.1）。
func (r *Registry) Kinds() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]string(nil), r.ord...)
}

// Get 按 kind 取适配器。
func (r *Registry) Get(kind string) (Adapter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.by[kind]
	return a, ok
}

// DefaultKind 返回新建会话预填的 agentKind。
func (r *Registry) DefaultKind() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.def == "" {
		return KindCatBuild
	}
	return r.def
}

// SetDefaultKind 只影响新建会话预填。
func (r *Registry) SetDefaultKind(kind string) {
	r.mu.Lock()
	if kind == "" {
		kind = KindCatBuild
	}
	r.def = kind
	r.mu.Unlock()
}

// CreatableKinds 是 v0.36 可创建的 agentKind，固定顺序（GetCatStatus.agents[] / 欢迎页）。
func CreatableKinds() []string {
	return []string{KindCatBuild, KindClaudeCLI, KindCodexCLI}
}

// Phase1Creatable 报告该 kind 是否允许创建会话（v0.36：三家）。
func Phase1Creatable(kind string) bool {
	switch kind {
	case KindCatBuild, KindClaudeCLI, KindCodexCLI:
		return true
	}
	return false
}

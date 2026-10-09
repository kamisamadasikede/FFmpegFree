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

// Registry 按 agentKind 查找适配器。一期只注册 cat_build。
type Registry struct {
	mu  sync.RWMutex
	by  map[string]Adapter
	def string // 新建会话默认 kind
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
	r.by[a.Kind()] = a
	r.mu.Unlock()
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

// Phase1Creatable 一期是否允许用该 kind 创建会话。
func Phase1Creatable(kind string) bool {
	return kind == KindCatBuild
}

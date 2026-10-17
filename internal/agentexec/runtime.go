// Package agentexec owns CLI execution shared by individual chats and teams.
package agentexec

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"FFmpegFree/internal/catagent"
)

type Scope struct {
	OwnerID   string
	Workspace string
	// NativeConfigKey identifies compatible native Skill/MCP configurations.
	NativeConfigKey string
	Prepare         func(context.Context) error
}

type Runtime struct {
	mu          sync.Mutex
	changed     chan struct{}
	closed      bool
	active      map[string]context.CancelFunc
	roots       map[string]string
	configKey   string
	configUsers int
	preparing   bool
	wg          sync.WaitGroup
}

func New() *Runtime {
	return &Runtime{changed: make(chan struct{}), active: map[string]context.CancelFunc{}, roots: map[string]string{}}
}

func CanonicalRoot(root string) (string, error) {
	p, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	p, err = filepath.EvalSymlinks(p)
	if err != nil {
		return "", err
	}
	st, err := os.Stat(p)
	if err != nil {
		return "", err
	}
	if !st.IsDir() {
		return "", errors.New("工作目录不是文件夹")
	}
	return filepath.Clean(p), nil
}

func overlaps(a, b string) bool {
	// Windows roots and junction aliases must use the same physical identity.
	a, b = filepath.Clean(a), filepath.Clean(b)
	if filepath.Separator == '\\' {
		a, b = strings.ToLower(a), strings.ToLower(b)
	}
	return a == b || strings.HasPrefix(a, b+string(filepath.Separator)) || strings.HasPrefix(b, a+string(filepath.Separator))
}

func (r *Runtime) wakeLocked() { close(r.changed); r.changed = make(chan struct{}) }

// WorkspaceBusy is only a scheduling hint; Run still performs the atomic lease.
func (r *Runtime) WorkspaceBusy(root string) bool {
	root, e := CanonicalRoot(root)
	if e != nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, held := range r.roots {
		if overlaps(root, held) {
			return true
		}
	}
	return false
}

// Run reserves a native session and physical workspace until the native process
// exits. Waiting is cancellable; no database transaction is held here.
func (r *Runtime) Run(a catagent.Adapter, scope Scope, opts catagent.TurnOptions) (catagent.TurnResponse, error) {
	if opts.Ctx == nil {
		opts.Ctx = context.Background()
	}
	root, err := CanonicalRoot(scope.Workspace)
	if err != nil {
		return catagent.TurnResponse{}, err
	}
	if scope.OwnerID == "" || opts.ConversationID == "" {
		return catagent.TurnResponse{}, errors.New("执行身份不能为空")
	}
	ctx, cancel := context.WithCancel(opts.Ctx)
	defer cancel()
	key := a.Kind() + ":" + opts.ConversationID
	for {
		r.mu.Lock()
		if r.closed {
			r.mu.Unlock()
			return catagent.TurnResponse{}, context.Canceled
		}
		if _, exists := r.active[key]; exists {
			r.mu.Unlock()
			return catagent.TurnResponse{}, errors.New("原生会话正在执行")
		}
		blocked := r.preparing || (r.configUsers > 0 && r.configKey != scope.NativeConfigKey)
		for _, held := range r.roots {
			blocked = blocked || overlaps(root, held)
		}
		if !blocked {
			first := r.configUsers == 0
			if first {
				r.preparing = true
			}
			r.configKey = scope.NativeConfigKey
			r.configUsers++
			r.active[key], r.roots[key] = cancel, root
			r.wg.Add(1)
			r.mu.Unlock()
			defer func() {
				r.mu.Lock()
				delete(r.active, key)
				delete(r.roots, key)
				r.configUsers--
				if first {
					r.preparing = false
				}
				r.wakeLocked()
				r.mu.Unlock()
				r.wg.Done()
			}()
			if first {
				if scope.Prepare != nil {
					err = scope.Prepare(ctx)
				}
				r.mu.Lock()
				r.preparing = false
				r.wakeLocked()
				r.mu.Unlock()
				if err != nil {
					return catagent.TurnResponse{}, err
				}
			}
			opts.Ctx, opts.ProjectPath = ctx, root
			return a.RunTurn(opts)
		}
		ch := r.changed
		r.mu.Unlock()
		select {
		case <-ctx.Done():
			return catagent.TurnResponse{}, ctx.Err()
		case <-ch:
		}
	}
}

// Close explicitly cancels runs, including callers using context.WithoutCancel.
func (r *Runtime) Close() {
	r.mu.Lock()
	r.closed = true
	for _, c := range r.active {
		c()
	}
	r.wakeLocked()
	r.mu.Unlock()
	r.wg.Wait()
}

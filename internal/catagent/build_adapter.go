package catagent

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/id"
)

// 占位入口名（老板发布包后以实际名为准；路径只进注册表，不回前端）。
const (
	entryNameUnix    = "cat-build"
	entryNameWindows = "cat-build.exe"
)

// DefaultBuildTimeout 是一轮对话超时（协议 TBD）。
const DefaultBuildTimeout = 10 * time.Minute

// BuildConfig 配置 Build 适配器。
type BuildConfig struct {
	// ComponentDir 是组件父目录（…/components）；实际根为 …/cat/build/<version>/。
	ComponentDir string
	DataTemp     string // <数据目录>/tmp，工作目录在 cat/<id>/
	Emit         func(event string, payload any)
	Logf         func(format string, args ...any)
	// Exec 可覆盖（测试注入假二进制）。
	Exec func(ctx context.Context, name string, args ...string) *exec.Cmd
	// Now 可覆盖。
	Now func() time.Time
}

// BuildAdapter 是 cat_build 适配器。
type BuildAdapter struct {
	cfg BuildConfig

	mu      sync.Mutex
	st      Status
	version string // 探测到的版本目录名
	root    string // 组件根（含入口）
	models  []Model
	thinks  []ThinkLevel
	checked bool
}

// NewBuildAdapter 创建适配器（不立即检测）。
func NewBuildAdapter(cfg BuildConfig) *BuildAdapter {
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	a := &BuildAdapter{cfg: cfg}
	a.st = Status{State: StateChecking, CanDownload: false}
	return a
}

func (a *BuildAdapter) Kind() string           { return KindCatBuild }
func (a *BuildAdapter) ProtocolVersion() int   { return ProtocolVersion }
func (a *BuildAdapter) ExecutablePath() string { return a.entryPathLocked() }

func (a *BuildAdapter) entryPathLocked() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return EntryBinary(a.root)
}

// EntryBinary 返回组件根下的入口路径；不存在返回 ""。
func EntryBinary(componentRoot string) string {
	if componentRoot == "" {
		return ""
	}
	name := entryNameUnix
	if runtime.GOOS == "windows" {
		name = entryNameWindows
	}
	p := filepath.Join(componentRoot, name)
	fi, err := os.Stat(p)
	if err != nil || fi.IsDir() {
		return ""
	}
	return p
}

// DefaultComponentDir 返回 …/components（与文档 / 语音识别组件同级）。
func DefaultComponentDir(dataRoot string) string {
	if runtime.GOOS == "windows" {
		if la := os.Getenv("LOCALAPPDATA"); la != "" {
			return filepath.Join(la, "FFmpegFree", "components")
		}
	}
	return filepath.Join(dataRoot, "components")
}

func (a *BuildAdapter) buildRootBase() string {
	return filepath.Join(a.cfg.ComponentDir, "cat", "build")
}

// Start 后台检测一次。
func (a *BuildAdapter) Start() { go a.detect() }

func (a *BuildAdapter) Status() Status {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.st
}

func (a *BuildAdapter) Recheck() Status {
	a.detect()
	return a.Status()
}

func (a *BuildAdapter) setStatus(st Status) {
	a.mu.Lock()
	a.st = st
	a.mu.Unlock()
	if a.cfg.Emit != nil {
		a.cfg.Emit(EventStatus, st)
	}
}

func (a *BuildAdapter) detect() {
	a.setStatus(Status{State: StateChecking, CanDownload: false})
	base := a.buildRootBase()
	root, ver := findLatestBuildRoot(base)
	exe := EntryBinary(root)
	if exe == "" {
		// 一期未发布：canDownload=false，无下载按钮。
		a.mu.Lock()
		a.root, a.version, a.models, a.thinks, a.checked = "", "", nil, nil, true
		a.mu.Unlock()
		a.setStatus(Status{
			State:       StateMissing,
			CanDownload: false,
			Error:       apperr.New(apperr.CatNotReady, MsgNotReady),
		})
		return
	}
	caps, err := a.probeCapabilities(root, exe)
	if err != nil {
		a.mu.Lock()
		a.root, a.version, a.checked = root, ver, true
		a.mu.Unlock()
		a.setStatus(Status{
			State:       StateFailed,
			Version:     ver,
			CanDownload: false,
			Error:       apperr.From(err),
		})
		return
	}
	a.mu.Lock()
	a.root, a.version = root, ver
	a.models, a.thinks = caps.Models, caps.ThinkLevels
	a.checked = true
	a.mu.Unlock()
	a.setStatus(Status{State: StateReady, Version: ver, CanDownload: false})
}

func findLatestBuildRoot(base string) (root, version string) {
	entries, err := os.ReadDir(base)
	if err != nil {
		return "", ""
	}
	// 选名字字典序最大的目录（版本号字符串 TBD；占位可用 "0.0.0-dev"）。
	var best string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "tmp" {
			continue
		}
		if name > best {
			best = name
		}
	}
	if best == "" {
		return "", ""
	}
	return filepath.Join(base, best), best
}

func (a *BuildAdapter) probeCapabilities(root, exe string) (Capabilities, error) {
	work := filepath.Join(a.cfg.DataTemp, "cat", "cap-"+id.New())
	if err := os.MkdirAll(work, 0o755); err != nil {
		return Capabilities{}, apperr.Wrap(apperr.IOError, "无法创建临时文件夹", err)
	}
	defer os.RemoveAll(work)
	capPath := filepath.Join(work, "capabilities.json")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	run := a.cfg.Exec
	if run == nil {
		run = exec.CommandContext
	}
	cmd := run(ctx, exe, "--component-root", root, "--capabilities", capPath)
	cmd.Dir = work
	var stderr strings.Builder
	cmd.Stderr = &stderr
	cmd.Stdout = &stderr
	if err := cmd.Run(); err != nil {
		return Capabilities{}, apperr.Wrap(apperr.CatNotReady, MsgNotReady, err).WithDetail("reason=capabilities_failed")
	}
	raw, err := os.ReadFile(capPath)
	if err != nil {
		return Capabilities{}, apperr.Wrap(apperr.CatNotReady, MsgNotReady, err).WithDetail("reason=capabilities_missing")
	}
	var caps Capabilities
	if err := json.Unmarshal(raw, &caps); err != nil {
		return Capabilities{}, apperr.Wrap(apperr.CatNotReady, MsgNotReady, err).WithDetail("reason=capabilities_invalid")
	}
	if caps.Models == nil {
		caps.Models = []Model{}
	}
	if caps.ThinkLevels == nil {
		caps.ThinkLevels = []ThinkLevel{}
	}
	return caps, nil
}

func (a *BuildAdapter) ListModels() ([]Model, error) {
	st := a.Status()
	if st.State != StateReady {
		return nil, apperr.New(apperr.CatNotReady, MsgNotReady)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	out := append([]Model(nil), a.models...)
	if out == nil {
		out = []Model{}
	}
	return out, nil
}

func (a *BuildAdapter) ListThinkLevels() ([]ThinkLevel, error) {
	st := a.Status()
	if st.State != StateReady {
		return nil, apperr.New(apperr.CatNotReady, MsgNotReady)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	out := append([]ThinkLevel(nil), a.thinks...)
	if out == nil {
		out = []ThinkLevel{}
	}
	return out, nil
}

// TurnOptions 是一轮对话调用参数。
type TurnOptions struct {
	Ctx            context.Context
	ConversationID string
	TurnID         string
	ModelID        string
	ThinkLevelID   string
	ProjectPath    string
	Messages       []WireMessage
	Timeout        time.Duration
	// OnTextDelta 可选：适配器拿到真流式输出时逐段回调（CLI 流式协议 TBD）。
	// 一轮内只要回调过一次，服务端就以回调累积文本为准，不再拆整段回复。
	OnTextDelta func(delta string)
}

// RunTurn 写 request → 调入口 → 读 response；未就绪 / 失败返回约定错误码。
func (a *BuildAdapter) RunTurn(opts TurnOptions) (TurnResponse, error) {
	st := a.Status()
	if st.State != StateReady {
		return TurnResponse{}, apperr.New(apperr.CatNotReady, MsgNotReady)
	}
	a.mu.Lock()
	root := a.root
	a.mu.Unlock()
	exe := EntryBinary(root)
	if exe == "" {
		return TurnResponse{}, apperr.New(apperr.CatNotReady, MsgNotReady)
	}
	turnID := opts.TurnID
	if turnID == "" {
		turnID = id.New()
	}
	work := filepath.Join(a.cfg.DataTemp, "cat", opts.ConversationID+"-"+turnID)
	if err := os.MkdirAll(work, 0o755); err != nil {
		return TurnResponse{}, apperr.Wrap(apperr.IOError, "无法创建临时文件夹", err)
	}
	reqPath := filepath.Join(work, "request.json")
	respPath := filepath.Join(work, "response.json")
	req := TurnRequest{
		Version:        ProtocolVersion,
		ConversationID: opts.ConversationID,
		TurnID:         turnID,
		ModelID:        opts.ModelID,
		ThinkLevelID:   opts.ThinkLevelID,
		ProjectPath:    opts.ProjectPath,
		Messages:       opts.Messages,
	}
	b, err := json.Marshal(req)
	if err != nil {
		return TurnResponse{}, apperr.Wrap(apperr.Internal, "内部错误", err)
	}
	if err := os.WriteFile(reqPath, b, 0o644); err != nil {
		return TurnResponse{}, apperr.Wrap(apperr.IOError, "无法写入请求", err)
	}
	_ = os.Remove(respPath)

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultBuildTimeout
	}
	ctx := opts.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	run := a.cfg.Exec
	if run == nil {
		run = exec.CommandContext
	}
	cmd := run(runCtx, exe,
		"--component-root", root,
		"--request", reqPath,
		"--response", respPath,
	)
	cmd.Dir = work
	var stderr strings.Builder
	cmd.Stderr = &stderr
	cmd.Stdout = &stderr
	err = cmd.Run()
	if a.cfg.Logf != nil && stderr.Len() > 0 {
		a.cfg.Logf("cat build turn stderr: %s", stderr.String())
	}
	if runCtx.Err() != nil {
		if ctx.Err() != nil {
			return TurnResponse{}, apperr.New(apperr.Canceled, "已取消")
		}
		return TurnResponse{}, apperr.New(apperr.CatReplyFailed, MsgReplyFailed).WithDetail("reason=timeout")
	}
	if err != nil {
		return TurnResponse{}, apperr.Wrap(apperr.CatReplyFailed, MsgReplyFailed, err)
	}
	raw, err := os.ReadFile(respPath)
	if err != nil {
		return TurnResponse{}, apperr.Wrap(apperr.CatReplyFailed, MsgReplyFailed, err).WithDetail("reason=response_missing")
	}
	var resp TurnResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return TurnResponse{}, apperr.Wrap(apperr.CatReplyFailed, MsgReplyFailed, err).WithDetail("reason=response_invalid")
	}
	if resp.Message.Role == "" {
		resp.Message.Role = "assistant"
	}
	return resp, nil
}

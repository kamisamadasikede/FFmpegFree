package catagent

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/id"
	"FFmpegFree/internal/proc"
)

// 占位入口名（组件目录回退；正式路径以 PATH 上的 grok 为准，不回前端）。
const (
	entryNameUnix    = "cat-build"
	entryNameWindows = "cat-build.exe"
	grokEntryUnix    = "grok"
	grokEntryWindows = "grok.exe"
)

// DefaultBuildTimeout 是一轮对话超时。
const DefaultBuildTimeout = 10 * time.Minute

// 默认能力（PATH 上找到 grok 后使用；CLI 未提供独立 capabilities 探测）。
var (
	defaultModelIDs = []string{"grok-4.6", "grok-4.5"}
	defaultThinks   = []ThinkLevel{
		{ID: "low", DisplayName: "低"},
		{ID: "medium", DisplayName: "中"},
		{ID: "high", DisplayName: "高"},
	}
)

// BuildConfig 配置 Build 适配器。
type BuildConfig struct {
	// ComponentDir 是组件父目录（…/components）；PATH 未命中时回退 …/cat/build/<version>/。
	ComponentDir string
	DataTemp     string // <数据目录>/tmp；无项目时用作 --cwd
	Emit         func(event string, payload any)
	Logf         func(format string, args ...any)
	// Exec 可覆盖（测试注入假二进制）。
	Exec func(ctx context.Context, name string, args ...string) *exec.Cmd
	// LookPath 可覆盖（测试注入）；默认 exec.LookPath。
	LookPath func(file string) (string, error)
	// Now 可覆盖。
	Now func() time.Time
}

// BuildAdapter 是 cat_build 适配器：优先 PATH 上的 grok，调 headless streaming-json。
type BuildAdapter struct {
	cfg BuildConfig

	mu      sync.Mutex
	st      Status
	version string // 探测到的版本标签（path / 组件目录名）
	root    string // 组件根（仅组件回退时有值）
	exe     string // 绝对或 PATH 解析后的入口路径（不回前端）
	viaPATH bool   // true = grok CLI；false = 旧组件入口（仍走 grok 参数若名为 grok）
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
	if cfg.LookPath == nil {
		cfg.LookPath = exec.LookPath
	}
	a := &BuildAdapter{cfg: cfg}
	a.st = Status{State: StateChecking, CanDownload: false}
	return a
}

func (a *BuildAdapter) Kind() string         { return KindCatBuild }
func (a *BuildAdapter) ProtocolVersion() int { return ProtocolVersion }

func (a *BuildAdapter) ExecutablePath() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.exe
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

func (a *BuildAdapter) markAuthInvalid() {
	a.mu.Lock()
	ver := a.version
	a.mu.Unlock()
	a.setStatus(Status{
		State:       StateFailed,
		Version:     ver,
		CanDownload: false,
		Error:       apperr.New(apperr.CatNotReady, MsgAuthInvalid).WithDetail("reason=auth"),
	})
}

func (a *BuildAdapter) lookPath(file string) (string, error) {
	if a.cfg.LookPath != nil {
		return a.cfg.LookPath(file)
	}
	return exec.LookPath(file)
}

// resolveGrokExe 按 PATH 找 grok / grok.exe。
func (a *BuildAdapter) resolveGrokExe() string {
	candidates := []string{grokEntryUnix}
	if runtime.GOOS == "windows" {
		candidates = []string{grokEntryWindows, grokEntryUnix}
	}
	for _, name := range candidates {
		p, err := a.lookPath(name)
		if err == nil && p != "" {
			if fi, e := os.Stat(p); e == nil && !fi.IsDir() {
				return p
			}
			// 测试注入的假路径可能不存在文件，仍接受（由 Exec 接管）。
			if a.cfg.Exec != nil {
				return p
			}
		}
	}
	return ""
}

func (a *BuildAdapter) detect() {
	a.setStatus(Status{State: StateChecking, CanDownload: false})

	if exe := a.resolveGrokExe(); exe != "" {
		ver := a.probeGrokVersion(exe)
		if ver == "" {
			ver = "path"
		}
		models := a.probeGrokModels(exe)
		a.mu.Lock()
		a.exe, a.root, a.version = exe, "", ver
		a.viaPATH = true
		a.models = models
		a.thinks = append([]ThinkLevel(nil), defaultThinks...)
		a.checked = true
		a.mu.Unlock()
		a.setStatus(Status{State: StateReady, Version: ver, CanDownload: false})
		return
	}

	// 回退：组件目录里的 cat-build（旧占位；仍按 grok 协议调不了，标 missing）。
	base := a.buildRootBase()
	root, ver := findLatestBuildRoot(base)
	exe := EntryBinary(root)
	if exe == "" {
		a.mu.Lock()
		a.exe, a.root, a.version, a.models, a.thinks, a.viaPATH, a.checked = "", "", "", nil, nil, false, true
		a.mu.Unlock()
		a.setStatus(Status{
			State:       StateMissing,
			CanDownload: false,
			Error:       apperr.New(apperr.CatNotReady, MsgNotReady),
		})
		return
	}
	// 组件入口不是 grok CLI：一期不跑旧 request/response 协议，当作未就绪，避免假回复。
	a.mu.Lock()
	a.exe, a.root, a.version, a.viaPATH, a.checked = exe, root, ver, false, true
	a.models, a.thinks = nil, nil
	a.mu.Unlock()
	a.setStatus(Status{
		State:       StateMissing,
		Version:     ver,
		CanDownload: false,
		Error:       apperr.New(apperr.CatNotReady, MsgNotReady).WithDetail("reason=need_grok_on_path"),
	})
}

func findLatestBuildRoot(base string) (root, version string) {
	entries, err := os.ReadDir(base)
	if err != nil {
		return "", ""
	}
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
	// OnTextDelta 可选：适配器拿到真流式输出时逐段回调。
	// 一轮内只要回调过一次，服务端就以回调累积文本为准，不再拆整段回复。
	OnTextDelta func(delta string)
}

// RunTurn 调 PATH 上的 grok headless：streaming-json → OnTextDelta；取消杀进程树。
func (a *BuildAdapter) RunTurn(opts TurnOptions) (TurnResponse, error) {
	st := a.Status()
	if st.State != StateReady {
		return TurnResponse{}, apperr.New(apperr.CatNotReady, MsgNotReady)
	}
	a.mu.Lock()
	exe := a.exe
	a.mu.Unlock()
	if exe == "" {
		return TurnResponse{}, apperr.New(apperr.CatNotReady, MsgNotReady)
	}

	prompt := lastUserContent(opts.Messages)
	if strings.TrimSpace(prompt) == "" {
		return TurnResponse{}, apperr.New(apperr.InvalidArgument, "消息不能为空")
	}

	cwd := strings.TrimSpace(opts.ProjectPath)
	if cwd == "" {
		base := a.cfg.DataTemp
		if base == "" {
			base = os.TempDir()
		}
		cwd = filepath.Join(base, "cat", "cwd-"+safeID(opts.ConversationID))
		if err := os.MkdirAll(cwd, 0o755); err != nil {
			return TurnResponse{}, apperr.Wrap(apperr.IOError, "无法创建临时文件夹", err)
		}
	}

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

	args := []string{
		"-p", prompt,
		"--output-format", "streaming-json",
		"--cwd", cwd,
		"--no-auto-update",
		"--no-alt-screen",
		// headless 无法点批准，一期保留自动批准防挂死；工具同时收窄成只读（见 cli_readonly.go）。
		"--always-approve",
	}
	args = append(args, readOnlyCLIArgs()...)
	if sid := strings.TrimSpace(opts.ConversationID); sid != "" {
		uuid := sessionIDFromConv(sid)
		if hasPriorAssistant(opts.Messages) {
			args = append(args, "--resume", uuid)
		} else {
			args = append(args, "--session-id", uuid)
		}
	}
	if mid := strings.TrimSpace(opts.ModelID); mid != "" && mid != "default" {
		args = append(args, "-m", mid)
	}
	if tl := strings.TrimSpace(opts.ThinkLevelID); tl != "" {
		args = append(args, "--reasoning-effort", tl)
	}

	run := a.cfg.Exec
	if run == nil {
		run = exec.CommandContext
	}
	cmd := run(runCtx, exe, args...)
	cmd.Dir = cwd
	cmd.Env = grokTurnEnv(os.Environ())
	proc.Configure(cmd)
	cmd.Cancel = func() error { return proc.Kill(cmd) }

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return TurnResponse{}, apperr.Wrap(apperr.CatReplyFailed, MsgReplyFailed, err).WithDetail("reason=stdout_pipe")
	}
	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return TurnResponse{}, apperr.Wrap(apperr.CatReplyFailed, MsgReplyFailed, err).WithDetail("reason=start_failed")
	}

	var (
		acc      strings.Builder
		streamed bool
		cliErr   string
	)
	scanErr := scanStreamingJSON(stdout, func(delta string) {
		streamed = true
		acc.WriteString(delta)
		if opts.OnTextDelta != nil {
			opts.OnTextDelta(delta)
		}
	}, func(msg string) {
		cliErr = msg
	})

	waitErr := cmd.Wait()
	if a.cfg.Logf != nil && stderrBuf.Len() > 0 {
		a.cfg.Logf("cat build turn stderr: %s", truncate(stderrBuf.String(), 2000))
	}
	if scanErr != nil && a.cfg.Logf != nil {
		a.cfg.Logf("cat build stream scan: %v", scanErr)
	}

	if runCtx.Err() != nil {
		if ctx.Err() != nil {
			return TurnResponse{}, apperr.New(apperr.Canceled, "已取消")
		}
		return TurnResponse{}, apperr.New(apperr.CatReplyFailed, MsgReplyFailed).WithDetail("reason=timeout")
	}
	if cliErr != "" {
		if isAuthFailure(cliErr) {
			a.markAuthInvalid()
			return TurnResponse{}, authNotReadyErr()
		}
		return TurnResponse{}, apperr.New(apperr.CatReplyFailed, MsgReplyFailed).WithDetail("reason=cli_error " + truncate(cliErr, 500))
	}
	if waitErr != nil {
		// 已有流式正文时，非零退出仍把已出文字交回（取消另走上面分支）。
		if streamed && strings.TrimSpace(acc.String()) != "" {
			return TurnResponse{
				Version: ProtocolVersion,
				Message: WireMessage{Role: "assistant", Content: acc.String()},
			}, nil
		}
		errText := waitErr.Error() + " " + stderrBuf.String() + " " + cliErr
		if isAuthFailure(errText) {
			a.markAuthInvalid()
			return TurnResponse{}, authNotReadyErr()
		}
		return TurnResponse{}, apperr.Wrap(apperr.CatReplyFailed, MsgReplyFailed, waitErr)
	}

	body := acc.String()
	if strings.TrimSpace(body) == "" {
		return TurnResponse{}, apperr.New(apperr.CatReplyFailed, MsgReplyFailed).WithDetail("reason=empty_reply")
	}
	return TurnResponse{
		Version: ProtocolVersion,
		Message: WireMessage{Role: "assistant", Content: body},
	}, nil
}

func lastUserContent(msgs []WireMessage) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" && strings.TrimSpace(msgs[i].Content) != "" {
			return msgs[i].Content
		}
	}
	if len(msgs) > 0 {
		return msgs[len(msgs)-1].Content
	}
	return ""
}

func safeID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return id.New()
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
		return id.New()
	}
	if len(out) > 80 {
		out = out[:80]
	}
	return out
}

func grokTurnEnv(base []string) []string {
	out := make([]string, 0, len(base)+4)
	skip := map[string]bool{
		"GROK_SANDBOX":             true,
		"GROK_WRITE_FILE":          true,
		"GROK_DISABLE_AUTOUPDATER": true,
	}
	for _, e := range base {
		if i := strings.IndexByte(e, '='); i > 0 {
			if skip[e[:i]] {
				continue
			}
		}
		out = append(out, e)
	}
	// 沙箱 read-only 是第三道保险（Linux/macOS 生效；Windows 上 CLI 无沙箱实现）。
	out = append(out,
		"GROK_SANDBOX=read-only",
		"GROK_WRITE_FILE=0",
		"GROK_DISABLE_AUTOUPDATER=1",
	)
	return out
}

// isAuthFailure 判断 CLI 报错是否像登录 / API Key 失效（不把厂商名写进用户文案）。
func isAuthFailure(msg string) bool {
	s := strings.ToLower(msg)
	keys := []string{
		"unauthorized", "unauthenticated", "authentication", "not logged", "please log in", "please login",
		"api key", "api_key", "xai_api_key", "invalid key", "invalid token", "expired token",
		"auth failed", "auth error", "login required", "not authenticated", "not signed in", "401",
		"登录", "未登录", "鉴权", "认证失败",
	}
	for _, k := range keys {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

func authNotReadyErr() error {
	return apperr.New(apperr.CatNotReady, MsgAuthInvalid).WithDetail("reason=auth")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// streamEvent 是 grok --output-format streaming-json 的一行（捕获自 CLI 0.2.x；官方未正式发布 schema）。
// text.data = 助手正文增量；thought 忽略；error.message = 失败；end = 回合结束。
type streamEvent struct {
	Type    string `json:"type"`
	Data    string `json:"data"`
	Message string `json:"message"`
}

func scanStreamingJSON(r io.Reader, onText func(string), onErr func(string)) error {
	sc := bufio.NewScanner(r)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 4*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		delta, errMsg, isErr := parseStreamLine(line)
		if isErr {
			if onErr != nil && errMsg != "" {
				onErr(errMsg)
			}
			continue
		}
		if delta != "" && onText != nil {
			onText(delta)
		}
	}
	return sc.Err()
}

func parseStreamLine(line []byte) (delta string, errMsg string, isErr bool) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(line, &raw); err != nil {
		return "", "", false
	}
	if typ, ok := rawStr(raw, "type"); ok && strings.EqualFold(typ, "error") {
		msg, _ := rawStr(raw, "message")
		if msg == "" {
			msg, _ = rawStr(raw, "data")
		}
		return "", msg, true
	}
	if method, ok := rawStr(raw, "method"); ok && method == "session/update" {
		if params, ok := raw["params"]; ok {
			var p struct {
				Update struct {
					SessionUpdate string          `json:"sessionUpdate"`
					Content       json.RawMessage `json:"content"`
				} `json:"update"`
			}
			if json.Unmarshal(params, &p) == nil {
				su := p.Update.SessionUpdate
				if su == "agent_thought_chunk" || strings.Contains(su, "tool") || su == "plan" {
					return "", "", false
				}
				if su == "agent_message_chunk" || su == "agent_message" {
					return extractContentDelta(p.Update.Content), "", false
				}
			}
		}
	}
	var ev streamEvent
	if json.Unmarshal(line, &ev) == nil {
		switch strings.ToLower(ev.Type) {
		case "text", "delta", "content", "message":
			if ev.Data != "" {
				return ev.Data, "", false
			}
			if ev.Message != "" {
				return ev.Message, "", false
			}
			if d, _ := rawStr(raw, "text"); d != "" {
				return d, "", false
			}
			if d, _ := rawStr(raw, "delta"); d != "" {
				return d, "", false
			}
		case "thought", "thinking", "end", "tool", "tool_call":
			return "", "", false
		}
	}
	return "", "", false
}

func extractContentDelta(content json.RawMessage) string {
	content = bytes.TrimSpace(content)
	if len(content) == 0 {
		return ""
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(content, &obj) == nil {
		if t, ok := rawStr(obj, "text"); ok {
			return t
		}
	}
	var arr []map[string]json.RawMessage
	if json.Unmarshal(content, &arr) == nil {
		var b strings.Builder
		for _, block := range arr {
			typ, _ := rawStr(block, "type")
			if typ != "" && typ != "text" {
				continue
			}
			if t, ok := rawStr(block, "text"); ok {
				b.WriteString(t)
			}
		}
		return b.String()
	}
	var s string
	_ = json.Unmarshal(content, &s)
	return s
}

func rawStr(m map[string]json.RawMessage, key string) (string, bool) {
	v, ok := m[key]
	if !ok {
		return "", false
	}
	var s string
	if json.Unmarshal(v, &s) != nil {
		return "", false
	}
	return s, true
}

func (a *BuildAdapter) runCmd(ctx context.Context, name string, args ...string) *exec.Cmd {
	if a.cfg.Exec != nil {
		return a.cfg.Exec(ctx, name, args...)
	}
	return exec.CommandContext(ctx, name, args...)
}

func (a *BuildAdapter) probeGrokVersion(exe string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := a.runCmd(ctx, exe, "-v")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	_ = cmd.Run()
	fields := strings.Fields(strings.TrimSpace(out.String()))
	for i, f := range fields {
		if strings.EqualFold(f, "grok") && i+1 < len(fields) {
			return fields[i+1]
		}
	}
	if len(fields) >= 2 {
		return fields[1]
	}
	return ""
}

func (a *BuildAdapter) probeGrokModels(exe string) []Model {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := a.runCmd(ctx, exe, "models")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	_ = cmd.Run()
	if errb.Len() > 0 && a.cfg.Logf != nil {
		a.cfg.Logf("cat build models stderr: %s", truncate(errb.String(), 500))
	}
	ids := parseGrokModelsList(out.String())
	if len(ids) == 0 {
		ids = append([]string(nil), defaultModelIDs...)
	}
	models := make([]Model, 0, len(ids))
	for _, id := range ids {
		models = append(models, Model{ID: id, DisplayName: modelDisplayName(id)})
	}
	return models
}

func parseGrokModelsList(out string) []string {
	var ids []string
	seen := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.TrimLeft(line, "*-•")
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "available") || strings.HasPrefix(lower, "default model") ||
			strings.HasPrefix(lower, "you are") || strings.Contains(lower, "not authenticated") {
			continue
		}
		id := line
		if i := strings.IndexAny(id, " \t("); i >= 0 {
			id = id[:i]
		}
		id = strings.TrimSpace(id)
		if id == "" || len(id) > 64 || seen[id] {
			continue
		}
		ok := true
		for _, r := range id {
			if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.') {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}

func modelDisplayName(id string) string {
	name := strings.TrimSpace(id)
	if name == "" {
		return "Cat 助手"
	}
	lower := strings.ToLower(name)
	switch {
	case strings.HasPrefix(lower, "grok-"):
		name = name[len("grok-"):]
	case strings.HasPrefix(lower, "grok "):
		name = name[len("grok "):]
	case lower == "grok":
		name = ""
	default:
		for _, old := range []string{"Grok", "GROK", "grok"} {
			name = strings.ReplaceAll(name, old, "")
		}
		name = strings.Join(strings.Fields(name), " ")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "Cat 助手"
	}
	return "Cat 助手 " + name
}

func hasPriorAssistant(msgs []WireMessage) bool {
	for _, m := range msgs {
		if m.Role == "assistant" && strings.TrimSpace(m.Content) != "" {
			return true
		}
	}
	return false
}

func sessionIDFromConv(convID string) string {
	sum := sha1.Sum([]byte("ffmpegfree/cat/session/" + convID))
	sum[6] = (sum[6] & 0x0f) | 0x50
	sum[8] = (sum[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}

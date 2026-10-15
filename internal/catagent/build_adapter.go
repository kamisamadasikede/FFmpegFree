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
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/id"
	"FFmpegFree/internal/paths"
	"FFmpegFree/internal/proc"
)

// 占位入口名（组件目录回退；正式路径以 PATH 上的 grok 为准，不回前端）。
const (
	entryNameUnix    = "cat-build"
	entryNameWindows = "cat-build.exe"
	grokEntryUnix    = "grok"
	grokEntryWindows = "grok.exe"
)

// DefaultBuildTimeout 是旧的一轮总时长上限；v0.53.1 起普通对话改用 DefaultIdleTimeout（无响应超时），此常量仅保留兼容。
const DefaultBuildTimeout = 10 * time.Minute

// 默认思考强度档位（CLI 不报告每档/默认档，只能写死；不预选、不标默认，未选时不传 --reasoning-effort）。
// 模型列表无写死回退：`grok models` 解析失败即空列表，发送时不传 -m（契约 v0.31.4 后续）。
var (
	defaultThinks = []ThinkLevel{
		{ID: "low", DisplayName: "低"},
		{ID: "medium", DisplayName: "中"},
		{ID: "high", DisplayName: "高"},
		// xhigh：CLI --reasoning-effort 文档列出的档位；模型不支持时 CLI 忽略（契约 v0.31.4 ④）。
		// 显示名「极高」（契约 v0.35 ④，产品 10-13 定）；值仍是 xhigh。
		{ID: "xhigh", DisplayName: "很高"}, // v0.44 6.19.18.5a
	}
)

// BuildConfig 配置 Build 适配器。
type BuildConfig struct {
	// ComponentDir 是组件父目录（…/components）；PATH 未命中时回退 …/cat/build/<version>/。
	ComponentDir string
	// DataRoot 是应用数据目录。无项目时 --cwd 用 paths.ResolveCatConvDir（与文件面板同一处）。
	DataRoot string
	DataTemp string // <数据目录>/tmp；DataRoot 为空时的旧回退
	// OnChange（v0.47.1 6.19.21.1）：检测状态变化时调用；适配器不再自己发 cat:status，由服务层发全量。
	OnChange func(agentKind string)
	Logf     func(format string, args ...any)
	// Exec 可覆盖（测试注入假二进制）。
	Exec func(ctx context.Context, name string, args ...string) *exec.Cmd
	// LookPath 可覆盖（测试注入）；默认 exec.LookPath。
	LookPath func(file string) (string, error)
	// Now 可覆盖。
	Now func() time.Time
	// SessionsRoot 可覆盖 CLI 会话目录（测试）。空 = ~/.grok/sessions。
	SessionsRoot string
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
	// v0.47.1：detecting = detect() 进行中；checkedAt = 最近一次检测完成（Unix 毫秒）
	detecting bool
	checkedAt int64
	detGen    uint64
	timedOut  bool // 本轮检测超过 DetectTimeout 仍未返回
	// provider 是 v0.38 自定义服务商叠加；仅影响子进程 env（List/Recheck 探测与 RunTurn 旁路字段配合）。
	provider ProviderOverlay
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

// SetProviderOverlay 供服务层在 List/Recheck 前写入自定义服务商（契约 6.19.12.8）。
// 从不调用 os.Setenv。
func (a *BuildAdapter) SetProviderOverlay(o ProviderOverlay) {
	a.mu.Lock()
	a.provider = o
	a.mu.Unlock()
}

func (a *BuildAdapter) providerOverlay() ProviderOverlay {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.provider
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
	if st.State != StateChecking && st.State != "" && a.detecting {
		a.checkedAt = a.cfg.Now().UnixMilli()
		a.detecting = false
		a.timedOut = false
	}
	a.mu.Unlock()
	if a.cfg.OnChange != nil {
		a.cfg.OnChange(KindCatBuild)
	}
}

// DetectTimedOut 报告当前这轮检测是否已超时仍未返回（v0.47.1）。
func (a *BuildAdapter) DetectTimedOut() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.timedOut
}

// CheckedAt 是最近一次实时检测完成的时间（Unix 毫秒）；本进程还没检测完过 = 0（v0.47.1）。
func (a *BuildAdapter) CheckedAt() int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.checkedAt
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
	if winFallbackEnabled() {
		return winFallback(KindCatBuild, "grok")
	}
	return ""
}

// detect 检测一次；超过 DetectTimeout（15 秒）未返回 → 标 timedOut 并通知（服务层显示 checking=false、stale=true），
// 不阻塞调用方；迟到结果照常生效。
func (a *BuildAdapter) detect() {
	a.mu.Lock()
	a.detecting = true
	a.detGen++
	gen := a.detGen
	a.mu.Unlock()
	a.setStatus(Status{State: StateChecking, CanDownload: false})
	done := make(chan struct{})
	go func() {
		defer close(done)
		a.detectBody(gen)
	}()
	t := time.NewTimer(DetectTimeout)
	defer t.Stop()
	select {
	case <-done:
	case <-t.C:
		// 契约 6.19.21.5：超时 → checking=false、stale=true、保留上次已知结果；迟到结果照常替换并清除 stale。
		a.mu.Lock()
		hit := a.detecting && a.detGen == gen
		if hit {
			a.timedOut = true
		}
		a.mu.Unlock()
		if hit && a.cfg.OnChange != nil {
			a.cfg.OnChange(KindCatBuild)
		}
	}
}

// setStatusGen 只在这一轮检测仍有效时生效。
func (a *BuildAdapter) setStatusGen(gen uint64, st Status) {
	a.mu.Lock()
	ok := a.detGen == gen
	a.mu.Unlock()
	if ok { // 只有更新的一轮 detect 开始后，旧一轮的结果才作废
		a.setStatus(st)
	}
}

func (a *BuildAdapter) detectBody(gen uint64) {

	if exe := a.resolveGrokExe(); exe != "" {
		ver := cachedCLIVersion(exe, a.probeGrokVersion)
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
		a.setStatusGen(gen, Status{State: StateReady, Version: ver, CanDownload: false})
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
		a.setStatusGen(gen, Status{
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
	a.setStatusGen(gen, Status{
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
	// C6：启用自定义时列表探测与 RunTurn 同一套 URL/Key → 重新探测。
	if o := a.providerOverlay(); o.Enabled {
		a.mu.Lock()
		exe := a.exe
		a.mu.Unlock()
		models := a.probeGrokModels(exe)
		a.mu.Lock()
		a.models = models
		out := append([]Model(nil), models...)
		a.mu.Unlock()
		if out == nil {
			out = []Model{}
		}
		return out, nil
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
	// Resume 为真时接上 CLI 已有会话（--resume）。不靠应用数据库里的历史判断。
	Resume  bool
	Timeout time.Duration
	// OnTextDelta 可选：适配器拿到真流式输出时逐段回调。
	// 一轮内只要回调过一次，服务端就以回调累积文本为准，不再拆整段回复。
	OnTextDelta func(delta string)
	// OnActivity 是思考 / 工具调用。不并入助手正文。replace 为真时 text 是这一条的完整标签，不是增量。
	OnActivity func(kind, id, text string, replace bool)
	// PermissionMode 是这一轮的批准方式：normal/default、plan、auto、approve。空等于全部批准。
	PermissionMode string
	// Files 是这一轮附上的本机文件。图片收成内容块，其它文件把路径写进文字。
	Files []PromptFile
	// Rules 是全局自定义指令（契约 v0.35 6.19.12.4）。非空时每一轮都带 --rules=<指令>，空则不带。
	Rules string
	// Provider 是 v0.38 自定义服务商；Enabled 时只注入子进程 env，不写父进程（6.19.12.8）。
	Provider ProviderOverlay
	// Skills retains shared-library paths for turn metadata; the service syncs native configs before launch.
	Skills SkillsOverlay
	// ToolEnv holds ephemeral capabilities of application-hosted tools.
	ToolEnv map[string]string
	// OnError 在失败原文出现时立刻回调（进程还没退出也会调）。同一句可能来自标准输出和标准错误，调用方自己去重。
	OnError func(msg string)
	// LiveSteer uses a bidirectional transport when the adapter supports it.
	LiveSteer    bool
	OnSteerReady func(SteerFunc)
}

// SteerFunc adds input to the active native turn. It must reject a stale turn.
type SteerFunc func(context.Context, string, []PromptFile) error

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

	cwd := strings.TrimSpace(opts.ProjectPath)
	if cwd == "" {
		var err error
		cwd, err = a.plainCwd(opts.ConversationID)
		if err != nil {
			return TurnResponse{}, err
		}
	}

	ctx := opts.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	runCtx, idle, cancel := newTurnContext(ctx, opts.Timeout, nil)
	defer cancel()

	// 纯文字写进临时文件再传 --prompt-file，避免以 - 开头的句子被当成开关。
	// 带了图片就改用 --prompt-json 内容块（data + mimeType）。进程结束后删掉临时文件。
	spec, err := preparePrompt(prompt, opts.Files)
	if err != nil {
		return TurnResponse{}, err
	}
	if spec.clean != nil {
		defer spec.clean()
	}

	args := []string{
		spec.flag, spec.value,
		"--output-format", "streaming-json",
		"--cwd", cwd,
		// --no-auto-update 不在 grok 1.0.40 真机 --help 里（隐藏开关），按「以真机 help 为准」不再传。
		"--no-alt-screen",
		// 不传 --tools / --disallowed-tools / --deny / --no-subagents / --disable-web-search，
		// 工具集用本机 grok 自己的完整集合。批准方式只带一个开关。
	}
	args = append(args, permissionCLIArgs(opts.PermissionMode)...)
	usageSID := ""
	if sid := strings.TrimSpace(opts.ConversationID); sid != "" {
		uuid := sessionIDFromConv(sid)
		usageSID = uuid
		if opts.Resume || hasPriorAssistant(opts.Messages) {
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
	args = append(args, rulesCLIArgs(opts.Rules)...)

	cmd := a.runCmd(runCtx, exe, args...)
	cmd.Dir = cwd
	cmd.Env = MergeToolEnv(MergeSkillsEnv(MergeProviderEnv(grokTurnEnv(os.Environ()), opts.Provider), opts.Skills), opts.ToolEnv)

	// 管道上的 stdout 经常要攒满一块才吐出来。会话文件是逐条追加的，有它就跟着文件走。
	var followMode int32
	updatesPath := sessionUpdatesPath(a.sessionRoot(), cwd, opts.ConversationID)
	var updatesOff int64
	if updatesPath != "" {
		if st, err := os.Stat(updatesPath); err == nil {
			updatesOff = st.Size()
		}
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return TurnResponse{}, apperr.Wrap(apperr.CatReplyFailed, MsgReplyFailed, err).WithDetail("reason=stdout_pipe")
	}
	var errMu sync.Mutex
	var cliErr string
	seenErr := map[string]struct{}{}
	noteErr := func(msg string) {
		msg = truncate(grokCLIMessage(strings.TrimSpace(msg)), 500)
		if msg == "" {
			return
		}
		errMu.Lock()
		if cliErr == "" {
			cliErr = msg
		}
		_, dup := seenErr[msg]
		if !dup {
			seenErr[msg] = struct{}{}
		}
		errMu.Unlock()
		if dup || opts.OnError == nil {
			return
		}
		opts.OnError(msg)
	}
	stderrTap := &stderrTap{onLine: func(line string) {
		if !strongCLIError(line) {
			return
		}
		noteErr(line)
	}}
	if idle != nil {
		cmd.Stderr = activityWriter{inner: stderrTap, w: idle}
	} else {
		cmd.Stderr = stderrTap
	}

	turnStart := a.cfg.Now()
	if err := proc.Start(cmd); err != nil {
		// exec.CommandContext 在 ctx 已取消时 Start 直接返回 ctx 错误：
		// 取消发生在启动前（高负载下常见）时应报“已取消”，而不是 start_failed。
		if ctx.Err() != nil {
			return TurnResponse{}, apperr.New(apperr.Canceled, "已取消")
		}
		return TurnResponse{}, apperr.Wrap(apperr.CatReplyFailed, MsgReplyFailed, err).WithDetail("reason=start_failed")
	}

	var acc strings.Builder
	emitPiece := func(p streamPiece) {
		idle.touch() // 跟会话文件走时 stdout 可能长时间不吐，这里同样算有响应
		switch p.Kind {
		case "thought", "tool":
			if opts.OnActivity != nil && p.Text != "" {
				opts.OnActivity(p.Kind, p.ID, p.Text, p.Replace)
			}
		case "retry":
			if opts.OnActivity != nil {
				opts.OnActivity(p.Kind, p.ID, p.Text, p.Replace)
			}
		default:
			if p.Text == "" {
				return
			}
			acc.WriteString(p.Text)
			if opts.OnTextDelta != nil {
				opts.OnTextDelta(p.Text)
			}
		}
	}
	stopFollow := make(chan struct{})
	followDone := make(chan struct{})
	go func() {
		defer close(followDone)
		followSessionUpdates(updatesPath, updatesOff, &followMode, stopFollow, emitPiece, noteErr)
	}()
	scanErr := scanStreamingJSON(activityReader{r: stdout, w: idle}, func(p streamPiece) {
		if !claimFollow(&followMode, followStdout) {
			return
		}
		emitPiece(p)
	}, noteErr)

	waitErr := cmd.Wait()
	stderrTap.flush()
	close(stopFollow)
	<-followDone
	stderrText := stderrTap.String()
	// v0.45：终态后取本轮 token（成功 / 失败 / 取消都试；取不到就不带）。
	gu := a.grokTurnTokens(exe, cmd.Env, cwd, usageSID, turnStart)
	usage := TurnResponse{TokenTotal: gu.Total, OutputTokens: gu.Output}
	errMu.Lock()
	cliMsg := cliErr
	errMu.Unlock()
	if a.cfg.Logf != nil && stderrText != "" {
		a.cfg.Logf("cat build turn stderr: %s", truncate(stderrText, 2000))
	}
	if scanErr != nil && a.cfg.Logf != nil {
		a.cfg.Logf("cat build stream scan: %v", scanErr)
	}

	strong, other := stderrSummary(stderrText)
	if cliMsg == "" {
		cliMsg = strong
	}
	if runCtx.Err() != nil {
		if ctx.Err() != nil {
			return usage, apperr.New(apperr.Canceled, "已取消")
		}
		if idleTimedOut(runCtx) {
			return usage, apperr.New(apperr.CatReplyFailed, MsgIdleTimeout).WithDetail(ReasonIdleTimeout)
		}
		msg := cliMsg
		if msg == "" {
			msg = "回复超时"
		}
		return usage, apperr.New(apperr.CatReplyFailed, msg).WithDetail("reason=timeout")
	}
	if waitErr != nil && cliArgRejected(stderrText+"\n"+cliMsg) {
		a.mu.Lock()
		ver := a.version
		a.mu.Unlock()
		return usage, outdatedCLIErr(KindCatBuild, ver, stderrText)
	}
	// 非零退出、流里的失败、或标准错误里的超时 / 断网，都按失败结束。已经流出的正文留在界面上。
	if cliMsg != "" || waitErr != nil {
		msg := cliMsg
		if msg == "" {
			msg = other
		}
		if msg == "" {
			msg = MsgReplyFailed
		}
		detail := "reason=cli_error"
		if waitErr != nil && cliMsg == "" {
			detail = "reason=exit"
		}
		return usage, apperr.New(apperr.CatReplyFailed, truncate(msg, 500)).WithDetail(detail)
	}

	body := acc.String()
	if strings.TrimSpace(body) == "" {
		return usage, apperr.New(apperr.CatReplyFailed, MsgReplyFailed).WithDetail("reason=empty_reply")
	}
	usage.Version = ProtocolVersion
	usage.Message = WireMessage{Role: "assistant", Content: body}
	return usage, nil
}

func grokCLIMessage(msg string) string {
	if strings.Contains(msg, "model catalog: auth refresh timed out") {
		return "Grok 登录凭证刷新超时。若任务未开始，请重新登录 Grok，或检查自定义服务商配置。"
	}
	if strings.Contains(msg, "prompt_ack_timeout") {
		return "Grok CLI 未在 120 秒内确认任务，已结束等待。请检查 CLI 登录状态；若登录正常，请新开会话后重试。"
	}
	return msg
}

// plainCwd 是没有项目的对话的 --cwd。有 DataRoot 时与文件面板共用 paths.ResolveCatConvDir；
// 没有 DataRoot 时仍落到 <DataTemp>/cat/cwd-<id>（旧测试与未配置数据目录）。
func (a *BuildAdapter) plainCwd(convID string) (string, error) {
	if root := strings.TrimSpace(a.cfg.DataRoot); root != "" {
		dir := paths.ResolveCatConvDir(root, convID)
		if dir == "" {
			return "", apperr.New(apperr.IOError, "无法创建对话文件夹")
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", apperr.Wrap(apperr.IOError, "无法创建对话文件夹", err)
		}
		return dir, nil
	}
	base := a.cfg.DataTemp
	if base == "" {
		base = os.TempDir()
	}
	cwd := filepath.Join(base, "cat", "cwd-"+safeID(convID))
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		return "", apperr.Wrap(apperr.IOError, "无法创建临时文件夹", err)
	}
	return cwd, nil
}

func writePromptFile(prompt string) (string, error) {
	f, err := os.CreateTemp("", "cat-prompt-*.txt")
	if err != nil {
		return "", err
	}
	path := f.Name()
	_, werr := io.WriteString(f, prompt)
	cerr := f.Close()
	if werr != nil || cerr != nil {
		os.Remove(path)
		if werr != nil {
			return "", werr
		}
		return "", cerr
	}
	return path, nil
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
	out := make([]string, 0, len(base)+1)
	for _, e := range base {
		if i := strings.IndexByte(e, '='); i > 0 && e[:i] == "GROK_DISABLE_AUTOUPDATER" {
			continue
		}
		out = append(out, e)
	}
	// 不改 GROK_SANDBOX / GROK_WRITE_FILE：写文件、跑命令、联网都留给 CLI 自己的工具。
	out = append(out, "GROK_DISABLE_AUTOUPDATER=1")
	return out
}

// permissionCLIArgs 把这一轮的模式变成一个开关（契约 6.19.18.4，H grok.txt）。
// default / acceptEdits / auto → --permission-mode <id>；always-approve → --always-approve。两者不一起传。
// 旧值读时映射（NormalizeMode）；空 / 不认识 → default（最保守）。
func permissionCLIArgs(mode string) []string {
	switch id := NormalizeMode(KindCatBuild, mode); id {
	case ModeGrokAlwaysApprove:
		return []string{"--always-approve"}
	default:
		return []string{"--permission-mode", id}
	}
}

// stderrTap 收下标准错误，并在完整的一行出现时通知。
type stderrTap struct {
	mu      sync.Mutex
	buf     strings.Builder
	partial string
	onLine  func(string)
}

func (w *stderrTap) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.buf.Write(p)
	w.partial += string(p)
	var lines []string
	for {
		i := strings.IndexByte(w.partial, '\n')
		if i < 0 {
			break
		}
		line := strings.TrimSpace(w.partial[:i])
		w.partial = w.partial[i+1:]
		if line != "" {
			lines = append(lines, line)
		}
	}
	fn := w.onLine
	w.mu.Unlock()
	if fn != nil {
		for _, line := range lines {
			fn(line)
		}
	}
	return len(p), nil
}

func (w *stderrTap) flush() {
	w.mu.Lock()
	line := strings.TrimSpace(w.partial)
	w.partial = ""
	fn := w.onLine
	w.mu.Unlock()
	if line != "" && fn != nil {
		fn(line)
	}
}

func (w *stderrTap) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

var strongCLIErrorRe = regexp.MustCompile(`(?i)(timed?\s*out|timeout|deadline exceeded|i/o timeout|connection refused|connection reset|no such host|network is unreachable|network unreachable|tls handshake|certificate|dial tcp|context deadline|超时|网络错误|网络超时)`)

func stderrNoise(line string) bool {
	s := strings.ToLower(line)
	for _, k := range []string{
		"new version available",
		"already up to date",
		"auto-update",
		"autoupdate",
		"updating grok",
		"update check",
	} {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

func strongCLIError(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" || stderrNoise(line) {
		return false
	}
	return strongCLIErrorRe.MatchString(line)
}

// stderrSummary 把标准错误分成「要显示的失败」和「退出失败时可以带上的其余原文」。更新提示不算失败。
func stderrSummary(stderr string) (strong, other string) {
	var strongLines, otherLines []string
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || stderrNoise(line) {
			continue
		}
		if strongCLIError(line) {
			strongLines = append(strongLines, line)
		} else {
			otherLines = append(otherLines, line)
		}
	}
	return truncate(strings.Join(strongLines, "\n"), 500), truncate(strings.Join(otherLines, "\n"), 500)
}

func resultIsError(raw map[string]json.RawMessage) bool {
	if b, ok := rawBool(raw, "is_error"); ok && b {
		return true
	}
	sub, ok := rawStr(raw, "subtype")
	if !ok {
		return false
	}
	s := strings.ToLower(sub)
	return s == "error" || strings.HasPrefix(s, "error_")
}

func resultErrorText(raw map[string]json.RawMessage) string {
	if lines := rawErrorList(raw, "errors"); len(lines) > 0 {
		return strings.Join(lines, "\n")
	}
	if msg, ok := rawStr(raw, "message"); ok && strings.TrimSpace(msg) != "" {
		return strings.TrimSpace(msg)
	}
	if msg, ok := rawStr(raw, "result"); ok && strings.TrimSpace(msg) != "" {
		return strings.TrimSpace(msg)
	}
	if sub, ok := rawStr(raw, "subtype"); ok && sub != "" {
		return sub
	}
	return MsgReplyFailed
}

func rawBool(m map[string]json.RawMessage, key string) (bool, bool) {
	v, ok := m[key]
	if !ok {
		return false, false
	}
	var b bool
	if json.Unmarshal(v, &b) != nil {
		return false, false
	}
	return b, true
}

func rawErrorList(m map[string]json.RawMessage, key string) []string {
	v, ok := m[key]
	if !ok {
		return nil
	}
	var strs []string
	if json.Unmarshal(v, &strs) == nil {
		return nonempty(strs)
	}
	var objs []map[string]string
	if json.Unmarshal(v, &objs) != nil {
		return nil
	}
	out := make([]string, 0, len(objs))
	for _, o := range objs {
		s := strings.TrimSpace(o["message"])
		if s == "" {
			s = strings.TrimSpace(o["error"])
		}
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func nonempty(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// streamEvent 是 grok --output-format streaming-json 的一行（ACP session update，或旧的 type/data）。
// text = 助手正文增量；thought = 思考；tool = 工具调用（replace 时 text 是完整标签）。error.message = 失败；end = 回合结束。
type streamEvent struct {
	Type    string `json:"type"`
	Data    string `json:"data"`
	Message string `json:"message"`
	Name    string `json:"name"`
	Title   string `json:"title"`
}

// streamPiece 是解析后交给界面的一段。Kind 为 text / thought / tool。
type streamPiece struct {
	Kind    string
	ID      string
	Text    string
	Replace bool
	// Done：本轮已正常结束（Codex turn.completed / Claude 非错误 result）。v0.48.1
	Done bool
}

func scanStreamingJSON(r io.Reader, onPiece func(streamPiece), onErr func(string)) error {
	sc := bufio.NewScanner(r)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 4*1024*1024)
	tools := map[string]*toolSnap{}
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		piece, errMsg, isErr := parseStreamLine(line, tools)
		if isErr {
			if onErr != nil && errMsg != "" {
				onErr(errMsg)
			}
			continue
		}
		if onPiece != nil && (piece.Kind == "retry" || piece.Text != "") {
			onPiece(piece)
		}
	}
	return sc.Err()
}

func parseStreamLine(line []byte, tools map[string]*toolSnap) (piece streamPiece, errMsg string, isErr bool) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(line, &raw); err != nil {
		return streamPiece{}, "", false
	}
	if typ, ok := rawStr(raw, "type"); ok && strings.EqualFold(typ, "error") {
		msg, _ := rawStr(raw, "message")
		if msg == "" {
			msg, _ = rawStr(raw, "data")
		}
		return streamPiece{}, msg, true
	}
	if typ, ok := rawStr(raw, "type"); ok && strings.EqualFold(typ, "result") && resultIsError(raw) {
		return streamPiece{}, resultErrorText(raw), true
	}
	if method, ok := rawStr(raw, "method"); ok && isSessionUpdateMethod(method) {
		if params, ok := raw["params"]; ok {
			if p, ok := parseSessionUpdate(params, tools); ok {
				return p, "", false
			}
		}
	}
	var ev streamEvent
	if json.Unmarshal(line, &ev) == nil {
		switch strings.ToLower(ev.Type) {
		case "text", "delta", "content", "message":
			if ev.Data != "" {
				return streamPiece{Kind: "text", Text: ev.Data}, "", false
			}
			if ev.Message != "" {
				return streamPiece{Kind: "text", Text: ev.Message}, "", false
			}
			if d, _ := rawStr(raw, "text"); d != "" {
				return streamPiece{Kind: "text", Text: d}, "", false
			}
			if d, _ := rawStr(raw, "delta"); d != "" {
				return streamPiece{Kind: "text", Text: d}, "", false
			}
		case "thought", "thinking":
			text := ev.Data
			if text == "" {
				text = ev.Message
			}
			return streamPiece{Kind: "thought", Text: text}, "", false
		case "tool", "tool_call", "tool_use", "tool_call_update":
			return applyToolLine(tools, line), "", false
		case "end":
			return streamPiece{}, "", false
		}
	}
	return streamPiece{}, "", false
}

func isSessionUpdateMethod(method string) bool {
	return method == "session/update" || method == "_x.ai/session/update" || method == "x.ai/session/update"
}

// retryStatusText 是超时重试时界面上的那一行。次数按会话事件里的 attempt / max_retries。
func retryStatusText(attempt, max int) string {
	if attempt > 0 && max > 0 {
		return fmt.Sprintf("正在重试（%d/%d）", attempt, max)
	}
	if attempt > 0 {
		return fmt.Sprintf("正在重试（第 %d 次）", attempt)
	}
	return "正在重试"
}

func parseRetryState(params json.RawMessage) (streamPiece, bool) {
	var p struct {
		Update struct {
			SessionUpdate string `json:"sessionUpdate"`
			Type          string `json:"type"`
			Attempt       int    `json:"attempt"`
			MaxRetries    int    `json:"max_retries"`
			MaxRetriesAlt int    `json:"maxRetries"`
		} `json:"update"`
	}
	if json.Unmarshal(params, &p) != nil || p.Update.SessionUpdate != "retry_state" {
		return streamPiece{}, false
	}
	typ := strings.ToLower(strings.TrimSpace(p.Update.Type))
	if typ != "retrying" && typ != "retry" {
		return streamPiece{Kind: "retry", ID: "retry", Replace: true}, true
	}
	max := p.Update.MaxRetries
	if max == 0 {
		max = p.Update.MaxRetriesAlt
	}
	return streamPiece{Kind: "retry", ID: "retry", Text: retryStatusText(p.Update.Attempt, max), Replace: true}, true
}

func parseSessionUpdate(params json.RawMessage, tools map[string]*toolSnap) (streamPiece, bool) {
	if piece, ok := parseRetryState(params); ok {
		return piece, true
	}
	var p struct {
		Update toolWire `json:"update"`
	}
	if json.Unmarshal(params, &p) != nil {
		return streamPiece{}, false
	}
	u := p.Update
	switch u.SessionUpdate {
	case "agent_message_chunk", "agent_message":
		return streamPiece{Kind: "text", Text: extractContentDelta(u.Content)}, true
	case "agent_thought_chunk", "agent_thought":
		return streamPiece{Kind: "thought", Text: extractContentDelta(u.Content)}, true
	case "tool_call", "tool_call_update":
		return applyTool(tools, u), true
	case "plan":
		var b strings.Builder
		for _, e := range u.Entries {
			line := strings.TrimSpace(e.Content)
			if line == "" {
				continue
			}
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(line)
		}
		if b.Len() == 0 {
			return streamPiece{}, false
		}
		return streamPiece{Kind: "thought", ID: "plan", Text: b.String(), Replace: true}, true
	default:
		if strings.Contains(u.SessionUpdate, "tool") {
			return applyTool(tools, u), true
		}
	}
	return streamPiece{}, false
}

// toolWire 是 streaming-json 的 tool_call / tool_call_update，也是 ACP session/update 里的同一组字段。
type toolWire struct {
	SessionUpdate string          `json:"sessionUpdate"`
	Type          string          `json:"type"`
	ToolCallID    string          `json:"toolCallId"`
	Title         string          `json:"title"`
	Kind          string          `json:"kind"`
	Status        string          `json:"status"`
	ToolName      string          `json:"toolName"`
	Name          string          `json:"name"`
	Data          string          `json:"data"`
	RawInput      json.RawMessage `json:"rawInput"`
	Locations     json.RawMessage `json:"locations"`
	Content       json.RawMessage `json:"content"`
	Entries       []struct {
		Content string `json:"content"`
	} `json:"entries"`
}

// toolSnap 按 toolCallId 合并同一次调用。后续 update 往往不再带名字，只补文件。
type toolSnap struct {
	title, toolName, kind, status, query string
	paths                                []string
	seen                                 map[string]struct{}
	body                                 string // 写入的全文，或改动的红绿对比
}

func applyToolLine(tools map[string]*toolSnap, line []byte) streamPiece {
	var w toolWire
	if json.Unmarshal(line, &w) != nil {
		return streamPiece{}
	}
	return applyTool(tools, w)
}

func applyTool(tools map[string]*toolSnap, w toolWire) streamPiece {
	id := strings.TrimSpace(w.ToolCallID)
	if id == "" {
		id = strings.TrimSpace(w.Name)
	}
	if id == "" {
		id = strings.TrimSpace(w.ToolName)
	}
	if id == "" {
		id = "tool"
	}
	s := tools[id]
	if s == nil {
		s = &toolSnap{seen: map[string]struct{}{}}
		tools[id] = s
	}
	if t := strings.TrimSpace(w.Title); t != "" {
		s.title = t
	}
	if t := strings.TrimSpace(w.ToolName); t != "" {
		s.toolName = t
	} else if s.toolName == "" && s.title == "" {
		if t := strings.TrimSpace(w.Name); t != "" {
			s.toolName = t
		}
	}
	if t := strings.TrimSpace(w.Kind); t != "" {
		s.kind = t
	}
	if s.title == "" && s.toolName == "" && s.kind == "" {
		if t := strings.TrimSpace(w.Data); t != "" && !strings.HasPrefix(t, "{") && !strings.Contains(t, "\n") {
			s.title = t
		}
	}
	if t := strings.TrimSpace(w.Status); t != "" {
		s.status = t
	}
	if q := inputQuery(w.RawInput); q != "" {
		s.query = q
	}
	for _, p := range collectToolPaths(w) {
		if _, ok := s.seen[p]; ok {
			continue
		}
		s.seen[p] = struct{}{}
		s.paths = append(s.paths, p)
		if len(s.paths) >= 40 {
			break
		}
	}
	if body := changeBody(w, s); body != "" {
		s.body = body
	}
	return streamPiece{Kind: "tool", ID: id, Text: s.text(), Replace: true}
}

func (s *toolSnap) text() string {
	label := strings.TrimSpace(s.title)
	if label == "" {
		label = strings.TrimSpace(s.toolName)
	}
	if label == "" {
		label = strings.TrimSpace(s.kind)
	}
	if label == "" {
		label = strings.TrimSpace(s.query)
	}
	if label == "" {
		label = "工具"
	}
	if strings.EqualFold(strings.TrimSpace(s.status), "failed") && !strings.Contains(label, "失败") {
		label += " · 失败"
	}
	if s.query != "" && s.query != label && !strings.Contains(label, s.query) {
		label += " · " + s.query
	}
	var b strings.Builder
	b.WriteString(label)
	for _, p := range s.paths {
		b.WriteByte('\n')
		b.WriteString(p)
	}
	if s.body != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(s.body)
	}
	return b.String()
}

// fileEditRe 识别写入和修改。读文件、列目录、跑命令不进来。
var fileEditRe = regexp.MustCompile(`(?i)(?:^|[^a-z])(?:search_replace|str_replace|apply_patch|patch|writes?|writing|wrote|edits?|edited|editing)(?:[^a-z]|$)`)

// changeBody 取出这次写入或修改的对比。新文件每行以 + 开头，删掉的以 - 开头，没变的以空格开头。
// 后来的更新没有对比时，保留已经拿到的内容。
func changeBody(w toolWire, s *toolSnap) string {
	if body := diffFromContent(w.Content); body != "" {
		return body
	}
	label := strings.Join([]string{s.title, s.toolName, s.kind, w.Title, w.ToolName, w.Kind}, " ")
	if !fileEditRe.MatchString(label) {
		return ""
	}
	raw := bytes.TrimSpace(w.RawInput)
	if len(raw) == 0 || raw[0] != '{' {
		return ""
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return ""
	}
	old := firstRawString(obj, "old_string", "oldString", "old_text", "oldText")
	neu := firstRawString(obj, "new_string", "newString", "new_text", "newText", "content", "contents")
	if old == "" && neu == "" {
		return ""
	}
	return capDiff(formatChange(old, neu))
}

func diffFromContent(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '[' {
		return ""
	}
	var items []struct {
		Type    string `json:"type"`
		OldText string `json:"oldText"`
		NewText string `json:"newText"`
	}
	if json.Unmarshal(raw, &items) != nil {
		return ""
	}
	var b strings.Builder
	n := 0
	for _, it := range items {
		if it.Type != "" && !strings.EqualFold(it.Type, "diff") {
			continue
		}
		if it.OldText == "" && it.NewText == "" {
			continue
		}
		piece := formatChange(it.OldText, it.NewText)
		if piece == "" {
			continue
		}
		if n > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(piece)
		n++
	}
	return capDiff(b.String())
}

func firstRawString(obj map[string]json.RawMessage, keys ...string) string {
	for _, k := range keys {
		if s, ok := rawStr(obj, k); ok && s != "" {
			return s
		}
	}
	return ""
}

const diffLineCap = 400

func capDiff(s string) string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= diffLineCap {
		return s
	}
	return strings.Join(lines[:diffLineCap], "\n") + "\n…其余已省略"
}

func prefixLines(mark, s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = mark + line
	}
	return strings.Join(lines, "\n")
}

func formatChange(old, neu string) string {
	old = strings.TrimRight(strings.ReplaceAll(old, "\r\n", "\n"), "\n")
	neu = strings.TrimRight(strings.ReplaceAll(neu, "\r\n", "\n"), "\n")
	if old == "" && neu == "" {
		return ""
	}
	if old == "" {
		return prefixLines("+", neu)
	}
	if neu == "" {
		return prefixLines("-", old)
	}
	a := strings.Split(old, "\n")
	b := strings.Split(neu, "\n")
	if len(a) > 250 || len(b) > 250 {
		return prefixLines("-", old) + "\n" + prefixLines("+", neu)
	}
	return renderLineDiff(a, b)
}

func renderLineDiff(a, b []string) string {
	n, m := len(a), len(b)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	var bld strings.Builder
	i, j := 0, 0
	first := true
	write := func(line string) {
		if !first {
			bld.WriteByte('\n')
		}
		first = false
		bld.WriteString(line)
	}
	for i < n && j < m {
		if a[i] == b[j] {
			write(" " + a[i])
			i++
			j++
			continue
		}
		if dp[i+1][j] >= dp[i][j+1] {
			write("-" + a[i])
			i++
			continue
		}
		write("+" + b[j])
		j++
	}
	for ; i < n; i++ {
		write("-" + a[i])
	}
	for ; j < m; j++ {
		write("+" + b[j])
	}
	return bld.String()
}

func collectToolPaths(w toolWire) []string {
	var out []string
	out = appendPathValues(out, w.Locations)
	out = appendInputPaths(out, w.RawInput)
	out = appendPathValues(out, w.Content)
	return out
}

func appendPathValues(dst []string, raw json.RawMessage) []string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return dst
	}
	var objs []struct {
		Path string `json:"path"`
	}
	if json.Unmarshal(raw, &objs) == nil {
		for _, o := range objs {
			if p := strings.TrimSpace(o.Path); p != "" {
				dst = append(dst, p)
			}
		}
		if len(objs) > 0 {
			return dst
		}
	}
	var strs []string
	if json.Unmarshal(raw, &strs) == nil {
		for _, p := range strs {
			if p = strings.TrimSpace(p); p != "" {
				dst = append(dst, p)
			}
		}
	}
	return dst
}

func appendInputPaths(dst []string, raw json.RawMessage) []string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" || raw[0] != '{' {
		return dst
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return dst
	}
	for _, key := range []string{"path", "file_path", "filePath", "target_file", "filename", "directory", "target_directory", "file"} {
		if p, ok := rawStr(obj, key); ok {
			if p = strings.TrimSpace(p); p != "" {
				dst = append(dst, p)
			}
		}
	}
	for _, key := range []string{"paths", "files", "target_files"} {
		v, ok := obj[key]
		if !ok {
			continue
		}
		dst = appendPathValues(dst, v)
	}
	return dst
}

func inputQuery(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" || raw[0] != '{' {
		return ""
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return ""
	}
	for _, key := range []string{"pattern", "query", "glob"} {
		if q, ok := rawStr(obj, key); ok {
			q = strings.TrimSpace(q)
			if q != "" {
				return q
			}
		}
	}
	return ""
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

// runCmd 是本包构造子进程的唯一入口（源码检查测试锁定）：
// proc.Configure 在 Windows 上隐藏窗口（HideWindow + CREATE_NO_WINDOW），
// 其它平台单独成组；取消 / 超时时结束整个进程树。
func (a *BuildAdapter) runCmd(ctx context.Context, name string, args ...string) *exec.Cmd {
	var cmd *exec.Cmd
	if a.cfg.Exec != nil {
		cmd = a.cfg.Exec(ctx, name, args...)
	} else {
		cmd = exec.CommandContext(ctx, name, args...)
	}
	proc.Configure(cmd)
	wrapShim(cmd, name, args) // v0.47.1：Windows .cmd / .ps1 经 cmd.exe / powershell 启动
	cmd.Cancel = func() error { return proc.Kill(cmd) }
	return cmd
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

// modelsProbeTimeout 是 `grok models` 探测超时（测试可改小）。
var modelsProbeTimeout = 30 * time.Second

func (a *BuildAdapter) probeGrokModels(exe string) []Model {
	ctx, cancel := context.WithTimeout(context.Background(), modelsProbeTimeout)
	defer cancel()
	cmd := a.runCmd(ctx, exe, "models")
	cmd.Env = MergeProviderEnv(grokTurnEnv(os.Environ()), a.providerOverlay())
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	_ = cmd.Run()
	if errb.Len() > 0 && a.cfg.Logf != nil {
		a.cfg.Logf("cat build models stderr: %s", truncate(errb.String(), 500))
	}
	if ctx.Err() != nil {
		// 超时：输出可能不完整，按失败处理 → 空列表（不传 -m，用 CLI 自己的默认模型）。
		if a.cfg.Logf != nil {
			a.cfg.Logf("cat build models probe timed out")
		}
		return []Model{}
	}
	// 未登录时 CLI 仍会列出模型，能解析到就照常用；解析不到（报错 / 输出不认识）→ 空列表。
	ids := parseGrokModelsList(out.String())
	models := make([]Model, 0, len(ids))
	for _, id := range ids {
		// 老板 10-09 定：选择器直接显示 CLI 的真实模型名，id 与 label 相同，不再映射。
		models = append(models, grokModelWithEfforts(id))
	}
	return models
}

// parseGrokModelsList 解析 `grok models` 的纯文本输出（CLI 1.0.40 实测）：
//
//	Default model: grok-4.6
//
//	Available models:
//	  * grok-4.6 (default)
//	  - grok-4.5
//
// 未登录时首行另有 "You are not authenticated."。默认模型（"(default)" 标记或
// "Default model:" 行）排在第一位，供前端作为默认选中。
func parseGrokModelsList(out string) []string {
	var ids []string
	seen := map[string]bool{}
	defaultID := ""
	inList := false
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if l := strings.ToLower(line); strings.HasPrefix(l, "default model:") {
			if f := strings.Fields(line[len("default model:"):]); len(f) > 0 {
				defaultID = f[0]
			}
			continue
		}
		markedDefault := strings.Contains(strings.ToLower(line), "(default)")
		if strings.HasPrefix(strings.ToLower(line), "available models") {
			inList = true
			continue
		}
		// 只认「Available models:」段内的行或带列表符号（* / - / •）的行；
		// 其它行（报错、提示）一律不当模型名，避免把 "Error" 之类解析成模型。
		bulleted := strings.HasPrefix(line, "*") || strings.HasPrefix(line, "-") || strings.HasPrefix(line, "•")
		if !inList && !bulleted {
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
		if markedDefault {
			defaultID = id
		}
	}
	if defaultID != "" {
		for i, id := range ids {
			if id == defaultID && i > 0 {
				ids = append([]string{id}, append(ids[:i:i], ids[i+1:]...)...)
				break
			}
		}
	}
	return ids
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

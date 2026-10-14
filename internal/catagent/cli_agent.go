package catagent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/paths"
	"FFmpegFree/internal/proc"
)

// CLIAgentConfig 配置 Claude / Codex 真适配器（契约 v0.37）。
type CLIAgentConfig struct {
	DataRoot string
	DataTemp string
	// OnChange（v0.47.1 6.19.21.1）：检测状态变化时调用；适配器不再自己发 cat:status，由服务层发全量。
	OnChange func(agentKind string)
	Logf     func(format string, args ...any)
	Exec     func(ctx context.Context, name string, args ...string) *exec.Cmd
	LookPath func(file string) (string, error)
	Now      func() time.Time
	// CodexHome 覆盖 Codex 的会话根（默认 $CODEX_HOME，再退回 ~/.codex）。测试用。
	CodexHome string
	// ClaudeConfigDir 覆盖 Claude 的配置根（默认 $CLAUDE_CONFIG_DIR，再退回 ~/.claude）。测试用。
	ClaudeConfigDir string
}

// cliKind 区分两家 CLI 的 argv / 流式协议（具体 flag 见附录 6.19.14，多为 GUESS）。
type cliKind int

const (
	cliKindClaude cliKind = iota
	cliKindCodex
)

// CLIAgentAdapter 是 claude_cli / codex_cli 共用骨架。
type CLIAgentAdapter struct {
	cfg  CLIAgentConfig
	kind cliKind
	name string // KindClaudeCLI / KindCodexCLI

	mu      sync.Mutex
	st      Status
	version string
	exe     string
	models  []Model
	thinks  []ThinkLevel
	checked bool
	// v0.47.1：detecting = detect() 进行中；checkedAt = 最近一次检测完成（Unix 毫秒）
	detecting bool
	checkedAt int64
	detGen    uint64
	timedOut  bool            // 本轮检测超过 DetectTimeout 仍未返回
	provider  ProviderOverlay // v0.38；仅子进程
}

// NewClaudeAdapter 创建 Claude Code 真适配器（不立即检测）。
func NewClaudeAdapter(cfg CLIAgentConfig) *CLIAgentAdapter {
	return newCLIAgent(cfg, cliKindClaude, KindClaudeCLI)
}

// NewCodexAdapter 创建 Codex CLI 真适配器（不立即检测）。
func NewCodexAdapter(cfg CLIAgentConfig) *CLIAgentAdapter {
	return newCLIAgent(cfg, cliKindCodex, KindCodexCLI)
}

func newCLIAgent(cfg CLIAgentConfig, kind cliKind, name string) *CLIAgentAdapter {
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.LookPath == nil {
		cfg.LookPath = exec.LookPath
	}
	a := &CLIAgentAdapter{cfg: cfg, kind: kind, name: name}
	a.st = Status{State: StateChecking, CanDownload: false}
	return a
}

func (a *CLIAgentAdapter) Kind() string         { return a.name }
func (a *CLIAgentAdapter) ProtocolVersion() int { return ProtocolVersion }

func (a *CLIAgentAdapter) ExecutablePath() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.exe
}

// SetProviderOverlay 供服务层在 List/Recheck 前写入自定义服务商。
func (a *CLIAgentAdapter) SetProviderOverlay(o ProviderOverlay) {
	a.mu.Lock()
	a.provider = o
	a.mu.Unlock()
}

func (a *CLIAgentAdapter) providerOverlay() ProviderOverlay {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.provider
}

// Start 后台检测一次。
func (a *CLIAgentAdapter) Start() { go a.detect() }

func (a *CLIAgentAdapter) Status() Status {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.st
}

func (a *CLIAgentAdapter) Recheck() Status {
	a.detect()
	return a.Status()
}

func (a *CLIAgentAdapter) setStatus(st Status) {
	a.mu.Lock()
	a.st = st
	if st.State != StateChecking && st.State != "" && a.detecting {
		a.checkedAt = a.cfg.Now().UnixMilli()
		a.detecting = false
		a.timedOut = false
	}
	a.mu.Unlock()
	if a.cfg.OnChange != nil {
		a.cfg.OnChange(a.name)
	}
}

// DetectTimedOut 报告当前这轮检测是否已超时仍未返回（v0.47.1）。
func (a *CLIAgentAdapter) DetectTimedOut() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.timedOut
}

// CheckedAt 是最近一次实时检测完成的时间（Unix 毫秒）；本进程还没检测完过 = 0（v0.47.1）。
func (a *CLIAgentAdapter) CheckedAt() int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.checkedAt
}

func (a *CLIAgentAdapter) markAuthInvalid() {
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

func (a *CLIAgentAdapter) lookPath(file string) (string, error) {
	if a.cfg.LookPath != nil {
		return a.cfg.LookPath(file)
	}
	return exec.LookPath(file)
}

// entryCandidates 是 LookPath 候选（附录 B1/B2 GUESS：claude / codex，Windows 可带 .exe）。
func (a *CLIAgentAdapter) entryCandidates() []string {
	unix, win := "claude", "claude.exe"
	if a.kind == cliKindCodex {
		unix, win = "codex", "codex.exe"
	}
	if runtime.GOOS == "windows" {
		return []string{win, unix}
	}
	return []string{unix}
}

func (a *CLIAgentAdapter) resolveExe() string {
	for _, name := range a.entryCandidates() {
		p, err := a.lookPath(name)
		if err == nil && p != "" {
			if fi, e := os.Stat(p); e == nil && !fi.IsDir() {
				return p
			}
			if a.cfg.Exec != nil {
				return p
			}
		}
	}
	if winFallbackEnabled() {
		base := "claude"
		if a.kind == cliKindCodex {
			base = "codex"
		}
		return winFallback(a.name, base)
	}
	return ""
}

// detect 检测一次；超过 DetectTimeout（15 秒）未返回 → 标 timedOut 并通知（服务层显示 checking=false、stale=true），
// 不阻塞调用方；迟到结果照常生效。
func (a *CLIAgentAdapter) detect() {
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
			a.cfg.OnChange(a.name)
		}
	}
}

// setStatusGen 只在这一轮检测仍有效时生效。
func (a *CLIAgentAdapter) setStatusGen(gen uint64, st Status) {
	a.mu.Lock()
	ok := a.detGen == gen
	a.mu.Unlock()
	if ok { // 只有更新的一轮 detect 开始后，旧一轮的结果才作废
		a.setStatus(st)
	}
}

func (a *CLIAgentAdapter) detectBody(gen uint64) {
	exe := a.resolveExe()
	if exe == "" {
		a.mu.Lock()
		a.exe, a.version, a.models, a.thinks, a.checked = "", "", nil, nil, true
		a.mu.Unlock()
		a.setStatusGen(gen, Status{
			State:       StateMissing,
			CanDownload: false,
			Error:       apperr.New(apperr.CatNotReady, MsgNotReady),
		})
		return
	}
	ver := cachedCLIVersion(exe, a.probeVersion)
	if ver == "" {
		ver = "path"
	}
	models := a.probeModels(exe)
	thinks := a.probeThinks(exe)
	a.mu.Lock()
	a.exe, a.version = exe, ver
	a.models, a.thinks = models, thinks
	a.checked = true
	a.mu.Unlock()
	a.setStatusGen(gen, Status{State: StateReady, Version: ver, CanDownload: false})
}

func (a *CLIAgentAdapter) ListModels() ([]Model, error) {
	st := a.Status()
	if st.State != StateReady {
		// 与桩一致：未就绪时给空列表，便于设置页隐藏 / 不报错。
		return []Model{}, nil
	}
	if o := a.providerOverlay(); o.Enabled {
		a.mu.Lock()
		exe := a.exe
		a.mu.Unlock()
		models := a.probeModels(exe)
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

func (a *CLIAgentAdapter) ListThinkLevels() ([]ThinkLevel, error) {
	st := a.Status()
	if st.State != StateReady {
		return []ThinkLevel{}, nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	out := append([]ThinkLevel(nil), a.thinks...)
	if out == nil {
		out = []ThinkLevel{}
	}
	return out, nil
}

// Inspect：两家暂无稳定只读检查命令 → unsupported（附录 B11/B12 GUESS）。
func (a *CLIAgentAdapter) Inspect(context.Context, string) InspectResult {
	return EmptyInspect(InspectUnsupported)
}

// RunTurn 调对应 CLI headless，流式进 OnTextDelta；取消杀进程树。
func (a *CLIAgentAdapter) RunTurn(opts TurnOptions) (TurnResponse, error) {
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

	a.mu.Lock()
	curVer := a.version
	a.mu.Unlock()
	if a.kind == cliKindClaude && cliVersionBelow(curVer, claudeMinVersion) {
		return TurnResponse{}, outdatedCLIErr(a.name, curVer, "")
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

	// service 传的 Resume 来自 Grok 的会话目录，对 Codex / Claude 恒为 false。
	// 两家都以我们存下的会话 id 为准（Codex thread_id；Claude stream-json 的 session_id，v0.45 §6.19.19.1）：
	// 有就续，否则开新会话。
	sessionID := a.loadSessionID(opts.ConversationID)
	resume := sessionID != ""

	args := a.buildTurnArgs(opts, prompt, cwd, sessionID, resume)
	var stdinText string
	viaStdin := isShimExt(exe)
	if viaStdin {
		// v0.47.1 6.19.21.4：.cmd / .ps1 包装不经命令行传提示词 / 规则。
		var clean func()
		var serr error
		args, stdinText, clean, serr = shimStdinArgs(a.kind, args, a.cfg.DataTemp)
		if serr != nil {
			return TurnResponse{}, apperr.Wrap(apperr.CatReplyFailed, MsgReplyFailed, serr).WithDetail("reason=prompt_file")
		}
		if clean != nil {
			defer clean()
		}
	}
	cmd := a.runCmd(runCtx, exe, args...)
	cmd.Dir = cwd
	if viaStdin {
		cmd.Stdin = strings.NewReader(stdinText)
	}
	cmd.Env = MergeSkillsEnv(MergeProviderEnv(os.Environ(), opts.Provider), opts.Skills)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return TurnResponse{}, apperr.Wrap(apperr.CatReplyFailed, MsgReplyFailed, err).WithDetail("reason=stdout_pipe")
	}
	var errMu sync.Mutex
	var cliErr string
	seenErr := map[string]struct{}{}
	var sessionGone bool
	noteErr := func(msg string) {
		msg = truncate(strings.TrimSpace(msg), 500)
		if msg == "" {
			return
		}
		if a.kind == cliKindClaude && resume && claudeSessionMissing(msg) {
			// 原文带 CLI 的会话编号，不上屏；换成应用自己的话。
			msg = MsgSessionGone
			errMu.Lock()
			sessionGone = true
			errMu.Unlock()
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
	cmd.Stderr = stderrTap

	if err := cmd.Start(); err != nil {
		// exec.CommandContext 在 ctx 已取消时 Start 直接返回 ctx 错误：
		// 取消发生在启动前（高负载下常见）时应报“已取消”，而不是 start_failed。
		if ctx.Err() != nil {
			return TurnResponse{}, apperr.New(apperr.Canceled, "已取消")
		}
		return TurnResponse{}, apperr.Wrap(apperr.CatReplyFailed, MsgReplyFailed, err).WithDetail("reason=start_failed")
	}

	var acc strings.Builder
	var gotSession string
	var resultFallback string
	var usageTokens, usageDur int64
	scanErr := a.scanCLIStream(stdout, func(p streamPiece) {
		switch p.Kind {
		case "thought", "tool", "retry":
			if opts.OnActivity != nil && (p.Kind == "retry" || p.Text != "") {
				opts.OnActivity(p.Kind, p.ID, p.Text, p.Replace)
			}
		case "result_text":
			// Claude 最终 result 全文：仅作无增量时的兜底，不与 partial 叠加。
			resultFallback = p.Text
		case "usage":
			// ID 复用为 tokenTotal；Replace 无关。Text 未用。
			if n, err := parseInt64(p.ID); err == nil && n > 0 {
				usageTokens = n
			}
			if n, err := parseInt64(p.Text); err == nil && n > 0 {
				usageDur = n
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
	}, noteErr, func(sid string) {
		if strings.TrimSpace(sid) != "" {
			gotSession = strings.TrimSpace(sid)
		}
	})

	waitErr := cmd.Wait()
	stderrTap.flush()
	stderrText := stderrTap.String()
	errMu.Lock()
	cliMsg := cliErr
	errMu.Unlock()
	if a.cfg.Logf != nil && stderrText != "" {
		a.cfg.Logf("cat %s turn stderr: %s", a.name, truncate(stderrText, 2000))
	}
	if scanErr != nil && a.cfg.Logf != nil {
		a.cfg.Logf("cat %s stream scan: %v", a.name, scanErr)
	}
	if a.kind == cliKindClaude && resume && claudeSessionMissing(stderrText) {
		errMu.Lock()
		sessionGone = true
		errMu.Unlock()
	}
	errMu.Lock()
	gone := sessionGone
	errMu.Unlock()
	if gone {
		// v0.45：会话已被 CLI 清掉 → 清掉存下的 id，下一句开新会话；本轮不可重试。
		a.clearSessionID(opts.ConversationID)
	} else if gotSession != "" {
		a.saveSessionID(opts.ConversationID, gotSession)
	}

	strong, other := stderrSummary(stderrText)
	if cliMsg == "" {
		cliMsg = strong
	}
	authText := cliMsg + "\n" + stderrText
	if waitErr != nil {
		authText += "\n" + waitErr.Error()
	}
	usageResp := TurnResponse{TokenTotal: usageTokens, DurationMs: usageDur}
	if isAuthFailure(authText) {
		a.markAuthInvalid()
		return usageResp, authNotReadyErr()
	}
	if runCtx.Err() != nil {
		if ctx.Err() != nil {
			return usageResp, apperr.New(apperr.Canceled, "已取消")
		}
		msg := cliMsg
		if msg == "" {
			msg = "回复超时"
		}
		return usageResp, apperr.New(apperr.CatReplyFailed, msg).WithDetail("reason=timeout")
	}
	if gone {
		return usageResp, apperr.New(apperr.CatReplyFailed, MsgSessionGone).WithDetail(ReasonSessionNotFound)
	}
	if waitErr != nil && cliArgRejected(stderrText+"\n"+cliMsg) {
		return usageResp, outdatedCLIErr(a.name, curVer, stderrText)
	}
	if a.kind == cliKindCodex && opts.Provider.Enabled && codexResponsesMissing(cliMsg+"\n"+stderrText) {
		// 契约 6.19.20.5：服务商没有 /responses → 固定文案，不回传 CLI 原文（里面有地址）。
		return usageResp, providerResponsesUnsupportedErr(KindCodexCLI)
	}
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
		return usageResp, apperr.New(apperr.CatReplyFailed, truncate(msg, 500)).WithDetail(detail)
	}
	body := acc.String()
	if strings.TrimSpace(body) == "" && strings.TrimSpace(resultFallback) != "" {
		body = resultFallback
		if opts.OnTextDelta != nil {
			opts.OnTextDelta(body)
		}
	}
	if strings.TrimSpace(body) == "" {
		return TurnResponse{}, apperr.New(apperr.CatReplyFailed, MsgReplyFailed).WithDetail("reason=empty_reply")
	}
	usageResp.Version = ProtocolVersion
	usageResp.Message = WireMessage{Role: "assistant", Content: body}
	return usageResp, nil
}

func (a *CLIAgentAdapter) plainCwd(convID string) (string, error) {
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

func (a *CLIAgentAdapter) sessionPath(convID string) string {
	root := strings.TrimSpace(a.cfg.DataRoot)
	if root == "" {
		root = strings.TrimSpace(a.cfg.DataTemp)
	}
	if root == "" {
		root = os.TempDir()
	}
	return filepath.Join(root, "cat", "cli-sessions", a.name, safeID(convID))
}

func (a *CLIAgentAdapter) loadSessionID(convID string) string {
	p := a.sessionPath(convID)
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func (a *CLIAgentAdapter) clearSessionID(convID string) {
	if strings.TrimSpace(convID) == "" {
		return
	}
	_ = os.Remove(a.sessionPath(convID))
}

// claudeSessionMissing：续聊时 CLI 报会话不存在。真机 claude 2.1.296：
// `claude -p x --resume <不存在的 id>` → stderr「No conversation found with session ID: <id>」、
// result.errors 同句、退出码 1（2026-10-14 box 采集）。
func claudeSessionMissing(text string) bool {
	return strings.Contains(text, "No conversation found with session ID")
}

func (a *CLIAgentAdapter) saveSessionID(convID, sid string) {
	sid = strings.TrimSpace(sid)
	if convID == "" || sid == "" {
		return
	}
	p := a.sessionPath(convID)
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	_ = os.WriteFile(p, []byte(sid), 0o600)
}

// runCmd 是本包构造子进程的唯一入口之一（源码检查锁定函数名 runCmd）。
func (a *CLIAgentAdapter) runCmd(ctx context.Context, name string, args ...string) *exec.Cmd {
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

func (a *CLIAgentAdapter) probeVersion(exe string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var args []string
	switch a.kind {
	case cliKindCodex:
		args = []string{"--version"}
	default:
		args = []string{"--version"}
	}
	cmd := a.runCmd(ctx, exe, args...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	_ = cmd.Run()
	line := strings.TrimSpace(out.String())
	if line == "" {
		return ""
	}
	// 只取首行、截短，避免把整段 help 塞进 version。
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	fields := strings.Fields(line)
	for _, f := range fields {
		if strings.ContainsAny(f, "0123456789") && !strings.HasPrefix(f, "-") {
			return truncate(f, 64)
		}
	}
	if len(fields) > 0 {
		return truncate(fields[len(fields)-1], 64)
	}
	return ""
}

func (a *CLIAgentAdapter) probeModels(exe string) []Model {
	switch a.kind {
	case cliKindCodex:
		return a.probeCodexModels(exe)
	default:
		// Claude Code 2.1.296 没有列模型的命令 → 官方文档静态表（claude_models.go）。
		return claudeStaticModels()
	}
}

func (a *CLIAgentAdapter) probeThinks(exe string) []ThinkLevel {
	_ = exe
	// 两家均无稳定「列思考强度」命令（附录 B5/B6 GUESS）→ 空列表，UI 整行隐藏。
	return []ThinkLevel{}
}

func (a *CLIAgentAdapter) probeCodexModels(exe string) []Model {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	// GUESS B6：codex debug models [--bundled] 输出 JSON 目录。
	env := MergeProviderEnv(os.Environ(), a.providerOverlay())
	cmd := a.runCmd(ctx, exe, "debug", "models", "--bundled")
	cmd.Env = env
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil || out.Len() == 0 {
		cmd2 := a.runCmd(ctx, exe, "debug", "models")
		cmd2.Env = env
		out.Reset()
		cmd2.Stdout, cmd2.Stderr = &out, &out
		if err2 := cmd2.Run(); err2 != nil || out.Len() == 0 {
			return []Model{}
		}
	}
	return parseCodexModelsJSON(out.Bytes())
}

// parseCodexModelsJSON 解析 `codex debug models` 目录（0.162.1：{"models":[{slug, visibility,
// default_reasoning_level, supported_reasoning_levels:[{effort}]}]}，快照 testdata/models/）。
// visibility=hide 的模型不下发（codex 自己的选择器也不列）；档位取 supported_reasoning_levels，
// 默认取 default_reasoning_level（不在列表里则为空）。
func parseCodexModelsJSON(raw []byte) []Model {
	raw = bytes.TrimSpace(raw)
	out := []Model{}
	if len(raw) == 0 {
		return out
	}
	var items []map[string]json.RawMessage
	if json.Unmarshal(raw, &items) != nil {
		var obj map[string]json.RawMessage
		if json.Unmarshal(raw, &obj) != nil {
			return out
		}
		for _, key := range []string{"models", "data", "items"} {
			if v, ok := obj[key]; ok {
				_ = json.Unmarshal(v, &items)
				break
			}
		}
	}
	seen := map[string]struct{}{}
	for _, it := range items {
		var m map[string]any
		b, _ := json.Marshal(it)
		_ = json.Unmarshal(b, &m)
		id := modelIDFromMap(m)
		if id == "" {
			continue
		}
		if v, _ := m["visibility"].(string); strings.EqualFold(v, "hide") {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		var lv struct {
			Default string `json:"default_reasoning_level"`
			Levels  []struct {
				Effort string `json:"effort"`
			} `json:"supported_reasoning_levels"`
		}
		_ = json.Unmarshal(b, &lv)
		model := Model{ID: id, DisplayName: id, Efforts: []string{}}
		for _, l := range lv.Levels {
			if e := strings.TrimSpace(l.Effort); e != "" {
				model.Efforts = append(model.Efforts, e)
			}
		}
		if ModelSupportsEffort(model, lv.Default) {
			model.DefaultEffort = lv.Default
		}
		out = append(out, model)
	}
	return out
}

func modelIDFromMap(m map[string]any) string {
	for _, k := range []string{"id", "slug", "name", "model", "model_id"} {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

// buildTurnArgs 按家拼 argv（A5/A6 / B7/B8 标 GUESS，不进用户可见文案）。
func (a *CLIAgentAdapter) buildTurnArgs(opts TurnOptions, prompt, cwd, sessionID string, resume bool) []string {
	switch a.kind {
	case cliKindCodex:
		return a.buildCodexArgs(opts, prompt, cwd, sessionID, resume)
	default:
		return a.buildClaudeArgs(opts, prompt, sessionID, resume)
	}
}

func (a *CLIAgentAdapter) buildClaudeArgs(opts TurnOptions, prompt, sessionID string, resume bool) []string {
	// GUESS B7：claude -p … --output-format stream-json --verbose --include-partial-messages
	args := []string{
		"-p", prompt,
		"--output-format", "stream-json",
		"--verbose",
		"--include-partial-messages",
	}
	a.mu.Lock()
	ver := a.version
	a.mu.Unlock()
	args = append(args, claudePermissionArgs(opts.PermissionMode, ver)...)
	if e := strings.TrimSpace(opts.ThinkLevelID); e != "" {
		// H claude.txt `--effort <level>`；D https://code.claude.com/docs/en/model-config（6.19.18.4）
		args = append(args, "--effort", e)
	}
	if rules := strings.TrimSpace(opts.Rules); rules != "" {
		// GUESS A6：--append-system-prompt（官方文档有；真机确认前仍标 GUESS）
		args = append(args, "--append-system-prompt", rules)
	}
	if mid := strings.TrimSpace(opts.ModelID); mid != "" && mid != "default" {
		args = append(args, "--model", mid)
	}
	if resume && sessionID != "" {
		// H testdata/help/claude.txt 2.1.296「-r, --resume [value]  Resume a conversation by session ID」；
		// D https://code.claude.com/docs/en/sessions。真机：不存在的 id → 见 claudeSessionMissing。
		args = append(args, "--resume", sessionID)
	}
	return args
}

// claudePermissionArgs：模式 id → --permission-mode（H claude.txt choices；契约 6.19.18.4）。
// manual 在 1.0.109–2.1.199 发 default（CHANGELOG 2.1.200）；版本未知按新版。
func claudePermissionArgs(mode, ver string) []string {
	id := NormalizeMode(KindClaudeCLI, mode)
	if id == ModeClaudeManual && cliVersionBelow(ver, claudeManualSince) {
		id = "default"
	}
	return []string{"--permission-mode", id}
}

func (a *CLIAgentAdapter) buildCodexArgs(opts TurnOptions, prompt, cwd, sessionID string, resume bool) []string {
	// GUESS B8：codex exec --json …
	// A6：无稳定「自定义指令」flag → 前缀拼进 prompt（契约允许；不捏造 flag 名）
	if rules := strings.TrimSpace(opts.Rules); rules != "" {
		prompt = rules + "\n\n" + prompt
	}
	// 真机 help 核对（codex-cli 0.162.1）：exec / exec resume 都不认 --ask-for-approval / -a；
	// exec resume 也不认 --sandbox / -C（cwd 靠子进程工作目录）。
	if resume && sessionID != "" {
		// codex exec resume [OPTIONS] <SESSION_ID> [PROMPT]
		args := []string{"exec", "resume", "--json", "--skip-git-repo-check"}
		args = append(args, opts.Provider.ExtraArgs()...)
		args = append(args, codexPermissionArgs(opts.PermissionMode, true)...)
		args = append(args, codexEffortArgs(opts.ThinkLevelID)...)
		if mid := strings.TrimSpace(opts.ModelID); mid != "" && mid != "default" {
			args = append(args, "-m", mid)
		}
		return append(args, sessionID, prompt)
	}
	args := []string{"exec", "--json", "--skip-git-repo-check"}
	args = append(args, opts.Provider.ExtraArgs()...)
	if cwd != "" {
		args = append(args, "-C", cwd)
	}
	args = append(args, codexPermissionArgs(opts.PermissionMode, false)...)
	args = append(args, codexEffortArgs(opts.ThinkLevelID)...)
	if mid := strings.TrimSpace(opts.ModelID); mid != "" && mid != "default" {
		args = append(args, "-m", mid)
	}
	return append(args, prompt)
}

// codexPermissionArgs：模式 id → codex 参数（契约 6.19.18.4；H codex-exec.txt / codex-exec-resume.txt 0.162.1；
// D https://developers.openai.com/codex/config-reference）。
// headless 不能弹批准 → 沙箱模式一律 approval_policy=never。exec resume 不认 --sandbox / --approve-for-me，
// 改用 -c sandbox_mode=…；approve-for-me 续聊时退回 workspace-write（界面不提示）。
func codexPermissionArgs(mode string, resume bool) []string {
	var sandbox string
	switch NormalizeMode(KindCodexCLI, mode) {
	case ModeCodexBypass:
		return []string{"--dangerously-bypass-approvals-and-sandbox"}
	case ModeCodexApproveForMe:
		if !resume {
			return []string{"--approve-for-me"}
		}
		sandbox = "workspace-write"
	case ModeCodexWorkspace:
		sandbox = "workspace-write"
	default:
		sandbox = "read-only"
	}
	if resume {
		return []string{"-c", `sandbox_mode="` + sandbox + `"`, "-c", `approval_policy="never"`}
	}
	return []string{"--sandbox", sandbox, "-c", `approval_policy="never"`}
}

// codexEffortArgs：-c model_reasoning_effort="<id>"（仅选了才传；D config-reference）。
// id 由服务层按模型 efforts 过滤过；这里再做格式校验，防止拼进 TOML 的值带引号。
func codexEffortArgs(effort string) []string {
	e := strings.TrimSpace(effort)
	if e == "" || !EffortFormatOK(e) {
		return nil
	}
	return []string{"-c", `model_reasoning_effort="` + e + `"`}
}

func (a *CLIAgentAdapter) scanCLIStream(r io.Reader, onPiece func(streamPiece), onErr func(string), onSession func(string)) error {
	sc := bufio.NewScanner(r)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 4*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		if a.kind == cliKindClaude {
			if handled := emitClaudeResultLine(line, onPiece, onErr, onSession); handled {
				continue
			}
		}
		piece, sid, errMsg, isErr := a.parseCLIStreamLine(line)
		if sid != "" && onSession != nil {
			onSession(sid)
		}
		if isErr {
			if onErr != nil && errMsg != "" {
				onErr(errMsg)
			}
			continue
		}
		if onPiece != nil && (piece.Kind == "retry" || piece.Kind == "usage" || piece.Kind == "result_text" || piece.Text != "") {
			onPiece(piece)
		}
	}
	return sc.Err()
}

// emitClaudeResultLine 处理 type=result：可同时产出 usage 与 result_text。
func emitClaudeResultLine(line []byte, onPiece func(streamPiece), onErr func(string), onSession func(string)) bool {
	var raw map[string]json.RawMessage
	if json.Unmarshal(line, &raw) != nil {
		return false
	}
	typ, _ := rawStr(raw, "type")
	if !strings.EqualFold(typ, "result") {
		return false
	}
	if s, ok := rawStr(raw, "session_id"); ok && s != "" && onSession != nil {
		onSession(s)
	}
	if resultIsError(raw) {
		if onErr != nil {
			onErr(resultErrorText(raw))
		}
		return true
	}
	if u := claudeUsageTotal(raw); u > 0 && onPiece != nil {
		onPiece(streamPiece{Kind: "usage", ID: fmtInt64(u), Text: fmtInt64(claudeDurationMs(raw))})
	}
	if t, ok := rawStr(raw, "result"); ok && strings.TrimSpace(t) != "" && onPiece != nil {
		onPiece(streamPiece{Kind: "result_text", Text: t})
	}
	return true
}

func (a *CLIAgentAdapter) parseCLIStreamLine(line []byte) (piece streamPiece, sessionID, errMsg string, isErr bool) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(line, &raw); err != nil {
		return streamPiece{}, "", "", false
	}
	typ, _ := rawStr(raw, "type")
	typ = strings.ToLower(typ)

	switch a.kind {
	case cliKindCodex:
		return parseCodexStreamLine(raw, typ)
	default:
		return parseClaudeStreamLine(raw, typ)
	}
}

func parseClaudeStreamLine(raw map[string]json.RawMessage, typ string) (streamPiece, string, string, bool) {
	sid := ""
	if s, ok := rawStr(raw, "session_id"); ok {
		sid = s
	}
	switch typ {
	case "error":
		msg, _ := rawStr(raw, "message")
		if msg == "" {
			msg, _ = rawStr(raw, "error")
		}
		return streamPiece{}, sid, msg, true
	case "result":
		if resultIsError(raw) {
			return streamPiece{}, sid, resultErrorText(raw), true
		}
		// usage 与全文拆成两次由上层处理：先 usage 再 result_text（同线只能回一条）。
		// 这里优先回 usage；全文在无 partial 时由上层用 result_text 兜底——见 parseClaudeResult。
		if piece, ok := parseClaudeResult(raw, sid); ok {
			return piece, sid, "", false
		}
		return streamPiece{}, sid, "", false
	case "stream_event":
		if text := claudeStreamEventText(raw["event"]); text != "" {
			return streamPiece{Kind: "text", Text: text}, sid, "", false
		}
		return streamPiece{}, sid, "", false
	case "assistant":
		// 有 --include-partial-messages 时 assistant 全文会与 delta 重复，忽略。
		return streamPiece{}, sid, "", false
	case "content_block_delta", "text_delta":
		if d, ok := rawStr(raw, "text"); ok && d != "" {
			return streamPiece{Kind: "text", Text: d}, sid, "", false
		}
		if d := claudeDeltaText(raw["delta"]); d != "" {
			return streamPiece{Kind: "text", Text: d}, sid, "", false
		}
	}
	return streamPiece{}, sid, "", false
}

func claudeStreamEventText(event json.RawMessage) string {
	if len(event) == 0 {
		return ""
	}
	var ev map[string]json.RawMessage
	if json.Unmarshal(event, &ev) != nil {
		return ""
	}
	if d := claudeDeltaText(ev["delta"]); d != "" {
		return d
	}
	// event.delta.type == text_delta nested under event
	if nested, ok := ev["event"]; ok {
		return claudeStreamEventText(nested)
	}
	return ""
}

func claudeDeltaText(delta json.RawMessage) string {
	if len(delta) == 0 {
		return ""
	}
	var d map[string]json.RawMessage
	if json.Unmarshal(delta, &d) != nil {
		var s string
		if json.Unmarshal(delta, &s) == nil {
			return s
		}
		return ""
	}
	typ, _ := rawStr(d, "type")
	if strings.EqualFold(typ, "text_delta") || typ == "" {
		if t, ok := rawStr(d, "text"); ok {
			return t
		}
	}
	return ""
}

func claudeAssistantText(raw map[string]json.RawMessage) string {
	msgRaw, ok := raw["message"]
	if !ok {
		msgRaw = raw["content"]
	}
	if len(msgRaw) == 0 {
		return ""
	}
	var msg map[string]json.RawMessage
	if json.Unmarshal(msgRaw, &msg) == nil {
		if c, ok := msg["content"]; ok {
			return claudeContentBlocksText(c)
		}
	}
	return claudeContentBlocksText(msgRaw)
}

func claudeContentBlocksText(raw json.RawMessage) string {
	var blocks []map[string]any
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var b strings.Builder
	for _, bl := range blocks {
		typ, _ := bl["type"].(string)
		if typ != "" && !strings.EqualFold(typ, "text") {
			continue
		}
		if t, ok := bl["text"].(string); ok {
			b.WriteString(t)
		}
	}
	return b.String()
}

func parseCodexStreamLine(raw map[string]json.RawMessage, typ string) (streamPiece, string, string, bool) {
	sid := ""
	if s, ok := rawStr(raw, "thread_id"); ok {
		sid = s
	}
	switch typ {
	case "error":
		msg, _ := rawStr(raw, "message")
		if msg == "" {
			msg, _ = rawStr(raw, "error")
		}
		return streamPiece{}, sid, msg, true
	case "turn.failed":
		msg, _ := rawStr(raw, "error")
		if msg == "" {
			msg, _ = rawStr(raw, "message")
		}
		if msg == "" {
			msg = MsgReplyFailed
		}
		return streamPiece{}, sid, msg, true
	case "thread.started":
		return streamPiece{}, sid, "", false
	case "turn.completed":
		if piece, ok := parseCodexUsage(raw); ok {
			return piece, sid, "", false
		}
		return streamPiece{}, sid, "", false
	case "item.completed", "item.started", "item.updated":
		itemRaw, ok := raw["item"]
		if !ok {
			return streamPiece{}, sid, "", false
		}
		var item map[string]json.RawMessage
		if json.Unmarshal(itemRaw, &item) != nil {
			return streamPiece{}, sid, "", false
		}
		itemType, _ := rawStr(item, "type")
		switch strings.ToLower(itemType) {
		case "agent_message", "message", "agent-message":
			if typ == "item.completed" || typ == "item.updated" {
				if t, ok := rawStr(item, "text"); ok && t != "" {
					return streamPiece{Kind: "text", Text: t}, sid, "", false
				}
			}
		case "reasoning", "agent_reasoning":
			if t, ok := rawStr(item, "text"); ok && t != "" {
				return streamPiece{Kind: "thought", Text: t}, sid, "", false
			}
		case "command_execution", "file_change", "mcp_tool_call", "web_search":
			title, _ := rawStr(item, "command")
			if title == "" {
				title, _ = rawStr(item, "text")
			}
			if title == "" {
				title = itemType
			}
			return streamPiece{Kind: "tool", ID: firstNonEmpty(rawStrVal(item, "id"), itemType), Text: truncate(title, 200)}, sid, "", false
		}
	}
	return streamPiece{}, sid, "", false
}

func rawStrVal(m map[string]json.RawMessage, key string) string {
	s, _ := rawStr(m, key)
	return s
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func parseInt64(s string) (int64, error) {
	return strconv.ParseInt(strings.TrimSpace(s), 10, 64)
}

// parseClaudeResult 从 result 事件抽出 usage（优先）或全文兜底。
func parseClaudeResult(raw map[string]json.RawMessage, sid string) (streamPiece, bool) {
	if u := claudeUsageTotal(raw); u > 0 {
		dur := claudeDurationMs(raw)
		return streamPiece{Kind: "usage", ID: fmtInt64(u), Text: fmtInt64(dur)}, true
	}
	if t, ok := rawStr(raw, "result"); ok && strings.TrimSpace(t) != "" {
		return streamPiece{Kind: "result_text", Text: t}, true
	}
	_ = sid
	return streamPiece{}, false
}

func claudeUsageTotal(raw map[string]json.RawMessage) int64 {
	usageRaw, ok := raw["usage"]
	if !ok {
		return 0
	}
	var u map[string]json.RawMessage
	if json.Unmarshal(usageRaw, &u) != nil {
		return 0
	}
	var total int64
	for _, k := range []string{"input_tokens", "output_tokens", "cache_creation_input_tokens", "cache_read_input_tokens"} {
		if n := jsonInt64(u, k); n > 0 {
			total += n
		}
	}
	if total > 0 {
		return total
	}
	return jsonInt64(u, "total_tokens")
}

func claudeDurationMs(raw map[string]json.RawMessage) int64 {
	for _, k := range []string{"duration_ms", "durationMs", "elapsed_ms"} {
		if n := jsonInt64(raw, k); n > 0 {
			return n
		}
	}
	return 0
}

func parseCodexUsage(raw map[string]json.RawMessage) (streamPiece, bool) {
	usageRaw, ok := raw["usage"]
	if !ok {
		return streamPiece{}, false
	}
	var u map[string]json.RawMessage
	if json.Unmarshal(usageRaw, &u) != nil {
		return streamPiece{}, false
	}
	var total int64
	for _, k := range []string{"input_tokens", "output_tokens", "cached_input_tokens", "reasoning_output_tokens"} {
		if n := jsonInt64(u, k); n > 0 {
			total += n
		}
	}
	if total <= 0 {
		total = jsonInt64(u, "total_tokens")
	}
	if total <= 0 {
		return streamPiece{}, false
	}
	dur := jsonInt64(raw, "duration_ms")
	if dur <= 0 {
		dur = jsonInt64(u, "duration_ms")
	}
	return streamPiece{Kind: "usage", ID: fmtInt64(total), Text: fmtInt64(dur)}, true
}

func jsonInt64(m map[string]json.RawMessage, key string) int64 {
	v, ok := m[key]
	if !ok {
		return 0
	}
	var n int64
	if json.Unmarshal(v, &n) == nil {
		return n
	}
	var f float64
	if json.Unmarshal(v, &f) == nil {
		return int64(f)
	}
	return 0
}

func fmtInt64(n int64) string {
	if n <= 0 {
		return ""
	}
	return strconv.FormatInt(n, 10)
}

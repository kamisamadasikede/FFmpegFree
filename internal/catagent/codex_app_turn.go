package catagent

// Protocol fields verified against codex-cli 0.162.1's offline schema bundle.
// https://developers.openai.com/codex/app-server/
import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/proc"
)

// Select the established transport before submitting any user input. Some
// third-party Responses gateways stall with app-server while exec works.
var errCodexExecTransport = errors.New("Codex provider uses exec transport")

type codexRPCMessage struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type codexRPC struct {
	writeMu    sync.Mutex
	writer     *json.Encoder
	mu         sync.Mutex
	next       int
	pending    map[string]chan codexRPCMessage
	events     chan codexRPCMessage
	eventMu    sync.Mutex
	queue      []codexRPCMessage
	wake       chan struct{}
	done       chan struct{}
	readErr    error
	jsonRPC    bool
	queueBytes int
	eventCount uint64
	onRequest  func(codexRPCMessage) (any, error)
}

func newCodexRPC(reader io.Reader, writer io.Writer) *codexRPC {
	return newNativeRPC(reader, writer, false)
}
func newNativeRPC(reader io.Reader, writer io.Writer, jsonRPC bool) *codexRPC {
	return newNativeRPCWithHandler(reader, writer, jsonRPC, nil)
}
func newNativeRPCWithHandler(reader io.Reader, writer io.Writer, jsonRPC bool, handler func(codexRPCMessage) (any, error)) *codexRPC {
	r := &codexRPC{writer: json.NewEncoder(writer), pending: map[string]chan codexRPCMessage{}, events: make(chan codexRPCMessage, 128), done: make(chan struct{}), wake: make(chan struct{}, 1)}
	r.jsonRPC = jsonRPC
	r.onRequest = handler
	go r.pumpEvents()
	go r.read(reader)
	return r
}

func (r *codexRPC) write(value any) error {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	if r.jsonRPC {
		if obj, ok := value.(map[string]any); ok {
			obj["jsonrpc"] = "2.0"
		}
	}
	return r.writer.Encode(value)
}

func (r *codexRPC) read(reader io.Reader) {
	defer close(r.done)
	sc := bufio.NewScanner(reader)
	sc.Buffer(make([]byte, 65536), 16<<20)
	for sc.Scan() {
		var msg codexRPCMessage
		if json.Unmarshal(sc.Bytes(), &msg) != nil {
			continue
		}
		if len(msg.ID) > 0 && msg.Method == "" {
			r.mu.Lock()
			ch := r.pending[string(msg.ID)]
			r.mu.Unlock()
			if ch != nil {
				select {
				case ch <- msg:
				default:
				}
			}
		} else if len(msg.ID) > 0 {
			if r.onRequest != nil {
				value, e := r.onRequest(msg)
				if e == nil {
					_ = r.write(map[string]any{"id": msg.ID, "result": value})
					continue
				}
			}
			// No manual approval surface here. Never silently grant a request.
			_ = r.write(map[string]any{"id": msg.ID, "error": map[string]any{"code": -32601, "message": "This client does not support interactive approval requests"}})
		} else if msg.Method != "" {
			r.eventMu.Lock()
			if r.queueBytes+len(sc.Bytes()) > 16<<20 {
				r.eventMu.Unlock()
				r.mu.Lock()
				r.readErr = errors.New("原生事件积压超过上限")
				r.mu.Unlock()
				return
			}
			r.queue = append(r.queue, msg)
			r.eventCount++
			r.queueBytes += len(sc.Bytes())
			r.eventMu.Unlock()
			select {
			case r.wake <- struct{}{}:
			default:
			}
		}
	}
	r.mu.Lock()
	r.readErr = sc.Err()
	r.mu.Unlock()
}

// Notifications may arrive before initialize/thread/start replies. Buffer them
// without blocking the reader that must deliver those replies (including steer).
func (r *codexRPC) pumpEvents() {
	defer close(r.events)
	for {
		r.eventMu.Lock()
		batch := r.queue
		r.queue = nil
		r.queueBytes = 0
		r.eventMu.Unlock()
		for _, event := range batch {
			r.events <- event
		}
		if len(batch) > 0 {
			continue
		}
		select {
		case <-r.wake:
		case <-r.done:
			r.eventMu.Lock()
			batch = r.queue
			r.queue = nil
			r.eventMu.Unlock()
			for _, event := range batch {
				r.events <- event
			}
			return
		}
	}
}

func (r *codexRPC) call(ctx context.Context, method string, params any, out any) error {
	r.mu.Lock()
	r.next++
	id := r.next
	key := fmt.Sprint(id)
	ch := make(chan codexRPCMessage, 1)
	r.pending[key] = ch
	r.mu.Unlock()
	defer func() { r.mu.Lock(); delete(r.pending, key); r.mu.Unlock() }()
	if err := r.write(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return err
	}
	decode := func(msg codexRPCMessage) error {
		if msg.Error != nil {
			return fmt.Errorf("%s: %s", method, msg.Error.Message)
		}
		if out != nil {
			return json.Unmarshal(msg.Result, out)
		}
		return nil
	}
	select {
	case msg := <-ch:
		return decode(msg)
	case <-ctx.Done():
		return ctx.Err()
	case <-r.done:
		select {
		case msg := <-ch:
			return decode(msg)
		default:
		}
		r.mu.Lock()
		err := r.readErr
		r.mu.Unlock()
		if err != nil {
			return err
		}
		return io.EOF
	}
}

func codexAppInput(text string, files []PromptFile) ([]map[string]any, error) {
	files, err := NormalizePromptFiles(files)
	if err != nil {
		return nil, err
	}
	var images []map[string]any
	var paths []string
	for _, f := range files {
		switch strings.ToLower(filepath.Ext(f.Path)) {
		case ".png", ".jpg", ".jpeg", ".webp", ".gif":
			images = append(images, map[string]any{"type": "localImage", "path": f.Path})
		default:
			paths = append(paths, f.Path)
		}
	}
	text = joinPrompt(text, paths)
	input := []map[string]any{{"type": "text", "text": text}}
	return append(input, images...), nil
}

func codexAppThreadParams(opts TurnOptions, cwd string) map[string]any {
	params := map[string]any{"cwd": cwd, "approvalPolicy": "never", "sandbox": "read-only"}
	switch NormalizeMode(KindCodexCLI, opts.PermissionMode) {
	case ModeCodexWorkspace:
		params["sandbox"] = "workspace-write"
	case ModeCodexApproveForMe:
		params["sandbox"], params["approvalPolicy"], params["approvalsReviewer"] = "workspace-write", "on-request", "auto_review"
	case ModeCodexBypass:
		params["sandbox"] = "danger-full-access"
	}
	if opts.ModelID != "" && opts.ModelID != "default" {
		params["model"] = opts.ModelID
	}
	if opts.Provider.Enabled && opts.Provider.AgentKind == KindCodexCLI && strings.TrimSpace(opts.Provider.ApiURL) != "" {
		params["modelProvider"] = CodexProviderID
	}
	// Override the previous bridge on resume, including when the bridge is off.
	params["developerInstructions"] = ""
	if opts.Rules != "" {
		params["developerInstructions"] = opts.Rules
	}
	return params
}

func (a *CLIAgentAdapter) runCodexAppTurn(opts TurnOptions, exe string) (TurnResponse, error) {
	ctx := opts.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	cwd := strings.TrimSpace(opts.ProjectPath)
	if cwd == "" {
		var err error
		cwd, err = a.plainCwd(opts.ConversationID)
		if err != nil {
			return TurnResponse{}, err
		}
	}
	input, err := codexAppInput(lastUserContent(opts.Messages), opts.Files)
	if err != nil {
		return TurnResponse{}, err
	}
	runCtx, idle, cancel := newTurnContext(ctx, opts.Timeout, nil)
	defer cancel()
	// Headless turns must finish on exhausted retries, even when the user's TUI
	// configuration enables indefinite reconnection.
	args := append([]string{"app-server", "--listen", "stdio://", "-c", "features.unbounded_connection_retries=false"}, opts.Provider.ExtraArgs()...)
	cmd := a.runCmd(runCtx, exe, args...)
	cmd.WaitDelay = 2 * time.Second
	cmd.Dir, cmd.Env = cwd, MergeToolEnv(MergeSkillsEnv(MergeProviderEnv(os.Environ(), opts.Provider), opts.Skills), opts.ToolEnv)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return TurnResponse{}, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return TurnResponse{}, err
	}
	stderr := &stderrTap{}
	cmd.Stderr = activityWriter{inner: stderr, w: idle}
	if err = proc.Start(cmd); err != nil {
		_ = stdin.Close()
		return TurnResponse{}, apperr.Wrap(apperr.CatReplyFailed, MsgReplyFailed, err)
	}
	// Reader shutdown must complete before Wait closes its pipe.
	rpc := newCodexRPC(activityReader{r: stdout, w: idle}, stdin)
	defer func() {
		if opts.OnSteerReady != nil {
			opts.OnSteerReady(nil)
		}
		_ = stdin.Close()
		cancel()
		// A persistent CLI host may retain an inherited pipe. Close our read end
		// explicitly so transport selection/cancellation never waits on that host.
		_ = stdout.Close()
		for range rpc.events {
		}
		_ = cmd.Wait()
	}()
	initCtx, initCancel := context.WithTimeout(runCtx, 20*time.Second)
	defer initCancel()
	if err = rpc.call(initCtx, "initialize", map[string]any{"clientInfo": map[string]string{"name": "ffc", "version": "1.0"}}, nil); err != nil {
		return TurnResponse{}, a.codexAppError(ctx, runCtx, err)
	}
	if err = rpc.write(map[string]any{"method": "initialized"}); err != nil {
		return TurnResponse{}, a.codexAppError(ctx, runCtx, err)
	}
	threadParams := codexAppThreadParams(opts, cwd)
	method := "thread/start"
	if sid := a.loadSessionID(opts.ConversationID); sid != "" {
		method = "thread/resume"
		threadParams["threadId"] = sid
		threadParams["excludeTurns"] = true
	}
	var threadResp struct {
		ModelProvider string `json:"modelProvider"`
		Thread        struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err = rpc.call(initCtx, method, threadParams, &threadResp); err != nil {
		return TurnResponse{}, a.codexAppError(ctx, runCtx, err)
	}
	if provider := threadResp.ModelProvider; provider != "" && provider != "openai" && opts.Transport != "app-server" {
		a.cfg.Logf("cat Codex: using exec for configured provider %q", provider)
		return TurnResponse{}, errCodexExecTransport
	}
	sid := threadResp.Thread.ID
	if sid == "" {
		return TurnResponse{}, apperr.New(apperr.CatReplyFailed, "助手未返回会话编号。")
	}
	a.saveSessionID(opts.ConversationID, sid)
	turnParams := map[string]any{"threadId": sid, "input": input}
	if opts.ThinkLevelID != "" {
		turnParams["effort"] = opts.ThinkLevelID
	}
	var turnResp struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if err = rpc.call(initCtx, "turn/start", turnParams, &turnResp); err != nil {
		return TurnResponse{}, a.codexAppError(ctx, runCtx, err)
	}
	tid := turnResp.Turn.ID
	if tid == "" {
		return TurnResponse{}, apperr.New(apperr.CatReplyFailed, "助手未返回任务编号。")
	}
	initCancel()
	if opts.OnInputMode != nil {
		opts.OnInputMode("active_turn")
	}
	if opts.OnSteerReady != nil {
		opts.OnSteerReady(func(steerCtx context.Context, text string, files []PromptFile) error {
			input, err := codexAppInput(text, files)
			if err != nil {
				return err
			}
			steerCtx, done := context.WithTimeout(steerCtx, 10*time.Second)
			defer done()
			if err = rpc.call(steerCtx, "turn/steer", map[string]any{"threadId": sid, "expectedTurnId": tid, "input": input}, nil); err != nil {
				return apperr.Wrap(apperr.TaskConflict, "当前任务未接收这条引导，消息已保留。", err)
			}
			idle.touch()
			return nil
		})
	}
	resp := TurnResponse{Version: ProtocolVersion}
	var acc strings.Builder
	texts := map[string]string{}
	thoughts := map[string]string{}
	for {
		select {
		case <-runCtx.Done():
			return resp, a.codexAppError(ctx, runCtx, runCtx.Err())
		case event, ok := <-rpc.events:
			if !ok {
				return resp, a.codexAppError(ctx, runCtx, errors.New("Codex 连接在任务完成前关闭"))
			}
			var p struct {
				ThreadID  string `json:"threadId"`
				TurnID    string `json:"turnId"`
				ItemID    string `json:"itemId"`
				Delta     string `json:"delta"`
				WillRetry bool   `json:"willRetry"`
				Error     *struct {
					Message           string `json:"message"`
					AdditionalDetails string `json:"additionalDetails"`
				} `json:"error"`
				Item json.RawMessage `json:"item"`
				Turn struct {
					ID     string `json:"id"`
					Status string `json:"status"`
					Error  *struct {
						Message string `json:"message"`
					} `json:"error"`
				} `json:"turn"`
				TokenUsage struct {
					Last struct {
						Total  int64 `json:"totalTokens"`
						Output int64 `json:"outputTokens"`
						Input  int64 `json:"inputTokens"`
					} `json:"last"`
					Window int64 `json:"modelContextWindow"`
				} `json:"tokenUsage"`
			}
			if json.Unmarshal(event.Params, &p) != nil || p.ThreadID != sid {
				continue
			}
			if p.TurnID != "" && p.TurnID != tid {
				continue
			}
			switch event.Method {
			case "error":
				if p.Error == nil {
					continue
				}
				msg := strings.TrimSpace(p.Error.Message)
				if p.Error.AdditionalDetails != "" {
					msg += "\n" + p.Error.AdditionalDetails
				}
				for _, secret := range append([]string{opts.Provider.ApiKey}, toolSecrets(opts.ToolEnv)...) {
					if secret != "" {
						msg = strings.ReplaceAll(msg, secret, "[redacted]")
					}
				}
				msg = truncate(msg, 500)
				if p.WillRetry {
					if opts.OnActivity != nil {
						opts.OnActivity("retry", tid, msg, true)
					}
					continue
				}
				if opts.OnError != nil {
					opts.OnError(msg)
				}
				if opts.Provider.Enabled && codexResponsesMissing(msg) {
					return resp, providerResponsesUnsupportedErr(KindCodexCLI)
				}
				return resp, a.codexAppError(ctx, runCtx, errors.New(msg))
			case "item/agentMessage/delta":
				texts[p.ItemID] += p.Delta
				acc.WriteString(p.Delta)
				if opts.OnTextDelta != nil {
					opts.OnTextDelta(p.Delta)
				}
			case "item/reasoning/summaryTextDelta", "item/reasoning/textDelta":
				thoughts[p.ItemID] += p.Delta
				if opts.OnActivity != nil {
					opts.OnActivity("thought", p.ItemID, thoughts[p.ItemID], true)
				}
			case "item/started", "item/completed":
				var item struct {
					Type    string `json:"type"`
					ID      string `json:"id"`
					Text    string `json:"text"`
					Command string `json:"command"`
					Tool    string `json:"tool"`
					Query   string `json:"query"`
					Changes []struct {
						Path string `json:"path"`
						Diff string `json:"diff"`
					} `json:"changes"`
				}
				if json.Unmarshal(p.Item, &item) != nil {
					continue
				}
				if item.Type == "agentMessage" {
					if event.Method == "item/completed" && texts[item.ID] == "" && item.Text != "" {
						texts[item.ID] = item.Text
						acc.WriteString(item.Text)
						if opts.OnTextDelta != nil {
							opts.OnTextDelta(item.Text)
						}
					}
					continue
				}
				label := ""
				switch item.Type {
				case "commandExecution":
					label = "shell " + item.Command
				case "fileChange":
					label = "apply_patch"
					for _, c := range item.Changes {
						label += "\n" + c.Path
						if c.Diff != "" {
							label += "\n" + c.Diff
						}
					}
				case "mcpToolCall", "dynamicToolCall":
					label = item.Tool
				case "webSearch":
					label = "Search: " + item.Query
				}
				if label != "" && opts.OnActivity != nil {
					opts.OnActivity("tool", item.ID, label, true)
				}
			case "thread/tokenUsage/updated":
				resp.TokenTotal, resp.OutputTokens, resp.ContextUsed, resp.ContextWindow = p.TokenUsage.Last.Total, p.TokenUsage.Last.Output, p.TokenUsage.Last.Input, p.TokenUsage.Window
			case "turn/completed":
				if p.Turn.ID != tid {
					continue
				}
				resp.Message = WireMessage{Role: "assistant", Content: acc.String()}
				switch p.Turn.Status {
				case "completed":
					return resp, nil
				case "interrupted":
					return resp, apperr.New(apperr.Canceled, "已取消")
				default:
					msg := MsgReplyFailed
					if p.Turn.Error != nil && p.Turn.Error.Message != "" {
						msg = p.Turn.Error.Message
					}
					return resp, a.codexAppError(ctx, runCtx, errors.New(msg))
				}
			}
		}
	}
}

func toolSecrets(env map[string]string) []string {
	var secrets []string
	for key, value := range env {
		if strings.Contains(strings.ToUpper(key), "TOKEN") || strings.Contains(strings.ToUpper(key), "KEY") {
			secrets = append(secrets, value)
		}
	}
	return secrets
}

func (a *CLIAgentAdapter) codexAppError(ctx, runCtx context.Context, err error) error {
	if ctx.Err() != nil {
		return apperr.New(apperr.Canceled, "已取消")
	}
	if idleTimedOut(runCtx) {
		return apperr.New(apperr.CatReplyFailed, MsgIdleTimeout).WithDetail(ReasonIdleTimeout)
	}
	return apperr.Wrap(apperr.CatReplyFailed, truncate(err.Error(), 500), err).WithDetail("reason=app_server")
}

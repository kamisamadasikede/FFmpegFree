package catagent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"FFmpegFree/internal/proc"
)

// ACP session/prompt is a long request. Until an explicit native steer
// capability is verified, a second prompt is not advertised as active input.
func (a *BuildAdapter) runGrokACP(opts TurnOptions, exe string) (TurnResponse, error) {
	ctx := opts.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	runCtx, idle, cancel := newTurnContext(ctx, opts.Timeout, nil)
	defer cancel()
	cwd := opts.ProjectPath
	if cwd == "" {
		var e error
		cwd, e = a.plainCwd(opts.ConversationID)
		if e != nil {
			return TurnResponse{}, e
		}
	}
	args := []string{"agent", "--no-leader"}
	if opts.ModelID != "" && opts.ModelID != "default" {
		args = append(args, "--model", opts.ModelID)
	}
	if opts.ThinkLevelID != "" {
		args = append(args, "--reasoning-effort", opts.ThinkLevelID)
	}
	args = append(args, "stdio")
	cmd := a.runCmd(runCtx, exe, args...)
	cmd.Dir = cwd
	cmd.Env = MergeToolEnv(MergeSkillsEnv(MergeProviderEnv(grokTurnEnv(os.Environ()), opts.Provider), opts.Skills), opts.ToolEnv)
	cmd.WaitDelay = 2 * time.Second
	stdin, e := cmd.StdinPipe()
	if e != nil {
		return TurnResponse{}, e
	}
	stdout, e := cmd.StdoutPipe()
	if e != nil {
		return TurnResponse{}, e
	}
	stderr := &stderrTap{}
	cmd.Stderr = activityWriter{inner: stderr, w: idle}
	if e = proc.Start(cmd); e != nil {
		return TurnResponse{}, e
	}
	rpc := newNativeRPCWithHandler(activityReader{r: stdout, w: idle}, stdin, true, func(msg codexRPCMessage) (any, error) {
		if msg.Method != "session/request_permission" || opts.ToolEnv["FFC_TEAM_TOKEN"] == "" {
			return nil, errors.New("不支持此原生请求")
		}
		var params struct {
			ToolCall struct {
				Title    string         `json:"title"`
				RawInput map[string]any `json:"rawInput"`
			} `json:"toolCall"`
			Options []struct {
				ID   string `json:"optionId"`
				Kind string `json:"kind"`
			} `json:"options"`
		}
		if e := json.Unmarshal(msg.Params, &params); e != nil {
			return nil, e
		}
		if !isTeamNativePermission(params.ToolCall.RawInput) {
			if opts.OnActivity != nil {
				opts.OnActivity("permission", "permission", "此原生权限请求未获授权：仅支持本应用工具的内联参数调用", true)
			}
			return nil, errors.New("权限请求未关联本应用工具")
		}
		for _, option := range params.Options {
			if option.Kind == "allow_once" {
				return map[string]any{"outcome": map[string]string{"outcome": "selected", "optionId": option.ID}}, nil
			}
		}
		return nil, errors.New("缺少单次授权选项")
	})
	defer func() {
		if opts.OnSteerReady != nil {
			opts.OnSteerReady(nil)
		}
		_ = stdin.Close()
		cancel()
		_ = stdout.Close()
		for range rpc.events {
		}
		_ = cmd.Wait()
	}()
	initCtx, done := context.WithTimeout(runCtx, 20*time.Second)
	defer done()
	var initResult map[string]any
	if e = rpc.call(initCtx, "initialize", map[string]any{"protocolVersion": 1, "clientInfo": map[string]string{"name": "ffc", "version": "1.0"}, "clientCapabilities": map[string]any{}}, &initResult); e != nil {
		return TurnResponse{}, e
	}
	params := map[string]any{"cwd": cwd, "mcpServers": grokTeamMCP(opts), "_meta": map[string]any{"rules": opts.Rules}}
	mode := NormalizeMode(KindCatBuild, opts.PermissionMode)
	if mode == ModeAuto {
		params["_meta"].(map[string]any)["autoMode"] = true
	}
	if mode == ModeAcceptEdits {
		return TurnResponse{}, errors.New("当前原生通道尚未验证自动编辑权限模式，请选择标准权限或已支持的模式")
	}
	// Preserve actual permission choices. Never turn manual approval into yolo.
	if mode == ModeGrokAlwaysApprove {
		params["_meta"].(map[string]any)["yoloMode"] = true
	}
	path := filepath.Join(a.cfg.DataRoot, "team", "grok-acp", safeID(opts.ConversationID))
	var session struct {
		SessionID string `json:"sessionId"`
	}
	method := "session/new"
	if strings.HasPrefix(opts.ConversationID, "team-") || opts.Resume {
		if raw, e := os.ReadFile(path); e == nil && strings.TrimSpace(string(raw)) != "" {
			params["sessionId"] = strings.TrimSpace(string(raw))
			method = "session/load"
		}
	}
	if e = rpc.call(initCtx, method, params, &session); e != nil {
		return TurnResponse{}, e
	}
	if session.SessionID == "" {
		if old, ok := params["sessionId"].(string); ok {
			session.SessionID = old
		}
	}
	if session.SessionID == "" {
		return TurnResponse{}, errors.New("原生会话未返回标识")
	}
	_ = os.MkdirAll(filepath.Dir(path), 0700)
	_ = os.WriteFile(path, []byte(session.SessionID), 0600)
	done()
	if opts.OnInputMode != nil {
		opts.OnInputMode("next_turn")
	}
	if opts.OnSteerReady != nil {
		opts.OnSteerReady(nil)
	}
	prompt := []map[string]any{{"type": "text", "text": lastUserContent(opts.Messages)}}
	for _, f := range opts.Files {
		prompt = append(prompt, map[string]any{"type": "text", "text": "Attached workspace file: " + f.Path})
	}
	rpc.eventMu.Lock()
	historyEvents := rpc.eventCount
	rpc.eventMu.Unlock()
	result := make(chan error, 1)
	go func() {
		result <- rpc.call(runCtx, "session/prompt", map[string]any{"sessionId": session.SessionID, "prompt": prompt}, nil)
	}()
	resp := TurnResponse{Version: ProtocolVersion}
	var text strings.Builder
	var consumed uint64
	consume := func(event codexRPCMessage) {
		consumed++
		if consumed <= historyEvents {
			return
		}
		var p struct {
			SessionID string `json:"sessionId"`
			Update    struct {
				Type    string `json:"sessionUpdate"`
				Content struct {
					Text string `json:"text"`
				} `json:"content"`
				Title      string `json:"title"`
				ToolCallID string `json:"toolCallId"`
				Status     string `json:"status"`
			} `json:"update"`
		}
		if json.Unmarshal(event.Params, &p) != nil || p.SessionID != session.SessionID {
			return
		}
		switch p.Update.Type {
		case "agent_message_chunk":
			text.WriteString(p.Update.Content.Text)
			if opts.OnTextDelta != nil {
				opts.OnTextDelta(p.Update.Content.Text)
			}
		case "tool_call", "tool_call_update":
			if opts.OnActivity != nil {
				opts.OnActivity("tool", p.Update.ToolCallID, p.Update.Title, true)
			}
		case "agent_thought_chunk":
			if opts.OnActivity != nil {
				opts.OnActivity("thought", "thought", p.Update.Content.Text, false)
			}
		}
	}
	for {
		select {
		case <-runCtx.Done():
			return resp, runCtx.Err()
		case e := <-result:
			// Responses and notifications use separate channels. Drain every update
			// read before this result, including a batch still in the pump goroutine.
			rpc.eventMu.Lock()
			target := rpc.eventCount
			rpc.eventMu.Unlock()
			for consumed < target {
				select {
				case <-runCtx.Done():
					return resp, runCtx.Err()
				case event, ok := <-rpc.events:
					if !ok {
						resp.Message = WireMessage{Role: "assistant", Content: text.String()}
						return resp, e
					}
					consume(event)
				}
			}
			resp.Message = WireMessage{Role: "assistant", Content: text.String()}
			return resp, e
		case event, ok := <-rpc.events:
			if !ok {
				return resp, errors.New("原生连接已断开")
			}
			consume(event)
		}
	}
}

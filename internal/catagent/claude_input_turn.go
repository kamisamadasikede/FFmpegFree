package catagent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/id"
	"FFmpegFree/internal/proc"
)

// Claude stream-json accepts additional user messages, but does not promise
// turn/steer semantics. Report checkpoint input rather than active-turn steering.
func (a *CLIAgentAdapter) runClaudeInputStream(opts TurnOptions, exe string) (TurnResponse, error) {
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
	sid := a.loadSessionID(opts.ConversationID)
	args := a.buildClaudeArgs(opts, "", sid, sid != "")
	args = args[2:]
	args = append([]string{"-p"}, args...)
	args = append(args, "--input-format", "stream-json", "--replay-user-messages")
	a.mu.Lock()
	version := a.version
	a.mu.Unlock()
	if version != "" && !cliVersionBelow(version, "2.1.300") {
		args = append(args, "--system-prompt-snapshot", "off")
	}
	// Keep private rules out of a cmd.exe wrapper's argument expansion.
	if isShimExt(exe) {
		var clean func()
		var e error
		args, _, clean, e = shimStdinArgs(a.kind, append([]string{"-p", ""}, args[1:]...), a.cfg.DataTemp)
		if e != nil {
			return TurnResponse{}, e
		}
		if clean != nil {
			defer clean()
		}
	}
	cmd := a.runCmd(runCtx, exe, args...)
	cmd.Dir = cwd
	cmd.Env = MergeToolEnv(MergeSkillsEnv(MergeProviderEnv(os.Environ(), opts.Provider), opts.Skills), opts.ToolEnv)
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
	var writeMu sync.Mutex
	var stateMu sync.Mutex
	pending := map[string]chan struct{}{}
	closed := false
	submitted, finished := 1, 0
	writeInput := func(text string, files []PromptFile, key string) error {
		input, err := claudeUserInput(text, files, key)
		if err != nil {
			return err
		}
		writeMu.Lock()
		defer writeMu.Unlock()
		return json.NewEncoder(stdin).Encode(input)
	}
	defer func() {
		stateMu.Lock()
		closed = true
		stateMu.Unlock()
		if opts.OnSteerReady != nil {
			opts.OnSteerReady(nil)
		}
		_ = stdin.Close()
		cancel()
		_ = stdout.Close()
		_ = cmd.Wait()
	}()
	if e = writeInput(lastUserContent(opts.Messages), opts.Files, id.New()); e != nil {
		return TurnResponse{}, e
	}
	if opts.OnInputMode != nil {
		opts.OnInputMode("checkpoint")
	}
	steer := func(inputCtx context.Context, text string, files []PromptFile) error {
		if len(files) > 0 {
			return apperr.New(apperr.Unsupported, "当前追加输入暂不支持附件")
		}
		key := id.New()
		ack := make(chan struct{})
		stateMu.Lock()
		if closed {
			stateMu.Unlock()
			return errors.New("原生输入通道已结束")
		}
		pending[key] = ack
		submitted++
		stateMu.Unlock()
		if e := writeInput(text, files, key); e != nil {
			cancel()
			return e
		}
		defer func() { stateMu.Lock(); delete(pending, key); stateMu.Unlock() }()
		select {
		case <-ack:
			return nil
		case <-inputCtx.Done():
			return inputCtx.Err()
		case <-runCtx.Done():
			return runCtx.Err()
		}
	}
	ready := false
	resp := TurnResponse{Version: ProtocolVersion}
	var text strings.Builder
	var cliErr string
	turnText := false
	sc := bufio.NewScanner(activityReader{r: stdout, w: idle})
	sc.Buffer(make([]byte, 65536), 16<<20)
	for sc.Scan() {
		line := sc.Bytes()
		var raw map[string]json.RawMessage
		if json.Unmarshal(line, &raw) != nil {
			continue
		}
		typ, _ := rawStr(raw, "type")
		if !ready && (typ == "system" || typ == "user") {
			ready = true
			if opts.OnSteerReady != nil {
				opts.OnSteerReady(steer)
			}
		}
		if session, _ := rawStr(raw, "session_id"); session != "" {
			a.saveSessionID(opts.ConversationID, session)
		}
		if typ == "user" {
			key, _ := rawStr(raw, "uuid")
			stateMu.Lock()
			if ch := pending[key]; ch != nil {
				close(ch)
				delete(pending, key)
			}
			stateMu.Unlock()
			continue
		}
		onPiece := func(p streamPiece) {
			switch p.Kind {
			case "done":
			case "usage":
				resp.TokenTotal, _ = parseInt64(p.ID)
				resp.OutputTokens, _ = parseInt64(p.Text)
			case "ctxwin":
				resp.ContextWindow, _ = parseInt64(p.ID)
			case "result_text":
				if !turnText {
					text.WriteString(p.Text)
					if opts.OnTextDelta != nil {
						opts.OnTextDelta(p.Text)
					}
				}
			case "thought", "tool", "retry":
				if opts.OnActivity != nil {
					opts.OnActivity(p.Kind, p.ID, p.Text, p.Replace)
				}
			default:
				if p.Text != "" {
					turnText = true
					text.WriteString(p.Text)
					if opts.OnTextDelta != nil {
						opts.OnTextDelta(p.Text)
					}
				}
			}
		}
		if typ == "result" {
			emitClaudeResultLine(line, onPiece, func(msg string) { cliErr = msg }, func(sid string) { a.saveSessionID(opts.ConversationID, sid) })
			stateMu.Lock()
			finished++
			done := finished >= submitted
			if done {
				closed = true
			}
			turnText = false
			stateMu.Unlock()
			if cliErr != "" {
				return resp, errors.New(cliErr)
			}
			if done {
				resp.Message = WireMessage{Role: "assistant", Content: text.String()}
				return resp, nil
			}
			continue
		}
		piece, _, errMsg, isErr := a.parseCLIStreamLine(line)
		if isErr {
			cliErr = errMsg
			if opts.OnError != nil {
				opts.OnError(errMsg)
			}
		} else {
			onPiece(piece)
		}
	}
	if runCtx.Err() != nil {
		return resp, runCtx.Err()
	}
	if e = sc.Err(); e != nil {
		return resp, e
	}
	if cliErr == "" {
		cliErr = MsgReplyFailed
	}
	return resp, errors.New(cliErr)
}

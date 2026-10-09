package langasr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/id"
)

// DefaultASRTimeout 是识别超时（契约 TBD；先 30 分钟）。
const DefaultASRTimeout = 30 * time.Minute

// EntryBinary 返回组件根下的入口路径；不存在返回 ""。
func EntryBinary(componentRoot string) string {
	if componentRoot == "" {
		return ""
	}
	name := "asr"
	if runtime.GOOS == "windows" {
		name = "asr.exe"
	}
	p := filepath.Join(componentRoot, name)
	fi, err := os.Stat(p)
	if err != nil || fi.IsDir() {
		return ""
	}
	return p
}

// Request 是 request.json（契约 6.18.4）。
type Request struct {
	Version   int    `json:"version"`
	AudioPath string `json:"audioPath"`
	Language  string `json:"language"`
	Tier      string `json:"tier"`
}

// Response 是 response.json（契约 6.18.4）。
type Response struct {
	Version int           `json:"version"`
	Cues    []SubtitleCue `json:"cues"`
}

// RunOptions 是一次适配器调用。
type RunOptions struct {
	ComponentRoot string
	WorkDir       string // <tmp>/lang/<taskId>/asr/
	AudioPath     string
	Language      string // 默认 auto
	Tier          string
	Timeout       time.Duration
	// Exec 可覆盖（测试注入假二进制）；nil 用 exec.CommandContext。
	Exec func(ctx context.Context, name string, args ...string) *exec.Cmd
	Logf func(format string, args ...any)
}

// RunASR 按 6.18.4 调用入口二进制，返回规范化后的 cues。
func RunASR(ctx context.Context, opts RunOptions) ([]SubtitleCue, error) {
	exe := EntryBinary(opts.ComponentRoot)
	if exe == "" {
		return nil, apperr.New(apperr.LangAsrNotReady, MsgNotReady).WithDetail("reason=missing_entry")
	}
	if opts.WorkDir == "" || opts.AudioPath == "" {
		return nil, apperr.New(apperr.InvalidArgument, "识别参数不完整")
	}
	lang := opts.Language
	if lang == "" {
		lang = "auto"
	}
	tier := opts.Tier
	if tier != TierHD {
		tier = TierStandard
	}
	if err := os.MkdirAll(opts.WorkDir, 0o755); err != nil {
		return nil, apperr.Wrap(apperr.IOError, "无法创建临时文件夹", err)
	}
	reqPath := filepath.Join(opts.WorkDir, "request.json")
	respPath := filepath.Join(opts.WorkDir, "response.json")
	req := Request{Version: 1, AudioPath: opts.AudioPath, Language: lang, Tier: tier}
	b, err := json.Marshal(req)
	if err != nil {
		return nil, apperr.Wrap(apperr.Internal, "内部错误", err)
	}
	if err := os.WriteFile(reqPath, b, 0o644); err != nil {
		return nil, apperr.Wrap(apperr.IOError, "无法写入识别请求", err)
	}
	_ = os.Remove(respPath)

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultASRTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	run := opts.Exec
	if run == nil {
		run = exec.CommandContext
	}
	cmd := run(runCtx, exe,
		"--component-root", opts.ComponentRoot,
		"--request", reqPath,
		"--response", respPath,
	)
	cmd.Dir = opts.WorkDir
	var stderr strings.Builder
	cmd.Stderr = &stderr
	cmd.Stdout = &stderr
	err = cmd.Run()
	if opts.Logf != nil && stderr.Len() > 0 {
		opts.Logf("asr stderr: %s", trimTail(stderr.String(), 4000))
	}
	if runCtx.Err() != nil {
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return nil, apperr.New(apperr.LangAsrFailed, MsgFailed).WithDetail("reason=timeout")
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, apperr.New(apperr.LangAsrFailed, MsgFailed).WithDetail("reason=canceled")
	}
	if err != nil {
		code := 0
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		}
		return nil, apperr.New(apperr.LangAsrFailed, MsgFailed).WithDetail(fmt.Sprintf("exit=%d", code))
	}
	raw, err := os.ReadFile(respPath)
	if err != nil {
		return nil, apperr.New(apperr.LangAsrFailed, MsgFailed).WithDetail("reason=response_missing")
	}
	var resp Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, apperr.New(apperr.LangAsrFailed, MsgFailed).WithDetail("reason=response_invalid")
	}
	cues := normalizeCues(resp.Cues)
	if !hasEffectiveText(cues) {
		return nil, apperr.New(apperr.LangAsrEmpty, MsgEmpty).WithDetail("reason=empty")
	}
	return cues, nil
}

func normalizeCues(in []SubtitleCue) []SubtitleCue {
	out := make([]SubtitleCue, 0, len(in))
	for _, c := range in {
		c.Text = strings.TrimSpace(c.Text)
		if c.ID == "" {
			c.ID = id.New()
		}
		out = append(out, c)
	}
	return out
}

func hasEffectiveText(cues []SubtitleCue) bool {
	for _, c := range cues {
		if strings.TrimSpace(c.Text) != "" {
			return true
		}
	}
	return false
}

func trimTail(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[len(r)-n:])
}

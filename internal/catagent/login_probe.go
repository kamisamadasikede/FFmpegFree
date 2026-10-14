package catagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"
)

// v0.47.1（6.19.21）：agents[].loggedIn 只来自实时检测；判断不了 = nil。
//
// 依据（真机 help 快照 testdata/help/*-login*.txt / claude-auth*.txt，未登录样本 testdata/login/）：
//   - Codex：`codex login status`（codex 0.162.1 `codex login --help` 列出 status 子命令）。
//     官方 https://developers.openai.com/codex/cli/slash-commands ：「exits with 0 when credentials are present」。
//     本机未登录实跑：exit 1，stderr「Not logged in」。exit 0 → true；exit 1 且含 Not logged in → false；其余 nil。
//     环境变量 Key / 自定义服务商不被该命令反映（官方说明只针对缓存凭据）→ 这两种情况不判断（nil）。
//   - Claude：`claude auth status --json`（2.1.296 `claude auth status --help` 列出 --json）。
//     官方 https://code.claude.com/docs/en/cli-reference ：JSON 输出，「Exits with code 0 if logged in, 1 if not」。
//     本机未登录实跑：exit 1，stdout JSON {"loggedIn": false, ...}。取 JSON 的 loggedIn 布尔值；解析不了 nil。
//     自定义服务商（子进程注入 Key）时不判断（nil）。
//   - Grok（cat_build）：1.0.40 只有 login / logout，没有查询登录状态的命令，也没有文档化的凭据文件 → 不填。

// loginProbeTimeout 在 15 秒检测超时之内。
const loginProbeTimeout = 5 * time.Second

// parseCodexLoginStatus 按 `codex login status` 的退出码和输出判断。
func parseCodexLoginStatus(exitCode int, output string) *bool {
	switch {
	case exitCode == 0:
		v := true
		return &v
	case exitCode == 1 && strings.Contains(output, "Not logged in"):
		v := false
		return &v
	}
	return nil
}

// parseClaudeAuthStatus 取 `claude auth status --json` stdout 里的 loggedIn 布尔值。
func parseClaudeAuthStatus(stdout []byte) *bool {
	var v struct {
		LoggedIn *bool `json:"loggedIn"`
	}
	if json.Unmarshal(bytes.TrimSpace(stdout), &v) != nil {
		return nil
	}
	return v.LoggedIn
}

// probeLogin 实时探一次登录状态；失败 / 超时 / 判断不了 → nil。
func (a *CLIAgentAdapter) probeLogin(exe string) *bool {
	if exe == "" || a.providerOverlay().Enabled {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), loginProbeTimeout)
	defer cancel()
	var args []string
	switch a.kind {
	case cliKindCodex:
		if os.Getenv("CODEX_API_KEY") != "" || os.Getenv("OPENAI_API_KEY") != "" {
			return nil
		}
		args = []string{"login", "status"}
	case cliKindClaude:
		args = []string{"auth", "status", "--json"}
	default:
		return nil
	}
	cmd := a.runCmd(ctx, exe, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		return nil
	}
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			return nil
		}
		code = ee.ExitCode()
	}
	if a.kind == cliKindCodex {
		return parseCodexLoginStatus(code, stdout.String()+stderr.String())
	}
	if code != 0 && code != 1 {
		return nil
	}
	return parseClaudeAuthStatus(stdout.Bytes())
}

// LiveLoggedIn 是最近一次实时检测得到的登录状态；本进程没检测完过 / 判断不了 = nil。
// 检测进行中保留上一次的值（与其他保留字段一致）。只供 cat:status，不落库。
func (a *CLIAgentAdapter) LiveLoggedIn() *bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.loggedIn == nil {
		return nil
	}
	v := *a.loggedIn
	return &v
}

package catagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 依据（v0.46，契约 6.19.20.1）：env 名不在任何 --help 里，出处是官方文档：
//   - Grok GROK_MODELS_BASE_URL / XAI_API_KEY / GROK_CODE_XAI_API_KEY / GROK_MODELS_LIST_URL：
//     grok 1.0.40 自带 ~/.grok/docs/user-guide/11-custom-models.md「Custom Models Endpoint」
//     与 26-config-reference.md「endpoints.models_base_url … Also GROK_MODELS_BASE_URL」。
//   - Claude ANTHROPIC_BASE_URL / ANTHROPIC_AUTH_TOKEN / ANTHROPIC_API_KEY：
//     https://code.claude.com/docs/en/llm-gateway-connect（「ANTHROPIC_AUTH_TOKEN in Authorization: Bearer,
//     ANTHROPIC_API_KEY in x-api-key」）；ANTHROPIC_API_KEY 另见 testdata/help/claude.txt（--bare 说明）。
//   - Codex FFCAT_CODEX_API_KEY 是我们自己起的名字，经 -c model_providers.ffcat.env_key 指过去：
//     https://developers.openai.com/codex/config-reference（model_providers.<id>.env_key）。
var providerEnvWhitelist = map[string][]string{
	KindCatBuild:  {"GROK_MODELS_BASE_URL", "XAI_API_KEY"},
	KindClaudeCLI: {"ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_API_KEY"},
	KindCodexCLI:  {CodexProviderKeyEnv},
}

func envNames(env []string) []string {
	var out []string
	for _, kv := range env {
		if i := strings.IndexByte(kv, '='); i > 0 {
			out = append(out, kv[:i])
		}
	}
	return out
}

func TestProviderEnvWhitelist(t *testing.T) {
	for kind, allowed := range providerEnvWhitelist {
		ok := map[string]bool{}
		for _, k := range allowed {
			ok[k] = true
		}
		for _, u := range []string{"https://gw.example.com/v1", "https://api.anthropic.com"} {
			o := ProviderOverlay{Enabled: true, AgentKind: kind, ApiKey: "k", ApiURL: u}
			for _, n := range envNames(o.ExtraEnv()) {
				if !ok[n] {
					t.Errorf("%s 注入了未核实的环境变量 %s", kind, n)
				}
			}
		}
	}
}

func TestExtraEnvByKind(t *testing.T) {
	cases := []struct {
		kind, url string
		want      []string
		not       []string
	}{
		{KindCatBuild, "https://gw/v1", []string{"GROK_MODELS_BASE_URL=https://gw/v1", "XAI_API_KEY=k"}, []string{"XAI_BASE_URL", "OPENAI_BASE_URL"}},
		{KindClaudeCLI, "https://gw.example.com", []string{"ANTHROPIC_BASE_URL=https://gw.example.com", "ANTHROPIC_AUTH_TOKEN=k"}, []string{"ANTHROPIC_API_KEY"}},
		{KindClaudeCLI, "https://api.anthropic.com", []string{"ANTHROPIC_API_KEY=k"}, []string{"ANTHROPIC_AUTH_TOKEN"}},
		{KindCodexCLI, "https://gw/v1", []string{CodexProviderKeyEnv + "=k"}, []string{"OPENAI_API_KEY", "OPENAI_BASE_URL"}},
	}
	for _, tc := range cases {
		o := ProviderOverlay{Enabled: true, AgentKind: tc.kind, ApiKey: "k", ApiURL: tc.url}
		env := strings.Join(o.ExtraEnv(), "\n")
		for _, w := range tc.want {
			if !strings.Contains(env, w) {
				t.Errorf("%s %s: want %s in %s", tc.kind, tc.url, w, env)
			}
		}
		for _, n := range tc.not {
			if strings.Contains(env, n) {
				t.Errorf("%s %s: unexpected %s in %s", tc.kind, tc.url, n, env)
			}
		}
	}
}

func TestMergeProviderEnvStripsParentAuth(t *testing.T) {
	base := []string{"PATH=/usr/bin", "XAI_API_KEY=parent-xai", "GROK_CODE_XAI_API_KEY=parent2", "GROK_MODELS_BASE_URL=parent-url", "HOME=/tmp"}
	o := ProviderOverlay{Enabled: true, AgentKind: KindCatBuild, ApiURL: "https://custom/v1", ApiKey: "sk-custom"}
	joined := strings.Join(MergeProviderEnv(base, o), "\n")
	for _, bad := range []string{"parent-xai", "parent2", "parent-url"} {
		if strings.Contains(joined, bad) {
			t.Fatalf("parent auth leaked: %s", joined)
		}
	}
	if !strings.Contains(joined, "XAI_API_KEY=sk-custom") || !strings.Contains(joined, "PATH=/usr/bin") {
		t.Fatalf("missing overlay/path: %s", joined)
	}
	if same := MergeProviderEnv(base, ProviderOverlay{AgentKind: KindCatBuild}); len(same) != len(base) {
		t.Fatalf("disabled changed env: %v", same)
	}
	c := strings.Join(MergeProviderEnv([]string{"ANTHROPIC_API_KEY=old", "ANTHROPIC_AUTH_TOKEN=old2"},
		ProviderOverlay{Enabled: true, AgentKind: KindClaudeCLI, ApiURL: "https://gw", ApiKey: "new"}), "\n")
	if strings.Contains(c, "old") || !strings.Contains(c, "ANTHROPIC_AUTH_TOKEN=new") {
		t.Fatalf("claude strip: %s", c)
	}
}

func TestCodexProviderArgs(t *testing.T) {
	o := ProviderOverlay{Enabled: true, AgentKind: KindCodexCLI, ApiKey: "secret-key", ApiURL: "https://gw.example.com/v1"}
	a := NewCodexAdapter(CLIAgentConfig{DataTemp: t.TempDir()})
	for _, resume := range []bool{false, true} {
		args := a.buildCodexArgs(TurnOptions{Provider: o, ModelID: "my-model"}, "hi", "/p", "sess", resume)
		j := strings.Join(args, " ")
		for _, w := range []string{`model_provider="ffcat"`, `model_providers.ffcat.base_url="https://gw.example.com/v1"`,
			`model_providers.ffcat.env_key="FFCAT_CODEX_API_KEY"`, `model_providers.ffcat.wire_api="responses"`} {
			if !strings.Contains(j, w) {
				t.Errorf("resume=%v missing %s: %s", resume, w, j)
			}
		}
		if strings.Contains(j, "secret-key") {
			t.Fatalf("key leaked into argv: %s", j)
		}
		snap := "codex-exec.txt"
		if resume {
			snap = "codex-exec-resume.txt"
		}
		assertWhitelisted(t, "codex", snap, args)
		assertCodexConfigKeys(t, args)
	}
	if (ProviderOverlay{AgentKind: KindCodexCLI}).ExtraArgs() != nil {
		t.Fatal("disabled overlay must not add args")
	}
	if (ProviderOverlay{Enabled: true, AgentKind: KindClaudeCLI, ApiURL: "x"}).ExtraArgs() != nil {
		t.Fatal("only codex gets args")
	}
}

// 真机样本：codex 0.162.1 exec 对假服务（POST /v1/responses 返回 404 / 405）的完整输出。
func TestCodexResponsesUnsupportedSamples(t *testing.T) {
	for _, f := range []string{"codex-provider-responses-404.txt", "codex-provider-responses-405.txt"} {
		raw, err := os.ReadFile(filepath.Join("testdata", f))
		if err != nil {
			t.Fatal(err)
		}
		// 只看 turn.failed 那一行（与适配器取 cliMsg 的口径一致）。
		var failed string
		for _, ln := range strings.Split(string(raw), "\n") {
			if strings.Contains(ln, `"type":"turn.failed"`) {
				failed = ln
			}
		}
		if failed == "" || !codexResponsesMissing(failed) {
			t.Fatalf("%s: sample not matched (%q)", f, failed)
		}
	}
	for _, neg := range []string{
		"We’re currently experiencing high demand, which may cause temporary errors.", // 真机：假服务 500
		"unexpected status 401 Unauthorized: bad key, url: http://h/v1/responses",
		"unexpected status 404 Not Found: x, url: http://h/v1/models",
		"",
	} {
		if codexResponsesMissing(neg) {
			t.Errorf("false positive: %q", neg)
		}
	}
	e := providerResponsesUnsupportedErr(KindCodexCLI)
	if !strings.Contains(e.Detail, "reason=provider_responses_unsupported") || !strings.Contains(e.Detail, "agentKind=codex_cli") {
		t.Fatalf("detail: %q", e.Detail)
	}
}

package catagent

import (
	"strings"
	"testing"
)

func TestMergeProviderEnvStripsParentAuth(t *testing.T) {
	base := []string{
		"PATH=/usr/bin",
		"XAI_API_KEY=parent-xai",
		"OPENAI_API_KEY=parent-oai",
		"HOME=/tmp",
	}
	o := ProviderOverlay{
		Enabled:   true,
		AgentKind: KindCatBuild,
		ApiURL:    "https://custom/v1",
		ApiKey:    "sk-custom",
	}
	out := MergeProviderEnv(base, o)
	joined := strings.Join(out, "\n")
	if strings.Contains(joined, "parent-xai") || strings.Contains(joined, "parent-oai") {
		t.Fatalf("parent auth leaked: %v", out)
	}
	if !strings.Contains(joined, "XAI_API_KEY=sk-custom") || !strings.Contains(joined, "PATH=/usr/bin") {
		t.Fatalf("missing overlay/path: %v", out)
	}
	// 未启用：原样。
	same := MergeProviderEnv(base, ProviderOverlay{AgentKind: KindCatBuild})
	if len(same) != len(base) {
		t.Fatalf("disabled changed env: %v", same)
	}
}

func TestExtraEnvByKind(t *testing.T) {
	cases := []struct {
		kind string
		want string
	}{
		{KindClaudeCLI, "ANTHROPIC_API_KEY=k"},
		{KindCodexCLI, "OPENAI_API_KEY=k"},
		{KindCatBuild, "XAI_API_KEY=k"},
	}
	for _, tc := range cases {
		o := ProviderOverlay{Enabled: true, AgentKind: tc.kind, ApiKey: "k", ApiURL: "https://u"}
		env := strings.Join(o.ExtraEnv(), "\n")
		if !strings.Contains(env, tc.want) {
			t.Fatalf("%s: %s", tc.kind, env)
		}
	}
}

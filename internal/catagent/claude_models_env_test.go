package catagent

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestClaudeModelsFor_NoEnvEqualsStatic(t *testing.T) {
	if got := claudeModelsFor(map[string]string{}); !reflect.DeepEqual(got, claudeStaticModels()) {
		t.Fatalf("%+v", got)
	}
}

// 中转把 haiku 指到 claude-sonnet-5：下拉要照实显示，档位按 sonnet-5。
func TestClaudeModelsFor_PinnedAlias(t *testing.T) {
	got := claudeModelsFor(map[string]string{
		"ANTHROPIC_DEFAULT_HAIKU_MODEL": "claude-sonnet-5",
		"ANTHROPIC_DEFAULT_OPUS_MODEL":  "gw-opus", "ANTHROPIC_DEFAULT_OPUS_MODEL_NAME": "Opus via GW",
		"ANTHROPIC_CUSTOM_MODEL_OPTION": "my/model",
	})
	if len(got) != 5 || got[2].ID != "haiku" || got[2].DisplayName != "haiku → claude-sonnet-5" || got[2].DefaultEffort != "high" {
		t.Fatalf("%+v", got)
	}
	if got[0].DisplayName != "opus → Opus via GW" || len(got[0].Efforts) != 0 || got[4].ID != "my/model" {
		t.Fatalf("%+v", got)
	}
}

func TestClaudeModelEnv_SettingsJSONWins(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"env":{"ANTHROPIC_DEFAULT_HAIKU_MODEL":"from-settings","OTHER":"x"}}`), 0o644)
	t.Setenv("ANTHROPIC_DEFAULT_HAIKU_MODEL", "from-env")
	t.Setenv("ANTHROPIC_DEFAULT_SONNET_MODEL", "sonnet-env")
	a := NewClaudeAdapter(CLIAgentConfig{DataTemp: t.TempDir(), ClaudeConfigDir: dir})
	env := a.claudeModelEnv()
	if env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != "from-settings" || env["ANTHROPIC_DEFAULT_SONNET_MODEL"] != "sonnet-env" || env["OTHER"] != "" {
		t.Fatalf("%+v", env)
	}
}

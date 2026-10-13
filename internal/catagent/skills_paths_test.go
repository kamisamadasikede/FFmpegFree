package catagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectCatPaths(t *testing.T) {
	root, mcp := ProjectCatPaths("/proj")
	if !strings.HasSuffix(root, filepath.Join(".ffmpegfree", "cat", "skills")) {
		t.Fatalf("skills: %s", root)
	}
	if !strings.HasSuffix(mcp, filepath.Join(".ffmpegfree", "cat", "mcp.json")) {
		t.Fatalf("mcp: %s", mcp)
	}
	if EnabledStatePath("/proj") != filepath.Join("/proj", ".ffmpegfree", "cat", "enabled.json") &&
		EnabledStatePath("/proj") != filepath.Join(filepath.Clean("/proj"), ".ffmpegfree", "cat", "enabled.json") {
		t.Fatalf("enabled: %s", EnabledStatePath("/proj"))
	}
}

func TestMergeSkillsEnvInjectsGuessKeys(t *testing.T) {
	base := []string{"PATH=/bin", "GROK_SKILLS_DIR=/old"}
	o := SkillsOverlay{AgentKind: KindCatBuild, SkillsRoot: "/p/.ffmpegfree/cat/skills", McpConfigPath: "/p/.ffmpegfree/cat/mcp.json"}
	out := MergeSkillsEnv(base, o)
	joined := strings.Join(out, "\n")
	if strings.Contains(joined, "GROK_SKILLS_DIR=/old") {
		t.Fatalf("old not stripped: %v", out)
	}
	if !strings.Contains(joined, "GROK_SKILLS_DIR=/p/.ffmpegfree/cat/skills") {
		t.Fatalf("missing skills: %v", out)
	}
	if !strings.Contains(joined, "MCP_CONFIG_PATH=") {
		t.Fatalf("missing mcp: %v", out)
	}
	empty := MergeSkillsEnv(base, SkillsOverlay{})
	if len(empty) != len(base) {
		t.Fatalf("empty overlay changed env")
	}
}

func TestListProjectSkillAndMCPNames(t *testing.T) {
	dir := t.TempDir()
	skills := filepath.Join(dir, "skills")
	_ = os.MkdirAll(filepath.Join(skills, "demo-skill"), 0o755)
	_ = os.WriteFile(filepath.Join(skills, "note.md"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(skills, "skip.txt"), []byte("x"), 0o644)
	names := ListProjectSkillNames(skills)
	joined := strings.Join(names, ",")
	if !strings.Contains(joined, "demo-skill") || !strings.Contains(joined, "note") {
		t.Fatalf("%v", names)
	}
	if strings.Contains(joined, "skip") {
		t.Fatalf("txt should skip: %v", names)
	}
	mcp := filepath.Join(dir, "mcp.json")
	_ = os.WriteFile(mcp, []byte(`{"mcpServers":{"alpha":{},"beta":{}}}`), 0o644)
	mcpNames := ListProjectMCPNames(mcp)
	if len(mcpNames) != 2 {
		t.Fatalf("%v", mcpNames)
	}
	if ListProjectMCPNames(filepath.Join(dir, "missing.json")) != nil {
		t.Fatal("missing should be nil/empty")
	}
}

package catagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// 项目内技能 / MCP 路径（契约 v0.39 6.19.12.9；产品锁定）。
const (
	ProjectCatDirName = ".ffmpegfree"
	ProjectCatSubDir  = "cat"
	SkillsDirName     = "skills"
	MCPConfigFileName = "mcp.json"
	EnabledFileName   = "enabled.json" // D7 GUESS：启停旁路文件，三家共用
)

// ProjectCatPaths 返回项目下技能根与 mcp.json 绝对路径（不要求目录 / 文件已存在）。
func ProjectCatPaths(projectAbs string) (skillsRoot, mcpConfigPath string) {
	base := filepath.Join(filepath.Clean(projectAbs), ProjectCatDirName, ProjectCatSubDir)
	return filepath.Join(base, SkillsDirName), filepath.Join(base, MCPConfigFileName)
}

// EnabledStatePath 是项目内启停旁路文件路径（附录 D7 GUESS）。
func EnabledStatePath(projectAbs string) string {
	return filepath.Join(filepath.Clean(projectAbs), ProjectCatDirName, ProjectCatSubDir, EnabledFileName)
}

// SkillsOverlay retains the legacy turn metadata shape. Native links and MCP
// configs are synchronized by the service; these paths are not env variables.
type SkillsOverlay struct {
	AgentKind     string
	SkillsRoot    string
	McpConfigPath string
}

// Native sharing is synchronized before launch; no undocumented env keys.
func (o SkillsOverlay) ExtraEnv() []string                   { return nil }
func MergeSkillsEnv(base []string, o SkillsOverlay) []string { return base }

// ListProjectSkillNames 列技能目录下的一级项名（附录 D5 GUESS：无 CLI inspect 时仍可列项目内技能）。
// 只取子目录名，以及 *.md / *.skill 文件名（去扩展名）；忽略隐藏项与 enabled/mcp 旁路文件。
func ListProjectSkillNames(skillsRoot string) []string {
	skillsRoot = strings.TrimSpace(skillsRoot)
	if skillsRoot == "" {
		return nil
	}
	ents, err := os.ReadDir(skillsRoot)
	if err != nil {
		return nil
	}
	var out []string
	seen := map[string]struct{}{}
	for _, e := range ents {
		name := e.Name()
		if name == "" || strings.HasPrefix(name, ".") {
			continue
		}
		item := name
		if !e.IsDir() {
			ext := strings.ToLower(filepath.Ext(name))
			if ext != ".md" && ext != ".skill" {
				continue
			}
			item = strings.TrimSuffix(name, filepath.Ext(name))
			if item == "" {
				continue
			}
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

// ListProjectMCPNames 从项目 mcp.json 读服务器名（附录 D5 GUESS；格式未确认）。
// 接受顶层 mcpServers / servers 对象的 key；文件不存在或认不出 → 空。
func ListProjectMCPNames(mcpConfigPath string) []string {
	mcpConfigPath = strings.TrimSpace(mcpConfigPath)
	if mcpConfigPath == "" {
		return nil
	}
	raw, err := os.ReadFile(mcpConfigPath)
	if err != nil || len(raw) == 0 {
		return nil
	}
	var top map[string]json.RawMessage
	if json.Unmarshal(raw, &top) != nil || top == nil {
		return nil
	}
	for _, key := range []string{"mcpServers", "servers"} {
		v, ok := top[key]
		if !ok {
			continue
		}
		var obj map[string]json.RawMessage
		if json.Unmarshal(v, &obj) != nil || obj == nil {
			continue
		}
		out := make([]string, 0, len(obj))
		for name := range obj {
			name = strings.TrimSpace(name)
			if name != "" {
				out = append(out, name)
			}
		}
		return out
	}
	return nil
}

// SkillsRootExists 报告技能目录是否存在且为文件夹。
func SkillsRootExists(skillsRoot string) bool {
	st, err := os.Stat(skillsRoot)
	return err == nil && st.IsDir()
}

package catagent

// Native sharing uses each CLI's real discovery paths. No guessed environment
// variables and no replacements of the user's existing skills/configuration.
import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type SharedMCPServer struct {
	Command  string            `json:"command,omitempty"`
	Args     []string          `json:"args,omitempty"`
	Env      map[string]string `json:"env,omitempty"`
	URL      string            `json:"url,omitempty"`
	Type     string            `json:"type,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
	Disabled bool              `json:"disabled,omitempty"`
}

type NativeShareTarget struct {
	AgentKind  string `json:"agentKind"`
	SkillsPath string `json:"skillsPath"`
	ConfigPath string `json:"configPath"`
	Format     string `json:"-"`
}

type NativeShareResult struct {
	NativeShareTarget
	SkillCount int      `json:"skillCount"`
	MCPCount   int      `json:"mcpCount"`
	Warnings   []string `json:"warnings"`
	State      string   `json:"state"`
}

type nativeOwnership struct {
	Links   map[string]string `json:"links"`
	Servers map[string]string `json:"servers"`
}

var nativeShareName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,80}$`)

type NativeShareManifest map[string]nativeOwnership

func NativeShareTargets(home, project string, useEnvironment bool) []NativeShareTarget {
	codexHome, grokHome, claudeHome := filepath.Join(home, ".codex"), filepath.Join(home, ".grok"), filepath.Join(home, ".claude")
	claudeConfig := filepath.Join(home, ".claude.json")
	if useEnvironment {
		if v := os.Getenv("CODEX_HOME"); v != "" {
			codexHome = v
		}
		if v := os.Getenv("GROK_HOME"); v != "" {
			grokHome = v
		}
		if v := os.Getenv("CLAUDE_CONFIG_DIR"); v != "" {
			claudeHome = v
			claudeConfig = filepath.Join(v, ".claude.json")
		}
	}
	if project != "" {
		return []NativeShareTarget{
			{KindCatBuild, filepath.Join(project, ".grok", "skills"), filepath.Join(project, ".grok", "config.toml"), "toml"},
			{KindClaudeCLI, filepath.Join(project, ".claude", "skills"), filepath.Join(project, ".mcp.json"), "json"},
			{KindCodexCLI, filepath.Join(project, ".agents", "skills"), filepath.Join(project, ".codex", "config.toml"), "toml"},
		}
	}
	return []NativeShareTarget{
		{KindCatBuild, filepath.Join(grokHome, "skills"), filepath.Join(grokHome, "config.toml"), "toml"},
		{KindClaudeCLI, filepath.Join(claudeHome, "skills"), claudeConfig, "json"},
		{KindCodexCLI, filepath.Join(home, ".agents", "skills"), filepath.Join(codexHome, "config.toml"), "toml"},
	}
}

func ShareFingerprint(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func NativeServer(kind string, server SharedMCPServer) map[string]any {
	m := map[string]any{}
	if server.Command != "" {
		m["command"] = server.Command
		if len(server.Args) > 0 {
			m["args"] = server.Args
		}
		if len(server.Env) > 0 {
			m["env"] = server.Env
		}
	}
	if server.URL != "" {
		m["url"] = server.URL
	}
	if kind == KindClaudeCLI {
		if server.URL != "" {
			t := server.Type
			if t == "" {
				t = "http"
			}
			m["type"] = t
		}
		if len(server.Headers) > 0 {
			m["headers"] = server.Headers
		}
	} else {
		m["enabled"] = !server.Disabled
		if server.URL != "" && kind == KindCatBuild {
			t := server.Type
			if t == "" {
				t = "http"
			}
			if t == "sse" {
				m["type"] = t
			}
		}
		if len(server.Headers) > 0 {
			if kind == KindCodexCLI {
				m["http_headers"] = server.Headers
			} else {
				m["headers"] = server.Headers
			}
		}
	}
	return m
}

func loadNative(t NativeShareTarget) (map[string]any, error) {
	if st, e := os.Lstat(t.ConfigPath); e == nil && st.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
		return nil, fmt.Errorf("配置是外部链接，未修改")
	}
	raw, err := os.ReadFile(t.ConfigPath)
	if os.IsNotExist(err) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(raw) > 4<<20 {
		return nil, fmt.Errorf("配置文件过大，未修改")
	}
	out := map[string]any{}
	if t.Format == "json" {
		err = json.Unmarshal(raw, &out)
	} else {
		err = toml.Unmarshal(raw, &out)
	}
	if err != nil || out == nil {
		return nil, fmt.Errorf("已有配置无法解析，未修改")
	}
	return out, nil
}

func nativeServers(t NativeShareTarget, config map[string]any) (map[string]any, error) {
	key := "mcp_servers"
	if t.Format == "json" {
		key = "mcpServers"
	}
	if value, ok := config[key]; ok {
		m, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("已有 MCP 配置格式不兼容，未修改")
		}
		return m, nil
	}
	m := map[string]any{}
	config[key] = m
	return m, nil
}

func WriteShareFile(path string, raw []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".ffc-write-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(raw)
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(name, path)
}

func writeNative(t NativeShareTarget, config map[string]any, originalHash string) error {
	current, err := loadNative(t)
	if err != nil {
		return err
	}
	if ShareFingerprint(current) != originalHash {
		return fmt.Errorf("配置同步期间被修改，请重试")
	}
	var raw []byte
	if t.Format == "json" {
		raw, err = json.MarshalIndent(config, "", "  ")
	} else {
		raw, err = toml.Marshal(config)
	}
	if err != nil {
		return err
	}
	if original, readErr := os.ReadFile(t.ConfigPath); readErr == nil {
		backup := t.ConfigPath + ".ffc-backup"
		if _, err := os.Lstat(backup); os.IsNotExist(err) {
			if err := WriteShareFile(backup, original); err != nil {
				return err
			}
		}
	}
	return WriteShareFile(t.ConfigPath, raw)
}

func sharedLinkTarget(path string) string {
	// Readlink resolves Windows directory junctions and dangling owned links.
	st, err := os.Lstat(path)
	if err != nil {
		return ""
	}
	if st.Mode()&os.ModeSymlink == 0 && st.Mode()&os.ModeIrregular == 0 {
		return ""
	}
	v, err := os.Readlink(path)
	if err != nil {
		return ""
	}
	if !filepath.IsAbs(v) {
		v = filepath.Join(filepath.Dir(path), v)
	}
	return filepath.Clean(v)
}

func sameSharePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	aa, e := os.Stat(a)
	bb, f := os.Stat(b)
	return e == nil && f == nil && os.SameFile(aa, bb)
}

// Only links/config entries recorded by ffc and still matching that record can
// be removed or replaced. A user edit turns the entry into a visible conflict.
func SyncNativeShare(t NativeShareTarget, skills map[string]string, servers map[string]SharedMCPServer, owned nativeOwnership, sync bool) (NativeShareResult, nativeOwnership) {
	// Disabled shared servers are removed from owned native entries. No CLI
	// specific, undocumented "disabled" property is required.
	active := make(map[string]SharedMCPServer, len(servers))
	for name, server := range servers {
		if !server.Disabled {
			active[name] = server
		}
	}
	servers = active
	r := NativeShareResult{NativeShareTarget: t, Warnings: []string{}, State: "synced"}
	if owned.Links == nil {
		owned.Links = map[string]string{}
	}
	if owned.Servers == nil {
		owned.Servers = map[string]string{}
	}
	for name, target := range owned.Links {
		if desired, ok := skills[name]; ok && sameSharePath(desired, target) {
			continue
		}
		dest := filepath.Join(t.SkillsPath, name)
		if sameSharePath(sharedLinkTarget(dest), target) {
			if sync {
				if err := os.Remove(dest); err != nil {
					r.Warnings = append(r.Warnings, "无法移除已停用的技能链接："+name)
					continue
				}
				delete(owned.Links, name)
			} else {
				r.State = "pending"
			}
		} else if _, err := os.Lstat(dest); os.IsNotExist(err) {
			if sync {
				delete(owned.Links, name)
			}
		} else {
			r.Warnings = append(r.Warnings, "技能链接已由外部修改，已保留："+name)
		}
	}
	names := make([]string, 0, len(skills))
	for name := range skills {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		dest, target := filepath.Join(t.SkillsPath, name), skills[name]
		if sameSharePath(sharedLinkTarget(dest), target) {
			if !sameSharePath(owned.Links[name], target) {
				r.Warnings = append(r.Warnings, "已有同名技能链接，不接管外部内容："+name)
			}
			r.SkillCount++
			continue
		}
		if _, err := os.Lstat(dest); err == nil {
			r.Warnings = append(r.Warnings, "已有同名技能，已保留原目录："+name)
			continue
		} else if !os.IsNotExist(err) {
			r.Warnings = append(r.Warnings, "无法读取技能目录："+name)
			continue
		}
		if !sync {
			r.State = "pending"
			continue
		}
		if err := os.MkdirAll(t.SkillsPath, 0700); err != nil {
			r.Warnings = append(r.Warnings, "无法创建技能目录")
			continue
		}
		if err := createSharedDirectoryLink(dest, target); err != nil {
			r.Warnings = append(r.Warnings, "无法创建技能链接："+name)
			continue
		}
		owned.Links[name] = target
		r.SkillCount++
	}
	if len(servers) > 0 || len(owned.Servers) > 0 {
		config, err := loadNative(t)
		if err != nil {
			r.Warnings = append(r.Warnings, err.Error())
		} else {
			originalHash := ShareFingerprint(config)
			entries, err := nativeServers(t, config)
			if err != nil {
				r.Warnings = append(r.Warnings, err.Error())
			} else {
				changed := false
				next := map[string]string{}
				for k, v := range owned.Servers {
					next[k] = v
				}
				for name, hash := range owned.Servers {
					if _, ok := servers[name]; !ok {
						if entry, ok := entries[name]; !ok {
							delete(next, name)
						} else if ShareFingerprint(entry) == hash {
							if sync {
								delete(entries, name)
								delete(next, name)
								changed = true
							} else {
								r.State = "pending"
							}
						} else {
							r.Warnings = append(r.Warnings, "MCP 配置已由外部修改，已保留："+name)
						}
					}
				}
				for name, server := range servers {
					if t.AgentKind == KindCodexCLI && server.Type == "sse" {
						r.Warnings = append(r.Warnings, "Codex 不支持旧式 SSE，请改用 HTTP："+name)
						continue
					}
					value := NativeServer(t.AgentKind, server)
					hash := ShareFingerprint(value)
					if entry, ok := entries[name]; ok {
						current := ShareFingerprint(entry)
						if current == hash {
							if owned.Servers[name] != current {
								r.Warnings = append(r.Warnings, "已有相同 MCP 配置，不接管外部内容："+name)
							}
							r.MCPCount++
							continue
						}
						if owned.Servers[name] != current {
							r.Warnings = append(r.Warnings, "已有同名或外部修改的 MCP，已保留："+name)
							continue
						}
					}
					if !sync {
						r.State = "pending"
						continue
					}
					entries[name] = value
					next[name] = hash
					changed = true
					r.MCPCount++
				}
				if changed {
					if err := writeNative(t, config, originalHash); err != nil {
						r.Warnings = append(r.Warnings, "写入 MCP 配置失败，原配置已保留")
						r.MCPCount = 0
					} else {
						owned.Servers = next
					}
				} else if sync {
					owned.Servers = next
				}
			}
		}
	}
	if len(r.Warnings) > 0 {
		r.State = "error"
	}
	return r, owned
}

func LoadNativeShareManifest(base string) (NativeShareManifest, error) {
	return LoadNativeShareManifestWithSkills(base, nil)
}

// Built-in skills live outside the user shared library. Only application-provided
// exact name/path pairs may extend the normal ownership validation.
func LoadNativeShareManifestWithSkills(base string, builtin map[string]string) (NativeShareManifest, error) {
	out := NativeShareManifest{}
	raw, err := os.ReadFile(filepath.Join(base, "native-sync.json"))
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if len(raw) > 4<<20 || json.Unmarshal(raw, &out) != nil || out == nil {
		return nil, fmt.Errorf("同步记录无法解析，请恢复 native-sync.json 后重试")
	}
	for kind, owned := range out {
		if kind != KindCatBuild && kind != KindClaudeCLI && kind != KindCodexCLI {
			return nil, fmt.Errorf("同步记录包含未知 CLI")
		}
		for name, target := range owned.Links {
			if !nativeShareName.MatchString(name) || !filepath.IsAbs(target) || (!sameSharePath(target, filepath.Join(base, "skills", name)) && !sameSharePath(target, builtin[name])) {
				return nil, fmt.Errorf("同步记录的技能路径无效，请修复 native-sync.json")
			}
		}
		for name, hash := range owned.Servers {
			if !strings.HasPrefix(name, "ffc_") || !nativeShareName.MatchString(name) || len(hash) != 64 {
				return nil, fmt.Errorf("同步记录的 MCP 标记无效，请修复 native-sync.json")
			}
		}
	}
	return out, nil
}

func SaveNativeShareManifest(base string, m NativeShareManifest) error {
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return WriteShareFile(filepath.Join(base, "native-sync.json"), raw)
}

func ShareNativeName(name string) string { return "ffc_" + strings.TrimSpace(name) }

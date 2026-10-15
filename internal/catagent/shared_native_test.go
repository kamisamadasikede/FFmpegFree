package catagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func shareWrite(t *testing.T, path, text string) {
	t.Helper()
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(path, []byte(text), 0600); e != nil {
		t.Fatal(e)
	}
}
func TestNativeSharingLifecycle(t *testing.T) {
	for _, target := range NativeShareTargets(t.TempDir(), "", false) {
		t.Run(target.AgentKind, func(t *testing.T) {
			source := filepath.Join(t.TempDir(), "skill")
			shareWrite(t, filepath.Join(source, "SKILL.md"), "original")
			original := "model = \"preserved\"\n[mcp_servers.existing]\ncommand = \"existing-cli\"\n"
			if target.Format == "json" {
				original = `{"theme":"preserved","mcpServers":{"existing":{"command":"existing-cli"}}}`
			}
			shareWrite(t, target.ConfigPath, original)
			desired := map[string]SharedMCPServer{"ffc_demo": {Command: "test-only-no-launch", Args: []string{"a b", "c"}, Env: map[string]string{"TOKEN": "test-not-secret"}}}
			skills := map[string]string{"demo": source}
			pending, empty := SyncNativeShare(target, skills, desired, nativeOwnership{}, false)
			if pending.State != "pending" || len(empty.Servers) > 0 {
				t.Fatalf("inspect mutated: %+v", pending)
			}
			if _, e := os.Stat(target.ConfigPath + ".ffc-backup"); !os.IsNotExist(e) {
				t.Fatal("inspection wrote backup")
			}
			result, owned := SyncNativeShare(target, skills, desired, nativeOwnership{}, true)
			if result.State != "synced" || result.SkillCount != 1 || result.MCPCount != 1 {
				t.Fatalf("%+v", result)
			}
			linked := filepath.Join(target.SkillsPath, "demo", "SKILL.md")
			b, e := os.ReadFile(linked)
			if e != nil || string(b) != "original" {
				t.Fatalf("link not readable %s %v", b, e)
			}
			shareWrite(t, filepath.Join(source, "SKILL.md"), "changed")
			b, _ = os.ReadFile(linked)
			if string(b) != "changed" {
				t.Fatal("skill was copied instead of linked")
			}
			backup, _ := os.ReadFile(target.ConfigPath + ".ffc-backup")
			if string(backup) != original {
				t.Fatal("backup changed")
			}
			result, owned = SyncNativeShare(target, skills, desired, owned, true)
			if result.State != "synced" {
				t.Fatalf("idempotence: %+v", result)
			}
			cfg, e := loadNative(target)
			if e != nil {
				t.Fatal(e)
			}
			entries, e := nativeServers(target, cfg)
			if e != nil {
				t.Fatal(e)
			}
			if entries["existing"] == nil || entries["ffc_demo"] == nil {
				t.Fatal("lost config")
			}
			server := desired["ffc_demo"]
			server.Disabled = true
			desired["ffc_demo"] = server
			result, _ = SyncNativeShare(target, nil, desired, owned, true)
			if result.State != "synced" {
				t.Fatalf("disable: %+v", result)
			}
			if _, e = os.Lstat(filepath.Join(target.SkillsPath, "demo")); !os.IsNotExist(e) {
				t.Fatalf("link not removed: %v", e)
			}
			if _, e = os.Stat(filepath.Join(source, "SKILL.md")); e != nil {
				t.Fatal("deleted original skill")
			}
			cfg, _ = loadNative(target)
			entries, _ = nativeServers(target, cfg)
			if entries["existing"] == nil || entries["ffc_demo"] != nil {
				t.Fatal("wrong MCP removed")
			}
		})
	}
}
func TestNativeSharingConflictsAndExternalEdits(t *testing.T) {
	target := NativeShareTargets(t.TempDir(), "", false)[2]
	source := t.TempDir()
	shareWrite(t, filepath.Join(source, "SKILL.md"), "canonical")
	shareWrite(t, filepath.Join(target.SkillsPath, "demo", "SKILL.md"), "native")
	shareWrite(t, target.ConfigPath, "[mcp_servers.ffc_demo]\ncommand = \"native-command\"\n")
	want := map[string]SharedMCPServer{"ffc_demo": {Command: "shared-command"}}
	r, owned := SyncNativeShare(target, map[string]string{"demo": source}, want, nativeOwnership{}, true)
	if r.State != "error" || len(r.Warnings) != 2 || len(owned.Servers) != 0 || len(owned.Links) != 0 {
		t.Fatalf("%+v %+v", r, owned)
	}
	b, _ := os.ReadFile(filepath.Join(target.SkillsPath, "demo", "SKILL.md"))
	if string(b) != "native" {
		t.Fatal("overwrote native skill")
	}
	target = NativeShareTargets(t.TempDir(), "", false)[2]
	_, owned = SyncNativeShare(target, nil, want, nativeOwnership{}, true)
	shareWrite(t, target.ConfigPath, "[mcp_servers.ffc_demo]\ncommand = \"external-edit\"\n")
	r, _ = SyncNativeShare(target, nil, nil, owned, true)
	b, _ = os.ReadFile(target.ConfigPath)
	if r.State != "error" || !strings.Contains(string(b), "external-edit") {
		t.Fatal("removed external edit")
	}
}
func TestNativeSharingDoesNotAdoptUnownedContent(t *testing.T) {
	target := NativeShareTargets(t.TempDir(), "", false)[1]
	source := t.TempDir()
	shareWrite(t, filepath.Join(source, "SKILL.md"), "native")
	if e := os.MkdirAll(target.SkillsPath, 0700); e != nil {
		t.Fatal(e)
	}
	if e := createSharedDirectoryLink(filepath.Join(target.SkillsPath, "demo"), source); e != nil {
		t.Fatal(e)
	}
	want := map[string]SharedMCPServer{"ffc_demo": {Command: "same-command"}}
	_, created := SyncNativeShare(target, nil, want, nativeOwnership{}, true)
	if len(created.Servers) != 1 {
		t.Fatal("setup failed")
	}
	r, owned := SyncNativeShare(target, map[string]string{"demo": source}, want, nativeOwnership{}, true)
	if r.State != "error" || len(owned.Servers) != 0 || len(owned.Links) != 0 {
		t.Fatalf("adopted: %+v %+v", r, owned)
	}
	SyncNativeShare(target, nil, nil, owned, true)
	if _, e := os.Stat(filepath.Join(target.SkillsPath, "demo")); e != nil {
		t.Fatal("removed foreign link")
	}
}
func TestNativeSharingInvalidConfigAndOptimisticWrite(t *testing.T) {
	target := NativeShareTargets(t.TempDir(), "", false)[2]
	shareWrite(t, target.ConfigPath, "broken = [")
	r, _ := SyncNativeShare(target, nil, map[string]SharedMCPServer{"ffc_demo": {Command: "test"}}, nativeOwnership{}, true)
	b, _ := os.ReadFile(target.ConfigPath)
	if r.State != "error" || string(b) != "broken = [" {
		t.Fatal("replaced invalid file")
	}
	shareWrite(t, target.ConfigPath, "model = \"first\"\n")
	cfg, _ := loadNative(target)
	hash := ShareFingerprint(cfg)
	shareWrite(t, target.ConfigPath, "model = \"external\"\n")
	cfg["model"] = "ours"
	if e := writeNative(target, cfg, hash); e == nil {
		t.Fatal("overwrote concurrent edit")
	}
	if _, e := LoadNativeShareManifest(t.TempDir()); e != nil {
		t.Fatal(e)
	}
	base := t.TempDir()
	shareWrite(t, filepath.Join(base, "native-sync.json"), "broken")
	if _, e := LoadNativeShareManifest(base); e == nil {
		t.Fatal("invalid manifest accepted")
	}
	shareWrite(t, filepath.Join(base, "native-sync.json"), `{"codex_cli":{"links":{"../outside":"C:/outside"},"servers":{}}}`)
	if _, e := LoadNativeShareManifest(base); e == nil {
		t.Fatal("manifest traversal accepted")
	}
}
func TestNativeSharingTransportConversion(t *testing.T) {
	for _, kind := range []string{KindCatBuild, KindClaudeCLI, KindCodexCLI} {
		value := NativeServer(kind, SharedMCPServer{URL: "https://example.invalid/mcp", Headers: map[string]string{"X-Test": "dummy"}})
		if value["transport"] != nil {
			t.Fatal("unsupported transport key")
		}
		if kind == KindCodexCLI && value["http_headers"] == nil {
			t.Fatal("missing Codex header conversion")
		}
		if kind == KindClaudeCLI && value["type"] != "http" {
			t.Fatal("missing Claude type")
		}
	}
	target := NativeShareTargets(t.TempDir(), "", false)[2]
	r, _ := SyncNativeShare(target, nil, map[string]SharedMCPServer{"ffc_sse": {URL: "https://example.invalid/sse", Type: "sse"}}, nativeOwnership{}, true)
	if r.State != "error" || r.MCPCount != 0 {
		t.Fatal("silently accepted SSE")
	}
}

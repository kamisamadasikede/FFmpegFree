package catagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStructuredArgs_Claude(t *testing.T) {
	args := BuildStructuredClaudeArgs("生成职责", CharterSchema, true, true)
	assertWhitelisted(t, "claude -p", "claude.txt", args)
	joined := strings.Join(args, "\x00")
	for _, want := range []string{"-p", "--output-format", "json", "--json-schema", "--no-session-persistence", "--tools"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %q", want, args)
		}
	}
	// tools value is empty string after --tools
	for i, a := range args {
		if a == "--tools" {
			if i+1 >= len(args) || args[i+1] != "" {
				t.Fatalf("--tools should be followed by empty string: %q", args)
			}
		}
	}
}

func TestStructuredArgs_Codex(t *testing.T) {
	dir := t.TempDir()
	schemaPath := filepath.Join(dir, "schema.json")
	if err := os.WriteFile(schemaPath, []byte(RouteSchema), 0o600); err != nil {
		t.Fatal(err)
	}
	args := BuildStructuredCodexArgs("挑人", "/proj", schemaPath, true, true, "群规")
	assertWhitelisted(t, "codex exec", "codex-exec.txt", args)
	assertCodexConfigKeys(t, args)
	joined := strings.Join(args, " ")
	for _, want := range []string{"exec", "--json", "--ephemeral", "--output-schema", schemaPath, "--sandbox", "read-only"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %q", want, args)
		}
	}
	hasDev := false
	for i, a := range args {
		if a == "-c" && i+1 < len(args) && strings.HasPrefix(args[i+1], "developer_instructions=") {
			hasDev = true
		}
	}
	if !hasDev {
		t.Fatalf("expected developer_instructions -c: %q", args)
	}
}

func TestStructuredArgs_Grok(t *testing.T) {
	args := BuildStructuredGrokArgs("--prompt-file", "/tmp/p.txt", "/proj", RouteSchema, true, "规则")
	assertWhitelisted(t, "grok", "grok.txt", args)
	joined := strings.Join(args, "\x00")
	for _, want := range []string{"--output-format", "json", "--json-schema", "--tools", "--rules="} {
		if !strings.Contains(joined, want) && want != "--rules=" {
			if want == "--rules=" && !hasRulesArg(args) {
				t.Fatalf("missing rules in %q", args)
			} else if want != "--rules=" {
				t.Fatalf("missing %q in %q", want, args)
			}
		}
	}
	if hasRulesArg(args) == false {
		t.Fatalf("missing --rules in %q", args)
	}
	for _, a := range args {
		if a == "--session-id" || a == "--resume" {
			t.Fatalf("ephemeral grok must not set session flags: %q", args)
		}
	}
}

func hasRulesArg(args []string) bool {
	for _, a := range args {
		if strings.HasPrefix(a, "--rules=") || a == "--rules" {
			return true
		}
	}
	return false
}

func TestMaterializeCodexOutputSchema(t *testing.T) {
	opts, clean, err := materializeCodexOutputSchema(TurnOptions{JSONSchema: `{"type":"object"}`})
	if err != nil {
		t.Fatal(err)
	}
	defer clean()
	if !strings.HasSuffix(opts.JSONSchema, ".json") {
		t.Fatalf("expected file path, got %q", opts.JSONSchema)
	}
	b, err := os.ReadFile(opts.JSONSchema)
	if err != nil || string(b) != `{"type":"object"}` {
		t.Fatalf("schema file: %v %q", err, b)
	}
	// path passthrough
	opts2, clean2, err := materializeCodexOutputSchema(TurnOptions{JSONSchema: "/tmp/x.json"})
	if err != nil || clean2 != nil || opts2.JSONSchema != "/tmp/x.json" {
		t.Fatalf("passthrough: schema=%q cleanNil=%v err=%v", opts2.JSONSchema, clean2 == nil, err)
	}
}

func TestHelpWhitelist_StructuredFlagsPresent(t *testing.T) {
	// Sanity: live help snapshots include the flags we rely on for v0.55.
	for _, tc := range []struct {
		snap, flag string
	}{
		{"claude.txt", "--json-schema"},
		{"claude.txt", "--no-session-persistence"},
		{"claude.txt", "--tools"},
		{"codex-exec.txt", "--ephemeral"},
		{"codex-exec.txt", "--output-schema"},
		{"grok.txt", "--json-schema"},
		{"grok.txt", "--tools"},
		{"grok.txt", "--rules"},
	} {
		set := loadHelpFlags(t, tc.snap)
		if !set[tc.flag] {
			t.Errorf("%s missing %s", tc.snap, tc.flag)
		}
	}
}

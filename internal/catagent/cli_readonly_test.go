package catagent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// 写 / 编辑 / 执行类工具 ID，白名单里绝不能出现。
var forbiddenInAllowlist = []string{
	CLIToolShell, CLIToolSearchReplace, CLIToolWriteFile, CLIToolApplyPatch,
	CLIToolTask, CLIToolAgent, CLIToolWebFetch, CLIToolWebSearch,
	CLIToolMCPSearch, CLIToolMCPUse, CLIToolImageGen, CLIToolImageEdit, CLIToolVideoGen,
	CLIToolAliasBash, CLIToolAliasEdit, CLIToolAliasWrite, CLIToolHashlineEdit, CLIToolRunTerminalCommandAlias,
	"Bash", "Edit", "Write",
}

func TestDeniedCLITools_IncludesAliases(t *testing.T) {
	for _, must := range []string{"bash", "edit", "write", "hashline_edit", "run_terminal_command"} {
		in := false
		for _, d := range DeniedCLITools {
			in = in || d == must
		}
		if !in {
			t.Fatalf("denylist missing alias %q", must)
		}
		for _, a := range ReadOnlyCLITools {
			if a == must {
				t.Fatalf("allowlist must not contain %q", must)
			}
		}
	}
	if got := strings.Join(ReadOnlyCLITools, ","); got != "read_file,list_dir,grep" {
		t.Fatalf("allowlist changed: %s", got)
	}
}

func TestReadOnlyCLITools_NoWriteEditExec(t *testing.T) {
	for _, tool := range ReadOnlyCLITools {
		for _, bad := range forbiddenInAllowlist {
			if tool == bad {
				t.Fatalf("allowlist contains forbidden tool %q", tool)
			}
		}
	}
	for _, must := range []string{CLIToolShell, CLIToolSearchReplace, CLIToolWriteFile, CLIToolApplyPatch, CLIToolTask, CLIToolAgent} {
		found := false
		for _, d := range DeniedCLITools {
			found = found || d == must
		}
		if !found {
			t.Fatalf("denylist missing %q", must)
		}
	}
}

// 用假二进制逐行记录 argv，断言首轮与续聊都带只读限制，且仍保留 --always-approve。
func TestRunTurn_PassesReadOnlyToolArgs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix script fake")
	}
	dir := t.TempDir()
	fake := filepath.Join(dir, "grok-args")
	out := filepath.Join(dir, "argv.txt")
	script := "#!/bin/sh\n: > '" + out + "'\nfor a in \"$@\"; do printf '%s\\n' \"$a\" >> '" + out + "'; done\n" +
		"printf '%s\\n' '{\"type\":\"text\",\"data\":\"ok\"}'\nprintf '%s\\n' '{\"type\":\"end\",\"stopReason\":\"end_turn\"}'\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewBuildAdapter(BuildConfig{
		DataTemp: t.TempDir(),
		LookPath: func(string) (string, error) { return fake, nil },
		Exec: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, name, args...)
		},
	})
	a.detect()

	turns := [][]WireMessage{
		{{Role: "user", Content: "first"}},
		{{Role: "user", Content: "first"}, {Role: "assistant", Content: "ok"}, {Role: "user", Content: "second"}},
	}
	for i, msgs := range turns {
		if _, err := a.RunTurn(TurnOptions{Ctx: context.Background(), ConversationID: "conv-ro", Messages: msgs}); err != nil {
			t.Fatalf("turn %d: %v", i, err)
		}
		raw, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		argv := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
		val := func(flag string) []string {
			var vs []string
			for j := 0; j+1 < len(argv); j++ {
				if argv[j] == flag {
					vs = append(vs, argv[j+1])
				}
			}
			return vs
		}
		has := func(flag string) bool {
			for _, a := range argv {
				if a == flag {
					return true
				}
			}
			return false
		}
		if !has("--always-approve") || !has("--no-subagents") || !has("--disable-web-search") {
			t.Fatalf("turn %d: missing flags:\n%s", i, raw)
		}
		tools := val("--tools")
		if len(tools) != 1 || tools[0] != "read_file,list_dir,grep" {
			t.Fatalf("turn %d: --tools = %v", i, tools)
		}
		for _, tool := range strings.Split(tools[0], ",") {
			for _, bad := range forbiddenInAllowlist {
				if tool == bad {
					t.Fatalf("turn %d: allowlist has %q", i, tool)
				}
			}
		}
		dis := val("--disallowed-tools")
		if len(dis) != 1 {
			t.Fatalf("turn %d: --disallowed-tools = %v", i, dis)
		}
		for _, must := range []string{"run_terminal_cmd", "search_replace", "write_file", "apply_patch", "task", "Agent", "web_fetch", "use_tool", "bash", "edit", "write", "hashline_edit", "run_terminal_command"} {
			if !strings.Contains(","+dis[0]+",", ","+must+",") {
				t.Fatalf("turn %d: denylist %q missing %q", i, dis[0], must)
			}
		}
		deny := strings.Join(val("--deny"), "|")
		for _, must := range []string{"Bash", "Edit", "Write", "WebFetch", "MCPTool(*)"} {
			if !strings.Contains("|"+deny+"|", "|"+must+"|") {
				t.Fatalf("turn %d: --deny %q missing %q", i, deny, must)
			}
		}
	}
}

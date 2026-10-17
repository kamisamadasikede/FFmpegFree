package catagent

import (
	"encoding/json"
	"strconv"
)

// A native permission request may approve only these app-owned endpoints. Shell,
// filesystem and third-party MCP requests retain the employee's chosen policy.
func isTeamNativeTool(name string) bool {
	for _, tool := range []string{"team_members", "team_member", "team_publish", "team_delegate", "team_ask", "team_report", "team_task_status", "team_read_messages", "team_search_messages", "team_steer", "team_yield", "team_finish", "team_open_file", "office_open", "office_read", "office_guide", "office_page", "office_apply", "office_save", "office_audit"} {
		if name == "ffc_team__"+tool {
			return true
		}
	}
	return false
}

func isTeamNativePermission(input map[string]any) bool {
	name, _ := input["tool_name"].(string)
	if !isTeamNativeTool(name) {
		return false
	}
	// ACP adds the native enum discriminator to use_tool's inline arguments.
	// Never allow a file indirection, shell variant, or an unknown new field.
	if variant, ok := input["variant"]; ok && variant != "UseTool" {
		return false
	}
	if args, ok := input["tool_input"]; ok {
		if _, ok := args.(map[string]any); !ok {
			return false
		}
	}
	for key := range input {
		if key != "variant" && key != "tool_name" && key != "tool_input" {
			return false
		}
	}
	return true
}

func teamNativeArgs(kind string, opts TurnOptions) []string {
	exe := opts.ToolEnv["FFC_TEAM_CLI"]
	if exe == "" || opts.ToolEnv["FFC_TEAM_TOKEN"] == "" {
		return nil
	}
	switch kind {
	case KindCodexCLI:
		return []string{"-c", "mcp_servers.ffc_team.command=" + strconv.Quote(exe), "-c", `mcp_servers.ffc_team.args=["team-mcp"]`, "-c", `mcp_servers.ffc_team.env_vars=["FFC_TEAM_CLI","FFC_TEAM_URL","FFC_TEAM_TOKEN"]`, "-c", `mcp_servers.ffc_team.default_tools_approval_mode="approve"`}
	case KindClaudeCLI:
		b, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{"ffc_team": map[string]any{"command": exe, "args": []string{"team-mcp"}, "env": map[string]string{"FFC_TEAM_CLI": "${FFC_TEAM_CLI}", "FFC_TEAM_URL": "${FFC_TEAM_URL}", "FFC_TEAM_TOKEN": "${FFC_TEAM_TOKEN}"}}}})
		return []string{"--mcp-config", string(b), "--allowedTools", "mcp__ffc_team__*"}
	}
	return nil
}
func grokTeamMCP(opts TurnOptions) []any {
	exe := opts.ToolEnv["FFC_TEAM_CLI"]
	if exe == "" {
		return []any{}
	}
	env := []map[string]string{}
	for _, name := range []string{"FFC_TEAM_CLI", "FFC_TEAM_URL", "FFC_TEAM_TOKEN"} {
		env = append(env, map[string]string{"name": name, "value": opts.ToolEnv[name]})
	}
	return []any{map[string]any{"name": "ffc_team", "command": exe, "args": []string{"team-mcp"}, "env": env}}
}
func appendTeamArgsBeforePrompt(kind string, args []string, opts TurnOptions) []string {
	extra := teamNativeArgs(kind, opts)
	if len(extra) == 0 {
		return args
	}
	if kind == KindCodexCLI && len(args) > 0 {
		prompt := args[len(args)-1]
		args = append(args[:len(args)-1], extra...)
		return append(args, prompt)
	}
	return append(args, extra...)
}

package catagent

import "testing"

func TestTeamNativePermissionScope(t *testing.T) {
	for _, input := range []map[string]any{
		{"variant": "UseTool", "tool_name": "ffc_team__team_members", "tool_input": map[string]any{}},
		{"tool_name": "ffc_team__office_apply", "tool_input": map[string]any{"documentId": "native-document"}},
	} {
		if !isTeamNativePermission(input) {
			t.Fatalf("app-owned inline tool was rejected: %#v", input)
		}
	}
	for _, input := range []map[string]any{
		{"variant": "Shell", "tool_name": "ffc_team__team_members"},
		{"variant": "UseTool", "tool_name": "other_server__team_members"},
		{"variant": "UseTool", "tool_name": "ffc_team__unlisted_tool"},
		{"tool_name": "ffc_team__team_members", "tool_input_file": "arguments.json"},
		{"tool_name": "ffc_team__team_members", "file": "other-tool.json"},
		{"tool_name": "ffc_team__team_members", "tool_input": "not-an-object"},
		{"tool_name": "ffc_team__team_members", "command": "shell"},
	} {
		if isTeamNativePermission(input) {
			t.Fatalf("untrusted native request was accepted: %#v", input)
		}
	}
}

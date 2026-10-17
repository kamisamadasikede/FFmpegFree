package catagent

import (
	"os"
	"strings"
)

// CharterSchema is the JSON Schema for role-spec generation (契约 6.21.4).
const CharterSchema = `{
  "type": "object",
  "properties": {
    "owns": {"type": "string"},
    "notTouch": {"type": "string"},
    "handoff": {"type": "string"},
    "dirs": {"type": "array", "items": {"type": "string"}}
  },
  "required": ["owns", "notTouch", "handoff", "dirs"],
  "additionalProperties": false
}`

// RouteSchema is the JSON Schema for dispatch judgment (契约 6.21.6).
const RouteSchema = `{
  "type": "object",
  "properties": {
    "memberId": {"type": "string"},
    "reason": {"type": "string", "maxLength": 20}
  },
  "required": ["memberId", "reason"],
  "additionalProperties": false
}`

// materializeCodexOutputSchema writes JSON Schema text to a temp file when needed.
// Codex --output-schema expects a path; Claude/Grok take the schema string inline.
func materializeCodexOutputSchema(opts TurnOptions) (TurnOptions, func(), error) {
	schema := strings.TrimSpace(opts.JSONSchema)
	if schema == "" {
		return opts, nil, nil
	}
	if !strings.HasPrefix(schema, "{") && !strings.HasPrefix(schema, "[") {
		// Already a path.
		return opts, nil, nil
	}
	f, err := os.CreateTemp("", "ffc-output-schema-*.json")
	if err != nil {
		return opts, nil, err
	}
	path := f.Name()
	if _, err := f.WriteString(schema); err != nil {
		f.Close()
		os.Remove(path)
		return opts, nil, err
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return opts, nil, err
	}
	opts.JSONSchema = path
	return opts, func() { _ = os.Remove(path) }, nil
}

// BuildStructuredClaudeArgs builds Claude argv for a one-shot structured call (tests / docs).
func BuildStructuredClaudeArgs(prompt, schema string, ephemeral, noTools bool) []string {
	a := &CLIAgentAdapter{kind: cliKindClaude}
	return a.buildClaudeArgs(TurnOptions{
		JSONSchema: schema,
		Ephemeral:  ephemeral,
		NoTools:    noTools,
	}, prompt, "", false)
}

// BuildStructuredCodexArgs builds Codex exec argv for a one-shot structured call.
// schemaPath must already be a filesystem path.
func BuildStructuredCodexArgs(prompt, cwd, schemaPath string, ephemeral, noTools bool, rules string) []string {
	a := &CLIAgentAdapter{kind: cliKindCodex}
	return a.buildCodexArgs(TurnOptions{
		JSONSchema: schemaPath,
		Ephemeral:  ephemeral,
		NoTools:    noTools,
		Rules:      rules,
	}, prompt, cwd, "", false)
}

// BuildStructuredGrokArgs builds Grok argv for a one-shot structured call.
func BuildStructuredGrokArgs(promptFileFlag, promptFile, cwd, schema string, noTools bool, rules string) []string {
	args := []string{
		promptFileFlag, promptFile,
		"--output-format", "json",
		"--cwd", cwd,
		"--no-alt-screen",
		"--json-schema", schema,
	}
	if noTools {
		args = append(args, "--tools", "")
	}
	args = append(args, rulesCLIArgs(rules)...)
	return args
}

// ReportSchema constrains the member's final reply (契约 6.21.15). Files are never in the schema.
const ReportSchema = `{
  "type": "object",
  "required": ["did"],
  "additionalProperties": false,
  "properties": {
    "did":  { "type": "string", "minLength": 1, "maxLength": 500 },
    "attn": { "type": "string", "maxLength": 500 },
    "give": { "type": "string", "maxLength": 500 }
  }
}`

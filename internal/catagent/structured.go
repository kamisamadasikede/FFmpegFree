package catagent

import (
	"bytes"
	"encoding/json"
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

// RouteSchema is the JSON Schema for dispatch judgment (契约 6.21.6 / v0.57).
// memberIds 1～N；everyone=true 表示 @全体；旧单字段 memberId 仅兼容读入（additionalProperties 放开）。
const RouteSchema = `{
  "type": "object",
  "properties": {
    "memberIds": {"type": "array", "items": {"type": "string"}, "minItems": 1},
    "memberId": {"type": "string"},
    "everyone": {"type": "boolean"},
    "reason": {"type": "string", "maxLength": 40}
  },
  "required": ["reason"],
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

// ExtractStructuredJSONContent turns a CLI --output-format json (or mixed) stdout
// blob into the structured payload string for Message.Content.
//
// Real Grok/Claude with --json-schema emit one JSON document (sometimes wrapped
// in result / message). Fake e2e scripts still wrap the payload in streaming-json
// text lines; both must yield the inner object so route/charter parsing works.
func ExtractStructuredJSONContent(raw []byte) string {
	s := strings.TrimSpace(string(raw))
	if s == "" {
		return ""
	}
	if body := unwrapStructuredJSON([]byte(s)); body != "" {
		return body
	}
	// NDJSON / mixed: prefer streamed text deltas; else last structured object line.
	var acc strings.Builder
	var lastStruct string
	tools := map[string]*toolSnap{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if u := unwrapStructuredJSON([]byte(line)); u != "" {
			lastStruct = u
		}
		piece, _, isErr := parseStreamLine([]byte(line), tools)
		if isErr || piece.Text == "" {
			continue
		}
		if piece.Kind == "" || piece.Kind == "text" {
			acc.WriteString(piece.Text)
		}
	}
	if t := strings.TrimSpace(acc.String()); t != "" {
		if u := unwrapStructuredJSON([]byte(t)); u != "" {
			return u
		}
		return t
	}
	return lastStruct
}

func unwrapStructuredJSON(b []byte) string {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || (b[0] != '{' && b[0] != '"' && b[0] != '[') {
		return ""
	}
	var v any
	if json.Unmarshal(b, &v) != nil {
		return ""
	}
	return unwrapStructuredValue(v, 0)
}

func unwrapStructuredValue(v any, depth int) string {
	if depth > 6 || v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return ""
		}
		if inner := unwrapStructuredJSON([]byte(s)); inner != "" {
			return inner
		}
		return ""
	case map[string]any:
		if looksLikeStructuredPayload(x) {
			return mustCompactJSON(x)
		}
		for _, key := range []string{"structured_output", "text", "result", "data", "content", "output", "message"} {
			if raw, ok := x[key]; ok {
				if got := unwrapStructuredValue(raw, depth+1); got != "" {
					return got
				}
			}
		}
		if msg, ok := x["message"].(map[string]any); ok {
			if got := unwrapStructuredValue(msg["content"], depth+1); got != "" {
				return got
			}
		}
		return ""
	default:
		return ""
	}
}

func looksLikeStructuredPayload(m map[string]any) bool {
	if _, ok := m["memberId"]; ok {
		return true
	}
	if _, ok := m["owns"]; ok {
		return true
	}
	if _, ok := m["did"]; ok {
		return true
	}
	return false
}

func mustCompactJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

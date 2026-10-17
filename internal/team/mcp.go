package team

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

type MCPTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func toolSchemas() []MCPTool {
	str := func(description string) map[string]any {
		return map[string]any{"type": "string", "description": description}
	}
	array := map[string]any{"type": "array", "items": map[string]string{"type": "string"}}
	common := map[string]any{"requestId": str("Unique request ID; reuse only for an identical retry"), "content": str("Public message or task instruction"), "toMemberId": str("Exact memberId from team_members"), "targetMemberIds": array, "taskId": str("Exact task ID"), "expectedRunId": str("Current native execution ID")}
	specs := []struct {
		name, description string
		props             map[string]any
		required          []string
	}{
		{"team_members", "Read the complete current roster, including the human owner", nil, nil},
		{"team_member", "Read a member's complete role and delivery standards", map[string]any{"memberId": str("Member ID")}, []string{"memberId"}},
		{"team_publish", "Publish progress in the group; human mentions notify; agent mentions do not dispatch", nil, []string{"requestId", "content"}},
		{"team_delegate", "Dispatch a real colleague task; returns immediately. No fixed workflow", map[string]any{"title": str("Short task title"), "acceptance": array, "dependsOn": array, "requiredForCompletion": map[string]string{"type": "boolean"}}, []string{"requestId", "toMemberId", "content"}},
		{"team_ask", "Ask an agent in a separate task or ask the human for a decision", nil, []string{"requestId", "toMemberId", "content"}},
		{"team_report", "Report progress or a blocker to a human; completion uses team_finish", map[string]any{"category": str("progress or blocked")}, []string{"requestId", "toMemberId", "content"}},
		{"team_task_status", "Read current task or an exact colleague task result", nil, nil},
		{"team_open_file", "Open a workspace Office/PDF/HTML/Markdown artifact in this group's preview panel", map[string]any{"path": str("Workspace-relative artifact path")}, []string{"path"}},
		{"team_read_messages", "Read original group messages as bounded excerpts. For full text use messageId with offset/limit and follow nextOffset", map[string]any{"messageId": str("Optional exact message ID"), "before": map[string]string{"type": "integer"}, "limit": map[string]string{"type": "integer"}, "offset": map[string]string{"type": "integer"}}, nil},
		{"team_search_messages", "Search original messages in this group only", map[string]any{"query": str("2–200 character search query")}, []string{"query"}},
		{"team_steer", "Add input to a related active native execution; check actual input mode", nil, []string{"requestId", "toMemberId", "taskId", "expectedRunId", "content"}},
		{"team_yield", "Record colleague waits, then end the native turn normally without polling", map[string]any{"waitFor": array}, []string{"requestId"}},
		{"team_finish", "Submit final logical result; runtime settles required dependencies after native exit", map[string]any{"result": str("Final result with artifact paths and actual validation evidence"), "validation": map[string]any{"type": "string", "enum": []string{"not_run", "unknown", "passed", "failed"}}}, []string{"requestId", "result", "validation"}},
	}
	out := []MCPTool{}
	for _, spec := range specs {
		props := map[string]any{}
		for k, v := range common {
			props[k] = v
		}
		for k, v := range spec.props {
			props[k] = v
		}
		schema := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
		if len(spec.required) > 0 {
			schema["required"] = spec.required
		}
		out = append(out, MCPTool{spec.name, spec.description, schema})
	}
	return append(out, officeSchemas()...)
}

func officeSchemas() []MCPTool {
	str := map[string]string{"type": "string"}
	integer := map[string]string{"type": "integer"}
	common := map[string]any{"path": str}
	defs := []struct {
		name, description string
		props             map[string]any
		required          []string
	}{
		{"office_open", "Open or create a native editable PPTX and show its live GenOffice canvas", map[string]any{"create": map[string]string{"type": "boolean"}}, nil},
		{"office_read", "Read native slides, element IDs and revision before editing", nil, nil},
		{"office_guide", "Read exact native operation schemas; group text/element/slide/insert/table/deck", map[string]any{"group": str}, nil},
		{"office_page", "Draw one 1280x720 page, visible before the next call. Wait, then send next page", map[string]any{"page": map[string]string{"type": "object"}, "index": integer, "replace": map[string]string{"type": "boolean"}, "expectedRevision": integer, "requestId": str}, []string{"page", "expectedRevision", "requestId"}},
		{"office_apply", "Atomic native edit operations. Read guide and current revision first", map[string]any{"ops": map[string]any{"type": "array", "items": map[string]string{"type": "object"}, "maxItems": 50}, "expectedRevision": integer, "requestId": str, "dryRun": map[string]string{"type": "boolean"}}, []string{"ops", "expectedRevision", "requestId"}},
		{"office_save", "Save native PPTX atomically. Verify savedRevision equals revision", map[string]any{"expectedRevision": integer, "copyPath": str}, nil},
		{"office_audit", "Inspect rendered pages for text overflow and layout findings", nil, nil},
	}
	out := []MCPTool{}
	for _, d := range defs {
		p := map[string]any{}
		for k, v := range common {
			p[k] = v
		}
		for k, v := range d.props {
			p[k] = v
		}
		out = append(out, MCPTool{d.name, d.description, map[string]any{"type": "object", "properties": p, "required": append([]string{"path"}, d.required...), "additionalProperties": false}})
	}
	return out
}

// RunMCP is a capability-scoped sidecar using this application's executable.
func RunMCP(input io.Reader, output io.Writer) error {
	sc := bufio.NewScanner(input)
	sc.Buffer(make([]byte, 65536), 1<<20)
	enc := json.NewEncoder(output)
	for sc.Scan() {
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(sc.Bytes(), &request) != nil {
			continue
		}
		if len(request.ID) == 0 {
			continue
		}
		var result any
		var rpcErr any
		switch request.Method {
		case "initialize":
			var params struct {
				ProtocolVersion string `json:"protocolVersion"`
			}
			_ = json.Unmarshal(request.Params, &params)
			if params.ProtocolVersion == "" {
				params.ProtocolVersion = "2024-11-05"
			}
			result = map[string]any{"protocolVersion": params.ProtocolVersion, "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]string{"name": "ffc-team", "version": "1.0"}}
		case "ping":
			result = map[string]any{}
		case "tools/list":
			result = map[string]any{"tools": toolSchemas()}
		case "tools/call":
			var params struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			if json.Unmarshal(request.Params, &params) != nil {
				rpcErr = map[string]any{"code": -32602, "message": "Invalid params"}
				break
			}
			exists := false
			for _, tool := range toolSchemas() {
				exists = exists || tool.Name == params.Name
			}
			if !exists {
				rpcErr = map[string]any{"code": -32601, "message": "Unknown tool"}
				break
			}
			body := params.Arguments
			if len(body) == 0 {
				body = []byte("{}")
			}
			var out bytes.Buffer
			e := RunTool([]string{params.Name, "--json", string(body)}, bytes.NewReader(nil), &out)
			text := out.String()
			if e != nil {
				text = e.Error()
			}
			result = map[string]any{"isError": e != nil, "content": []map[string]string{{"type": "text", "text": text}}}
		default:
			rpcErr = map[string]any{"code": -32601, "message": "Unsupported method"}
		}
		response := map[string]any{"jsonrpc": "2.0", "id": request.ID}
		if rpcErr != nil {
			response["error"] = rpcErr
		} else {
			response["result"] = result
		}
		if e := enc.Encode(response); e != nil {
			return fmt.Errorf("tool output: %w", e)
		}
	}
	return sc.Err()
}

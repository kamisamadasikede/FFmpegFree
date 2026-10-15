package cat_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/proc"
	"FFmpegFree/internal/service/cat"
	"gopkg.in/yaml.v3"
)

func TestPptBridgePrepareFuturePathScopeAndSkill(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)
	reg := catagent.NewRegistry()
	started := make(chan catagent.TurnOptions, 1)
	gate := make(chan struct{})
	reg.Register(&fakeAdapter{run: func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		started <- o
		select {
		case <-gate:
		case <-o.Ctx.Done():
		}
		return catagent.TurnResponse{}, nil
	}})
	root := t.TempDir()
	var mu sync.Mutex
	var opens []cat.OpenFileEvent
	s := cat.New(cat.Config{Store: st, Registry: reg, DataRoot: root, CLIHome: t.TempDir(), FileBridge: true, Emit: func(name string, value any) {
		if name == "cat:open-file" {
			mu.Lock()
			opens = append(opens, value.(cat.OpenFileEvent))
			mu.Unlock()
		}
	}})
	c, e := s.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild})
	if e != nil {
		t.Fatal(e)
	}
	files, e := s.ListCatFiles(ctx, cat.ListFilesRequest{ConvID: c.ID})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.SendCatMessage(ctx, cat.SendMessageRequest{ConversationID: c.ID, Content: "generate native ppt"}); e != nil {
		t.Fatal(e)
	}
	o := <-started
	defer func() { close(gate); s.Wait(); s.ClosePptPreviews() }()
	endpoint := strings.TrimSuffix(o.ToolEnv["FFC_OFFICE_ENDPOINT"], "/office") + "/ppt"
	token := o.ToolEnv["FFC_OFFICE_TOKEN"]
	if endpoint == "/ppt" || len(token) != 64 {
		t.Fatal("missing native PPT capability")
	}
	skill := filepath.Join(root, "cat", "builtin-skills", "ffc-ppt-live", "SKILL.md")
	b, e := os.ReadFile(skill)
	if e != nil {
		t.Fatal(e)
	}
	parts := strings.SplitN(string(b), "---", 3)
	if len(parts) != 3 {
		t.Fatal("skill metadata missing")
	}
	var meta struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	if yaml.Unmarshal([]byte(parts[1]), &meta) != nil || meta.Name != "ffc-ppt-live" || meta.Description == "" {
		t.Fatal("invalid skill metadata")
	}
	if _, e = os.Stat(filepath.Join(filepath.Dir(skill), "scripts", "office.cjs")); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(catagent.StripInternalInstructions(o.Rules), token) {
		t.Fatal("PPT capability leaked into visible user text")
	}
	post := func(path, auth, origin string) int {
		b, _ := json.Marshal(map[string]string{"action": "prepare", "path": path})
		req, _ := http.NewRequest("POST", endpoint, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", auth)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		r, e := (&http.Client{Timeout: time.Second}).Do(req)
		if e != nil {
			return 0
		}
		r.Body.Close()
		return r.StatusCode
	}
	auth := "Bearer " + token
	for _, p := range []string{"../escape.pptx", "output/../../escape.pptx", "output/future.docx", filepath.Join(t.TempDir(), "outside.pptx")} {
		if got := post(p, auth, ""); got != 400 {
			t.Fatalf("path %q accepted: %d", p, got)
		}
	}
	if post("output/future.pptx", "Bearer wrong", "") != 403 {
		t.Fatal("bad capability accepted")
	}
	if post("output/future.pptx", auth, "null") != 403 {
		t.Fatal("iframe origin accepted")
	}
	if post("output/future.pptx", auth, "") != 200 {
		t.Fatal("future slide preview could not open")
	}
	mu.Lock()
	if len(opens) != 1 || opens[0].ConvID != c.ID || opens[0].RelPath != "output/future.pptx" {
		t.Fatal(opens)
	}
	mu.Unlock()
	if _, e = os.Stat(filepath.Join(files.Root, "output", "future.pptx")); !os.IsNotExist(e) {
		t.Fatal("prepare must not write PPTX")
	}
	// Existing symlink/junction paths must not become future output permissions.
	outside := t.TempDir()
	link := filepath.Join(files.Root, "linked")
	if e = os.Symlink(outside, link); e == nil {
		if post("linked/future.pptx", auth, "") != 400 {
			t.Fatal("symlink escaped workspace")
		}
	}
}

func TestGenOfficeSkillAndMCPUseRealTurnCapability(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	st := openStore(t)
	reg := catagent.NewRegistry()
	started := make(chan catagent.TurnOptions, 1)
	gate := make(chan struct{})
	reg.Register(&fakeAdapter{run: func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		started <- o
		select {
		case <-gate:
		case <-o.Ctx.Done():
		}
		return catagent.TurnResponse{}, nil
	}})
	data := t.TempDir()
	s := cat.New(cat.Config{Store: st, Registry: reg, DataRoot: data, CLIHome: t.TempDir(), FileBridge: true})
	c, e := s.CreateCatConversation(ctx, cat.CreateConversationRequest{AgentKind: catagent.KindCatBuild})
	if e != nil {
		t.Fatal(e)
	}
	files, e := s.ListCatFiles(ctx, cat.ListFilesRequest{ConvID: c.ID})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.SendCatMessage(ctx, cat.SendMessageRequest{ConversationID: c.ID, Content: "make a PPT"}); e != nil {
		t.Fatal(e)
	}
	o := <-started
	defer func() { close(gate); s.Wait(); s.ClosePptPreviews() }()
	wrapper := filepath.Join(data, "cat", "builtin-skills", "ffc-ppt-live", "scripts", "office.cjs")
	cmd := exec.CommandContext(ctx, node, wrapper, "open", "--path", "deck.pptx", "--create")
	cmd.Env = catagent.MergeToolEnv(os.Environ(), o.ToolEnv)
	proc.Configure(cmd)
	output, e := cmd.Output()
	if e != nil {
		t.Fatal(e)
	}
	var opened struct {
		Revision int `json:"revision"`
	}
	if e = json.Unmarshal(output, &opened); e != nil {
		t.Fatal(e)
	}
	client := exec.CommandContext(ctx, node, wrapper, "--mcp")
	client.Env = cmd.Env
	proc.Configure(client)
	in, e := client.StdinPipe()
	if e != nil {
		t.Fatal(e)
	}
	out, e := client.StdoutPipe()
	if e != nil {
		t.Fatal(e)
	}
	if e = proc.Start(client); e != nil {
		t.Fatal(e)
	}
	defer func() { _ = in.Close(); _ = client.Wait() }()
	reader := bufio.NewScanner(out)
	reader.Buffer(make([]byte, 64<<10), 1<<20)
	rpc := func(id int, method string, params any) json.RawMessage {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
		if _, e = in.Write(append(body, '\n')); e != nil {
			t.Fatal(e)
		}
		if !reader.Scan() {
			t.Fatal("MCP response missing")
		}
		var response struct {
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if e = json.Unmarshal(reader.Bytes(), &response); e != nil {
			t.Fatal(e)
		}
		if len(response.Error) > 0 {
			t.Fatalf("MCP error: %s", response.Error)
		}
		return response.Result
	}
	_ = rpc(1, "initialize", map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "qa", "version": "1"}})
	if !bytes.Contains(rpc(2, "tools/list", nil), []byte("office_page")) {
		t.Fatal("office tools not registered")
	}
	result := rpc(3, "tools/call", map[string]any{"name": "office_page", "arguments": map[string]any{"path": "deck.pptx", "expectedRevision": opened.Revision, "requestId": "mcp-page-1", "page": map[string]any{"elements": []any{map[string]any{"type": "text", "x": 72, "y": 96, "w": 1000, "h": 120, "paragraphs": []any{map[string]any{"runs": []any{map[string]any{"text": "真实 MCP 生成", "sizePt": 36}}}}}}}}})
	if bytes.Contains(result, []byte(`"isError":true`)) {
		t.Fatalf("MCP edit failed: %s", result)
	}
	_ = rpc(4, "tools/call", map[string]any{"name": "office_save", "arguments": map[string]any{"path": "deck.pptx"}})
	if fi, e := os.Stat(filepath.Join(files.Root, "deck.pptx")); e != nil || fi.Size() < 1000 {
		t.Fatalf("native PPTX missing: %v", e)
	}
	shared, e := s.GetCatSharedExtensions(ctx, cat.SkillsDirRequest{})
	if e != nil {
		t.Fatal(e)
	}
	for _, target := range shared.Targets {
		native, _ := os.ReadFile(target.ConfigPath)
		if bytes.Contains(native, []byte(o.ToolEnv["FFC_OFFICE_TOKEN"])) || bytes.Contains(native, []byte(o.ToolEnv["FFC_OFFICE_ENDPOINT"])) {
			t.Fatal("ephemeral capability persisted in native config")
		}
	}
}

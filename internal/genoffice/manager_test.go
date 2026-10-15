package genoffice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixturePage(title string) map[string]any {
	return map[string]any{"background": "#EEF4FF", "elements": []any{
		map[string]any{"type": "shape", "shape": "rect", "x": 0, "y": 0, "w": 1280, "h": 18, "fill": "#2874D0"},
		map[string]any{"type": "text", "x": 72, "y": 96, "w": 1100, "h": 120, "paragraphs": []any{map[string]any{"runs": []any{map[string]any{"text": title, "sizePt": 36, "bold": true, "font": "Microsoft YaHei", "color": "#203B60"}}}}},
		map[string]any{"type": "text", "x": 72, "y": 280, "w": 1040, "h": 100, "paragraphs": []any{map[string]any{"runs": []any{map[string]any{"text": "原生文本 · 实时生成与编辑", "sizePt": 22, "font": "Microsoft YaHei"}}}}},
	}}
}
func TestManagerNativePreviewAndAuthorization(t *testing.T) {
	if _, err := nodePath(t.TempDir(), false); err != nil {
		t.Skip(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	root := t.TempDir()
	file := filepath.Join(root, "live.pptx")
	m := New(Config{Dir: t.TempDir()})
	t.Cleanup(m.Close)
	guard := func() error { return nil }
	state, err := m.Open(ctx, file, root, true, guard)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := m.Start(ctx, file, root, guard)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Mode != "genoffice" || preview.URL == "" || preview.Token == "" {
		t.Fatal("missing canvas capability")
	}
	post := func(token, origin string) int {
		body, _ := json.Marshal(map[string]any{"id": preview.DocumentID, "action": "state"})
		req, _ := http.NewRequest("POST", strings.TrimSuffix(preview.URL, "/editor.html")+"/api", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Origin", origin)
		req.Header.Set("Content-Type", "application/json")
		r, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer r.Body.Close()
		return r.StatusCode
	}
	base := strings.TrimSuffix(preview.URL, "/editor.html")
	if post("wrong", base) != 401 || post(preview.Token, "https://example.com") != 403 || post(preview.Token, base) != 200 {
		t.Fatal("preview origin/token protection failed")
	}
	_, err = m.Call(ctx, preview.DocumentID, map[string]any{"action": "page", "expectedRevision": state["revision"], "requestId": "first-page", "page": fixturePage("ffc · 第一页")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Call(ctx, preview.DocumentID, map[string]any{"action": "save"}); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(file); err != nil {
		t.Fatal(err)
	}
	m.Release(preview.ID)
	if _, err = m.Call(ctx, preview.DocumentID, map[string]any{"action": "read"}); err != nil {
		t.Fatalf("closing view killed live document: %v", err)
	}
}
func TestEmbeddedCanvasBrowser(t *testing.T) {
	if os.Getenv("FFC_GENOFFICE_BROWSER_QA") != "1" {
		t.Skip("opt-in browser integration")
	}
	root := t.TempDir()
	m := New(Config{Dir: t.TempDir()})
	defer m.Close()
	ctx := context.Background()
	file := filepath.Join(root, "live.pptx")
	guard := func() error { return nil }
	if _, err := m.Open(ctx, file, root, true, guard); err != nil {
		t.Fatal(err)
	}
	preview, err := m.Start(ctx, file, root, guard)
	if err != nil {
		t.Fatal(err)
	}
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		step := r.URL.Query().Get("step")
		var deck struct {
			Revision int `json:"revision"`
			Slides   []struct {
				Elements []struct {
					ID   string `json:"id"`
					Text string `json:"text"`
				} `json:"elements"`
			} `json:"slides"`
		}
		raw, e := m.Call(ctx, preview.DocumentID, map[string]any{"action": "read"})
		if e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		_ = json.Unmarshal(raw, &deck)
		args := map[string]any{"action": "page", "expectedRevision": deck.Revision, "requestId": "browser-" + step, "page": fixturePage("ffc · 第" + step + "页")}
		if step == "edit" || step == "conflict" {
			title := "ffc · 标题实时更新"
			if step == "conflict" {
				title = "ffc · Agent 已写入新版本"
			}
			args = map[string]any{"action": "apply", "expectedRevision": deck.Revision, "requestId": "browser-" + step, "ops": []any{map[string]any{"op": "setText", "target": map[string]any{"slide": 0, "el": deck.Slides[0].Elements[1].ID}, "paragraphs": []any{map[string]any{"srcPara": 0, "runs": []any{map[string]any{"text": title, "srcPara": 0, "srcRun": 0}}}}}}}
		}
		result, e := m.Call(ctx, preview.DocumentID, args)
		if e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		_, _ = w.Write(result)
	}))
	defer control.Close()
	previewJSON, _ := json.Marshal(preview)
	script, err := filepath.Abs("../../office/tests/browser.cjs")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("node", script)
	cmd.Env = append(os.Environ(), "FFC_OFFICE_QA_PREVIEW="+string(previewJSON), "FFC_OFFICE_QA_CONTROL="+control.URL)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("canvas browser QA: %v\n%s", err, output)
	}
	t.Log(strings.TrimSpace(string(output)))
}
func TestRPCRejectsCapabilityOverride(t *testing.T) {
	if _, err := nodePath(t.TempDir(), false); err != nil {
		t.Skip(err)
	}
	m := New(Config{Dir: t.TempDir()})
	defer m.Close()
	root := t.TempDir()
	ctx := context.Background()
	allowed := true
	guard := func() error {
		if !allowed {
			return fmt.Errorf("authorization revoked")
		}
		return nil
	}
	s, err := m.Open(ctx, filepath.Join(root, "deck.pptx"), root, true, guard)
	if err != nil {
		t.Fatal(err)
	}
	id := s["documentId"].(string)
	allowed = false
	if _, err = m.Call(ctx, id, map[string]any{"action": "read", "root": t.TempDir()}); err == nil {
		t.Fatal("revoked capability accepted")
	}
}

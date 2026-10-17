package team

import (
	"archive/zip"
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/genoffice"
)

func TestGroupNativePPTLivePreviewAndSave(t *testing.T) {
	t.Setenv("FFC_OFFICE_PAINT_MS", "0")
	var service *Service
	s, _ := testService(t, func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		open, e := toolCall(o, "office_open", map[string]any{"path": "deck.pptx", "create": true})
		if e != nil {
			return catagent.TurnResponse{}, e
		}
		var state map[string]any
		_ = json.Unmarshal(open, &state)
		task, _ := inputTask(o)
		preview, e := service.StartPreview(o.Ctx, task.RoomID, "deck.pptx")
		if e != nil {
			return catagent.TurnResponse{}, e
		}
		defer service.ReleasePreview(preview.ID)
		if preview.URL == "" || preview.DocumentID != state["documentId"] {
			return catagent.TurnResponse{}, bad("群画布没有关联到同一原生文档")
		}
		_, e = toolCall(o, "office_page", map[string]any{"path": "deck.pptx", "expectedRevision": state["revision"], "requestId": "page1", "page": map[string]any{"elements": []any{map[string]any{"type": "text", "x": 80, "y": 100, "w": 1100, "h": 120, "paragraphs": []any{map[string]any{"runs": []any{map[string]any{"text": "Group native live PPT", "sizePt": 36}}}}}}}})
		if e != nil {
			return catagent.TurnResponse{}, e
		}
		raw, e := toolCall(o, "office_read", map[string]any{"path": "deck.pptx"})
		if e != nil {
			return catagent.TurnResponse{}, e
		}
		if !strings.Contains(string(raw), "Group native live PPT") {
			return catagent.TurnResponse{}, bad("画布读取未反映新页面")
		}
		_, e = toolCall(o, "office_save", map[string]any{"path": "deck.pptx"})
		if e != nil {
			return catagent.TurnResponse{}, e
		}
		_, e = toolCall(o, "team_finish", map[string]any{"requestId": "done", "result": "原生 PPT 已保存", "validation": "passed"})
		return catagent.TurnResponse{}, e
	})
	service = s
	manager := genoffice.New(genoffice.Config{Dir: t.TempDir()})
	defer manager.Close()
	if state := manager.Tool(false); state.State != "ready" {
		t.Skip(state.Error)
	}
	s.cfg.Office = manager
	r, ms := testRoom(t, s, 1)
	employee := employeeMembers(ms)[0]
	boot, e := s.Bootstrap(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	emp := boot.Employees[0]
	emp.PermissionMode = catagent.ModeCodexWorkspace
	if _, e = s.SaveEmployee(context.Background(), emp); e != nil {
		t.Fatal(e)
	}
	_, e = s.Command(context.Background(), Command{RequestID: "start", RoomID: r.ID, Intent: "new_task", Content: "生成一页PPT", TargetMemberIDs: []string{employee.ID}})
	if e != nil {
		t.Fatal(e)
	}
	eventually(t, func() bool {
		v, _ := s.Snapshot(context.Background(), r.ID, 0)
		return len(v.Tasks) == 1 && terminal(v.Tasks[0].Status)
	})
	v, e := s.Snapshot(context.Background(), r.ID, 0)
	if e != nil || v.Tasks[0].Status != "completed" {
		t.Fatalf("result=%+v err=%v", v.Tasks, e)
	}
	if len(v.Artifacts) != 1 || v.Artifacts[0].TaskID != v.Tasks[0].ID {
		t.Fatalf("artifact=%+v", v.Artifacts)
	}
	events, e := s.Events(context.Background(), r.ID, 0)
	if e != nil {
		t.Fatal(e)
	}
	opened := false
	for _, event := range events {
		if event.Kind == "open_file" {
			opened = true
		}
	}
	if !opened {
		t.Fatal("canvas never auto-opened")
	}
	z, e := zip.OpenReader(filepath.Join(r.Workspace, "deck.pptx"))
	if e != nil {
		t.Fatal(e)
	}
	defer z.Close()
	editable := false
	for _, f := range z.File {
		if strings.HasPrefix(f.Name, "ppt/slides/slide") && strings.HasSuffix(f.Name, ".xml") {
			reader, e := f.Open()
			if e != nil {
				t.Fatal(e)
			}
			data, _ := io.ReadAll(reader)
			reader.Close()
			if strings.Contains(string(data), "Group native live PPT") {
				editable = true
			}
		}
	}
	if !editable {
		t.Fatal("PPTX did not contain editable native slide text")
	}
}

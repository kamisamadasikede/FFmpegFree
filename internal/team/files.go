package team

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/genoffice"
)

type Artifact struct {
	ID        string `json:"id"`
	RoomID    string `json:"roomId"`
	TaskID    string `json:"taskId"`
	Path      string `json:"path"`
	Version   string `json:"version"`
	UpdatedAt int64  `json:"updatedAt"`
}
type FileEntry struct {
	Path       string `json:"path"`
	Size       int64  `json:"size"`
	ModifiedAt int64  `json:"modifiedAt"`
}
type FileList struct {
	Files     []FileEntry `json:"files"`
	Truncated bool        `json:"truncated"`
}

func officeExt(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".pptx", ".docx", ".xlsx", ".pdf", ".md", ".markdown", ".htm", ".html":
		return true
	}
	return false
}

// Resolve existing components physically, including Windows junctions. A future
// file is allowed, but never an absent or changed project root.
func scopedPath(root, name string) (string, string, error) {
	base, e := filepath.EvalSymlinks(root)
	if e != nil {
		return "", "", e
	}
	rel := strings.ReplaceAll(name, `\`, "/")
	if filepath.IsAbs(name) {
		rel, e = filepath.Rel(root, name)
		if e != nil {
			return "", "", e
		}
		rel = filepath.ToSlash(rel)
	}
	if strings.Contains(rel, ":") || strings.HasPrefix(rel, "/") || strings.TrimSpace(rel) == "" {
		return "", "", bad("文件路径无效")
	}
	current := root
	for _, seg := range strings.Split(rel, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return "", "", bad("文件必须位于当前群工作目录")
		}
		current = filepath.Join(current, seg)
		if _, e := os.Lstat(current); os.IsNotExist(e) {
			continue
		} else if e != nil {
			return "", "", e
		}
		actual, e := filepath.EvalSymlinks(current)
		if e != nil {
			return "", "", e
		}
		check, e := filepath.Rel(base, actual)
		if e != nil || check == ".." || strings.HasPrefix(check, ".."+string(filepath.Separator)) || filepath.IsAbs(check) {
			return "", "", bad("文件链接超出群工作目录")
		}
	}
	return current, rel, nil
}
func (s *Service) room(ctx context.Context, id string) (Room, error) {
	var r Room
	_, e := s.cfg.Repository.Write(ctx, func(t *Tx) error { var e error; r, e = t.Room(id); return e })
	return r, e
}
func (s *Service) Files(ctx context.Context, roomID string) (FileList, error) {
	r, e := s.room(ctx, roomID)
	if e != nil {
		return FileList{}, e
	}
	return scanFiles(ctx, r.Workspace)
}
func scanFiles(ctx context.Context, root string) (FileList, error) {
	out := FileList{Files: []FileEntry{}}
	visited := 0
	e := filepath.WalkDir(root, func(path string, d fs.DirEntry, e error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if e != nil {
			if path == root {
				return e
			}
			return nil
		}
		visited++
		if visited > 12000 || len(out.Files) >= 500 {
			out.Truncated = true
			return fs.SkipAll
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			if path != root && (strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules" || d.Name() == "vendor" || d.Name() == "target" || d.Name() == "build" || d.Name() == "dist") {
				return fs.SkipDir
			}
			return nil
		}
		if !officeExt(path) {
			return nil
		}
		rel, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		if _, _, e = scopedPath(root, rel); e != nil {
			return nil
		}
		fi, e := d.Info()
		if e != nil {
			return nil
		}
		out.Files = append(out.Files, FileEntry{filepath.ToSlash(rel), fi.Size(), fi.ModTime().UnixMilli()})
		return nil
	})
	return out, e
}

func (s *Service) watchFiles(ctx context.Context, r Room, taskID string, previous FileList) {
	versions := map[string]string{}
	version := func(f FileEntry) string { return fmt.Sprintf("%d:%d", f.ModifiedAt, f.Size) }
	for _, f := range previous.Files {
		versions[f.Path] = version(f)
	}
	opened := false
	scan := func(scanCtx context.Context) {
		files, e := scanFiles(scanCtx, r.Workspace)
		if e != nil {
			return
		}
		for _, f := range files.Files {
			v := version(f)
			if versions[f.Path] != v {
				versions[f.Path] = v
				_, e = s.artifact(scanCtx, r.ID, taskID, f.Path, !opened)
				if e == nil {
					opened = true
				}
			}
		}
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			final, done := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
			scan(final)
			done()
			return
		case <-ticker.C:
			scan(ctx)
		}
	}
}
func (s *Service) artifact(ctx context.Context, roomID, taskID, name string, open bool) (Artifact, error) {
	r, e := s.room(ctx, roomID)
	if e != nil {
		return Artifact{}, e
	}
	abs, rel, e := scopedPath(r.Workspace, name)
	if e != nil {
		return Artifact{}, e
	}
	if !officeExt(rel) {
		return Artifact{}, bad("当前群预览支持 Office、PDF、HTML 和 Markdown 文件")
	}
	sum := sha256.Sum256([]byte(roomID + "\x00" + rel))
	v := Artifact{ID: hex.EncodeToString(sum[:16]), RoomID: roomID, TaskID: taskID, Path: rel, UpdatedAt: time.Now().UnixMilli()}
	if fi, e := os.Stat(abs); e == nil {
		v.Version = fmt.Sprintf("%d:%d", fi.ModTime().UnixNano(), fi.Size())
	} else if !os.IsNotExist(e) {
		return v, e
	}
	e = s.write(ctx, func(t *Tx) error {
		data, e := encode(v)
		if e != nil {
			return e
		}
		if e = t.exec("INSERT INTO team_artifacts(id,room_id,path,data) VALUES(?,?,?,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data", v.ID, v.RoomID, v.Path, data); e != nil {
			return e
		}
		if e = t.Event(v.RoomID, "artifact", v); e != nil {
			return e
		}
		if open {
			return t.Event(v.RoomID, "open_file", v)
		}
		return nil
	})
	return v, e
}
func (s *Service) fileGuard(roomID, path, expected string) func() error {
	return func() error {
		r, e := s.room(context.Background(), roomID)
		if e != nil {
			return e
		}
		abs, _, e := scopedPath(r.Workspace, path)
		if e != nil {
			return e
		}
		if abs != expected {
			return bad("工作区路径已变化，请重新打开文件")
		}
		return nil
	}
}
func (s *Service) StartPreview(ctx context.Context, roomID, path string) (genoffice.Preview, error) {
	if s.cfg.Office == nil {
		return genoffice.Preview{}, bad("办公引擎尚未就绪")
	}
	r, e := s.room(ctx, roomID)
	if e != nil {
		return genoffice.Preview{}, e
	}
	abs, _, e := scopedPath(r.Workspace, path)
	if e != nil {
		return genoffice.Preview{}, e
	}
	if !officeExt(path) {
		return genoffice.Preview{}, bad("暂不支持该文件预览")
	}
	return s.cfg.Office.Start(ctx, abs, r.Workspace, s.fileGuard(roomID, path, abs))
}
func (s *Service) ReleasePreview(id string) {
	if s.cfg.Office != nil {
		s.cfg.Office.Release(id)
	}
}
func (s *Service) officeTool(ctx context.Context, cap capability, name string, args map[string]any) (json.RawMessage, error) {
	var task Task
	_, e := s.cfg.Repository.Write(ctx, func(t *Tx) error { var e error; task, e = s.authorize(t, cap); return e })
	if e != nil {
		return nil, e
	}
	if task.Status != "running" {
		return nil, conflict("员工已交接或在等待答复，请先结束当前原生轮次")
	}
	if s.cfg.Office == nil {
		return nil, bad("办公引擎尚未就绪")
	}
	path, _ := args["path"].(string)
	if !strings.EqualFold(filepath.Ext(path), ".pptx") {
		return nil, bad("办公工具目前仅支持原生 PPTX")
	}
	action := strings.TrimPrefix(name, "office_")
	switch action {
	case "open", "read", "page", "apply", "save", "guide", "audit":
	default:
		return nil, bad("未知办公工具")
	}
	if action == "open" || action == "page" || action == "apply" || action == "save" {
		var emp Employee
		_, e = s.cfg.Repository.Write(ctx, func(t *Tx) error {
			record, e := one[RunRecord](ctx, t.tx, "SELECT data FROM team_runs WHERE id=?", task.RunID)
			emp = record.Employee
			return e
		})
		if e != nil {
			return nil, e
		}
		writeAllowed := false
		for _, mode := range catagent.ModesFor(emp.AgentKind) {
			if mode.ID == emp.PermissionMode {
				writeAllowed = mode.WritesFiles
			}
		}
		if !writeAllowed && (action != "open" || args["create"] == true) {
			return nil, bad("员工当前为只读权限，不能修改演示文稿")
		}
	}
	r, e := s.room(ctx, task.RoomID)
	if e != nil {
		return nil, e
	}
	abs, _, e := scopedPath(r.Workspace, path)
	if e != nil {
		return nil, e
	}
	if copyPath, _ := args["copyPath"].(string); copyPath != "" {
		copyAbs, _, e := scopedPath(r.Workspace, copyPath)
		if e != nil {
			return nil, e
		}
		args["copyPath"] = copyAbs
	}
	create, _ := args["create"].(bool)
	state, e := s.cfg.Office.Open(ctx, abs, r.Workspace, action == "open" && create, s.fileGuard(task.RoomID, path, abs))
	if e != nil {
		return nil, e
	}
	var result json.RawMessage
	if action == "open" {
		result, e = json.Marshal(state)
	} else {
		args["action"] = action
		doc, _ := state["documentId"].(string)
		result, e = s.cfg.Office.Call(ctx, doc, args)
	}
	if e != nil {
		return nil, e
	}
	_, e = s.artifact(ctx, task.RoomID, task.ID, path, action == "open")
	return result, e
}

package doc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/paths"
	"FFmpegFree/internal/store"
	"FFmpegFree/internal/task"
)

// 可编辑的文本扩展名。
var textEditExts = map[string]string{
	"txt": "text", "md": "md", "markdown": "md", "html": "html", "htm": "html", "csv": "csv",
}

func textKindOf(ext string) string { return textEditExts[ext] }

func isTextEditExt(ext string) bool { return textEditExts[ext] != "" }

func isDocx(ext string) bool { return ext == "docx" }

// editTarget 是一次预览 / 保存解析出的目标。
type editTarget struct {
	SourceID    string
	TaskID      string
	Source      *store.ConvertSource // 源文件行时非 nil
	Task        *task.Task           // 结果行时非 nil
	WritePath   string               // 覆盖保存写哪里（原文件或输出）
	ReadPath    string               // 预览 / revision 读哪里
	MissingOrig bool                 // 源文件行原文件不在，读的是副本
	Ext         string
	Name        string
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func fileSHA256(path string) (string, int64, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", 0, err
	}
	return sha256Hex(b), int64(len(b)), nil
}

// pathLocks 同一真实路径的保存串行。
var pathLocks sync.Map // string -> *sync.Mutex

func lockPath(p string) func() {
	key := pathKey(realPath(p))
	v, _ := pathLocks.LoadOrStore(key, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// resolveEditID 解析 sourceId|taskId 二者只给一个。
func (s *Service) resolveEditID(ctx context.Context, sourceID, taskID string) (*editTarget, error) {
	if (sourceID == "") == (taskID == "") {
		return nil, apperr.New(apperr.InvalidArgument, "请指定源文件或转换记录")
	}
	if err := s.docReady(); err != nil {
		return nil, err
	}
	if sourceID != "" {
		return s.resolveSource(ctx, sourceID)
	}
	return s.resolveTask(ctx, taskID)
}

func (s *Service) resolveSource(ctx context.Context, id string) (*editTarget, error) {
	src, err := s.getDocSource(ctx, id)
	if err != nil {
		return nil, err
	}
	ext := normExt(filepath.Ext(src.Path))
	t := &editTarget{SourceID: id, Source: &src, Ext: ext, Name: src.Name, WritePath: src.OriginalPath}
	// 读：优先原文件；不在则副本（missing）
	if fi, err := os.Lstat(src.OriginalPath); err == nil && fi.Mode().IsRegular() {
		t.ReadPath = src.OriginalPath
		if r, err := filepath.EvalSymlinks(src.OriginalPath); err == nil {
			t.WritePath = r
			t.ReadPath = r
		}
	} else if src.CopyState == store.CopyReady && src.StoredPath != "" {
		t.ReadPath = src.StoredPath
		t.MissingOrig = true
	} else {
		return nil, apperr.New(apperr.NotFound, "原文件已经不在了，改完只能另存为。").WithDetail("reason=file")
	}
	return t, nil
}

func (s *Service) resolveTask(ctx context.Context, id string) (*editTarget, error) {
	tk, err := s.getDocTask(ctx, id)
	if err != nil {
		return nil, err
	}
	if tk.Type != task.TypeDocConvert && tk.Type != task.TypeOfficePDF {
		return nil, apperr.New(apperr.Unsupported, "不支持这种记录").WithDetail("reason=format")
	}
	if tk.Status != task.StatusSucceeded || tk.OutputPath == "" {
		return nil, apperr.New(apperr.NotFound, "原文件已经不在了，改完只能另存为。").WithDetail("reason=file")
	}
	ext := normExt(filepath.Ext(tk.OutputPath))
	t := &editTarget{TaskID: id, Task: &tk, Ext: ext, Name: filepath.Base(tk.OutputPath), WritePath: tk.OutputPath, ReadPath: tk.OutputPath}
	if fi, err := os.Lstat(tk.OutputPath); err != nil || !fi.Mode().IsRegular() {
		return nil, apperr.New(apperr.NotFound, "原文件已经不在了，改完只能另存为。").WithDetail("reason=file")
	}
	if r, err := filepath.EvalSymlinks(tk.OutputPath); err == nil {
		t.WritePath, t.ReadPath = r, r
	}
	return t, nil
}

func (s *Service) getDocSource(ctx context.Context, id string) (store.ConvertSource, error) {
	type getter interface {
		GetDocSource(ctx context.Context, id string) (store.ConvertSource, error)
	}
	if g, ok := s.cfg.Sources.(getter); ok {
		return g.GetDocSource(ctx, id)
	}
	// 退回：DocSourceForSubmit 也能拿到行，但 copying 时 in 为空
	src, _, _, err := s.cfg.Sources.DocSourceForSubmit(ctx, id)
	if err != nil {
		if apperr.Is(err, apperr.NotFound) {
			return store.ConvertSource{}, apperr.New(apperr.NotFound, "找不到这条记录").WithDetail("reason=record")
		}
		return store.ConvertSource{}, err
	}
	if src.Kind != "" && src.Kind != store.SourceKindDoc {
		return store.ConvertSource{}, apperr.New(apperr.Unsupported, "不支持这种记录").WithDetail("reason=format")
	}
	return src, nil
}

func (s *Service) getDocTask(ctx context.Context, id string) (task.Task, error) {
	type getter interface {
		Get(taskID string) (task.Task, error)
	}
	if g, ok := s.cfg.Tasks.(getter); ok {
		tk, err := g.Get(id)
		if err != nil {
			return task.Task{}, apperr.New(apperr.NotFound, "找不到这条记录").WithDetail("reason=record")
		}
		return tk, nil
	}
	return task.Task{}, s.internalNotReady("任务管理器")
}

func (s *Service) sourceBusy(ctx context.Context, t *editTarget) error {
	if t.Source != nil {
		if t.Source.CopyState == store.CopyCopying {
			return apperr.New(apperr.TaskConflict, "文件还在准备中，准备好后再保存。").
				WithDetail("reason=copying\nsourceId=" + t.SourceID)
		}
		if has, err := s.sourceHasActive(ctx, t.SourceID); err == nil && has {
			return apperr.New(apperr.TaskConflict, "文件正在转换，转完再保存。").WithDetail("reason=converting")
		}
	}
	if t.Task != nil && t.Task.Reconverting {
		return apperr.New(apperr.TaskConflict, "文件正在转换，转完再保存。").WithDetail("reason=converting")
	}
	// 结果行也可能又被排队重转：再查一次最新
	if t.Task != nil {
		if cur, err := s.getDocTask(ctx, t.TaskID); err == nil && (cur.Reconverting || cur.Status.Active()) {
			return apperr.New(apperr.TaskConflict, "文件正在转换，转完再保存。").WithDetail("reason=converting")
		}
	}
	return nil
}

func (s *Service) sourceHasActive(ctx context.Context, sourceID string) (bool, error) {
	type checker interface {
		SourceHasActiveTasks(ctx context.Context, sourceID string) (bool, error)
	}
	if c, ok := s.cfg.Sources.(checker); ok {
		return c.SourceHasActiveTasks(ctx, sourceID)
	}
	return false, nil
}

func (s *Service) uploadsDir(ctx context.Context) string {
	if s.cfg.UploadsDir != nil {
		return s.cfg.UploadsDir(ctx)
	}
	if s.cfg.DataDir != "" {
		return filepath.Join(s.cfg.DataDir, "uploads")
	}
	return ""
}

// validateSaveAsPath 另存为目标路径校验（6.12.42）。
func (s *Service) validateSaveAsPath(ctx context.Context, target string) error {
	if target == "" || !filepath.IsAbs(target) {
		return apperr.New(apperr.InvalidArgument, "不能保存到应用自己的文件夹里，请换一个位置。")
	}
	if strings.HasPrefix(target, `\\?\`) || strings.HasPrefix(target, `\\.\`) {
		return apperr.New(apperr.InvalidArgument, "不能保存到应用自己的文件夹里，请换一个位置。")
	}
	target = filepath.Clean(target)
	if s.cfg.DataDir != "" && paths.InsideDataDir(s.cfg.DataDir, target) {
		return apperr.New(apperr.InvalidArgument, "不能保存到应用自己的文件夹里，请换一个位置。")
	}
	if up := s.uploadsDir(ctx); up != "" && paths.Within(up, target) {
		return apperr.New(apperr.InvalidArgument, "不能保存到应用自己的文件夹里，请换一个位置。")
	}
	dir := filepath.Dir(target)
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		return apperr.New(apperr.InvalidArgument, "不能保存到应用自己的文件夹里，请换一个位置。")
	}
	if fi, err := os.Lstat(target); err == nil && fi.IsDir() {
		return apperr.New(apperr.InvalidArgument, "不能保存到应用自己的文件夹里，请换一个位置。")
	}
	return nil
}

func sameFormatExt(srcExt, dstPath string) bool {
	d := normExt(filepath.Ext(dstPath))
	if srcExt == d {
		return true
	}
	// md ↔ markdown，html ↔ htm
	groups := [][]string{
		{"md", "markdown"},
		{"html", "htm"},
	}
	for _, g := range groups {
		inS, inD := false, false
		for _, e := range g {
			if e == srcExt {
				inS = true
			}
			if e == d {
				inD = true
			}
		}
		if inS && inD {
			return true
		}
	}
	return false
}

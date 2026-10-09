package doc

import (
	"bytes"
	"context"
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/store"
	"FFmpegFree/internal/task"
)

// DocSaveRequest 见契约 6.12.41。
type DocSaveRequest struct {
	SourceID string     `json:"sourceId,omitempty"`
	TaskID   string     `json:"taskId,omitempty"`
	Revision string     `json:"revision"`
	Text     *string    `json:"text,omitempty"`
	Rows     [][]string `json:"rows,omitempty"`
}

// DocSaveAsRequest 见契约 6.12.42。
type DocSaveAsRequest struct {
	SourceID   string     `json:"sourceId,omitempty"`
	TaskID     string     `json:"taskId,omitempty"`
	TargetPath string     `json:"targetPath"`
	Text       *string    `json:"text,omitempty"`
	Rows       [][]string `json:"rows,omitempty"`
	Encoding   string     `json:"encoding,omitempty"` // keep | utf8
}

// DocSaveResult 见契约 6.12.41。
type DocSaveResult struct {
	Path      string `json:"path"`
	Revision  string `json:"revision"`
	SizeBytes int64  `json:"sizeBytes"`
	SavedAt   int64  `json:"savedAt"`
}

// SaveDocText 写回原位置。
func (s *Service) SaveDocText(ctx context.Context, req DocSaveRequest) (DocSaveResult, error) {
	if req.Revision == "" {
		return DocSaveResult{}, apperr.New(apperr.InvalidArgument, "缺少版本信息")
	}
	t, err := s.resolveEditID(ctx, req.SourceID, req.TaskID)
	if err != nil {
		return DocSaveResult{}, err
	}
	if t.MissingOrig {
		return DocSaveResult{}, apperr.New(apperr.NotFound, "原文件已经不在了，改完只能另存为。").WithDetail("reason=file")
	}
	kind := textKindOf(t.Ext)
	if kind == "" {
		return DocSaveResult{}, apperr.New(apperr.InvalidArgument, "只能保存成同一种格式。").WithDetail("reason=format")
	}
	if err := checkTextOrRows(kind, req.Text, req.Rows); err != nil {
		return DocSaveResult{}, err
	}
	if err := s.sourceBusy(ctx, t); err != nil {
		return DocSaveResult{}, err
	}
	unlock := lockPath(t.WritePath)
	defer unlock()

	raw, err := os.ReadFile(t.WritePath)
	if err != nil {
		return DocSaveResult{}, mapReadWriteErr(err)
	}
	if int64(len(raw)) > maxEditTextBytes || sha256Hex(raw) != req.Revision {
		return DocSaveResult{}, apperr.New(apperr.TaskConflict, "文件在别处被改过了，请重新打开，或另存为。").WithDetail("reason=file_changed")
	}
	enc, line := detectEncoding(raw), detectLineEnding(raw)
	text, _ := decodeRawText(raw)
	if kind == "html" {
		cs := htmlMetaCharset(text)
		if cs != "" && !htmlEncodingSupported(cs) {
			return DocSaveResult{}, apperr.New(apperr.InvalidArgument, "只能保存成同一种格式。").WithDetail("reason=format")
		}
		if htmlDeclaredGBK(cs) {
			enc = encGBK
		}
	}
	data, err := buildSaveBytes(kind, enc, line, req.Text, req.Rows)
	if err != nil {
		return DocSaveResult{}, err
	}
	if int64(len(data)) > maxEditTextBytes {
		return DocSaveResult{}, apperr.New(apperr.InvalidArgument, "内容太多，没法在这里保存。请用默认程序打开编辑。").WithDetail("reason=too_large")
	}
	if err := writeAtomic(t.WritePath, data); err != nil {
		return DocSaveResult{}, err
	}
	s.afterTextSave(ctx, t, data)
	return DocSaveResult{
		Path: t.WritePath, Revision: sha256Hex(data), SizeBytes: int64(len(data)), SavedAt: time.Now().UnixMilli(),
	}, nil
}

// SaveDocTextAs 另存为。
func (s *Service) SaveDocTextAs(ctx context.Context, req DocSaveAsRequest) (DocSaveResult, error) {
	t, err := s.resolveEditID(ctx, req.SourceID, req.TaskID)
	if err != nil {
		return DocSaveResult{}, err
	}
	kind := textKindOf(t.Ext)
	if kind == "" {
		return DocSaveResult{}, apperr.New(apperr.InvalidArgument, "只能保存成同一种格式。").WithDetail("reason=format")
	}
	if err := checkTextOrRows(kind, req.Text, req.Rows); err != nil {
		return DocSaveResult{}, err
	}
	if err := s.validateSaveAsPath(ctx, req.TargetPath); err != nil {
		return DocSaveResult{}, err
	}
	target := filepath.Clean(req.TargetPath)
	if !sameFormatExt(t.Ext, target) {
		return DocSaveResult{}, apperr.New(apperr.InvalidArgument, "只能保存成同一种格式。").WithDetail("reason=format")
	}
	encMode := strings.TrimSpace(req.Encoding)
	if encMode == "" {
		encMode = "keep"
	}
	if encMode != "keep" && encMode != "utf8" {
		return DocSaveResult{}, apperr.New(apperr.InvalidArgument, "不支持的编码")
	}
	// 读原文件编码/换行（原文件不在时用副本）
	raw, err := os.ReadFile(t.ReadPath)
	if err != nil {
		return DocSaveResult{}, mapReadWriteErr(err)
	}
	enc, line := detectEncoding(raw), detectLineEnding(raw)
	text, _ := decodeRawText(raw)
	if kind == "html" {
		cs := htmlMetaCharset(text)
		if encMode == "keep" && cs != "" && !htmlEncodingSupported(cs) {
			return DocSaveResult{}, apperr.New(apperr.InvalidArgument, "只能保存成同一种格式。").WithDetail("reason=format")
		}
		if htmlDeclaredGBK(cs) && encMode == "keep" {
			enc = encGBK
		}
	}
	wantEnc := enc
	if encMode == "utf8" {
		wantEnc = encUTF8
		// html + utf8 + meta 声明别的编码 → 带 BOM
		if kind == "html" {
			cs := htmlMetaCharset(pickText(req.Text, text))
			if cs != "" && !strings.HasPrefix(strings.ToLower(cs), "utf") {
				wantEnc = encUTF8BOM
			}
		}
	}
	data, err := buildSaveBytes(kind, wantEnc, line, req.Text, req.Rows)
	if err != nil {
		return DocSaveResult{}, err
	}
	if int64(len(data)) > maxEditTextBytes || (kind == "csv" && req.Rows != nil && len(req.Rows) > maxEditCSVRows) {
		return DocSaveResult{}, apperr.New(apperr.InvalidArgument, "内容太多，没法在这里保存。请用默认程序打开编辑。").WithDetail("reason=too_large")
	}
	// 目标正在转换 → converting；被别的记录用着 → in_use；都不写。
	// 目标就是这一行自己的原文件 / 输出 → 当作覆盖保存，写完刷新这一行（6.12.42，架构师定）。
	own, err := s.saveAsTargetCheck(ctx, t, target)
	if err != nil {
		return DocSaveResult{}, err
	}
	unlock := lockPath(target)
	defer unlock()
	if err := writeAtomic(target, data); err != nil {
		return DocSaveResult{}, err
	}
	if own {
		s.afterTextSave(ctx, t, data)
	}
	s.allowReveal(target)
	return DocSaveResult{
		Path: target, Revision: sha256Hex(data), SizeBytes: int64(len(data)), SavedAt: time.Now().UnixMilli(),
	}, nil
}

func pickText(p *string, fallback string) string {
	if p != nil {
		return *p
	}
	return fallback
}

func checkTextOrRows(kind string, text *string, rows [][]string) error {
	if kind == "csv" {
		if text != nil || rows == nil {
			return apperr.New(apperr.InvalidArgument, "请按文件类型提供内容")
		}
		if len(rows) > maxEditCSVRows {
			return apperr.New(apperr.InvalidArgument, "内容太多，没法在这里保存。请用默认程序打开编辑。").WithDetail("reason=too_large")
		}
		for _, row := range rows {
			for _, c := range row {
				if len([]rune(c)) > maxCSVCellRunes {
					return apperr.New(apperr.InvalidArgument, "内容太多，没法在这里保存。请用默认程序打开编辑。").WithDetail("reason=too_large")
				}
			}
		}
		return nil
	}
	if text == nil || rows != nil {
		return apperr.New(apperr.InvalidArgument, "请按文件类型提供内容")
	}
	return nil
}

func buildSaveBytes(kind, enc, line string, text *string, rows [][]string) ([]byte, error) {
	if kind == "csv" {
		var buf bytes.Buffer
		w := csv.NewWriter(&buf)
		w.UseCRLF = line == lineCRLF
		if err := w.WriteAll(rows); err != nil {
			return nil, apperr.Wrap(apperr.Internal, "写入失败", err)
		}
		w.Flush()
		body := buf.Bytes()
		// csv 的换行已由 UseCRLF 处理；再按编码转
		if enc == encUTF8 || enc == encUTF8BOM {
			if enc == encUTF8BOM {
				return append(append([]byte{}, utf8BOM...), body...), nil
			}
			return body, nil
		}
		// body 是 UTF-8；转 GBK
		return encodeStrictGBK(string(body))
	}
	return encodeTextBytes(*text, enc, line)
}

func (s *Service) afterTextSave(ctx context.Context, t *editTarget, data []byte) {
	if t.Source != nil && t.Source.CopyState == store.CopyReady && t.Source.StoredPath != "" {
		if err := writeAtomic(t.Source.StoredPath, data); err != nil {
			s.markCopyFailed(ctx, t.Source, err)
		} else if fi, err := os.Stat(t.WritePath); err == nil {
			s.updateCopyIdentity(ctx, t.Source.CopyID, fi.Size(), fi.ModTime().UnixNano())
		}
	}
	if t.Task != nil {
		s.updateTaskSize(ctx, *t.Task, int64(len(data)))
	}
}

func (s *Service) markCopyFailed(ctx context.Context, src *store.ConvertSource, cause error) {
	type marker interface {
		MarkDocCopyFailed(ctx context.Context, copyID string, err *apperr.AppError) error
	}
	ae := apperr.New(apperr.IOError, "保存失败，请重试或另存为。").WithDetail("reason=io")
	if m, ok := s.cfg.Sources.(marker); ok {
		_ = m.MarkDocCopyFailed(ctx, src.CopyID, ae)
	}
	_ = cause
}

func (s *Service) updateCopyIdentity(ctx context.Context, copyID string, size, mtimeNs int64) {
	type upd interface {
		UpdateDocCopyIdentity(ctx context.Context, copyID string, size, mtimeNs int64) error
	}
	if u, ok := s.cfg.Sources.(upd); ok {
		_ = u.UpdateDocCopyIdentity(ctx, copyID, size, mtimeNs)
	}
}

func (s *Service) updateTaskSize(ctx context.Context, tk task.Task, size int64) {
	type upd interface {
		UpdateDocTaskResultSize(ctx context.Context, taskID string, size int64) error
	}
	if u, ok := s.cfg.Sources.(upd); ok {
		_ = u.UpdateDocTaskResultSize(ctx, tk.ID, size)
		return
	}
	type mgr interface {
		NotifyResultSize(taskID string, size int64) error
	}
	if m, ok := s.cfg.Tasks.(mgr); ok {
		_ = m.NotifyResultSize(tk.ID, size)
	}
}

func (s *Service) allowReveal(path string) {
	type allower interface {
		AllowReveal(path string)
	}
	if a, ok := s.cfg.Tasks.(allower); ok {
		a.AllowReveal(path)
	}
}

// isOwnTarget 另存为的目标是不是正在编辑的这一行自己的原文件（源文件行）或输出（结果行）。
func isOwnTarget(t *editTarget, target string) bool {
	k := pathKey(realPath(target))
	cands := []string{t.WritePath}
	if t.Source != nil {
		cands = append(cands, t.Source.OriginalPath)
	}
	if t.Task != nil {
		cands = append(cands, t.Task.OutputPath)
	}
	for _, c := range cands {
		if c == "" {
			continue
		}
		if pathKey(realPath(c)) == k || pathKey(c) == pathKey(target) {
			return true
		}
	}
	return false
}

// saveAsTargetCheck 另存为目标检查（6.12.42 / 6.12.49，架构师定）：
//   - 目标是任一记录正在转换的输入或输出（含这一行自己正在转换 / 重转）→ TASK_CONFLICT reason=converting；
//   - 目标是别的源文件行的原文件、别的记录的输出或某个副本 → IO_ERROR reason=in_use；
//   - 目标就是这一行自己的原文件 / 输出 → own=true，调用方按覆盖保存处理并刷新这一行。
func (s *Service) saveAsTargetCheck(ctx context.Context, t *editTarget, target string) (own bool, err error) {
	own = isOwnTarget(t, target)
	if own {
		if err := s.sourceBusy(ctx, t); err != nil {
			return true, err
		}
	}
	return own, s.conflictIfTargetBusy(ctx, target, t)
}

// conflictIfTargetBusy 目标正在转换 → converting；被别的记录用着 → in_use（这一行自己不算）。
func (s *Service) conflictIfTargetBusy(ctx context.Context, target string, t *editTarget) error {
	type usage interface {
		DocTargetPathUsage(ctx context.Context, target, ownSourceID, ownTaskID string) (converting, inUse bool, err error)
	}
	u, ok := s.cfg.Sources.(usage)
	if !ok {
		return nil
	}
	var ownSrc, ownTask string
	if t != nil {
		ownSrc, ownTask = t.SourceID, t.TaskID
	}
	converting, inUse, err := u.DocTargetPathUsage(ctx, target, ownSrc, ownTask)
	if err != nil {
		return apperr.Wrap(apperr.Internal, "出了点问题，请重试。", err)
	}
	if converting {
		return apperr.New(apperr.TaskConflict, "文件正在转换，转完再保存。").WithDetail("reason=converting")
	}
	if inUse {
		return apperr.New(apperr.IOError, "这个文件正被应用里的其他记录使用，请换一个位置另存。").WithDetail("reason=in_use")
	}
	return nil
}

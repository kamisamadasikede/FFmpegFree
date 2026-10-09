package doc

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"io"
	"os"
	"strings"
	"sync"
	"unicode/utf8"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/localassets"
)

// ---------- 文档预览（契约 6.12.32 + 6.12.37~6.12.48；本 PR 实现文字类 / raw / 简易路径） ----------

const (
	maxPreviewTextBytes = 2 << 20  // 2 MiB
	maxRawPreviewBytes  = 50 << 20 // 50 MiB
	maxDocxEditBytes    = 20 << 20 // 20 MiB
	maxLivePreviews     = 8
)

// DocPreviewRequest 见契约 6.12.32.1。
type DocPreviewRequest struct {
	SourceID string `json:"sourceId,omitempty"`
	TaskID   string `json:"taskId,omitempty"`
}

// DocPreview 见契约 6.12.32.1 / 6.12.38 / 6.12.48。
type DocPreview struct {
	PreviewID  string           `json:"previewId"`
	Kind       string           `json:"kind"`
	State      string           `json:"state"`
	Name       string           `json:"name"`
	Ext        string           `json:"ext"`
	URL        string           `json:"url,omitempty"`
	RawURL     string           `json:"rawUrl,omitempty"`
	Text       string           `json:"text,omitempty"`
	Rows       [][]string       `json:"rows,omitempty"`
	TotalRows  int64            `json:"totalRows,omitempty"`
	Truncated  bool             `json:"truncated,omitempty"`
	SizeBytes  int64            `json:"sizeBytes"`
	Reason     string           `json:"reason,omitempty"`
	Error      *apperr.AppError `json:"error,omitempty"`
	Editable   bool             `json:"editable"`
	EditBlock  string           `json:"editBlock,omitempty"`
	Revision   string           `json:"revision,omitempty"`
	Encoding   string           `json:"encoding,omitempty"`
	LineEnding string           `json:"lineEnding,omitempty"`
}

type previewEntry struct {
	id     string
	tokens []string           // localassets token（url / rawUrl / 生成后的 PDF）
	pin    string             // 预览缓存里钉住的 PDF；空 = 无
	cancel context.CancelFunc // 引擎生成中可取消；nil = 无
}

// previewHub 进程内未释放的预览（最多 8 个）。
type previewHub struct {
	mu   sync.Mutex
	list []previewEntry
}

func (s *Service) hub() *previewHub {
	if s.previews == nil {
		s.previews = &previewHub{}
	}
	return s.previews
}

// GetDocPreview 开始（或同步返回）一个预览。本 PR：text/md/html/csv/raw/unavailable；引擎 PDF 留给并行 v0.27 PR。
func (s *Service) GetDocPreview(ctx context.Context, req DocPreviewRequest) (DocPreview, error) {
	t, err := s.resolveEditID(ctx, req.SourceID, req.TaskID)
	if err != nil {
		return DocPreview{}, err
	}
	fi, err := os.Lstat(t.ReadPath)
	if err != nil || !fi.Mode().IsRegular() {
		return DocPreview{}, apperr.New(apperr.NotFound, "原文件已经不在了，改完只能另存为。").WithDetail("reason=file")
	}
	out := DocPreview{
		PreviewID: newPreviewID(),
		State:     "ready",
		Name:      t.Name,
		Ext:       t.Ext,
		SizeBytes: fi.Size(),
	}
	kind := textKindOf(t.Ext)
	if kind == "" && t.Ext != "pdf" {
		// 有本机 Office/WPS 或文档组件时：生成 PDF 预览（6.12.32 引擎路径 + 缓存）
		if engines := s.previewEngines(ctx, t.Ext); len(engines) > 0 {
			s.fillEnginePreview(ctx, t, &out, engines)
			return out, nil
		}
	}
	switch {
	case kind != "":
		s.fillTextPreview(ctx, t, &out, kind)
	case t.Ext == "pdf":
		s.fillPDFDirect(t, &out)
	case t.Ext == "docx" || t.Ext == "xlsx":
		s.fillRawOrUnavailable(t, &out)
	default:
		// doc/xls/ppt/odt/... 无引擎时 unavailable
		out.Kind = "unavailable"
		out.Reason = "needs_component"
		out.Editable = false
		out.EditBlock = "format"
	}
	s.trackPreview(out.PreviewID, out.URL, out.RawURL)
	return out, nil
}

// CancelDocPreview 释放预览（幂等）。
func (s *Service) CancelDocPreview(previewID string) error {
	if previewID == "" {
		return nil
	}
	h := s.hub()
	h.mu.Lock()
	defer h.mu.Unlock()
	for i, e := range h.list {
		if e.id == previewID {
			h.list = append(h.list[:i], h.list[i+1:]...)
			s.releaseEntry(e)
			return nil
		}
	}
	return nil
}

// releaseEntry 撤销 token、取消生成、解除缓存钉住。调用方持有 hub 锁。
func (s *Service) releaseEntry(e previewEntry) {
	if e.cancel != nil {
		e.cancel()
	}
	if s.cfg.Local != nil {
		for _, tok := range e.tokens {
			if tok != "" {
				s.cfg.Local.Revoke(tok)
			}
		}
	}
	if e.pin != "" && s.prevCache != nil {
		s.prevCache.Pin(e.pin, false)
	}
}

func (s *Service) trackPreview(id, url, rawURL string) {
	s.trackEntry(previewEntry{id: id, tokens: uniqTokens(tokenOfURL(url), tokenOfURL(rawURL))})
}

func uniqTokens(a, b string) []string {
	var out []string
	if a != "" {
		out = append(out, a)
	}
	if b != "" && b != a {
		out = append(out, b)
	}
	return out
}

func (s *Service) trackEntry(e previewEntry) {
	h := s.hub()
	h.mu.Lock()
	defer h.mu.Unlock()
	h.list = append(h.list, e)
	for len(h.list) > maxLivePreviews {
		old := h.list[0]
		h.list = h.list[1:]
		s.releaseEntry(old)
	}
}

// updateEntry 生成完成后把 PDF token / 钉住登记到仍存活的预览；预览已释放时返回 false。
func (s *Service) updateEntry(id, token, pin string) bool {
	h := s.hub()
	h.mu.Lock()
	defer h.mu.Unlock()
	for i := range h.list {
		if h.list[i].id == id {
			if token != "" {
				h.list[i].tokens = append(h.list[i].tokens, token)
			}
			h.list[i].pin = pin
			h.list[i].cancel = nil
			return true
		}
	}
	return false
}

func tokenOfURL(u string) string {
	if strings.HasPrefix(u, localassets.Prefix) {
		return strings.TrimPrefix(u, localassets.Prefix)
	}
	return ""
}

func newPreviewID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func (s *Service) fillTextPreview(ctx context.Context, t *editTarget, out *DocPreview, kind string) {
	raw, err := os.ReadFile(t.ReadPath)
	if err != nil {
		out.State = "failed"
		out.Kind = kind
		out.Error = mapReadWriteErr(err)
		out.Editable = false
		return
	}
	out.Revision = sha256Hex(raw)
	out.Encoding = detectEncoding(raw)
	out.LineEnding = detectLineEnding(raw)
	text, _ := decodeRawText(raw)

	out.Kind = kind
	truncated := false
	if kind == "csv" {
		rows, total, mal, trunc := parseCSVPreview(text)
		out.Rows, out.TotalRows, truncated = rows, total, trunc
		out.Truncated = trunc
		s.applyEditBlock(ctx, t, out, kind, truncated, mal, text)
		return
	}
	if len(raw) > maxPreviewTextBytes || int64(len([]byte(text))) > maxPreviewTextBytes {
		// 截断按 UTF-8 原文字节
		b := []byte(text)
		if len(b) > maxPreviewTextBytes {
			b = b[:maxPreviewTextBytes]
			for len(b) > 0 && !utf8.Valid(b) {
				b = b[:len(b)-1]
			}
			text = string(b)
		}
		truncated = true
	}
	out.Text = text
	out.Truncated = truncated
	s.applyEditBlock(ctx, t, out, kind, truncated, false, text)
}

func (s *Service) applyEditBlock(ctx context.Context, t *editTarget, out *DocPreview, kind string, truncated, csvMalformed bool, text string) {
	// 顺序：format → missing → too_large → encoding → malformed → converting（6.12.38）
	out.Editable = true
	out.EditBlock = ""
	if kind == "" || (kind != "text" && kind != "md" && kind != "html" && kind != "csv") {
		out.Editable, out.EditBlock = false, "format"
		return
	}
	if t.MissingOrig {
		out.Editable, out.EditBlock = false, "missing"
		// 产品允许前端仍可编辑只给另存为；editable=false 按契约
		return
	}
	if truncated || (kind == "csv" && out.TotalRows > maxEditCSVRows) {
		out.Editable, out.EditBlock = false, "too_large"
		return
	}
	if kind == "html" {
		cs := htmlMetaCharset(text)
		if cs != "" && !htmlEncodingSupported(cs) {
			out.Editable, out.EditBlock = false, "encoding"
			return
		}
	}
	if csvMalformed {
		out.Editable, out.EditBlock = false, "malformed"
		return
	}
	if err := s.sourceBusy(ctx, t); err != nil {
		out.Editable, out.EditBlock = false, "converting"
		return
	}
}

func parseCSVPreview(text string) (rows [][]string, total int64, malformed, truncated bool) {
	r := csv.NewReader(strings.NewReader(text))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			// 解析失败：整行塞进一个单元格（6.12.32.4），并标 malformed
			malformed = true
			if len(rows) < maxEditCSVRows {
				rows = append(rows, []string{""})
			}
			total++
			continue
		}
		total++
		if len(rows) < maxEditCSVRows {
			rows = append(rows, rec)
		}
	}
	if total > maxEditCSVRows {
		truncated = true
	}
	if rows == nil {
		rows = [][]string{}
	}
	return rows, total, malformed, truncated
}

func (s *Service) fillPDFDirect(t *editTarget, out *DocPreview) {
	out.Kind = "pdf"
	out.Editable = false
	out.EditBlock = "format"
	if url := s.registerLocal(t.ReadPath); url != "" {
		out.URL = url
	}
	if sum, _, err := fileSHA256(t.ReadPath); err == nil && out.SizeBytes <= maxDocxEditBytes {
		out.Revision = sum
	}
}

func (s *Service) fillRawOrUnavailable(t *editTarget, out *DocPreview) {
	// 无引擎：docx/xlsx → raw（≤50MiB）
	if out.SizeBytes > maxRawPreviewBytes {
		out.Kind = "unavailable"
		out.Reason = "too_large_for_simple"
		out.Editable = false
		out.EditBlock = "format"
		return
	}
	// 加密检测
	if _, err := inspectDoc(context.Background(), t.ReadPath, t.Ext); err != nil {
		if apperr.Is(err, apperr.DocEncrypted) {
			out.State = "failed"
			out.Kind = "raw"
			out.Error = apperr.From(err)
			out.Editable = false
			out.EditBlock = "format"
			return
		}
	}
	out.Kind = "raw"
	url := s.registerLocal(t.ReadPath)
	out.URL = url
	if t.Ext == "docx" {
		s.fillDocxEdit(t, out, url)
	} else {
		out.Editable = false
		out.EditBlock = "format"
	}
}

func (s *Service) fillDocxEdit(t *editTarget, out *DocPreview, rawURL string) {
	// editBlock 顺序：format → missing → too_large → malformed → macro → converting（6.12.48）
	out.RawURL = rawURL
	if out.SizeBytes <= maxDocxEditBytes {
		if sum, _, err := fileSHA256(t.ReadPath); err == nil {
			out.Revision = sum
		}
	}
	out.Editable = true
	out.EditBlock = ""
	if t.MissingOrig {
		out.Editable, out.EditBlock = false, "missing"
		return
	}
	if out.SizeBytes > maxDocxEditBytes {
		out.Editable, out.EditBlock = false, "too_large"
		return
	}
	if bad, reason := docxOpenCheck(t.ReadPath); bad {
		out.Editable, out.EditBlock = false, reason
		return
	}
	if err := s.sourceBusy(context.Background(), t); err != nil {
		out.Editable, out.EditBlock = false, "converting"
		return
	}
}

func (s *Service) registerLocal(path string) string {
	if s.cfg.Local == nil {
		return ""
	}
	e, err := s.cfg.Local.Register(path)
	if err != nil {
		return ""
	}
	return e.URL
}

// docxOpenCheck 打开时轻量检查：返回 (blocked, editBlock)。
func docxOpenCheck(path string) (bool, string) {
	// 加密 OLE
	f, err := os.Open(path)
	if err != nil {
		return true, "malformed"
	}
	head := make([]byte, 8)
	n, _ := f.Read(head)
	f.Close()
	if n == 8 && bytes.Equal(head, oleMagic) {
		return true, "malformed"
	}
	if err := checkZipEntries(path); err != nil {
		return true, "malformed"
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		return true, "malformed"
	}
	defer zr.Close()
	if findEntry(zr, "word/document.xml") == nil {
		return true, "malformed"
	}
	if docxHasMacro(zr) {
		return true, "macro"
	}
	return false, ""
}

func docxHasMacro(zr *zip.ReadCloser) bool {
	for _, f := range zr.File {
		name := strings.ToLower(f.Name)
		if strings.HasSuffix(name, "vbaproject.bin") {
			return true
		}
	}
	ct := findEntry(zr, "[Content_Types].xml")
	if ct == nil {
		return false
	}
	rc, err := ct.Open()
	if err != nil {
		return false
	}
	defer rc.Close()
	b, _ := io.ReadAll(io.LimitReader(rc, 2<<20))
	return bytes.Contains(bytes.ToLower(b), []byte("macroenabled"))
}

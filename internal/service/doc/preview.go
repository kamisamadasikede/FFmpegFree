package doc

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/doceng"
	"FFmpegFree/internal/paths"
	"FFmpegFree/internal/store"
	"FFmpegFree/internal/task"
)

// ---------- 文档预览（契约 v0.27，6.12.32） ----------

const (
	previewTextMax   = 2 << 20 // 2 MiB
	previewCSVMax    = 2 << 20 // 读入上限
	previewCSVRows   = 1000
	previewRawMax    = 50 << 20 // 50 MiB
	previewMaxActive = 8
	EventDocPreview  = "doc:preview"
)

// DocPreviewRequest / DocPreview 见契约 6.12.32.1 / 6.12.38。
type DocPreviewRequest struct {
	SourceID string `json:"sourceId,omitempty"`
	TaskID   string `json:"taskId,omitempty"`
}

type DocPreview struct {
	PreviewID string           `json:"previewId"`
	Kind      string           `json:"kind"`
	State     string           `json:"state"`
	Name      string           `json:"name"`
	Ext       string           `json:"ext"`
	URL       string           `json:"url,omitempty"`
	Text      string           `json:"text,omitempty"`
	Rows      [][]string       `json:"rows,omitempty"`
	TotalRows int64            `json:"totalRows,omitempty"`
	Truncated bool             `json:"truncated,omitempty"`
	SizeBytes int64            `json:"sizeBytes"`
	Reason    string           `json:"reason,omitempty"`
	Error     *apperr.AppError `json:"error,omitempty"`
	// v0.27.1 字段：本 PR 只 stub，不实现保存。
	Editable   bool   `json:"editable"`
	EditBlock  string `json:"editBlock,omitempty"`
	Revision   string `json:"revision,omitempty"`
	Encoding   string `json:"encoding,omitempty"`
	LineEnding string `json:"lineEnding,omitempty"`
	RawURL     string `json:"rawUrl,omitempty"` // v0.27.2；本 PR 暂不填（编辑未实现）
}

type DocPreviewEvent struct {
	PreviewID string           `json:"previewId"`
	State     string           `json:"state"`
	Kind      string           `json:"kind"`
	URL       string           `json:"url,omitempty"`
	Error     *apperr.AppError `json:"error,omitempty"`
}

type previewSlot struct {
	id, token, path string
	cancel          context.CancelFunc
	created         time.Time
}

func (s *Service) ensurePreview() {
	s.prevOnce.Do(func() {
		root := s.cfg.DataDir
		if root == "" {
			root = os.TempDir()
		}
		s.prevCache = newPreviewCache(root)
		s.previews = map[string]*previewSlot{}
		s.prevQueue = make(chan previewJob, 64)
		go s.previewWorker()
	})
}

type previewJob struct {
	id, path, ext, pathKey string
	size, mtime            int64
	engines                []string
	hasMacro               bool
	ctx                    context.Context
}

// GetDocPreview 开始或从缓存拿预览（6.12.32）。
func (s *Service) GetDocPreview(ctx context.Context, req DocPreviewRequest) (DocPreview, error) {
	s.ensurePreview()
	path, name, ext, size, mtime, pathKey, err := s.resolvePreviewFile(ctx, req)
	id := newPreviewID()
	out := DocPreview{PreviewID: id, Name: name, Ext: ext, SizeBytes: size, State: "ready"}
	if err != nil {
		if apperr.Is(err, apperr.DocEncrypted) {
			out.State = "failed"
			out.Kind = "pdf"
			out.Error = apperr.From(err)
			out.Editable, out.EditBlock = false, "format"
			return out, nil
		}
		return DocPreview{}, err
	}

	switch ext {
	case "pdf":
		url, tok, err := s.registerPreviewURL(path)
		if err != nil {
			return DocPreview{}, err
		}
		s.trackPreview(id, tok, path, nil)
		out.Kind, out.URL = "pdf", url
		out.Editable, out.EditBlock = false, "format"
		return out, nil
	case "txt":
		return s.previewText(out, path, "text")
	case "md", "markdown":
		out.Ext = "md"
		return s.previewText(out, path, "md")
	case "html", "htm":
		out.Ext = "html"
		return s.previewText(out, path, "html")
	case "csv":
		return s.previewCSV(out, path)
	}

	// Office 类：有引擎 → 生成 PDF；否则 raw / unavailable
	macro := hasMacro(path, ext)
	engines := []string{}
	if s.cfg.Engines != nil {
		pref := "auto"
		if s.cfg.DocEngine != nil {
			pref = s.cfg.DocEngine(ctx)
		}
		for _, c := range doceng.Pick(doceng.PrefOrder(pref), s.cfg.Engines.Engines(ctx), s.cfg.Engines.Skips(), ext, "pdf") {
			engines = append(engines, c.ID)
		}
	}

	if len(engines) == 0 {
		return s.previewNoEngine(out, path, ext, size)
	}

	// 缓存命中
	if pdf, _, ok := s.prevCache.Lookup(pathKey, size, mtime, engines); ok {
		url, tok, err := s.registerPreviewURL(pdf)
		if err != nil {
			return DocPreview{}, err
		}
		s.prevCache.Pin(pdf, true)
		s.trackPreview(id, tok, pdf, nil)
		out.Kind, out.URL, out.Editable, out.EditBlock = "pdf", url, false, "format"
		return out, nil
	}

	// 异步生成
	pctx, cancel := context.WithCancel(context.Background())
	s.trackPreview(id, "", "", cancel)
	out.Kind, out.State, out.Editable, out.EditBlock = "pdf", "generating", false, "format"
	if macro {
		out.EditBlock = "macro"
	}
	job := previewJob{id: id, path: path, ext: ext, pathKey: pathKey, size: size, mtime: mtime, engines: engines, hasMacro: macro, ctx: pctx}
	select {
	case s.prevQueue <- job:
	default:
		cancel()
		out.State = "failed"
		out.Error = apperr.New(apperr.Internal, "出了点问题，请重试。")
	}
	s.emitPreview(DocPreviewEvent{PreviewID: id, State: "generating", Kind: "pdf"})
	return out, nil
}

func (s *Service) previewNoEngine(out DocPreview, path, ext string, size int64) (DocPreview, error) {
	out.Editable, out.EditBlock = false, "format"
	if ext == "docx" || ext == "xlsx" {
		if size > previewRawMax {
			out.Kind, out.Reason = "unavailable", "too_large_for_simple"
			return out, nil
		}
		if hasMacro(path, ext) {
			// 加密已在 resolve 外层处理；宏的 raw 仍给，前端库可能打不开含宏的——契约说 raw 也先检测加密
		}
		url, tok, err := s.registerPreviewURL(path)
		if err != nil {
			return DocPreview{}, err
		}
		s.trackPreview(out.PreviewID, tok, path, nil)
		out.Kind, out.URL = "raw", url
		return out, nil
	}
	out.Kind, out.Reason = "unavailable", "needs_component"
	return out, nil
}

func (s *Service) previewText(out DocPreview, path, kind string) (DocPreview, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return DocPreview{}, readErr("读取文件失败", err)
	}
	enc, lineEnd := detectEncodingAndEnding(raw)
	rev := sha256Hex(raw)
	text := decodeText(raw)
	trunc := false
	if len(text) > previewTextMax {
		text = text[:previewTextMax]
		for len(text) > 0 && !utf8.Valid(text) {
			text = text[:len(text)-1]
		}
		trunc = true
	}
	out.Kind, out.Text, out.Truncated = kind, string(text), trunc
	out.Revision, out.Encoding, out.LineEnding = rev, enc, lineEnd
	out.Editable = !trunc && (kind == "text" || kind == "md" || kind == "html")
	if trunc {
		out.Editable, out.EditBlock = false, "too_large"
	}
	// 本 PR 不实现编辑保存；仍标 editable 供前端编译。Word 类已是 format。
	s.trackPreview(out.PreviewID, "", path, nil)
	return out, nil
}

func (s *Service) previewCSV(out DocPreview, path string) (DocPreview, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return DocPreview{}, readErr("读取文件失败", err)
	}
	enc, lineEnd := detectEncodingAndEnding(raw)
	rev := sha256Hex(raw)
	text := decodeText(raw)
	truncRead := false
	if len(text) > previewCSVMax {
		text = text[:previewCSVMax]
		truncRead = true
	}
	r := csv.NewReader(strings.NewReader(string(text)))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	var rows [][]string
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			// 解析出错的行按原样放进一个单元格
			rows = append(rows, []string{err.Error()})
			continue
		}
		for i, c := range rec {
			if len(c) > maxCellRunes {
				rec[i] = string([]rune(c)[:maxCellRunes])
			}
		}
		rows = append(rows, rec)
		if len(rows) >= previewCSVRows {
			break
		}
	}
	total := int64(len(rows))
	// 若截断了读入，继续数行
	if truncRead || len(rows) >= previewCSVRows {
		total = countCSVRows(path)
		out.Truncated = truncRead
	}
	out.Kind, out.Rows, out.TotalRows = "csv", rows, total
	out.Revision, out.Encoding, out.LineEnding = rev, enc, lineEnd
	out.Editable = total <= previewCSVRows && !truncRead
	if !out.Editable {
		out.EditBlock = "too_large"
	}
	s.trackPreview(out.PreviewID, "", path, nil)
	return out, nil
}

func countCSVRows(path string) int64 {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	deadline := time.Now().Add(5 * time.Second)
	var n int64
	buf := make([]byte, 64<<10)
	var carry byte
	for {
		if time.Now().After(deadline) {
			return n
		}
		k, err := f.Read(buf)
		for i := 0; i < k; i++ {
			if buf[i] == '\n' {
				n++
			}
			carry = buf[i]
		}
		if err == io.EOF {
			if k > 0 && carry != '\n' {
				n++
			}
			return n
		}
		if err != nil {
			return n
		}
	}
}

func detectEncodingAndEnding(raw []byte) (enc, lineEnd string) {
	enc = "utf8"
	if len(raw) >= 3 && raw[0] == 0xEF && raw[1] == 0xBB && raw[2] == 0xBF {
		enc = "utf8_bom"
	} else if !utf8.Valid(bytesTrimBOM(raw)) {
		enc = "gbk"
	}
	crlf, lf := 0, 0
	for i := 0; i < len(raw); i++ {
		if raw[i] == '\n' {
			if i > 0 && raw[i-1] == '\r' {
				crlf++
			} else {
				lf++
			}
		}
	}
	if crlf == 0 && lf == 0 {
		if isWindows() {
			lineEnd = "crlf"
		} else {
			lineEnd = "lf"
		}
	} else if crlf >= lf {
		lineEnd = "crlf"
	} else {
		lineEnd = "lf"
	}
	return enc, lineEnd
}

func bytesTrimBOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	return b
}

func isWindows() bool { return runtime.GOOS == "windows" }

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func newPreviewID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func (s *Service) registerPreviewURL(path string) (url, token string, err error) {
	if s.cfg.Local == nil {
		return "", "", apperr.New(apperr.Internal, "文档服务尚未初始化")
	}
	e, err := s.cfg.Local.Register(path)
	if err != nil {
		return "", "", apperr.Wrap(apperr.IOError, "无法准备预览", err)
	}
	return e.URL, e.Token, nil
}

func (s *Service) trackPreview(id, token, path string, cancel context.CancelFunc) {
	s.ensurePreview()
	s.prevMu.Lock()
	defer s.prevMu.Unlock()
	if s.previews == nil {
		s.previews = map[string]*previewSlot{}
	}
	s.previews[id] = &previewSlot{id: id, token: token, path: path, cancel: cancel, created: time.Now()}
	for len(s.previews) > previewMaxActive {
		var oldest string
		var t time.Time
		for k, v := range s.previews {
			if k == id {
				continue
			}
			if oldest == "" || v.created.Before(t) {
				oldest, t = k, v.created
			}
		}
		if oldest == "" {
			break
		}
		s.releasePreviewLocked(oldest)
	}
}

func (s *Service) releasePreviewLocked(id string) {
	p, ok := s.previews[id]
	if !ok {
		return
	}
	delete(s.previews, id)
	if p.cancel != nil {
		p.cancel()
	}
	if p.token != "" && s.cfg.Local != nil {
		s.cfg.Local.Revoke(p.token)
	}
	if p.path != "" && s.prevCache != nil {
		s.prevCache.Pin(p.path, false)
	}
}

// CancelDocPreview 取消 / 释放预览（幂等）。
func (s *Service) CancelDocPreview(previewID string) error {
	s.ensurePreview()
	s.prevMu.Lock()
	defer s.prevMu.Unlock()
	s.releasePreviewLocked(previewID)
	return nil
}

func (s *Service) emitPreview(ev DocPreviewEvent) {
	if s.cfg.Emit != nil {
		s.cfg.Emit(EventDocPreview, ev)
	}
}

func (s *Service) previewWorker() {
	for job := range s.prevQueue {
		s.runPreviewJob(job)
	}
}

func (s *Service) runPreviewJob(job previewJob) {
	deadline := time.Now().Add(doceng.TimeoutPreviewMax)
	work := filepath.Join(s.cfg.TempRoot, "preview-"+job.id)
	_ = os.MkdirAll(work, 0o755)
	defer os.RemoveAll(work)

	key := cacheKey(job.pathKey, job.size, job.mtime, job.engines[0]) // 用第一个引擎做 part 名；成功后再按实际引擎提交
	part := s.prevCache.PartPath(key)

	engineID, err := s.cfg.Engines.TryPreviewPDF(job.ctx, job.ext, job.path, part, work, job.hasMacro, nil, deadline)
	if err != nil {
		// 全部失败：docx/xlsx → 尝试在事件里... 契约：生成全部失败时不回退简易，docx/xlsx 退到 raw
		s.finishPreviewFailed(job, err)
		return
	}
	fi, err := os.Stat(part)
	if err != nil || fi.Size() > MaxPDFBytes {
		_ = os.Remove(part)
		s.finishPreviewFailed(job, apperr.New(apperr.Internal, "出了点问题，请重试。"))
		return
	}
	// 用实际引擎的 key 提交
	realKey := cacheKey(job.pathKey, job.size, job.mtime, engineID)
	if realKey != key {
		realPart := s.prevCache.PartPath(realKey)
		_ = os.Rename(part, realPart)
		part = realPart
		key = realKey
	}
	final, err := s.prevCache.Commit(key, engineID, fi.Size())
	if err != nil {
		s.finishPreviewFailed(job, apperr.Wrap(apperr.IOError, "无法准备预览", err))
		return
	}
	url, tok, err := s.registerPreviewURL(final)
	if err != nil {
		s.finishPreviewFailed(job, err)
		return
	}
	s.prevCache.Pin(final, true)
	s.prevMu.Lock()
	if p, ok := s.previews[job.id]; ok {
		p.token, p.path, p.cancel = tok, final, nil
	}
	s.prevMu.Unlock()
	s.emitPreview(DocPreviewEvent{PreviewID: job.id, State: "ready", Kind: "pdf", URL: url})
}

func (s *Service) finishPreviewFailed(job previewJob, err error) {
	ae := apperr.From(err)
	// docx/xlsx 全部失败 → 契约说退到 raw；通过事件带 failed，前端可重试 GetDocPreview。
	// 这里按契约发 failed；若想同步 raw 需前端再调。保持 failed。
	s.emitPreview(DocPreviewEvent{PreviewID: job.id, State: "failed", Kind: "pdf", Error: ae})
	s.prevMu.Lock()
	s.releasePreviewLocked(job.id)
	s.prevMu.Unlock()
}

func (s *Service) resolvePreviewFile(ctx context.Context, req DocPreviewRequest) (path, name, ext string, size, mtime int64, pathKey string, err error) {
	hasS, hasT := req.SourceID != "", req.TaskID != ""
	if hasS == hasT {
		return "", "", "", 0, 0, "", apperr.New(apperr.InvalidArgument, "请指定源文件或转换记录")
	}
	if hasS {
		if s.cfg.Sources == nil {
			return "", "", "", 0, 0, "", s.internalNotReady("文档服务")
		}
		src, in, _, e := s.cfg.Sources.DocSourceForSubmit(ctx, req.SourceID)
		if e != nil {
			return "", "", "", 0, 0, "", e
		}
		if src.Kind != store.SourceKindDoc {
			return "", "", "", 0, 0, "", apperr.New(apperr.Unsupported, "不支持这种文件。").WithDetail("reason=format")
		}
		// 显示路径：副本就绪用副本
		path = in
		if path == "" {
			path = src.Path
		}
		name = src.Name
		ext = normExt(filepath.Ext(path))
		if ext == "" {
			ext = normExt(filepath.Ext(name))
		}
	} else {
		if s.cfg.TaskGet == nil {
			return "", "", "", 0, 0, "", s.internalNotReady("文档服务")
		}
		t, e := s.cfg.TaskGet(ctx, req.TaskID)
		if e != nil {
			return "", "", "", 0, 0, "", e
		}
		if t.Type != task.TypeDocConvert && t.Type != task.TypeOfficePDF {
			return "", "", "", 0, 0, "", apperr.New(apperr.Unsupported, "不支持这种文件。").WithDetail("reason=format")
		}
		if t.Status != task.StatusSucceeded || t.OutputPath == "" {
			return "", "", "", 0, 0, "", apperr.New(apperr.NotFound, "文件不存在").WithDetail("reason=file")
		}
		path, name = t.OutputPath, filepath.Base(t.OutputPath)
		ext = normExt(filepath.Ext(path))
	}
	fi, e := os.Stat(path)
	if e != nil || !fi.Mode().IsRegular() {
		return "", "", "", 0, 0, "", apperr.New(apperr.NotFound, "文件不存在").WithDetail("reason=file")
	}
	// 加密检测（Office 类）
	if ext != "txt" && ext != "md" && ext != "html" && ext != "csv" && ext != "pdf" {
		if _, ie := inspectDoc(ctx, path, ext); apperr.Is(ie, apperr.DocEncrypted) {
			// 同步返回 failed 形态：契约说文件本身问题放 state=failed
			// 但 GetDocPreview 的同步错误不含加密——放在返回值里。这里用特殊处理：返回 ready 结构由调用方...
			// 重读契约：同步 Go 错误只有调用问题；文件问题放 state=failed + error。
			// 所以这里不 return error，而由上层处理。我们返回一个特殊 sentinel。
			return path, name, ext, fi.Size(), fi.ModTime().UnixNano(), "", ie
		}
	}
	_, key, _ := paths.Normalize(path)
	if key == "" {
		key = path
	}
	return path, name, ext, fi.Size(), fi.ModTime().UnixNano(), key, nil
}

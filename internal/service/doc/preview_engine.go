package doc

import (
	"context"
	"os"
	"path/filepath"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/doceng"
	"FFmpegFree/internal/paths"
)

// ---------- 引擎 PDF 预览（契约 6.12.32 引擎路径 + 6.12.33 缓存，v0.27） ----------

// EventDocPreview 生成完成 / 失败时推送。
const EventDocPreview = "doc:preview"

// DocPreviewEvent 见契约 6.12.32.2。
type DocPreviewEvent struct {
	PreviewID string           `json:"previewId"`
	State     string           `json:"state"`
	Kind      string           `json:"kind"`
	URL       string           `json:"url,omitempty"`
	Error     *apperr.AppError `json:"error,omitempty"`
}

type previewJob struct {
	id, path, ext, pathKey string
	size, mtime            int64
	engines                []string
	hasMacro               bool
	rawURL                 string // docx/xlsx 已登记的原文件地址；生成全部失败时退到 raw
	ctx                    context.Context
}

func (s *Service) ensurePreviewWorker() {
	s.prevOnce.Do(func() {
		root := s.cfg.DataDir
		if root == "" {
			root = os.TempDir()
		}
		s.prevCache = newPreviewCache(root)
		s.prevQueue = make(chan previewJob, 64)
		go func() {
			for job := range s.prevQueue {
				s.runPreviewJob(job)
			}
		}()
	})
}

// previewEngines 按设置与能力表挑出能把 ext 转成 PDF 的引擎；没有接引擎时返回空。
func (s *Service) previewEngines(ctx context.Context, ext string) []string {
	if s.cfg.Engines == nil {
		return nil
	}
	pref := "auto"
	if s.cfg.DocEngine != nil {
		if p := s.cfg.DocEngine(ctx); p != "" {
			pref = p
		}
	}
	var ids []string
	for _, c := range doceng.Pick(doceng.PrefOrder(pref), s.cfg.Engines.Engines(ctx), s.cfg.Engines.Skips(), ext, "pdf") {
		ids = append(ids, c.ID)
	}
	return ids
}

func (s *Service) fillEnginePreview(ctx context.Context, t *editTarget, out *DocPreview, engines []string) {
	s.ensurePreviewWorker()
	out.Kind = "pdf"
	out.Editable, out.EditBlock = false, "format"

	if _, err := inspectDoc(ctx, t.ReadPath, t.Ext); apperr.Is(err, apperr.DocEncrypted) {
		out.State = "failed"
		out.Error = apperr.From(err)
		s.trackPreview(out.PreviewID, "", "")
		return
	}
	fi, err := os.Stat(t.ReadPath)
	if err != nil {
		out.State = "failed"
		out.Error = mapReadWriteErr(err)
		s.trackPreview(out.PreviewID, "", "")
		return
	}
	// docx：编辑字段沿用 6.12.48（rawUrl 给前端编辑器）；xlsx：raw 只作生成失败时的回退
	rawURL := ""
	if (t.Ext == "docx" || t.Ext == "xlsx") && out.SizeBytes <= maxRawPreviewBytes {
		rawURL = s.registerLocal(t.ReadPath)
		if t.Ext == "docx" {
			s.fillDocxEdit(t, out, rawURL)
		}
	}
	_, pathKey, _ := paths.Normalize(t.ReadPath)
	if pathKey == "" {
		pathKey = t.ReadPath
	}
	size, mtime := fi.Size(), fi.ModTime().UnixNano()

	// 缓存命中：同步给 ready
	if pdf, _, ok := s.prevCache.Lookup(pathKey, size, mtime, engines); ok {
		if url := s.registerLocal(pdf); url != "" {
			s.prevCache.Pin(pdf, true)
			out.URL = url
			s.trackEntry(previewEntry{id: out.PreviewID, tokens: uniqTokens(tokenOfURL(url), tokenOfURL(rawURL)), pin: pdf})
			return
		}
	}

	pctx, cancel := context.WithCancel(context.Background())
	out.State = "generating"
	s.trackEntry(previewEntry{id: out.PreviewID, tokens: uniqTokens(tokenOfURL(rawURL), ""), cancel: cancel})
	job := previewJob{id: out.PreviewID, path: t.ReadPath, ext: t.Ext, pathKey: pathKey, size: size, mtime: mtime,
		engines: engines, hasMacro: hasMacro(t.ReadPath, t.Ext), rawURL: rawURL, ctx: pctx}
	select {
	case s.prevQueue <- job:
	default:
		cancel()
		out.State = "failed"
		out.Error = apperr.New(apperr.Internal, "出了点问题，请重试。")
	}
}

func (s *Service) emitPreview(ev DocPreviewEvent) {
	if s.cfg.Emit != nil {
		s.cfg.Emit(EventDocPreview, ev)
	}
}

func (s *Service) runPreviewJob(job previewJob) {
	if job.ctx.Err() != nil {
		return // 已取消
	}
	ctx, cancel := context.WithTimeout(job.ctx, doceng.TimeoutPreviewMax)
	defer cancel()
	deadline, _ := ctx.Deadline()

	tmp := s.cfg.TempRoot
	if tmp == "" {
		tmp = os.TempDir()
	}
	work := filepath.Join(tmp, "preview-"+job.id)
	_ = os.MkdirAll(work, 0o755)
	defer os.RemoveAll(work)

	key := cacheKey(job.pathKey, job.size, job.mtime, job.engines[0])
	part := s.prevCache.PartPath(key)
	engineID, err := s.cfg.Engines.TryPreviewPDF(ctx, job.ext, job.path, part, work, job.hasMacro, nil, deadline)
	if job.ctx.Err() != nil {
		_ = os.Remove(part)
		return
	}
	if err != nil {
		_ = os.Remove(part)
		s.finishPreviewFailed(job, err)
		return
	}
	fi, err := os.Stat(part)
	if err != nil || fi.Size() > MaxPDFBytes {
		_ = os.Remove(part)
		s.finishPreviewFailed(job, apperr.New(apperr.Internal, "出了点问题，请重试。"))
		return
	}
	if realKey := cacheKey(job.pathKey, job.size, job.mtime, engineID); realKey != key {
		realPart := s.prevCache.PartPath(realKey)
		if err := os.Rename(part, realPart); err != nil {
			_ = os.Remove(part)
			s.finishPreviewFailed(job, apperr.Wrap(apperr.IOError, "无法准备预览", err))
			return
		}
		key = realKey
	}
	final, err := s.prevCache.Commit(key, engineID, fi.Size())
	if err != nil {
		s.finishPreviewFailed(job, apperr.Wrap(apperr.IOError, "无法准备预览", err))
		return
	}
	url := s.registerLocal(final)
	if url == "" {
		s.finishPreviewFailed(job, apperr.New(apperr.Internal, "出了点问题，请重试。"))
		return
	}
	s.prevCache.Pin(final, true)
	if !s.updateEntry(job.id, tokenOfURL(url), final) {
		// 生成期间预览已被释放
		s.prevCache.Pin(final, false)
		if s.cfg.Local != nil {
			s.cfg.Local.Revoke(tokenOfURL(url))
		}
		return
	}
	s.emitPreview(DocPreviewEvent{PreviewID: job.id, State: "ready", Kind: "pdf", URL: url})
}

// finishPreviewFailed：docx/xlsx（≤50MiB）退到 raw；其余发 failed（6.12.32）。
func (s *Service) finishPreviewFailed(job previewJob, err error) {
	if job.rawURL != "" {
		if s.updateEntry(job.id, "", "") {
			s.emitPreview(DocPreviewEvent{PreviewID: job.id, State: "ready", Kind: "raw", URL: job.rawURL})
		}
		return
	}
	s.updateEntry(job.id, "", "")
	s.emitPreview(DocPreviewEvent{PreviewID: job.id, State: "failed", Kind: "pdf", Error: apperr.From(err)})
}

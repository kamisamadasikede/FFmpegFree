package convert

import (
	"context"
	"encoding/json"
	"os"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/store"
	"FFmpegFree/internal/task"
)

// ---------- 文档页的源文件行与重转（契约 v0.26，6.12.14 / 6.12.21） ----------

// DocReconverter 由文档服务注册：为一条已成功的 doc_convert / 文档页 office_pdf 记录造一个直接写 temp 的 Runner（原地重转，6.17）。
// in 是这一行当前的读取路径。返回新 params JSON 和日志里的摘要。
type DocReconverter func(ctx context.Context, old task.Task, in, temp string) (r task.Runner, params, summary string, err error)

// SetDocReconverter 注册文档记录的重转（文档服务在创建时调用）。
func (s *Service) SetDocReconverter(f DocReconverter) {
	s.docMu.Lock()
	s.docRC = f
	s.docMu.Unlock()
}

// reconvertDoc 是 doc_convert / 文档页 office_pdf 的重转：不接受 presetId / options（params_locked），同目标格式，规则同 6.17。
func (s *Service) reconvertDoc(ctx context.Context, tr TaskRecords, ss SourceStore, old task.Task, req ReconvertRequest) (task.Task, error) {
	if req.PresetID != "" || req.Options != nil {
		return task.Task{}, apperr.New(apperr.InvalidArgument, "文档转换的记录只能按原来的设置重转").WithDetail("reason=params_locked")
	}
	s.docMu.Lock()
	f := s.docRC
	s.docMu.Unlock()
	if f == nil {
		return task.Task{}, apperr.New(apperr.Internal, "文档服务尚未初始化")
	}
	var p params
	if err := json.Unmarshal([]byte(old.Params), &p); err != nil || (p.Input == "" && len(old.InputPaths) == 0) {
		return task.Task{}, apperr.New(apperr.InvalidArgument, "转换任务参数无效，无法重转")
	}
	in, src, err := s.reconvertInput(ctx, ss, old, p)
	if err != nil {
		return task.Task{}, err
	}
	if !regularFile(in) {
		return task.Task{}, sourceMissingError(in)
	}
	mode, block := task.ReconvertOutputMode(old)
	if block != "" {
		return task.Task{}, task.OutputMovedError()
	}
	temp := task.ReconvertTempPath(old.OutputPath, old.ID)
	r, np, summary, err := f(ctx, old, in, temp)
	if err != nil {
		return task.Task{}, err
	}
	t, err := tr.Reconvert(old.ID, task.ReconvertSpec{Params: np, InputPaths: []string{in}, Summary: summary, Mode: mode}, r)
	if err != nil {
		return task.Task{}, err
	}
	if src != nil {
		s.touch(ctx, []string{src.SourceID})
	} else if old.SourceID != "" {
		s.touch(ctx, []string{old.SourceID})
	}
	return t, nil
}

// AddDocSource 建 / 找文档页的源文件行（kind=doc，契约 6.12.16）并排副本复制，返回带实时进度的行。
// 调用方已完成扩展名、大小、加密、损坏检查；p / key 来自 paths.Normalize，fi 是 os.Stat(p)。
func (s *Service) AddDocSource(ctx context.Context, p, key string, fi os.FileInfo, sheetCount int) (ConvertSource, bool, error) {
	tr, ss, err := s.records()
	if err != nil {
		return ConvertSource{}, false, err
	}
	ds, ok := ss.(DocSourceStore)
	if !ok {
		return ConvertSource{}, false, apperr.New(apperr.Internal, "转换服务尚未初始化")
	}
	src, existed, err := ds.UpsertConvertSourceKind(ctx, p, key, store.SourceKindDoc, sheetCount, s.cfg.Now())
	if err != nil {
		return ConvertSource{}, false, apperr.Wrap(apperr.Internal, "保存源文件行失败", err)
	}
	if s.copyEnabled() {
		if err := s.ensureCopy(ctx, tr, src, p, key, fi); err != nil {
			return ConvertSource{}, existed, err
		}
		if src, err = s.source(ctx, ss, src.SourceID); err != nil {
			return ConvertSource{}, existed, err
		}
	}
	return s.liveSource(src), existed, nil
}

// ListDocSources / SearchDocSources 同 ListSources / SearchSources，只列 kind=doc 的行（6.12.14）。
func (s *Service) ListDocSources(ctx context.Context, f ConvertSourceFilter) (ConvertSourcePage, error) {
	return s.listSourcesKind(ctx, store.SourceKindDoc, "", f.Status, f.Limit, f.Offset, f.RecordLimit)
}

// SearchDocSources 见 ListDocSources。
func (s *Service) SearchDocSources(ctx context.Context, f ConvertSearchFilter) (ConvertSourcePage, error) {
	kw, err := searchKeyword(f.Keyword)
	if err != nil {
		return ConvertSourcePage{}, err
	}
	return s.listSourcesKind(ctx, store.SourceKindDoc, kw, f.Status, f.Limit, f.Offset, f.RecordLimit)
}

// DocSourceForSubmit 读一行给文档页提交用：返回行本身和读取路径（副本没就绪时 in 为 "" 且 reason 是 skipped 的原因）。
func (s *Service) DocSourceForSubmit(ctx context.Context, id string) (src ConvertSource, in, skipReason string, err error) {
	_, ss, err := s.records()
	if err != nil {
		return ConvertSource{}, "", "", err
	}
	src, err = s.source(ctx, ss, id)
	if err != nil {
		return ConvertSource{}, "", "", err
	}
	switch src.CopyState {
	case store.CopyCopying:
		return src, "", "copying", nil
	case store.CopyFailed:
		return src, "", "copy_failed", nil
	case store.CopyCanceled:
		return src, "", "copy_canceled", nil
	}
	return src, readPath(src), "", nil
}

// CopyNotReady 是一行都没就绪时的整体错误（同 SubmitSources）。
func CopyNotReady(src ConvertSource, anyCopying bool) error {
	if anyCopying {
		src.CopyState = store.CopyCopying
	}
	return copyNotReadyError(src)
}

// TouchSources 把行的 lastActivityAt 设为现在。
func (s *Service) TouchSources(ctx context.Context, ids []string) { s.touch(ctx, ids) }

// ResolveOutputDir 同提交时的输出目录解析（给文档页用）。
func (s *Service) ResolveOutputDir(ctx context.Context, dir string) (string, error) {
	return s.resolveOutputDir(ctx, dir)
}

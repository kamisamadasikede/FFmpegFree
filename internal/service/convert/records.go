package convert

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/ffmpeg"
	"FFmpegFree/internal/localassets"
	"FFmpegFree/internal/paths"
	"FFmpegFree/internal/store"
	"FFmpegFree/internal/task"
)

// ---------- 转换记录（契约 v0.23 / v0.23.1，6.14） ----------

// SourceStore 是源文件行的持久化能力（*store.Store 实现）。
type SourceStore interface {
	UpsertConvertSource(ctx context.Context, path, key string, now int64) (store.ConvertSource, bool, error)
	GetConvertSource(ctx context.Context, id string) (store.ConvertSource, error)
	TouchConvertSources(ctx context.Context, ids []string, now int64) error
	DeleteConvertSource(ctx context.Context, id string) (*store.ConvertCopy, error)
	ListConvertSources(ctx context.Context, keyword, status string, limit, offset int) ([]store.ConvertSource, int64, error)
	MatchedSourceTaskIDs(ctx context.Context, sourceID, keyword string, limit int) ([]string, error)
	ListSourceTasks(ctx context.Context, sourceID string, limit, offset int) (store.TaskPage, error)
	SourceTaskIDs(ctx context.Context, sourceID string) ([]string, error)
	MediaByPathKey(ctx context.Context, key string) (*store.MediaInfo, error)
}

// DocSourceStore 是文档页需要的源文件行能力（契约 v0.26，*store.Store 实现）。
type DocSourceStore interface {
	UpsertConvertSourceKind(ctx context.Context, path, key, kind string, sheetCount int, now int64) (store.ConvertSource, bool, error)
	ListConvertSourcesKind(ctx context.Context, kind, keyword, status string, limit, offset int) ([]store.ConvertSource, int64, error)
}

var _ DocSourceStore = (*store.Store)(nil)

var _ SourceStore = (*store.Store)(nil)

// Thumbnailer 生成默认缩略图并返回 data URL（*media.Service 实现）。
type Thumbnailer interface {
	DefaultThumbnailDataURL(ctx context.Context, path string, durationHint float64) (string, error)
}

// Previewer 是 6.13 的本地文件登记表（*localassets.Registry 实现）。
type Previewer interface {
	Register(path string) (localassets.Entry, error)
	RevokePath(path string) int
}

// ConvertSource 是转换页上的一行（契约 6.14.2）。
type ConvertSource = store.ConvertSource

// ConvertSourceEntry 是 ListSources / SearchSources 的一项，也是 GetSource 的返回值。
type ConvertSourceEntry struct {
	Source         ConvertSource `json:"source"`
	Records        []task.Task   `json:"records"`
	RecordCount    int64         `json:"recordCount"`
	NameMatched    bool          `json:"nameMatched,omitempty"`
	MatchedTaskIDs []string      `json:"matchedTaskIds,omitempty"`
}

// ConvertSourceFilter 是 ListSources 的分页参数。
type ConvertSourceFilter struct {
	Limit       int `json:"limit"`
	Offset      int `json:"offset"`
	RecordLimit int `json:"recordLimit"`
	// Status 筛选行（契约 v0.23.1）："" 全部；"active" 有排队 / 运行中的记录；"failed" 有失败 / 中断的记录（取消不算）。
	// 只筛行，不筛每行内嵌的记录和 recordCount。
	Status string `json:"status,omitempty"`
}

// ConvertSearchFilter 是 SearchSources 的参数。
type ConvertSearchFilter struct {
	Keyword     string `json:"keyword"`
	Limit       int    `json:"limit"`
	Offset      int    `json:"offset"`
	RecordLimit int    `json:"recordLimit"`
	// Status 与 ConvertSourceFilter.Status 完全相同（契约 v0.23.2）："" | "active" | "failed"，与关键字是 AND；
	// 只筛行，不筛每行内嵌的记录和 recordCount。
	Status string `json:"status,omitempty"`
}

// ConvertSourcePage 是 ListSources / SearchSources 的结果。
type ConvertSourcePage struct {
	Items []ConvertSourceEntry `json:"items"`
	Total int64                `json:"total"`
}

// AddSourceResult 与 AddSources 入参一一对应。
type AddSourceResult struct {
	Path    string           `json:"path"`
	Source  *ConvertSource   `json:"source,omitempty"`
	Existed bool             `json:"existed"`
	Error   *apperr.AppError `json:"error,omitempty"`
}

// ConvertSubmitRequest 是 SubmitSources 的参数。
type ConvertSubmitRequest struct {
	SourceIDs []string              `json:"sourceIds"`
	Options   ffmpeg.ConvertOptions `json:"options"`
	OutputDir string                `json:"outputDir"`
	PresetID  string                `json:"presetId"`
}

// SourcePathCheck 与 CheckSources 入参一一对应。
type SourcePathCheck struct {
	SourceID       string `json:"sourceId"`
	Found          bool   `json:"found"`
	Exists         bool   `json:"exists"`         // 读取路径存在（ready 看副本，其余看原文件）
	OriginalExists bool   `json:"originalExists"` // v0.24
	StoredExists   bool   `json:"storedExists"`   // v0.24：副本是普通文件；没有副本为 false
}

// ConvertSubmitResult 是 SubmitSources 的结果（契约 v0.24，6.15.4 第 6 条）。
type ConvertSubmitResult struct {
	Tasks   []task.Task     `json:"tasks"`   // 实际提交的任务；没有时是 []
	Skipped []SkippedSource `json:"skipped"` // 因为副本没就绪而跳过的行；没有时是 []
}

// SkippedSource 是 SubmitSources 跳过的一行。
type SkippedSource struct {
	SourceID string `json:"sourceId"`
	Reason   string `json:"reason"` // "copying" | "copy_failed" | "copy_canceled"
}

// ReconvertRequest 是 Reconvert 的参数（契约 v0.24，6.17.1）。
type ReconvertRequest struct {
	TaskID   string                 `json:"taskId"`
	PresetID string                 `json:"presetId,omitempty"`
	Options  *ffmpeg.ConvertOptions `json:"options,omitempty"`
}

// PreviewURL 是 /local/<token> 预览地址（同 EditService.GetPreviewURL 的返回值）。
type PreviewURL struct {
	URL  string `json:"url"`
	Mime string `json:"mime"`
	Size int64  `json:"size"`
}

const (
	maxBatch           = 500
	defaultSourceLimit = 50
	maxSourceLimit     = 200
	defaultRecordLimit = 20
	maxRecordLimit     = 100
	defaultRecordsPage = 50
	maxRecordsPage     = 200
	maxKeywordRunes    = 100
	maxMatchedTaskIDs  = 200
)

// previewExts 是转换页 / 任务中心应用内预览的白名单（契约 v0.24 改写的 6.14.7“可以预览”档）。
var previewExts = extSet("mp4 m4v mov webm mkv ogv mp3 m4a m4r aac wav flac ogg opus gif webp png jpg jpeg bmp ico")

// openExts 是“用系统程序打开”的白名单（6.14.7）：可以预览档 + 只能用系统程序打开档。只放媒体扩展名。
var openExts = extSet("mp4 m4v mov webm mkv ogv mp3 m4a m4r aac wav flac ogg opus gif webp png jpg jpeg bmp ico " +
	"avi flv wmv mpg mpeg vob 3gp swf ts mts m2ts wma amr ape wv mmf mp2 aif aiff tif tiff tga")

// docOpenExts 是文档页的行和记录（kind=doc / doc_convert / office_pdf）额外能用系统程序打开的扩展名（v0.26，6.12.14）。
var docOpenExts = extSet("pdf doc docx odt rtf txt html htm md markdown xls xlsx ods csv ppt pptx odp")

// mediaThumbExts 是能做缩略图的扩展名（同 openExts 的媒体部分；文档没有缩略图，6.12.14）。
var mediaThumbExts = openExts

func extSet(list string) map[string]bool {
	m := map[string]bool{}
	for _, e := range strings.Fields(list) {
		m["."+e] = true
	}
	return m
}

func hasExt(set map[string]bool, p string) bool { return set[strings.ToLower(filepath.Ext(p))] }

func formatUnsupported(msg string) error {
	return apperr.New(apperr.Unsupported, msg).WithDetail("reason=format")
}

func (s *Service) records() (TaskRecords, SourceStore, error) {
	tr, ok := s.cfg.Tasks.(TaskRecords)
	if !ok || s.cfg.Sources == nil {
		return nil, nil, apperr.New(apperr.Internal, "转换服务尚未初始化")
	}
	return tr, s.cfg.Sources, nil
}

// source 读源文件行；不存在 NOT_FOUND（reason=record）。
func (s *Service) source(ctx context.Context, ss SourceStore, id string) (ConvertSource, error) {
	src, err := ss.GetConvertSource(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return ConvertSource{}, task.RecordNotFound()
	}
	if err != nil {
		return ConvertSource{}, apperr.Wrap(apperr.Internal, "读取源文件行失败", err)
	}
	return src, nil
}

// displayPath 是“显示路径”（6.15.6）：副本 ready 用 storedPath，其余（none / copying / failed / canceled）用原文件。
func displayPath(src ConvertSource) string {
	if src.CopyState == store.CopyReady && src.StoredPath != "" {
		return src.StoredPath
	}
	return src.Path
}

// readPath 是“读取路径”（6.15.4 第 4 条）：ready 读副本，none 读原文件；copying / failed / canceled 返回 ""（不能转换）。
func readPath(src ConvertSource) string {
	switch src.CopyState {
	case store.CopyReady:
		return src.StoredPath
	case store.CopyNone, "":
		return src.Path
	}
	return ""
}

// copyNotReadyError 是副本没就绪时的 TASK_CONFLICT（6.15.4 第 6 条）：copying → reason=copying，其余 → reason=copy_failed。
func copyNotReadyError(src ConvertSource) error {
	if src.CopyState == store.CopyCopying {
		// 提交转换用。重转另有一句，见 reconvertInput（契约 v0.24.6）。
		return copyingConflict(src, "文件还在准备中，准备好后再转换。")
	}
	return apperr.New(apperr.TaskConflict, "文件复制没有完成，请先重试复制").WithDetail("reason=copy_failed\nsourceId=" + src.SourceID)
}

func copyingConflict(src ConvertSource, msg string) error {
	return apperr.New(apperr.TaskConflict, msg).WithDetail("reason=copying\nsourceId=" + src.SourceID)
}

// sourceFile 返回源文件行的显示路径；路径为空、文件不在或不是普通文件 NOT_FOUND（reason=file）。
func sourceFile(src ConvertSource) (string, error) {
	p := displayPath(src)
	if p == "" || !filepath.IsAbs(p) {
		return "", task.FileNotFound()
	}
	fi, err := os.Stat(p)
	if err != nil || !fi.Mode().IsRegular() {
		return "", task.FileNotFound()
	}
	return p, nil
}

// originalFile 是用户的原文件（v0.24.3）：始终用 originalPath，不用 uploads 里的副本。
// 原文件不在、不是普通文件或路径不合法时 NOT_FOUND（reason=file），不退回副本。
func originalFile(src ConvertSource) (string, error) {
	p := src.OriginalPath
	if p == "" {
		p = src.Path
	}
	if p == "" || !filepath.IsAbs(p) {
		return "", originalFileMissing()
	}
	fi, err := os.Stat(p)
	if err != nil || !fi.Mode().IsRegular() {
		return "", originalFileMissing()
	}
	return p, nil
}

func originalFileMissing() error {
	return apperr.New(apperr.NotFound, "原文件不存在，无法打开。").WithDetail("reason=file")
}

// AddSources 登记源文件（契约 6.14.3）：1~500 个，逐个规范化、stat，同 path_key 已有行只更新 lastActivityAt。不探测。
func (s *Service) AddSources(ctx context.Context, in []string) ([]AddSourceResult, error) {
	tr, ss, err := s.records()
	if err != nil {
		return nil, err
	}
	if len(in) == 0 || len(in) > maxBatch {
		return nil, apperr.New(apperr.InvalidArgument, fmt.Sprintf("一次添加 1~%d 个文件", maxBatch))
	}
	out := make([]AddSourceResult, len(in))
	for i, raw := range in {
		out[i].Path = raw
		if strings.TrimSpace(raw) == "" || !filepath.IsAbs(raw) {
			out[i].Error = apperr.New(apperr.InvalidArgument, "路径必须是绝对路径")
			continue
		}
		p, key, err := paths.Normalize(raw)
		if err != nil {
			out[i].Error = apperr.New(apperr.InvalidArgument, "路径不合法")
			continue
		}
		fi, err := os.Stat(p)
		switch {
		case errors.Is(err, os.ErrNotExist):
			out[i].Error = apperr.New(apperr.NotFound, "文件不存在").WithDetail("reason=file")
			continue
		case err != nil:
			out[i].Error = apperr.Wrap(apperr.IOError, "无法读取文件信息", err)
			continue
		case !fi.Mode().IsRegular():
			out[i].Error = apperr.New(apperr.InvalidArgument, "不是普通文件")
			continue
		case !inputExts[strings.ToLower(filepath.Ext(p))]:
			out[i].Error = apperr.New(apperr.Unsupported, "不支持这种文件").WithDetail("reason=format")
			continue
		}
		src, existed, err := ss.UpsertConvertSource(ctx, p, key, s.cfg.Now())
		if err != nil {
			return nil, apperr.Wrap(apperr.Internal, "保存源文件行失败", err)
		}
		if s.copyEnabled() {
			if err := s.ensureCopy(ctx, tr, src, p, key, fi); err != nil {
				out[i].Error = apperr.From(err)
				continue
			}
			if src, err = s.source(ctx, ss, src.SourceID); err != nil {
				return nil, err
			}
		}
		src = s.liveSource(src)
		out[i].Source, out[i].Existed = &src, existed
	}
	// v0.23.4：顺带探测并持久化媒体信息（同一行只探测一次；文件没变不重探；探测失败照样有行，media 省略）。
	byID := map[string]*ConvertSource{}
	var uniq []*ConvertSource
	for i := range out {
		if out[i].Source == nil {
			continue
		}
		if first, ok := byID[out[i].Source.SourceID]; ok {
			out[i].Source = first
			continue
		}
		byID[out[i].Source.SourceID] = out[i].Source
		uniq = append(uniq, out[i].Source)
	}
	s.refreshSourcesMedia(ctx, ss, uniq)
	for _, src := range uniq {
		s.fillMediaFallback(ctx, ss, src)
	}
	for i := range out { // 同一行出现多次时各项各拿一份副本，不共享指针
		if out[i].Source != nil {
			c := *out[i].Source
			out[i].Source = &c
		}
	}
	return out, nil
}

// hasActiveTasks：这一行有排队中 / 运行中的转换记录（含重转中的）。
func hasActiveTasks(tr TaskRecords, sourceID string) bool {
	for _, t := range tr.ListActive() {
		if t.SourceID == sourceID {
			return true
		}
	}
	return false
}

func activeTasksConflict() error {
	return apperr.New(apperr.TaskConflict, "这个文件还有正在进行的转换，请等转换结束后再添加")
}

// ensureCopy 是添加时的“找副本 / 排复制”（6.15.4 第 2 条，按 2.1 → 2.4 的顺序）。
func (s *Service) ensureCopy(ctx context.Context, tr TaskRecords, src ConvertSource, p, key string, fi os.FileInfo) error {
	cs := s.copyStore()
	size, mtime := fi.Size(), fi.ModTime().UnixNano()
	attach := func(c store.ConvertCopy) error {
		if src.CopyID == c.ID {
			return nil
		}
		if hasActiveTasks(tr, src.SourceID) {
			return activeTasksConflict()
		}
		old, err := cs.AttachCopy(ctx, src.SourceID, c.ID)
		if err != nil {
			return apperr.Wrap(apperr.Internal, "保存副本失败", err)
		}
		s.releaseCopy(ctx, old, src.SourceID)
		return nil
	}
	// 2.1 这个路径本身就是某份 ready 副本。
	if list, err := cs.FindReadyCopiesByStoredPath(ctx, p); err == nil {
		for _, c := range list {
			if _, k, err := paths.Normalize(c.StoredPath); err == nil && k == key && storedOK(c) {
				return attach(c)
			}
		}
	}
	// 2.2 这一行已有的副本还能用。
	if src.CopyID != "" {
		if c, err := cs.GetCopy(ctx, src.CopyID); err == nil && c.OriginalSize == size && c.OriginalMtimeNs == mtime &&
			(c.State == store.CopyCopying || c.State == store.CopyReady && storedOK(c)) {
			return nil
		}
	}
	// 2.3 别的“同一个文件”的副本。
	if list, err := cs.FindCopiesByIdentity(ctx, key, size, mtime); err == nil {
		for _, c := range list {
			if c.ID != src.CopyID && (c.State == store.CopyCopying || storedOK(c)) {
				return attach(c)
			}
		}
	}
	// 2.4 新做一份。
	if hasActiveTasks(tr, src.SourceID) {
		return activeTasksConflict()
	}
	return s.startCopy(ctx, src, fi, nil)
}

// CancelCopy 取消这一行副本的复制（6.15.6）：copying → canceled，删 .part，行保留；已经 canceled 时什么都不做。
func (s *Service) CancelCopy(ctx context.Context, sourceID string) error {
	_, ss, err := s.records()
	if err != nil {
		return err
	}
	src, err := s.source(ctx, ss, sourceID)
	if err != nil {
		return err
	}
	switch src.CopyState {
	case store.CopyCanceled:
		return nil
	case store.CopyCopying:
	default:
		return apperr.New(apperr.TaskConflict, "没有正在进行的复制")
	}
	cs := s.copyStore()
	c, err := cs.GetCopy(ctx, src.CopyID)
	if err != nil {
		return apperr.Wrap(apperr.Internal, "读取副本失败", err)
	}
	_, copied, ok := s.copier.cancel(c.ID)
	if !ok {
		// 不在队列里：要么刚复制完（库里已不是 copying），要么是没有复制协程的残留状态。
		if cur, err := cs.GetCopy(ctx, c.ID); err != nil || cur.State != store.CopyCopying {
			return apperr.New(apperr.TaskConflict, "没有正在进行的复制")
		}
		copied = c.CopiedBytes
		os.Remove(copyPartPath(c.StoredPath))
	}
	if err := cs.UpdateCopyState(ctx, c.ID, store.CopyCanceled, copied, nil, s.cfg.Now()); err != nil {
		return apperr.Wrap(apperr.Internal, "保存副本失败", err)
	}
	s.emitCopy(c, store.CopyCanceled, copied, nil)
	return nil
}

// RetryCopy 重新复制（6.15.6）：failed / canceled，或 ready 但副本文件不在 / 大小不对。空间不足、原文件不在时返回的行 failed，不是调用错误。
func (s *Service) RetryCopy(ctx context.Context, sourceID string) (ConvertSource, error) {
	tr, ss, err := s.records()
	if err != nil {
		return ConvertSource{}, err
	}
	src, err := s.source(ctx, ss, sourceID)
	if err != nil {
		return ConvertSource{}, err
	}
	switch src.CopyState {
	case store.CopyNone, "":
		return ConvertSource{}, apperr.New(apperr.InvalidArgument, "这个文件不需要复制")
	case store.CopyCopying:
		return ConvertSource{}, apperr.New(apperr.TaskConflict, "文件还在准备中，不需要重试。")
	case store.CopyReady:
		if c, err := s.copyStore().GetCopy(ctx, src.CopyID); err == nil && storedOK(c) {
			return ConvertSource{}, apperr.New(apperr.TaskConflict, "文件已经在复制或已复制完成")
		}
	}
	if hasActiveTasks(tr, sourceID) {
		return ConvertSource{}, activeTasksConflict()
	}
	fi, statErr := os.Stat(src.Path)
	if statErr == nil && !fi.Mode().IsRegular() {
		statErr = os.ErrNotExist
	}
	if err := s.startCopy(ctx, src, fi, statErr); err != nil {
		return ConvertSource{}, err
	}
	src, err = s.source(ctx, ss, sourceID)
	if err != nil {
		return ConvertSource{}, err
	}
	return s.liveSource(src), nil
}

// fillMediaFallback：没有持久化的探测结果时，退回按 path_key 关联 media 表（契约 6.14.2）。
func (s *Service) fillMediaFallback(ctx context.Context, ss SourceStore, src *ConvertSource) {
	if src.Media != nil || src.Path == "" {
		return
	}
	if _, key, err := paths.Normalize(src.Path); err == nil {
		if m, err := ss.MediaByPathKey(ctx, key); err == nil {
			src.Media = m
		}
	}
}

func pageArgs(limit, offset, def, max int) (int, int, error) {
	if limit < 0 || limit > max || offset < 0 {
		return 0, 0, apperr.New(apperr.InvalidArgument, "分页参数不正确").WithDetail(fmt.Sprintf("limit 范围 0~%d，offset 不能小于 0", max))
	}
	if limit == 0 {
		limit = def
	}
	return limit, offset, nil
}

// entry 组装一行：最新 recordLimit 条记录（进行中的带实时进度）、记录总数、media 关联。
func (s *Service) entry(ctx context.Context, tr TaskRecords, ss SourceStore, src ConvertSource, recordLimit int) (ConvertSourceEntry, error) {
	page, err := ss.ListSourceTasks(ctx, src.SourceID, recordLimit, 0)
	if err != nil {
		return ConvertSourceEntry{}, apperr.Wrap(apperr.Internal, "查询记录失败", err)
	}
	tr.Live(page.Items)
	src = s.liveSource(src)
	if src.Kind != store.SourceKindDoc {
		s.fillMediaFallback(ctx, ss, &src)
	}
	return ConvertSourceEntry{Source: src, Records: page.Items, RecordCount: page.Total}, nil
}

func (s *Service) listSources(ctx context.Context, keyword, status string, limit, offset, recordLimit int) (ConvertSourcePage, error) {
	return s.listSourcesKind(ctx, store.SourceKindMedia, keyword, status, limit, offset, recordLimit)
}

func (s *Service) listSourcesKind(ctx context.Context, kind, keyword, status string, limit, offset, recordLimit int) (ConvertSourcePage, error) {
	tr, ss, err := s.records()
	if err != nil {
		return ConvertSourcePage{}, err
	}
	if !store.ValidSourceStatus(status) {
		return ConvertSourcePage{}, apperr.New(apperr.InvalidArgument, "筛选条件不正确").WithDetail(`status 只能是 ""、"active" 或 "failed"`)
	}
	limit, offset, err = pageArgs(limit, offset, defaultSourceLimit, maxSourceLimit)
	if err != nil {
		return ConvertSourcePage{}, err
	}
	if recordLimit < 0 || recordLimit > maxRecordLimit {
		return ConvertSourcePage{}, apperr.New(apperr.InvalidArgument, "每行记录条数超出范围").WithDetail(fmt.Sprintf("recordLimit 范围 0~%d", maxRecordLimit))
	}
	if recordLimit == 0 {
		recordLimit = defaultRecordLimit
	}
	var srcs []ConvertSource
	var total int64
	if kind == store.SourceKindMedia {
		srcs, total, err = ss.ListConvertSources(ctx, keyword, status, limit, offset)
	} else if ds, ok := ss.(DocSourceStore); ok {
		srcs, total, err = ds.ListConvertSourcesKind(ctx, kind, keyword, status, limit, offset)
	} else {
		return ConvertSourcePage{}, apperr.New(apperr.Internal, "转换服务尚未初始化")
	}
	if err != nil {
		return ConvertSourcePage{}, apperr.Wrap(apperr.Internal, "查询源文件行失败", err)
	}
	refs := make([]*ConvertSource, len(srcs))
	for i := range srcs {
		refs[i] = &srcs[i]
	}
	s.refreshSourcesMedia(ctx, ss, refs) // v0.23.4：缺媒体信息或文件变了的行懒探测补上
	page := ConvertSourcePage{Items: []ConvertSourceEntry{}, Total: total}
	for _, src := range srcs {
		e, err := s.entry(ctx, tr, ss, src, recordLimit)
		if err != nil {
			return ConvertSourcePage{}, err
		}
		if keyword != "" {
			e.NameMatched = strings.Contains(strings.ToLower(src.Name), keyword)
			ids, err := ss.MatchedSourceTaskIDs(ctx, src.SourceID, keyword, maxMatchedTaskIDs)
			if err != nil {
				return ConvertSourcePage{}, apperr.Wrap(apperr.Internal, "查询记录失败", err)
			}
			e.MatchedTaskIDs = ids
		}
		page.Items = append(page.Items, e)
	}
	return page, nil
}

// ListSources 分页列出源文件行（lastActivityAt 倒序），每行内嵌最新的 recordLimit 条记录和记录总数；
// filter.status 只筛行（v0.23.1），内嵌记录和 recordCount 不受影响。
func (s *Service) ListSources(ctx context.Context, f ConvertSourceFilter) (ConvertSourcePage, error) {
	return s.listSources(ctx, "", f.Status, f.Limit, f.Offset, f.RecordLimit)
}

// GetSource 返回一行（契约 v0.23.1），与 ListSources 的一项完全相同（默认 recordLimit 20 条最新记录 + 记录总数）。
// 任务中心“在转换页查看”用：前端拿任务的 sourceId 调它，把这一行置顶、展开并高亮记录。不存在 NOT_FOUND（reason=record）。
func (s *Service) GetSource(ctx context.Context, sourceID string) (ConvertSourceEntry, error) {
	tr, ss, err := s.records()
	if err != nil {
		return ConvertSourceEntry{}, err
	}
	src, err := s.source(ctx, ss, sourceID)
	if err != nil {
		return ConvertSourceEntry{}, err
	}
	s.refreshSourceMedia(ctx, ss, &src)
	return s.entry(ctx, tr, ss, src, defaultRecordLimit)
}

// SearchSources 文件名搜索（契约 6.14.9）：源文件名或任一记录的输出文件名包含关键字（不区分大小写的子串）；
// filter.status（v0.23.2）同 ListSources，与关键字是 AND。
func (s *Service) SearchSources(ctx context.Context, f ConvertSearchFilter) (ConvertSourcePage, error) {
	kw, err := searchKeyword(f.Keyword)
	if err != nil {
		return ConvertSourcePage{}, err
	}
	return s.listSources(ctx, kw, f.Status, f.Limit, f.Offset, f.RecordLimit)
}

func searchKeyword(raw string) (string, error) {
	kw := strings.TrimSpace(raw)
	if kw == "" || utf8.RuneCountInString(kw) > maxKeywordRunes {
		return "", apperr.New(apperr.InvalidArgument, fmt.Sprintf("关键字为 1~%d 个字", maxKeywordRunes))
	}
	return strings.ToLower(kw), nil
}

// ListSourceRecords 返回某一行的更多记录（limit 默认 50 最大 200）。
func (s *Service) ListSourceRecords(ctx context.Context, sourceID string, limit, offset int) (task.Page, error) {
	tr, ss, err := s.records()
	if err != nil {
		return task.Page{}, err
	}
	limit, offset, err = pageArgs(limit, offset, defaultRecordsPage, maxRecordsPage)
	if err != nil {
		return task.Page{}, err
	}
	if _, err := s.source(ctx, ss, sourceID); err != nil {
		return task.Page{}, err
	}
	page, err := ss.ListSourceTasks(ctx, sourceID, limit, offset)
	if err != nil {
		return task.Page{}, apperr.Wrap(apperr.Internal, "查询记录失败", err)
	}
	tr.Live(page.Items)
	return page, nil
}

// CheckSources 检查源文件现在是否还在：1~500 个，不存在的 id found=false。只有“不存在”算 false。
func (s *Service) CheckSources(ctx context.Context, ids []string) ([]SourcePathCheck, error) {
	_, ss, err := s.records()
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 || len(ids) > maxBatch {
		return nil, apperr.New(apperr.InvalidArgument, fmt.Sprintf("一次检查 1~%d 个源文件", maxBatch))
	}
	out := make([]SourcePathCheck, len(ids))
	for i, id := range ids {
		out[i].SourceID = id
		src, err := ss.GetConvertSource(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, apperr.Wrap(apperr.Internal, "读取源文件行失败", err)
		}
		out[i].Found = true
		out[i].OriginalExists = src.Path != "" && task.RegularExists(src.Path, true)
		out[i].StoredExists = src.StoredPath != "" && task.RegularExists(src.StoredPath, true)
		if src.CopyState == store.CopyReady {
			out[i].Exists = out[i].StoredExists
		} else {
			out[i].Exists = out[i].OriginalExists
		}
	}
	return out, nil
}

// PreviewOutputName 返回此刻会用的完整输出路径（“将保存为”），不占位、不探测。
func (s *Service) PreviewOutputName(ctx context.Context, sourceID string, opts ffmpeg.ConvertOptions, outputDir string) (string, error) {
	tr, ss, err := s.records()
	if err != nil {
		return "", err
	}
	src, err := s.source(ctx, ss, sourceID)
	if err != nil {
		return "", err
	}
	if err := validateOptions(opts); err != nil {
		return "", err
	}
	dir, err := s.resolveOutputDir(ctx, outputDir)
	if err != nil {
		return "", err
	}
	if src.Path == "" {
		return "", apperr.New(apperr.InvalidArgument, "源文件路径无效（记录损坏），请重新添加文件")
	}
	if dir == "" {
		dir = filepath.Dir(src.Path)
	}
	name := src.Name
	if name == "" {
		name = filepath.Base(src.Path)
	}
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	return tr.PeekOutputName(filepath.Join(dir, stem+"."+opts.Container), task.TypeConvert), nil
}

// SubmitSources 等同 Submit，输入来自源文件行（契约 6.14.3）；presetId 非空时写入当时的预设名称快照。
// v0.24（6.15.4 第 6 条）：副本没就绪的行跳过（skipped），只提交就绪的；一行都没就绪时整体 TASK_CONFLICT。
func (s *Service) SubmitSources(ctx context.Context, req ConvertSubmitRequest) (ConvertSubmitResult, error) {
	res := ConvertSubmitResult{Tasks: []task.Task{}, Skipped: []SkippedSource{}}
	_, ss, err := s.records()
	if err != nil {
		return res, err
	}
	if len(req.SourceIDs) == 0 || len(req.SourceIDs) > MaxInputsPerSubmit {
		return res, apperr.New(apperr.InvalidArgument, fmt.Sprintf("一次提交 1~%d 个文件", MaxInputsPerSubmit))
	}
	var jobs []submitJob
	var firstSkipped *ConvertSource
	anyCopying := false
	for _, id := range req.SourceIDs {
		src, err := s.source(ctx, ss, id)
		if err != nil {
			return res, err
		}
		reason := ""
		switch src.CopyState {
		case store.CopyCopying:
			reason, anyCopying = "copying", true
		case store.CopyFailed:
			reason = "copy_failed"
		case store.CopyCanceled:
			reason = "copy_canceled"
		}
		if reason != "" {
			res.Skipped = append(res.Skipped, SkippedSource{SourceID: src.SourceID, Reason: reason})
			if firstSkipped == nil {
				cp := src
				firstSkipped = &cp
			}
			continue
		}
		jobs = append(jobs, submitJob{in: readPath(src), sourceID: src.SourceID})
	}
	if len(jobs) == 0 {
		fs := *firstSkipped
		if anyCopying {
			fs.CopyState = store.CopyCopying
		}
		return res, copyNotReadyError(fs)
	}
	presetName, err := s.presetName(ctx, req.PresetID)
	if err != nil {
		return res, err
	}
	ts, err := s.submitWrap(ctx, jobs, req.Options, req.OutputDir, req.PresetID, presetName)
	if ts != nil {
		res.Tasks = ts
	}
	return res, err
}

// presetName 返回预设名称快照；id 为空返回 ""；不存在 NOT_FOUND（reason=record）。
func (s *Service) presetName(ctx context.Context, presetID string) (string, error) {
	p, err := s.findPreset(ctx, presetID)
	if err != nil || p == nil {
		return "", err
	}
	return p.Name, nil
}

func (s *Service) findPreset(ctx context.Context, presetID string) (*Preset, error) {
	if presetID == "" {
		return nil, nil
	}
	ps, err := s.ListPresets(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range ps {
		if p.ID == presetID {
			cp := p
			return &cp, nil
		}
	}
	return nil, apperr.New(apperr.NotFound, "预设不存在").WithDetail("reason=record")
}

func formatChangeError() error {
	return apperr.New(apperr.InvalidArgument, "重转不能更换格式，要换格式请新转一条。").WithDetail("reason=format_change")
}

func sourceMissingError(p string) error {
	return apperr.New(apperr.NotFound, "源文件不存在，无法重转").WithDetail("reason=file\n" + p)
}

// reconvertInput 返回记录当前的读取路径（这一行的副本状态决定，6.15.4 第 4 条）；行不在了（旧记录）用 params.input。
// 副本没就绪时返回 copyNotReadyError。
func (s *Service) reconvertInput(ctx context.Context, ss SourceStore, t task.Task, p params) (string, *ConvertSource, error) {
	if t.SourceID != "" {
		if src, err := ss.GetConvertSource(ctx, t.SourceID); err == nil {
			in := readPath(src)
			if in == "" {
				if src.CopyState == store.CopyCopying {
					return "", &src, copyingConflict(src, "文件还在准备中，准备好后再重转。")
				}
				return "", &src, copyNotReadyError(src)
			}
			return in, &src, nil
		}
	}
	in := p.Input
	if in == "" && len(t.InputPaths) > 0 {
		in = t.InputPaths[0]
	}
	return in, nil, nil
}

func regularFile(p string) bool {
	if p == "" || !filepath.IsAbs(p) {
		return false
	}
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}

// reconvertBlock 是注册给任务管理器的 CheckPaths 检查（副本没就绪 → copy_not_ready，源文件不在 → source_missing）。
func (s *Service) reconvertBlock(t task.Task) string {
	_, ss, err := s.records()
	if err != nil {
		return ""
	}
	var p params
	_ = json.Unmarshal([]byte(t.Params), &p)
	in, _, err := s.reconvertInput(context.Background(), ss, t, p)
	if err != nil {
		return task.BlockCopyNotReady
	}
	if !regularFile(in) {
		return task.BlockSourceGone
	}
	return ""
}

// Reconvert 在同一条记录上原地重转（契约 v0.24 / v0.24.1，6.17）。同步校验的顺序（v0.24.1 架构师定）：
// 记录不存在 / 旧类型 → 不是 convert → 状态（invalid_state）→ 副本没就绪（copying / copy_failed）→ 源文件不在（NOT_FOUND reason=file）
// → 旧输出被移动或替换（output_moved）→ 旧输出不在时不能改参数（params_locked，PM 15a）→ 换格式（format_change）→ 参数与格式检查。
// 任何同步错误都不改记录、不发事件。
func (s *Service) Reconvert(ctx context.Context, req ReconvertRequest) (task.Task, error) {
	tr, ss, err := s.records()
	if err != nil {
		return task.Task{}, err
	}
	old, err := tr.Get(req.TaskID)
	if err != nil {
		if apperr.Is(err, apperr.NotFound) {
			return task.Task{}, task.RecordNotFound()
		}
		return task.Task{}, err
	}
	if old.Type == task.TypeEditExport || old.Type == task.TypeEditRender {
		return task.Task{}, task.LegacyExportError("重转")
	}
	if !task.IsRecordTask(old) {
		return task.Task{}, apperr.New(apperr.InvalidArgument, "不是转换记录")
	}
	if old.Reconverting {
		return task.Task{}, task.InvalidStateError("这条记录正在重转")
	}
	if old.Status != task.StatusSucceeded {
		return task.Task{}, task.InvalidStateError("只有已完成的记录可以重转")
	}
	if old.Type != task.TypeConvert {
		return s.reconvertDoc(ctx, tr, ss, old, req)
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
	if mode == task.ReconvertRegenerate && (req.PresetID != "" || req.Options != nil) {
		return task.Task{}, apperr.New(apperr.InvalidArgument, "原来的输出文件不在了，只能按原来的参数重新生成").WithDetail("reason=params_locked")
	}
	opts, presetID, presetName, summary := p.Options, p.PresetID, p.PresetName, p.ParamsSummary
	if req.PresetID != "" || req.Options != nil {
		pr, err := s.findPreset(ctx, req.PresetID)
		if pr != nil && pr.Options.Container != p.Options.Container {
			return task.Task{}, formatChangeError()
		}
		if req.Options != nil && req.Options.Container != p.Options.Container {
			return task.Task{}, formatChangeError()
		}
		if err != nil {
			return task.Task{}, err
		}
		presetID, presetName = "", ""
		if pr != nil {
			opts, presetID, presetName = pr.Options, pr.ID, pr.Name
		}
		if req.Options != nil {
			opts = *req.Options
		}
		summary = ParamsSummary(opts)
	}
	if summary == "" {
		summary = ParamsSummary(opts)
	}
	if err := validateOptions(opts); err != nil {
		return task.Task{}, err
	}
	bin, err := s.cfg.Require()
	if err != nil {
		return task.Task{}, err
	}
	if err := s.checkFormat(ctx, opts); err != nil {
		return task.Task{}, err
	}
	j, err := s.prepare(ctx, in, opts, filepath.Dir(old.OutputPath))
	if err != nil {
		return task.Task{}, withInput(err, in)
	}
	temp := task.ReconvertTempPath(old.OutputPath, old.ID)
	np := params{Input: in, Options: opts, OutputDir: p.OutputDir, PresetID: presetID, PresetName: presetName, ParamsSummary: summary}
	pj, _ := json.Marshal(np)
	r := s.newRunnerTo(bin, in, temp, true, opts, j)
	t, err := tr.Reconvert(old.ID, task.ReconvertSpec{Params: string(pj), InputPaths: []string{in}, Summary: summary, Mode: mode}, r)
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

// TakeInterruptedReconverts 返回本次启动时恢复的“上次退出时被中断的重转”条数（v0.24.1 PM 13），
// 第一次调用后清零，之后都返回 0。前端文案：“上次退出时有 n 条重转被中断，原来的文件没有变动。”（普通颜色，只显示一次）。
func (s *Service) TakeInterruptedReconverts() int { return int(s.interrupted.Swap(0)) }

// DeleteRecords 删除转换记录（契约 6.14.4）：先取消进行中的、尽量删、把没删成的列出来；永远不删源文件。
func (s *Service) DeleteRecords(ctx context.Context, ids []string, deleteOutputs bool) (task.DeleteResult, error) {
	tr, _, err := s.records()
	if err != nil {
		return task.NewDeleteResult(), err
	}
	if len(ids) == 0 || len(ids) > maxBatch {
		return task.NewDeleteResult(), apperr.New(apperr.InvalidArgument, fmt.Sprintf("一次删除 1~%d 条记录", maxBatch))
	}
	return tr.DeleteRecords(ids, task.TypeConvert, deleteOutputs, s.revoke)
}

// DeleteSource 删除一行的全部记录，全部删掉后再删这一行；不删源文件。
func (s *Service) DeleteSource(ctx context.Context, sourceID string, deleteOutputs bool) (task.DeleteResult, error) {
	tr, ss, err := s.records()
	if err != nil {
		return task.NewDeleteResult(), err
	}
	src, err := s.source(ctx, ss, sourceID)
	if err != nil {
		return task.NewDeleteResult(), err
	}
	// 6.15.7 第 1 步：副本在复制中先取消（同 CancelCopy）。v0.24.1 实现取舍：副本还被别的行共享时不取消（取消作用于副本本身，
	// 会让共享的行一起变成“已取消复制”）；这一行删掉后引用减一，复制照常写完给别的行用。
	shared := false
	if src.CopyState == store.CopyCopying && src.CopyID != "" {
		if cs := s.copyStore(); cs != nil {
			if c, err := cs.GetCopy(ctx, src.CopyID); err == nil && c.RefCount > 1 {
				shared = true
			}
		}
	}
	if src.CopyState == store.CopyCopying && !shared {
		if err := s.CancelCopy(ctx, sourceID); err != nil && !apperr.Is(err, apperr.TaskConflict) {
			s.logf("删除前取消复制失败: %v", err)
		}
	}
	ids, err := ss.SourceTaskIDs(ctx, sourceID)
	if err != nil {
		return task.NewDeleteResult(), apperr.Wrap(apperr.Internal, "查询记录失败", err)
	}
	res := task.NewDeleteResult()
	if len(ids) > 0 {
		if res, err = tr.DeleteRecords(ids, task.TypeConvert, deleteOutputs, s.revoke); err != nil {
			return res, err
		}
	}
	left, err := ss.SourceTaskIDs(ctx, sourceID)
	if err != nil {
		return res, apperr.Wrap(apperr.Internal, "查询记录失败", err)
	}
	if len(left) == 0 {
		released, err := ss.DeleteConvertSource(ctx, sourceID)
		if err != nil {
			return res, apperr.Wrap(apperr.Internal, "删除源文件行失败", err)
		}
		res.DeletedSourceIDs = append(res.DeletedSourceIDs, sourceID)
		if f := s.releaseCopy(ctx, released, sourceID); f != nil {
			res.Failures = append(res.Failures, *f)
		}
	}
	return res, nil
}

// revoke 删除输出文件前撤销 convert 登记表里这个路径的 token（契约 6.14.4 第 3 步）。
func (s *Service) revoke(path string) {
	if s.cfg.Preview != nil {
		s.cfg.Preview.RevokePath(path)
	}
}

// register 把文件登记到 convert 登记表；扩展名不在预览白名单 UNSUPPORTED（reason=format）；
// v0.23.4：扩展名通过后再按探测出的编码挡一层（media 由 media() 给出，nil = 没探测到，不挡），见 playable.go。
func (s *Service) register(path string, media func() *store.MediaInfo) (PreviewURL, error) {
	if s.cfg.Preview == nil {
		return PreviewURL{}, apperr.New(apperr.Internal, "预览服务尚未初始化")
	}
	if !hasExt(previewExts, path) {
		return PreviewURL{}, formatUnsupported("无法在应用内播放这种文件")
	}
	if media != nil {
		if err := previewGate(media()); err != nil {
			return PreviewURL{}, err
		}
	}
	e, err := s.cfg.Preview.Register(path)
	switch {
	case err == nil:
		return PreviewURL{URL: e.URL, Mime: e.Mime, Size: e.Size}, nil
	case errors.Is(err, localassets.ErrNotRegular), errors.Is(err, os.ErrNotExist):
		return PreviewURL{}, task.FileNotFound()
	default:
		return PreviewURL{}, apperr.Wrap(apperr.IOError, "无法读取文件", err)
	}
}

// open 用系统默认程序打开；扩展名不在“系统打开白名单”UNSUPPORTED（reason=format）。
func (s *Service) open(path string, doc bool) error {
	if s.cfg.Open == nil {
		return apperr.New(apperr.Internal, "转换服务尚未初始化")
	}
	if !hasExt(openExts, path) && !(doc && hasExt(docOpenExts, path)) {
		return formatUnsupported("只能用系统程序打开音视频和文档文件")
	}
	return s.cfg.Open(path)
}

// GetSourcePreviewURL 把源文件登记到 convert 登记表并返回预览地址。
func (s *Service) GetSourcePreviewURL(ctx context.Context, sourceID string) (PreviewURL, error) {
	_, ss, err := s.records()
	if err != nil {
		return PreviewURL{}, err
	}
	src, err := s.source(ctx, ss, sourceID)
	if err != nil {
		return PreviewURL{}, err
	}
	if src.CopyState == store.CopyCopying {
		return PreviewURL{}, apperr.New(apperr.TaskConflict, "文件还在准备中，准备好后才能预览。").WithDetail("reason=copying")
	}
	p, err := sourceFile(src)
	if err != nil {
		return PreviewURL{}, err
	}
	return s.register(p, func() *store.MediaInfo {
		s.refreshSourceMedia(ctx, ss, &src) // 持久化的结果对得上当前文件就不重探
		return src.Media
	})
}

// OpenSourceWithSystem 用系统默认程序打开用户的原文件（v0.24.3：始终 originalPath，不打开副本）。
func (s *Service) OpenSourceWithSystem(ctx context.Context, sourceID string) error {
	_, ss, err := s.records()
	if err != nil {
		return err
	}
	src, err := s.source(ctx, ss, sourceID)
	if err != nil {
		return err
	}
	p, err := originalFile(src)
	if err != nil {
		return err
	}
	return s.open(p, src.Kind == store.SourceKindDoc)
}

// RevealSource 在文件管理器里显示用户的原文件（v0.24.3：打开 originalPath 所在文件夹并选中原文件，
// 不打开 uploads 里的副本；原文件不在时不退回副本）。路径来自表，不走范围白名单。
func (s *Service) RevealSource(ctx context.Context, sourceID string) error {
	_, ss, err := s.records()
	if err != nil {
		return err
	}
	src, err := s.source(ctx, ss, sourceID)
	if err != nil {
		return err
	}
	p, err := originalFile(src)
	if err != nil {
		return err
	}
	err = s.reveal(p)
	if apperr.Is(err, apperr.NotFound) {
		return originalFileMissing()
	}
	return err
}

func (s *Service) reveal(p string) error {
	if s.cfg.Reveal == nil {
		return apperr.New(apperr.Internal, "转换服务尚未初始化")
	}
	err := s.cfg.Reveal(p)
	if apperr.Is(err, apperr.NotFound) {
		return task.FileNotFound()
	}
	return err
}

// recordOutput 取转换记录的输出文件：不存在 / 旧类型 NOT_FOUND（reason=record）；不是 convert INVALID_ARGUMENT；
// 不是 succeeded、文件不在、不是普通文件、是符号链接 NOT_FOUND（reason=file）。
func (s *Service) recordOutput(tr TaskRecords, taskID string) (string, task.Task, error) {
	p, t, err := tr.TaskFile(taskID, "output")
	if t.ID != "" && !task.IsRecordTask(t) {
		return "", t, apperr.New(apperr.InvalidArgument, "不是转换记录")
	}
	return p, t, err
}

// RevealRecord 在文件管理器里显示转换记录的输出文件（契约 v0.23.1；转换页不再用 RevealInFolder(path)）。
func (s *Service) RevealRecord(ctx context.Context, taskID string) error {
	tr, _, err := s.records()
	if err != nil {
		return err
	}
	p, _, err := s.recordOutput(tr, taskID)
	if err != nil {
		return err
	}
	return s.reveal(p)
}

// GetRecordThumbnail 返回转换记录输出文件的缩略图 data URL（契约 6.14.10）。
func (s *Service) GetRecordThumbnail(ctx context.Context, taskID string) (_ string, err error) {
	var p string
	defer func() { logThumbErr("record", taskID, p, err) }()
	tr, _, err := s.records()
	if err != nil {
		return "", err
	}
	p, t, err := s.recordOutput(tr, taskID)
	if err != nil {
		return "", err
	}
	hint := 0.0
	if t.Result != nil {
		hint = t.Result.DurationSec
	}
	return s.thumbnail(ctx, p, hint)
}

// GetSourceThumbnail 返回源文件的缩略图 data URL（契约 6.14.10）；时长取自 media 表。
func (s *Service) GetSourceThumbnail(ctx context.Context, sourceID string) (_ string, err error) {
	var p string
	defer func() { logThumbErr("source", sourceID, p, err) }()
	_, ss, err := s.records()
	if err != nil {
		return "", err
	}
	src, err := s.source(ctx, ss, sourceID)
	if err != nil {
		return "", err
	}
	// 显示路径（6.15.6）：副本 ready 用副本，复制中 / 失败 / 已取消 / 旧行用原文件，复制没完成也能出缩略图
	p = displayPath(src)
	if _, err = sourceFile(src); err != nil {
		return "", err
	}
	hint := 0.0
	s.fillMediaFallback(ctx, ss, &src) // 持久化的探测结果优先，没有时退回 media 表
	if src.Media != nil {
		hint = src.Media.Duration
	}
	return s.thumbnail(ctx, p, hint)
}

// logThumbErr 把转换页缩略图的失败写进应用日志（<数据目录>/logs/app.log），一次调用一行：
// 哪种缩略图、id、文件路径、错误码、message 和 detail 第一行。包 19 在 Windows 上全部失败却没有任何记录，前端只显示类型图标。
// ffmpeg 本身失败时 media 包另有一行带退出码和 stderr 的日志。失败不缓存，下次调用会重新生成。
func logThumbErr(kind, id, path string, err error) {
	if err == nil {
		return
	}
	ae := apperr.From(err)
	detail, _, _ := strings.Cut(ae.Detail, "\n")
	log.Printf("缩略图: 转换页 %s=%s path=%q code=%s msg=%q detail=%q", kind, id, path, ae.Code, ae.Message, detail)
}

func (s *Service) thumbnail(ctx context.Context, p string, hint float64) (string, error) {
	if s.cfg.Thumbs == nil {
		return "", apperr.New(apperr.Internal, "转换服务尚未初始化")
	}
	if !hasExt(mediaThumbExts, p) { // 扩展名不是音视频：做不出缩略图
		return "", formatUnsupported("这个文件做不出缩略图")
	}
	return s.cfg.Thumbs.DefaultThumbnailDataURL(ctx, p, hint)
}

// TaskPreviewURL 是 TaskService.GetPreviewURL（契约 6.14.3）：不限任务类型。
func (s *Service) TaskPreviewURL(ctx context.Context, taskID, which string) (PreviewURL, error) {
	tr, ok := s.cfg.Tasks.(TaskRecords)
	if !ok {
		return PreviewURL{}, apperr.New(apperr.Internal, "转换服务尚未初始化")
	}
	p, _, err := tr.TaskFile(taskID, which)
	if err != nil {
		return PreviewURL{}, err
	}
	return s.register(p, func() *store.MediaInfo { return s.inspectForPreview(ctx, p) })
}

// TaskOpenWithSystem 是 TaskService.OpenWithSystem（契约 6.14.3）。
func (s *Service) TaskOpenWithSystem(ctx context.Context, taskID, which string) error {
	tr, ok := s.cfg.Tasks.(TaskRecords)
	if !ok {
		return apperr.New(apperr.Internal, "转换服务尚未初始化")
	}
	p, t, err := tr.TaskFile(taskID, which)
	if err != nil {
		return err
	}
	return s.open(p, t.Type == task.TypeDocConvert || t.Type == task.TypeOfficePDF)
}

// ---------- 结果信息（契约 6.14.6） ----------

// resultRunner 包装 convert 的 FFmpegRunner：Run 成功（最终文件改名到位）后探测输出，实现 task.ResultReporter。
// image：图片输出，result 只有 sizeBytes / width / height（6.16.5）；expected > 0 时做 short_output 检查（6.14.6，v0.24 X1）。
type resultRunner struct {
	*task.FFmpegRunner
	probe    func(ctx context.Context, path string) *task.TaskResult
	res      *task.TaskResult
	image    bool
	expected float64
}

func (r *resultRunner) Run(ctx context.Context, report func(task.Progress)) (string, error) {
	out, err := r.FFmpegRunner.Run(ctx, report)
	if err == nil && out != "" && r.probe != nil {
		r.res = r.probe(ctx, out)
		if r.res != nil {
			if r.image {
				r.res = &task.TaskResult{SizeBytes: r.res.SizeBytes, Width: r.res.Width, Height: r.res.Height}
			} else if shortOutput(r.expected, r.res.DurationSec) {
				r.res.Warnings = append(r.res.Warnings, WarningShortOutput)
				fmt.Fprintf(task.LogWriter(ctx), "[FFmpegFree] short_output: expected=%s actual=%s\n", secs(r.expected), secs(r.res.DurationSec))
			}
		}
	}
	return out, err
}

// WarningShortOutput 是 TaskResult.warnings 的机器码：输出明显比预期短（6.14.6）。
const WarningShortOutput = "short_output"

// shortOutput：只有输入时长已知（expected > 0）且输出时长已知时判断；输出 < 0.9 × 预期 且 差值 > 2 秒。
func shortOutput(expected, actual float64) bool {
	return expected > 0 && actual > 0 && actual < 0.9*expected && expected-actual > 2
}

func secs(v float64) string { return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64) }

// Result 实现 task.ResultReporter。
func (r *resultRunner) Result() *task.TaskResult { return r.res }

// probeResult：os.Stat 拿 sizeBytes，再 ffprobe（门控、file: 前缀、30 秒超时、可取消，同 6.7）补齐其余字段。
// ffprobe 失败只有 sizeBytes；stat 失败没有 result；都不影响任务成功，只记日志。
func (s *Service) probeResult(ctx context.Context, path string) *task.TaskResult {
	fi, err := os.Stat(path)
	if err != nil {
		fmt.Fprintf(task.LogWriter(ctx), "[FFmpegFree] 读取输出文件信息失败：%v\n", err)
		return nil
	}
	res := &task.TaskResult{SizeBytes: fi.Size()}
	if s.cfg.Media == nil {
		return res
	}
	m, err := s.cfg.Media.Inspect(ctx, path)
	if err != nil {
		fmt.Fprintf(task.LogWriter(ctx), "[FFmpegFree] 探测输出文件失败：%v\n", err)
		return res
	}
	return resultFromMedia(fi.Size(), m)
}

func resultFromMedia(size int64, m store.MediaInfo) *task.TaskResult {
	res := &task.TaskResult{SizeBytes: size, DurationSec: m.Duration}
	if m.HasVideo {
		res.Width, res.Height = m.Width, m.Height
	}
	for _, st := range m.Streams {
		if st.Type == "audio" {
			if st.Bitrate > 0 {
				res.AudioBitrateKbps = int(math.Round(float64(st.Bitrate) / 1000))
			}
			break
		}
	}
	if res.AudioBitrateKbps == 0 && !m.HasVideo && m.HasAudio && m.Bitrate > 0 {
		res.AudioBitrateKbps = int(math.Round(float64(m.Bitrate) / 1000))
	}
	return res
}

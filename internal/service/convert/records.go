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
	DeleteConvertSource(ctx context.Context, id string) error
	ListConvertSources(ctx context.Context, keyword, status string, limit, offset int) ([]store.ConvertSource, int64, error)
	MatchedSourceTaskIDs(ctx context.Context, sourceID, keyword string, limit int) ([]string, error)
	ListSourceTasks(ctx context.Context, sourceID string, limit, offset int) (store.TaskPage, error)
	SourceTaskIDs(ctx context.Context, sourceID string) ([]string, error)
	MediaByPathKey(ctx context.Context, key string) (*store.MediaInfo, error)
}

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
	SourceID string `json:"sourceId"`
	Found    bool   `json:"found"`
	Exists   bool   `json:"exists"`
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

// previewExts 是转换页 / 任务中心应用内预览的白名单（契约 6.14.7）：EditService 的 v1 列表加 gif。
var previewExts = extSet("mp4 mov avi mkv flv webm m4v mp3 wav aac m4a flac ogg gif")

// openExts 是“用系统程序打开”的白名单（契约 6.14.7）：预览列表加常见音视频扩展名。
var openExts = extSet("mp4 mov avi mkv flv webm m4v mp3 wav aac m4a flac ogg gif " +
	"opus wmv mpg mpeg ts mts m2ts 3gp ogv wma amr aiff ape")

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

// sourceFile 返回源文件行登记的路径；路径为空、文件不在或不是普通文件 NOT_FOUND（reason=file）。
func sourceFile(src ConvertSource) (string, error) {
	if src.Path == "" || !filepath.IsAbs(src.Path) {
		return "", task.FileNotFound()
	}
	fi, err := os.Stat(src.Path)
	if err != nil || !fi.Mode().IsRegular() {
		return "", task.FileNotFound()
	}
	return src.Path, nil
}

// AddSources 登记源文件（契约 6.14.3）：1~500 个，逐个规范化、stat，同 path_key 已有行只更新 lastActivityAt。不探测。
func (s *Service) AddSources(ctx context.Context, in []string) ([]AddSourceResult, error) {
	_, ss, err := s.records()
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
		}
		src, existed, err := ss.UpsertConvertSource(ctx, p, key, s.cfg.Now())
		if err != nil {
			return nil, apperr.Wrap(apperr.Internal, "保存源文件行失败", err)
		}
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
		return 0, 0, apperr.New(apperr.InvalidArgument, fmt.Sprintf("limit 范围 0~%d，offset 不能小于 0", max))
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
	s.fillMediaFallback(ctx, ss, &src)
	return ConvertSourceEntry{Source: src, Records: page.Items, RecordCount: page.Total}, nil
}

func (s *Service) listSources(ctx context.Context, keyword, status string, limit, offset, recordLimit int) (ConvertSourcePage, error) {
	tr, ss, err := s.records()
	if err != nil {
		return ConvertSourcePage{}, err
	}
	if !store.ValidSourceStatus(status) {
		return ConvertSourcePage{}, apperr.New(apperr.InvalidArgument, `status 只能是 ""、"active" 或 "failed"`)
	}
	limit, offset, err = pageArgs(limit, offset, defaultSourceLimit, maxSourceLimit)
	if err != nil {
		return ConvertSourcePage{}, err
	}
	if recordLimit < 0 || recordLimit > maxRecordLimit {
		return ConvertSourcePage{}, apperr.New(apperr.InvalidArgument, fmt.Sprintf("recordLimit 范围 0~%d", maxRecordLimit))
	}
	if recordLimit == 0 {
		recordLimit = defaultRecordLimit
	}
	srcs, total, err := ss.ListConvertSources(ctx, keyword, status, limit, offset)
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
	kw := strings.TrimSpace(f.Keyword)
	if kw == "" || utf8.RuneCountInString(kw) > maxKeywordRunes {
		return ConvertSourcePage{}, apperr.New(apperr.InvalidArgument, fmt.Sprintf("关键字为 1~%d 个字", maxKeywordRunes))
	}
	return s.listSources(ctx, strings.ToLower(kw), f.Status, f.Limit, f.Offset, f.RecordLimit)
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
		out[i].Exists = src.Path != "" && task.RegularExists(src.Path, true)
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
	stem := strings.TrimSuffix(filepath.Base(src.Path), filepath.Ext(src.Path))
	return tr.PeekOutputName(filepath.Join(dir, stem+"."+opts.Container), task.TypeConvert), nil
}

// SubmitSources 等同 Submit，输入来自源文件行（契约 6.14.3）；presetId 非空时写入当时的预设名称快照。
func (s *Service) SubmitSources(ctx context.Context, req ConvertSubmitRequest) ([]task.Task, error) {
	_, ss, err := s.records()
	if err != nil {
		return nil, err
	}
	if len(req.SourceIDs) == 0 || len(req.SourceIDs) > MaxInputsPerSubmit {
		return nil, apperr.New(apperr.InvalidArgument, fmt.Sprintf("一次提交 1~%d 个文件", MaxInputsPerSubmit))
	}
	jobs := make([]submitJob, len(req.SourceIDs))
	for i, id := range req.SourceIDs {
		src, err := s.source(ctx, ss, id)
		if err != nil {
			return nil, err
		}
		jobs[i] = submitJob{in: src.Path, sourceID: src.SourceID}
	}
	presetName := ""
	if req.PresetID != "" {
		ps, err := s.ListPresets(ctx)
		if err != nil {
			return nil, err
		}
		found := false
		for _, p := range ps {
			if p.ID == req.PresetID {
				presetName, found = p.Name, true
				break
			}
		}
		if !found {
			return nil, apperr.New(apperr.NotFound, "预设不存在").WithDetail("reason=record")
		}
	}
	return s.submitWrap(ctx, jobs, req.Options, req.OutputDir, req.PresetID, presetName)
}

// Reconvert 只用于已成功的记录（契约 6.14.3）：照抄原记录的 sourceId、options、outputDir 和三个快照，新提交一条。
func (s *Service) Reconvert(ctx context.Context, taskID string) (task.Task, error) {
	tr, ss, err := s.records()
	if err != nil {
		return task.Task{}, err
	}
	old, err := tr.Get(taskID)
	if err != nil {
		if apperr.Is(err, apperr.NotFound) {
			return task.Task{}, task.RecordNotFound()
		}
		return task.Task{}, err
	}
	if old.Type != task.TypeConvert {
		return task.Task{}, apperr.New(apperr.InvalidArgument, "不是转换记录")
	}
	if old.Status != task.StatusSucceeded {
		return task.Task{}, apperr.New(apperr.TaskConflict, "只有已完成的记录可以再转一次")
	}
	var p params
	if err := json.Unmarshal([]byte(old.Params), &p); err != nil || p.Input == "" {
		return task.Task{}, apperr.New(apperr.InvalidArgument, "转换任务参数无效，无法再转一次")
	}
	if err := validateOptions(p.Options); err != nil {
		return task.Task{}, err
	}
	bin, err := s.cfg.Require()
	if err != nil {
		return task.Task{}, err
	}
	j, err := s.prepare(ctx, p.Input, p.Options, p.OutputDir)
	if err != nil {
		return task.Task{}, withInput(err, p.Input)
	}
	srcID := old.SourceID
	if srcID == "" { // 回填还没跑到的旧记录：按路径找到 / 建行
		_, key, _ := paths.Normalize(p.Input)
		src, _, err := ss.UpsertConvertSource(ctx, p.Input, key, s.cfg.Now())
		if err != nil {
			return task.Task{}, apperr.Wrap(apperr.Internal, "保存源文件行失败", err)
		}
		srcID = src.SourceID
	}
	t, err := s.submitOne(bin, p, srcID, j.out, j.dur, j.src)
	if err != nil {
		return task.Task{}, err
	}
	s.touch(ctx, []string{srcID})
	return t, nil
}

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
	if _, err := s.source(ctx, ss, sourceID); err != nil {
		return task.NewDeleteResult(), err
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
		if err := ss.DeleteConvertSource(ctx, sourceID); err != nil {
			return res, apperr.Wrap(apperr.Internal, "删除源文件行失败", err)
		}
		res.DeletedSourceIDs = append(res.DeletedSourceIDs, sourceID)
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
func (s *Service) open(path string) error {
	if s.cfg.Open == nil {
		return apperr.New(apperr.Internal, "转换服务尚未初始化")
	}
	if !hasExt(openExts, path) {
		return formatUnsupported("只能用系统程序打开音视频文件")
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
	p, err := sourceFile(src)
	if err != nil {
		return PreviewURL{}, err
	}
	return s.register(p, func() *store.MediaInfo {
		s.refreshSourceMedia(ctx, ss, &src) // 持久化的结果对得上当前文件就不重探
		return src.Media
	})
}

// OpenSourceWithSystem 用系统默认程序打开源文件（规则同 TaskService.OpenWithSystem）。
func (s *Service) OpenSourceWithSystem(ctx context.Context, sourceID string) error {
	_, ss, err := s.records()
	if err != nil {
		return err
	}
	src, err := s.source(ctx, ss, sourceID)
	if err != nil {
		return err
	}
	p, err := sourceFile(src)
	if err != nil {
		return err
	}
	return s.open(p)
}

// RevealSource 在文件管理器里显示源文件（平台命令同 RevealInFolder，路径来自表，不走范围白名单）。
func (s *Service) RevealSource(ctx context.Context, sourceID string) error {
	_, ss, err := s.records()
	if err != nil {
		return err
	}
	src, err := s.source(ctx, ss, sourceID)
	if err != nil {
		return err
	}
	if src.Path == "" || !filepath.IsAbs(src.Path) {
		return task.FileNotFound()
	}
	if _, err := os.Stat(src.Path); errors.Is(err, os.ErrNotExist) {
		return task.FileNotFound()
	}
	return s.reveal(src.Path)
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
	if t.ID != "" && t.Type != task.TypeConvert {
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
	p = src.Path
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
	if !hasExt(openExts, p) { // 扩展名不是音视频：做不出缩略图
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
	p, _, err := tr.TaskFile(taskID, which)
	if err != nil {
		return err
	}
	return s.open(p)
}

// ---------- 结果信息（契约 6.14.6） ----------

// resultRunner 包装 convert 的 FFmpegRunner：Run 成功（最终文件改名到位）后探测输出，实现 task.ResultReporter。
type resultRunner struct {
	*task.FFmpegRunner
	probe func(ctx context.Context, path string) *task.TaskResult
	res   *task.TaskResult
}

func (r *resultRunner) Run(ctx context.Context, report func(task.Progress)) (string, error) {
	out, err := r.FFmpegRunner.Run(ctx, report)
	if err == nil && out != "" && r.probe != nil {
		r.res = r.probe(ctx, out)
	}
	return out, err
}

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

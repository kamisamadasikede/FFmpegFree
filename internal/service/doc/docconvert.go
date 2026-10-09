package doc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/doccomp"
	"FFmpegFree/internal/doceng"
	"FFmpegFree/internal/fsutil"
	"FFmpegFree/internal/paths"
	"FFmpegFree/internal/service/convert"
	"FFmpegFree/internal/store"
	"FFmpegFree/internal/task"
)

// ---------- 文档多格式转换（契约 v0.26，6.12.9~6.12.21） ----------

const (
	matrixWait       = 6 * time.Second // GetFormatMatrix 在 checking 时最多等这么久
	inspectTimeout   = 5 * time.Second // 添加时每个文件的检查上限
	inspectParallel  = 4
	maxDocAddPerCall = 50
)

// Component 是文档组件管理器（*doccomp.Manager 实现）。
type Component interface {
	Status() doccomp.Status
	Wait(ctx context.Context, d time.Duration) doccomp.Status
	ExePath() string
	NotReadyError() *apperr.AppError
}

var _ Component = (*doccomp.Manager)(nil)

// DocSources 是文档页的源文件行能力（*convert.Service 实现，复用转换页的行 / 副本 / 记录）。
type DocSources interface {
	AddDocSource(ctx context.Context, p, key string, fi os.FileInfo, sheetCount int) (store.ConvertSource, bool, error)
	ListDocSources(ctx context.Context, f convert.ConvertSourceFilter) (convert.ConvertSourcePage, error)
	SearchDocSources(ctx context.Context, f convert.ConvertSearchFilter) (convert.ConvertSourcePage, error)
	DocSourceForSubmit(ctx context.Context, id string) (src store.ConvertSource, in, skipReason string, err error)
	TouchSources(ctx context.Context, ids []string)
	ResolveOutputDir(ctx context.Context, dir string) (string, error)
	SetDocReconverter(f convert.DocReconverter)
}

var _ DocSources = (*convert.Service)(nil)

// DocSource 见契约 6.12.14。
type DocSource struct {
	store.ConvertSource
	Ext        string `json:"ext"`
	Family     string `json:"family"`
	SheetCount int    `json:"sheetCount"`
}

// AddDocSourceResult 与 AddDocSources 入参一一对应。
type AddDocSourceResult struct {
	Path   string           `json:"path"`
	Source *DocSource       `json:"source,omitempty"`
	Error  *apperr.AppError `json:"error,omitempty"`
}

// DocSourceEntry 同 ConvertSourceEntry。
type DocSourceEntry struct {
	Source      DocSource   `json:"source"`
	Records     []task.Task `json:"records"`
	RecordCount int64       `json:"recordCount"`
}

// DocSourcePage 是 ListDocSources / SearchDocSources 的返回值。
type DocSourcePage struct {
	Items []DocSourceEntry `json:"items"`
	Total int64            `json:"total"`
}

// DocSubmitRequest 是 SubmitDocConvert 的参数。
type DocSubmitRequest struct {
	SourceIDs []string `json:"sourceIds"`
	Target    string   `json:"target"`
	OutputDir string   `json:"outputDir"`
}

// docParams 是 doc_convert（以及文档页简易转换的 office_pdf）的 params（6.12.21）。
type docParams struct {
	SourceID      string `json:"sourceId"`
	Input         string `json:"input"`
	Target        string `json:"target"`
	OutputDir     string `json:"outputDir"`
	Engine        string `json:"engine"`
	ParamsSummary string `json:"paramsSummary"`
	PresetID      string `json:"presetId"`
	PresetName    string `json:"presetName"`
}

func toDocSource(src store.ConvertSource) DocSource {
	ext := normExt(filepath.Ext(src.Path))
	d := DocSource{ConvertSource: src, Ext: ext, Family: familyOf(ext), SheetCount: src.SheetCount}
	if d.Family != FamilySheet {
		d.SheetCount = 0
	}
	d.Media = nil
	return d
}

func (s *Service) docReady() error {
	if s.cfg.Sources == nil || s.cfg.Tasks == nil {
		return s.internalNotReady("文档服务")
	}
	return nil
}

func (s *Service) componentStatus(ctx context.Context, wait time.Duration) doccomp.Status {
	if s.cfg.Component == nil {
		return doccomp.Status{State: doccomp.StateMissing}
	}
	if wait > 0 {
		return s.cfg.Component.Wait(ctx, wait)
	}
	return s.cfg.Component.Status()
}

// mergedStatus 把 Office / WPS / 组件合成 DocComponentStatus 形态。
func (s *Service) mergedStatus(ctx context.Context, wait time.Duration) doceng.StatusView {
	if wait > 0 && s.cfg.Component != nil {
		_ = s.cfg.Component.Wait(ctx, wait)
	}
	if s.cfg.Engines != nil {
		return s.cfg.Engines.Status(ctx)
	}
	st := s.componentStatus(ctx, 0)
	return doceng.StatusView{
		State: st.State, ComponentState: st.ComponentState, Version: st.Version, Source: st.Source,
		CanDownload: st.CanDownload, DownloadBytes: st.DownloadBytes, InstallBytes: st.InstallBytes,
		Phase: st.Phase, ReceivedBytes: st.ReceivedBytes, Error: st.Error, Path: st.Path,
	}
}

func (s *Service) notReady() error {
	if s.cfg.Component == nil {
		return apperr.New(apperr.DocComponentNotReady, "需要先下载文档组件。")
	}
	return s.cfg.Component.NotReadyError()
}

// GetFormatMatrix 按当前引擎状态生成格式表；组件在 checking 时最多等 6 秒。
func (s *Service) GetFormatMatrix(ctx context.Context) DocFormatMatrix {
	st := s.mergedStatus(ctx, matrixWait)
	pref := "auto"
	if s.cfg.DocEngine != nil {
		if p := s.cfg.DocEngine(ctx); p != "" {
			pref = p
		}
	}
	var engines []doceng.Detected
	var skips *doceng.SkipTracker
	if s.cfg.Engines != nil {
		engines = s.cfg.Engines.Engines(ctx)
		skips = s.cfg.Engines.Skips()
	}
	ready := st.State == "ready"
	return buildMatrix(ready, func(src, target string) []string {
		ids := doceng.EnginesForTarget(doceng.PrefOrder(pref), engines, skips, src, target)
		if len(ids) == 0 && ready {
			// 只有文档组件且 Registry 尚未填 engines 时
			ids = []string{engineComponent}
		}
		return ids
	})
}

// CleanupDocTemp 启动时删除 <数据目录>/tmp/doc/ 下的残留（6.12.18）。
func (s *Service) CleanupDocTemp() {
	if s.cfg.TempRoot != "" {
		_ = os.RemoveAll(s.cfg.TempRoot)
	}
}

// ---------- 添加 ----------

func formatUnsupportedAdd() *apperr.AppError {
	return apperr.New(apperr.DocFormatUnsupported, "不支持这种文件。")
}

func pdfInputUnsupported() *apperr.AppError {
	return apperr.New(apperr.DocPDFInputUnsupported, "PDF 暂时不能转成其他格式。")
}

// AddDocSources 登记文档页的源文件行（6.12.16）：扩展名、大小、加密、损坏检查都通过才建行（kind=doc），后台复制副本。
func (s *Service) AddDocSources(ctx context.Context, in []string) ([]AddDocSourceResult, error) {
	if err := s.docReady(); err != nil {
		return nil, err
	}
	if len(in) == 0 || len(in) > maxDocAddPerCall {
		return nil, apperr.New(apperr.InvalidArgument, fmt.Sprintf("一次添加 1~%d 个文件", maxDocAddPerCall))
	}
	type checked struct {
		p, key, ext string
		fi          os.FileInfo
		sheets      int
		err         *apperr.AppError
	}
	res := make([]AddDocSourceResult, len(in))
	cs := make([]checked, len(in))
	var wg sync.WaitGroup
	sem := make(chan struct{}, inspectParallel)
	for i, raw := range in {
		res[i].Path = raw
		c := &cs[i]
		if strings.TrimSpace(raw) == "" || !filepath.IsAbs(raw) {
			c.err = apperr.New(apperr.InvalidArgument, "路径必须是绝对路径")
			continue
		}
		p, key, err := paths.Normalize(raw)
		if err != nil {
			c.err = apperr.New(apperr.InvalidArgument, "路径不合法")
			continue
		}
		c.p, c.key = p, key
		raw := extLower(p)
		if raw == "pdf" {
			c.err = pdfInputUnsupported()
			continue
		}
		c.ext = normExt(raw)
		if c.ext == "" {
			c.err = formatUnsupportedAdd()
			continue
		}
		fi, err := os.Stat(p)
		switch {
		case errors.Is(err, os.ErrNotExist):
			c.err = apperr.New(apperr.NotFound, "文件不存在").WithDetail("reason=file")
			continue
		case err != nil:
			c.err = apperr.Wrap(apperr.IOError, "无法读取文件信息", err)
			continue
		case !fi.Mode().IsRegular():
			c.err = apperr.New(apperr.InvalidArgument, "不是普通文件")
			continue
		case fi.Size() > MaxInputBytes:
			c.err = reasonErr(apperr.InvalidArgument, "文件超过 100 MiB", reasonTooLarge, "")
			continue
		}
		c.fi = fi
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			c.sheets, c.err = inspectWithTimeout(ctx, c.p, c.ext)
		}()
	}
	wg.Wait()
	for i := range cs {
		c := cs[i]
		if c.err != nil {
			res[i].Error = c.err
			continue
		}
		src, _, err := s.cfg.Sources.AddDocSource(ctx, c.p, c.key, c.fi, c.sheets)
		if err != nil {
			res[i].Error = apperr.From(err)
			continue
		}
		ds := toDocSource(src)
		if ds.Family == FamilySheet && ds.SheetCount == 0 {
			ds.SheetCount = c.sheets
		}
		res[i].Source = &ds
	}
	return res, nil
}

// inspectWithTimeout 做一次检查，最多 5 秒：超时不拦（加密在提交和运行前还会再查），sheetCount 记 -1。
func inspectWithTimeout(ctx context.Context, p, ext string) (int, *apperr.AppError) {
	ctx, cancel := context.WithTimeout(ctx, inspectTimeout)
	defer cancel()
	type r struct {
		n   int
		err error
	}
	ch := make(chan r, 1)
	go func() {
		n, err := inspectDoc(ctx, p, ext)
		ch <- r{n, err}
	}()
	select {
	case v := <-ch:
		if v.err != nil {
			if ctx.Err() != nil && !apperr.Is(v.err, apperr.DocEncrypted) && !apperr.Is(v.err, apperr.DocCorrupt) {
				return unknownSheets(ext), nil
			}
			return 0, apperr.From(v.err)
		}
		return v.n, nil
	case <-ctx.Done():
		return unknownSheets(ext), nil
	}
}

// ---------- 列表 ----------

func wrapPage(p convert.ConvertSourcePage) DocSourcePage {
	out := DocSourcePage{Items: make([]DocSourceEntry, 0, len(p.Items)), Total: p.Total}
	for _, e := range p.Items {
		recs := e.Records
		if recs == nil {
			recs = []task.Task{}
		}
		out.Items = append(out.Items, DocSourceEntry{Source: toDocSource(e.Source), Records: recs, RecordCount: e.RecordCount})
	}
	return out
}

// ListDocSources 只列 kind=doc 的行。
func (s *Service) ListDocSources(ctx context.Context, f convert.ConvertSourceFilter) (DocSourcePage, error) {
	if err := s.docReady(); err != nil {
		return DocSourcePage{Items: []DocSourceEntry{}}, err
	}
	p, err := s.cfg.Sources.ListDocSources(ctx, f)
	if err != nil {
		return DocSourcePage{Items: []DocSourceEntry{}}, err
	}
	return wrapPage(p), nil
}

// SearchDocSources 同 ListDocSources，带关键字。
func (s *Service) SearchDocSources(ctx context.Context, f convert.ConvertSearchFilter) (DocSourcePage, error) {
	if err := s.docReady(); err != nil {
		return DocSourcePage{Items: []DocSourceEntry{}}, err
	}
	p, err := s.cfg.Sources.SearchDocSources(ctx, f)
	if err != nil {
		return DocSourcePage{Items: []DocSourceEntry{}}, err
	}
	return wrapPage(p), nil
}

// ---------- 提交 ----------

func formatUnsupportedTarget() *apperr.AppError {
	return apperr.New(apperr.DocFormatUnsupported, "不支持转成这个格式。")
}

// docJob 是一个文档转换的全部输入（提交、重试、重转共用）。
type docJob struct {
	sourceID   string
	in         string // 读取路径（副本或原文件）
	origDir    string // md 图片的相对路径基准：原文件所在目录，原文件不在时用副本目录
	name       string // 源文件名（标题用）
	src        string // 规范化后的源格式
	target     string
	outputDir  string // 已解析的最终输出目录
	desired    string // 期望输出名（重名顺延前）
	sheetCount int
	simple     bool // 简易转换（office_pdf）
}

func (j docJob) engine() string {
	if j.simple || goPair(j.src, j.target) {
		return engineGo
	}
	return engineComponent
}

func (j docJob) params() string {
	b, _ := json.Marshal(docParams{SourceID: j.sourceID, Input: j.in, Target: j.target, OutputDir: j.outputDir,
		Engine: j.engine(), ParamsSummary: paramsSummary(j.src, j.target)})
	return string(b)
}

func origDirOf(src store.ConvertSource, in string) string {
	if p := src.OriginalPath; p != "" {
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
			return filepath.Dir(p)
		}
	}
	return filepath.Dir(in)
}

// SubmitDocConvert 一个源一个任务（6.12.14）；副本没就绪的行跳过，一行都没就绪时整体 TASK_CONFLICT（同 SubmitSources）。
// 先整体校验（目标格式、组件状态、加密）再提交，任何一个不通过整体失败、不提交任何任务。
func (s *Service) SubmitDocConvert(ctx context.Context, req DocSubmitRequest) (convert.ConvertSubmitResult, error) {
	res := convert.ConvertSubmitResult{Tasks: []task.Task{}, Skipped: []convert.SkippedSource{}}
	if err := s.docReady(); err != nil {
		return res, err
	}
	if len(req.SourceIDs) == 0 || len(req.SourceIDs) > MaxInputsPerSubmit {
		return res, apperr.New(apperr.InvalidArgument, fmt.Sprintf("一次提交 1~%d 个文件", MaxInputsPerSubmit))
	}
	target := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(req.Target), "."))
	if target != "pdf" {
		target = normExt(target)
	}
	if target == "" {
		return res, formatUnsupportedTarget()
	}
	st := s.mergedStatus(ctx, 0)
	ready := st.State == "ready"
	pref := "auto"
	if s.cfg.DocEngine != nil {
		if p := s.cfg.DocEngine(ctx); p != "" {
			pref = p
		}
	}
	var engines []doceng.Detected
	var skips *doceng.SkipTracker
	if s.cfg.Engines != nil {
		engines = s.cfg.Engines.Engines(ctx)
		skips = s.cfg.Engines.Skips()
	}
	var jobs []docJob
	var firstSkipped *store.ConvertSource
	anyCopying := false
	for _, id := range req.SourceIDs {
		src, in, skip, err := s.cfg.Sources.DocSourceForSubmit(ctx, id)
		if err != nil {
			return res, err
		}
		if src.Kind != store.SourceKindDoc {
			return res, formatUnsupportedTarget()
		}
		if skip != "" {
			res.Skipped = append(res.Skipped, convert.SkippedSource{SourceID: src.SourceID, Reason: skip})
			anyCopying = anyCopying || skip == "copying"
			if firstSkipped == nil {
				cp := src
				firstSkipped = &cp
			}
			continue
		}
		ds := toDocSource(src)
		ids := doceng.EnginesForTarget(doceng.PrefOrder(pref), engines, skips, ds.Ext, target)
		if len(ids) == 0 && ready {
			ids = []string{engineComponent}
		}
		t, ok := targetFor(ds.Ext, target, ready, ids)
		if !ok {
			return res, formatUnsupportedTarget()
		}
		if !t.Available {
			return res, s.notReady()
		}
		if !regularFile(in) {
			return res, apperr.New(apperr.NotFound, "文件不存在").WithDetail("reason=file")
		}
		if _, err := inspectDoc(ctx, in, ds.Ext); apperr.Is(err, apperr.DocEncrypted) {
			return res, err
		}
		jobs = append(jobs, docJob{sourceID: src.SourceID, in: in, origDir: origDirOf(src, in), name: src.Name,
			src: ds.Ext, target: target, sheetCount: ds.SheetCount, simple: t.Simple})
	}
	if len(jobs) == 0 {
		return res, convert.CopyNotReady(*firstSkipped, anyCopying)
	}
	dir, err := s.cfg.Sources.ResolveOutputDir(ctx, req.OutputDir)
	if err != nil {
		return res, err
	}
	var touched []string
	for i := range jobs {
		j := &jobs[i]
		j.outputDir = dir
		outDir := dir
		if outDir == "" {
			outDir = j.origDir
		}
		if j.desired, err = s.desiredOutputExt(j.name, outDir, j.target); err != nil {
			return res, err
		}
	}
	for _, j := range jobs {
		typ := task.TypeDocConvert
		if j.simple {
			typ = task.TypeOfficePDF
		}
		spec := task.Spec{Type: typ, Title: j.name + " → " + strings.ToUpper(j.target), InputPaths: []string{j.in},
			OutputPath: j.desired, Params: j.params(), SourceID: j.sourceID, ReserveOutput: true}
		t, err := s.cfg.Tasks.Submit(spec, s.newDocRunner(j, ""))
		if err != nil {
			return res, err // 已提交的保留
		}
		res.Tasks = append(res.Tasks, t)
		touched = append(touched, j.sourceID)
	}
	s.cfg.Sources.TouchSources(ctx, touched)
	return res, nil
}

// desiredOutputExt 是 <outDir>/<净化后的源文件名去扩展名>.<目标扩展名>（Windows 按整条路径长度截短，同 desiredOutput）。
func (s *Service) desiredOutputExt(name, outDir, ext string) (string, error) {
	stem := fsutil.SanitizeFileNameOr(strings.TrimSuffix(name, filepath.Ext(name)), "document")
	dot := "." + ext
	if mp := s.maxPath(); mp > 0 {
		for fsutil.OutputPathLength(outDir, stem, dot) > mp {
			rs := []rune(stem)
			if len(rs) <= 1 {
				return "", apperr.New(apperr.InvalidArgument, "输出路径太长").WithDetail(outDir)
			}
			stem = fsutil.SanitizeFileNameOr(string(rs[:len(rs)-1]), "document")
		}
	}
	return filepath.Join(outDir, stem+dot), nil
}

func regularFile(p string) bool {
	if p == "" || !filepath.IsAbs(p) {
		return false
	}
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}

// ---------- Runner ----------

// docRunner 跑一个文档转换。direct 非空时直接写到这个路径（原地重转的临时文件），否则走 RunWithPart。
type docRunner struct {
	s      *Service
	j      docJob
	direct string

	mu         sync.Mutex
	result     *task.TaskResult
	usedEngine string
}

func (s *Service) newDocRunner(j docJob, direct string) *docRunner {
	return &docRunner{s: s, j: j, direct: direct}
}

// Pool：只有真正启动组件进程的任务进文档组件池；md ↔ html 和简易转换直接运行（6.12.18）。
func (r *docRunner) Pool() task.Pool {
	if r.j.engine() != engineComponent {
		return task.PoolFree
	}
	// 有本机 Office / WPS 可试时用 PoolFree（OfficeConverter 内部串行）；否则进文档组件池。
	if r.s.cfg.Engines != nil {
		engines := r.s.cfg.Engines.Engines(context.Background())
		for _, e := range engines {
			if (e.ID == doceng.IDOffice || e.ID == doceng.IDWPS) && e.Available {
				if doceng.CanConvert(e.ID, r.j.src, r.j.target, e.Families) {
					return task.PoolFree
				}
			}
		}
	}
	return task.PoolDoc
}

func (r *docRunner) DesiredOutput() string { return r.j.desired }

func (r *docRunner) Result() *task.TaskResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.result
}

func (r *docRunner) Run(ctx context.Context, report func(task.Progress)) (string, error) {
	j := r.j
	logw := task.LogWriter(ctx)
	if !regularFile(j.in) {
		return "", apperr.New(apperr.NotFound, "文件不存在").WithDetail("reason=file")
	}
	// 运行前再查一次加密（防止文件被替换），命中不启动组件。
	if _, err := inspectDoc(ctx, j.in, j.src); apperr.Is(err, apperr.DocEncrypted) || apperr.Is(err, apperr.DocCorrupt) {
		return "", err
	}
	produce := func(dst string) error { return r.produce(ctx, dst, logw, report) }
	var out string
	var err error
	if r.direct != "" {
		err = produce(r.direct)
		out = r.direct
	} else {
		out, err = task.RunWithPart(ctx, j.desired, produce)
	}
	if err != nil {
		return "", err
	}
	eng := r.usedEngine
	if eng == "" {
		eng = j.engine()
		if j.simple {
			eng = "simple"
		}
	}
	res := &task.TaskResult{Engine: eng}
	if fi, err := os.Stat(out); err == nil {
		res.SizeBytes = fi.Size()
	}
	if j.target == "csv" && j.src != "csv" && (j.sheetCount > 1 || j.sheetCount == -1) {
		res.Warnings = append(res.Warnings, WarningCSVFirstSheetOnly)
	}
	if eng == "simple" && !j.simple {
		// 引擎全失败回退简易转换
		res.Warnings = append(res.Warnings, "simple_fallback")
	}
	r.mu.Lock()
	r.result = res
	r.mu.Unlock()
	return out, nil
}

func (r *docRunner) produce(ctx context.Context, dst string, logw io.Writer, report func(task.Progress)) error {
	j := r.j
	fmt.Fprintf(logw, "文档转换：%s（%s）\n", paramsSummary(j.src, j.target), j.engine())
	switch {
	case j.simple:
		r.usedEngine = "simple"
		m, err := r.s.extract(ctx, j.in, j.src)
		if err != nil {
			return err
		}
		choice, ok := r.s.fonts.choose(m.runes)
		if !ok {
			return errNoFont
		}
		return renderPDF(ctx, m, choice, dst, report, logw)
	case j.src == "md" && j.target == "html":
		r.usedEngine = engineGo
		md, err := readTextFile(j.in)
		if err != nil {
			return err
		}
		h, err := markdownToHTML(md, strings.TrimSuffix(j.name, filepath.Ext(j.name)), imagesForHTML, j.origDir)
		if err != nil {
			return errCorrupt("markdown")
		}
		return writeOut(dst, h)
	case j.src == "html" && j.target == "md":
		r.usedEngine = engineGo
		h, err := readTextFile(j.in)
		if err != nil {
			return err
		}
		md, err := htmlToMarkdown(h)
		if err != nil {
			return errCorrupt("html")
		}
		return writeOut(dst, md)
	}
	return r.runEngines(ctx, dst, logw)
}

func (r *docRunner) runEngines(ctx context.Context, dst string, logw io.Writer) error {
	j := r.j
	id := "x"
	if info, ok := task.InfoFrom(ctx); ok {
		id = info.ID
	}
	work := filepath.Join(r.s.cfg.TempRoot, id+"-"+fmt.Sprint(time.Now().UnixNano()))
	defer os.RemoveAll(work)
	deadline := time.Now().Add(doceng.TimeoutTaskMax)
	macro := hasMacro(j.in, j.src)
	logf := func(format string, args ...any) { fmt.Fprintf(logw, format, args...) }

	if r.s.cfg.Engines == nil {
		// 没有 Registry 时退回只用组件（测试 / 旧接线）
		return r.runComponentLegacy(ctx, dst, logw, work, deadline)
	}
	eng, err := r.s.cfg.Engines.TryConvert(ctx, j.src, j.target, j.in, dst, work, j.origDir, j.name, macro, logf, deadline)
	if err == nil {
		r.usedEngine = eng
		return nil
	}
	if errors.Is(err, doceng.ErrSimpleFallback) {
		r.usedEngine = "simple"
		m, err2 := r.s.extract(ctx, j.in, j.src)
		if err2 != nil {
			return err2
		}
		choice, ok := r.s.fonts.choose(m.runes)
		if !ok {
			return errNoFont
		}
		return renderPDF(ctx, m, choice, dst, nil, logw)
	}
	return err
}

// runComponentLegacy 仅组件路径（Engines 未接线时）。
func (r *docRunner) runComponentLegacy(ctx context.Context, dst string, logw io.Writer, work string, deadline time.Time) error {
	j := r.j
	exe := ""
	if r.s.cfg.Component != nil {
		exe = r.s.cfg.Component.ExePath()
	}
	if exe == "" {
		return r.s.notReady()
	}
	d := doceng.Detected{ID: doceng.IDComponent, Name: doceng.NameComponent, ComponentExe: exe, Families: []string{FamilyText, FamilySheet, FamilySlide}, Available: true, Installed: true}
	cc := r.s.cfg.Engines
	_ = cc
	conv := &doceng.ComponentConverter{
		ConvertToArg: convertToArg, InFilterFor: inFilterFor, FamilyOf: familyOf,
		MarkdownToHTML: func(md []byte, title, mode, origDir string) ([]byte, error) {
			return markdownToHTML(md, title, imagesForComponent, origDir)
		},
		HTMLToMarkdown: htmlToMarkdown,
		ReadTextFile:   readTextFile,
		WriteUTF8Temp:  writeUTF8Temp,
	}
	req := doceng.ConvertRequest{SrcExt: j.src, Target: j.target, InputPath: j.in, OutputPath: dst, WorkDir: work, OrigDir: j.origDir, Name: j.name, Timeout: time.Until(deadline), Logf: func(f string, a ...any) { fmt.Fprintf(logw, f, a...) }}
	if err := conv.Convert(ctx, d, req); err != nil {
		return err
	}
	r.usedEngine = engineComponent
	return nil
}

func writeOut(dst string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return apperr.Wrap(apperr.IOError, "无法创建输出文件夹", err)
	}
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		return writeErr(err)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return apperr.Wrap(apperr.IOError, "读取文件失败", err)
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return apperr.Wrap(apperr.IOError, "无法创建输出文件夹", err)
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return writeErr(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return writeErr(err)
	}
	if err := out.Close(); err != nil {
		return writeErr(err)
	}
	return nil
}

// ---------- 重试 / 重转 ----------

func notRetryableError() error {
	return apperr.New(apperr.Unsupported, "这个文件不能重试，请移出后重新添加。").WithDetail("reason=not_retryable")
}

// jobFromTask 用记录的 params 重建 docJob：读取路径按这一行当前的副本状态（行不在了用 params.input）。
func (s *Service) jobFromTask(ctx context.Context, old task.Task, in string) (docJob, error) {
	var p docParams
	if err := json.Unmarshal([]byte(old.Params), &p); err != nil || (p.Input == "" && in == "") {
		return docJob{}, apperr.New(apperr.InvalidArgument, "转换任务参数无效")
	}
	sid := p.SourceID
	if sid == "" {
		sid = old.SourceID
	}
	j := docJob{sourceID: sid, in: p.Input, target: p.Target, outputDir: p.OutputDir, simple: old.Type == task.TypeOfficePDF}
	if j.simple {
		j.target = "pdf"
	}
	var src store.ConvertSource
	haveRow := false
	if sid != "" && s.cfg.Sources != nil {
		if row, rin, skip, err := s.cfg.Sources.DocSourceForSubmit(ctx, sid); err == nil {
			src, haveRow = row, true
			if in == "" {
				if skip != "" {
					return docJob{}, convert.CopyNotReady(row, skip == "copying")
				}
				in = rin
			}
		}
	}
	if in != "" {
		j.in = in
	}
	j.name = filepath.Base(j.in)
	if haveRow {
		ds := toDocSource(src)
		j.name, j.src, j.sheetCount = src.Name, ds.Ext, ds.SheetCount
		j.origDir = origDirOf(src, j.in)
	} else {
		j.src = normExt(filepath.Ext(j.in))
		j.origDir = filepath.Dir(j.in)
		j.sheetCount = unknownSheets(j.src)
	}
	if j.src == "" || j.target == "" {
		return docJob{}, formatUnsupportedTarget()
	}
	outDir := j.outputDir
	if outDir == "" {
		outDir = filepath.Dir(old.OutputPath)
	}
	var err error
	if j.desired, err = s.desiredOutputExt(j.name, outDir, j.target); err != nil {
		return docJob{}, err
	}
	return j, nil
}

// docRetryFactory 是 doc_convert / 文档页 office_pdf 的重试（原地，6.6）；不可重试的失败返回 UNSUPPORTED（reason=not_retryable）。
func (s *Service) docRetryFactory(old task.Task) (task.Runner, error) {
	if old.Error != nil && notRetryable(old.Error.Code) {
		return nil, notRetryableError()
	}
	ctx := context.Background()
	j, err := s.jobFromTask(ctx, old, "")
	if err != nil {
		return nil, err
	}
	if !regularFile(j.in) {
		return nil, apperr.New(apperr.NotFound, "文件不存在").WithDetail("reason=file")
	}
	if !j.simple && j.engine() == engineComponent {
		if st := s.componentStatus(ctx, 0); st.State != doccomp.StateReady {
			return nil, s.notReady()
		}
	}
	return s.newDocRunner(j, ""), nil
}

// docReconverter 注册给 ConvertService.Reconvert：同目标格式原地重转，简易转换的记录重转仍是简易转换（6.12.21）。
func (s *Service) docReconverter(ctx context.Context, old task.Task, in, temp string) (task.Runner, string, string, error) {
	j, err := s.jobFromTask(ctx, old, in)
	if err != nil {
		return nil, "", "", err
	}
	if !j.simple && j.engine() == engineComponent {
		if st := s.componentStatus(ctx, 0); st.State != doccomp.StateReady {
			return nil, "", "", s.notReady()
		}
	}
	if _, err := inspectDoc(ctx, j.in, j.src); apperr.Is(err, apperr.DocEncrypted) {
		return nil, "", "", err
	}
	return s.newDocRunner(j, temp), j.params(), paramsSummary(j.src, j.target), nil
}

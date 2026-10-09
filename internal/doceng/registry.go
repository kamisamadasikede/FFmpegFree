package doceng

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/doccomp"
)

// Registry 汇总本机 Office / WPS / 文档组件，负责检测、挑引擎、状态合并（6.12.25 / 6.12.28）。
type Registry struct {
	comp     *doccomp.Manager
	office   Converter
	wps      Converter // 与 office 共用 OfficeConverter
	compConv *ComponentConverter
	skips    *SkipTracker

	mu       sync.Mutex
	officeD  *Detected
	wpsD     *Detected
	detected bool

	// DocEngine 返回当前设置（auto|office|wps|component）；可空 = auto。
	DocEngine func(ctx context.Context) string
	// Emit 在 engines / state 变化时发 doc:component（可空；组件自身也会发）。
	Emit func(event string, payload any)
	Logf func(string, ...any)
}

// Config 创建 Registry。
type Config struct {
	Component *doccomp.Manager
	CompConv  *ComponentConverter
	DocEngine func(ctx context.Context) string
	Emit      func(event string, payload any)
	Logf      func(string, ...any)
}

func NewRegistry(cfg Config) *Registry {
	oc := NewOfficeConverter()
	r := &Registry{
		comp: cfg.Component, office: oc, wps: oc, compConv: cfg.CompConv, skips: NewSkipTracker(),
		DocEngine: cfg.DocEngine, Emit: cfg.Emit, Logf: cfg.Logf,
	}
	if r.Logf == nil {
		r.Logf = func(string, ...any) {}
	}
	return r
}

// StartDetect 后台检测 Office / WPS（与组件检测并行）。
func (r *Registry) StartDetect() {
	go func() {
		o, w := DetectOfficeWPS()
		r.mu.Lock()
		r.officeD, r.wpsD, r.detected = o, w, true
		r.mu.Unlock()
		r.emitStatus()
	}()
}

// Recheck 重新检测 Office / WPS，并重置跳过计数。
func (r *Registry) Recheck() {
	r.skips.Reset()
	o, w := DetectOfficeWPS()
	r.mu.Lock()
	r.officeD, r.wpsD, r.detected = o, w, true
	r.mu.Unlock()
	r.emitStatus()
}

func (r *Registry) EmitStatus() {
	r.emitStatus()
}

func (r *Registry) emitStatus() {
	if r.Emit == nil {
		return
	}
	r.Emit(doccomp.EventComponent, r.Status(context.Background()))
}

// Engines 返回当前检测到的引擎列表（含未下载的 component 项）。
func (r *Registry) Engines(ctx context.Context) []Detected {
	var out []Detected
	r.mu.Lock()
	od, wd, det := r.officeD, r.wpsD, r.detected
	r.mu.Unlock()
	_ = det
	if od != nil {
		cp := *od
		// 应用跳过状态
		cp.Available = od.Available && !r.familyAllSkipped(IDOffice, od.Families)
		out = append(out, cp)
	}
	if wd != nil {
		cp := *wd
		cp.Available = wd.Available && !r.familyAllSkipped(IDWPS, wd.Families)
		out = append(out, cp)
	}
	if r.comp != nil {
		st := r.comp.Status()
		for _, e := range st.Engines {
			d := Detected{
				ID: e.ID, Name: e.Name, Version: e.Version, Source: e.Source,
				Installed: e.Installed, Available: e.Available && !r.familyAllSkipped(IDComponent, e.Families),
				Families: append([]string(nil), e.Families...), ComponentExe: st.Path,
			}
			out = append(out, d)
		}
	}
	return out
}

func (r *Registry) familyAllSkipped(id string, fams []string) bool {
	if len(fams) == 0 {
		return r.skips.Skipped(id, FamilyText) // 保守
	}
	for _, f := range fams {
		if !r.skips.Skipped(id, f) {
			return false
		}
	}
	return true
}

// Status 合并为 DocComponentStatus 形态（给前端）。
type StatusView struct {
	State          string
	ComponentState string
	Version        string
	Source         string
	Engines        []EngineInfo
	CanDownload    bool
	DownloadBytes  int64
	InstallBytes   int64
	Phase          string
	ReceivedBytes  int64
	Error          *apperr.AppError
	Path           string
}

func (r *Registry) Status(ctx context.Context) StatusView {
	var st doccomp.Status
	if r.comp != nil {
		st = r.comp.Status()
	} else {
		st = doccomp.Status{State: doccomp.StateMissing, ComponentState: doccomp.StateMissing}
	}
	engines := r.Engines(ctx)
	infos := make([]EngineInfo, 0, len(engines))
	anyReady := false
	for _, e := range engines {
		infos = append(infos, e.Info())
		if e.Available {
			anyReady = true
		}
	}
	// 确保 Windows/macOS 始终有 component 项（组件 Manager.view 已加；这里 engines 已含）
	if runtime.GOOS != "linux" && !hasComponent(infos) {
		infos = append(infos, EngineInfo{ID: IDComponent, Name: NameComponent, Families: []string{FamilyText, FamilySheet, FamilySlide}})
	}

	pref := "auto"
	if r.DocEngine != nil {
		if p := r.DocEngine(ctx); p != "" {
			pref = p
		}
	}
	src, ver := PrimarySource(PrefOrder(pref), engines)

	v := StatusView{
		ComponentState: st.ComponentState,
		CanDownload:    st.CanDownload,
		DownloadBytes:  st.DownloadBytes,
		InstallBytes:   st.InstallBytes,
		Phase:          st.Phase,
		ReceivedBytes:  st.ReceivedBytes,
		Error:          st.Error,
		Path:           st.Path,
		Engines:        infos,
	}
	r.mu.Lock()
	det := r.detected
	r.mu.Unlock()
	compChecking := st.ComponentState == doccomp.StateChecking || st.State == doccomp.StateChecking
	if !det && compChecking {
		v.State = doccomp.StateChecking
	} else if anyReady {
		v.State = doccomp.StateReady
		v.Source = src
		v.Version = ver
	} else {
		v.State = st.ComponentState
		if v.State == "" {
			v.State = st.State
		}
	}
	return v
}

func hasComponent(infos []EngineInfo) bool {
	for _, e := range infos {
		if e.ID == IDComponent {
			return true
		}
	}
	return false
}

// DownloadedComponentDir 返回应用下载的文档组件目录（source=downloaded 且 ready 时）；否则 ""。
func (r *Registry) DownloadedComponentDir() string {
	if r.comp == nil {
		return ""
	}
	st := r.comp.Status()
	if st.State != doccomp.StateReady || st.Source != doccomp.SourceDownloaded || st.Path == "" {
		return ""
	}
	return filepath.Dir(st.Path)
}

// Skips 暴露给 RecheckDocComponent。
func (r *Registry) Skips() *SkipTracker { return r.skips }

// TryConvert 按顺序试引擎；返回实际 engine id 和错误。
// 全部失败且可简易转换时返回 err=errSimpleFallback（由调用方做简易转换）。
var ErrSimpleFallback = errors.New("simple_fallback") // 内部哨兵，不直接给前端

func (r *Registry) TryConvert(ctx context.Context, src, target, in, out, work, origDir, name string, hasMacro bool, logf func(string, ...any), taskDeadline time.Time) (engineID string, err error) {
	pref := "auto"
	if r.DocEngine != nil {
		if p := r.DocEngine(ctx); p != "" {
			pref = p
		}
	}
	cands := Pick(PrefOrder(pref), r.Engines(ctx), r.skips, src, target)
	if len(cands) == 0 {
		if SimplePDF(src) && target == "pdf" {
			return "", ErrSimpleFallback
		}
		return "", apperr.New(apperr.DocComponentNotReady, "需要先下载文档组件。")
	}
	var last error
	var allBusy, anyPresBusy bool
	for _, c := range cands {
		if !taskDeadline.IsZero() && time.Now().After(taskDeadline) {
			return "", apperr.New(apperr.DocTimeout, "文件处理太久没完成，可能已损坏，请检查后重试。")
		}
		remain := time.Until(taskDeadline)
		timeout := TimeoutOfficeConvert
		if c.ID == IDComponent {
			timeout = TimeoutComponentConvert
		}
		if !taskDeadline.IsZero() && remain > 0 && remain < timeout {
			timeout = remain
		}
		fam := familyOf(src)
		req := ConvertRequest{
			SrcExt: src, Target: target, InputPath: in, OutputPath: out, WorkDir: work,
			OrigDir: origDir, Name: name, Timeout: timeout, HasMacro: hasMacro, Logf: logf,
		}
		var cerr error
		switch c.ID {
		case IDOffice:
			cerr = r.office.Convert(ctx, c.Detected, req)
		case IDWPS:
			cerr = r.wps.Convert(ctx, c.Detected, req)
		case IDComponent:
			if r.compConv == nil {
				cerr = apperr.New(apperr.DocComponentNotReady, "需要先下载文档组件。")
			} else {
				cerr = r.compConv.Convert(ctx, c.Detected, req)
			}
		}
		if cerr == nil {
			r.skips.RecordSuccess(c.ID, fam)
			return c.ID, nil
		}
		last = cerr
		if apperr.Is(cerr, apperr.DocEncrypted) || apperr.Is(cerr, apperr.DocCorrupt) {
			return "", cerr // 不换引擎
		}
		if apperr.Is(cerr, apperr.DocPresentationBusy) {
			anyPresBusy = true
			allBusy = true
			continue
		}
		if apperr.Is(cerr, apperr.DocEngineBusy) {
			allBusy = true
			continue
		}
		allBusy = false
		if apperr.Is(cerr, apperr.DocTimeout) || isStartFail(cerr) {
			r.skips.RecordFailure(c.ID, fam)
			if logf != nil {
				logf("[FFmpegFree] 文档引擎：%s 失败，改用下一个\n", c.Name)
			}
			r.emitStatus()
		}
	}
	if allBusy {
		if anyPresBusy {
			return "", BusyErr(true, IDOffice, false)
		}
		return "", BusyErr(false, IDOffice, false)
	}
	if SimplePDF(src) && target == "pdf" {
		return "", ErrSimpleFallback
	}
	if last == nil {
		last = apperr.New(apperr.DocComponentNotReady, "需要先下载文档组件。")
	}
	return "", last
}

func isStartFail(err error) bool {
	ae := apperr.From(err)
	if ae == nil {
		return false
	}
	return ae.Code == apperr.DocComponentCrashed || ae.Code == apperr.DocTimeout
}

// TryPreviewPDF 预览专用：失败不回退简易转换（由调用方决定 raw）。
func (r *Registry) TryPreviewPDF(ctx context.Context, src, in, out, work string, hasMacro bool, logf func(string, ...any), deadline time.Time) (engineID string, err error) {
	pref := "auto"
	if r.DocEngine != nil {
		if p := r.DocEngine(ctx); p != "" {
			pref = p
		}
	}
	cands := Pick(PrefOrder(pref), r.Engines(ctx), r.skips, src, "pdf")
	if len(cands) == 0 {
		return "", apperr.New(apperr.DocComponentNotReady, "需要先下载文档组件。")
	}
	var last error
	var allBusy, anyPresBusy bool
	for _, c := range cands {
		if !deadline.IsZero() && time.Now().After(deadline) {
			return "", apperr.New(apperr.DocTimeout, "文件处理太久没完成，可能已损坏，请检查后重试。")
		}
		timeout := TimeoutOfficePreview
		if c.ID == IDComponent {
			timeout = TimeoutComponentPreview
		}
		if remain := time.Until(deadline); !deadline.IsZero() && remain > 0 && remain < timeout {
			timeout = remain
		}
		req := PreviewPDFRequest{SrcExt: src, InputPath: in, OutputPath: out, WorkDir: work, Timeout: timeout, HasMacro: hasMacro, Logf: logf}
		var cerr error
		switch c.ID {
		case IDOffice:
			cerr = r.office.PreviewPDF(ctx, c.Detected, req)
		case IDWPS:
			cerr = r.wps.PreviewPDF(ctx, c.Detected, req)
		case IDComponent:
			if r.compConv == nil {
				cerr = apperr.New(apperr.DocComponentNotReady, "需要先下载文档组件。")
			} else {
				cerr = r.compConv.PreviewPDF(ctx, c.Detected, req)
			}
		}
		if cerr == nil {
			r.skips.RecordSuccess(c.ID, familyOf(src))
			return c.ID, nil
		}
		last = cerr
		if apperr.Is(cerr, apperr.DocEncrypted) || apperr.Is(cerr, apperr.DocCorrupt) {
			return "", cerr
		}
		if apperr.Is(cerr, apperr.DocPresentationBusy) {
			anyPresBusy, allBusy = true, true
			continue
		}
		if apperr.Is(cerr, apperr.DocEngineBusy) {
			allBusy = true
			continue
		}
		allBusy = false
		if apperr.Is(cerr, apperr.DocTimeout) || isStartFail(cerr) {
			r.skips.RecordFailure(c.ID, familyOf(src))
			r.emitStatus()
		}
	}
	if allBusy {
		if anyPresBusy {
			return "", BusyErr(true, IDOffice, true)
		}
		return "", BusyErr(false, IDOffice, true)
	}
	if last == nil {
		last = apperr.New(apperr.DocComponentNotReady, "需要先下载文档组件。")
	}
	return "", last
}

// SetCompConv 注入组件转换器（启动时 doc.Service 建好回调后再设）。
func (r *Registry) SetCompConv(c *ComponentConverter) { r.compConv = c }

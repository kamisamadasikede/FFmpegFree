package doceng

import (
	"context"
	"sync"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/doccomp"
)

// Detected 是一次检测结果（本机 Office / WPS 或文档组件）。
type Detected struct {
	ID        string // office | wps | component
	Name      string
	Version   string
	Source    string // component: downloaded | system；office/wps 空
	Installed bool
	Available bool
	Families  []string // text | sheet | slide | pdf
	// ProgIDs / exe paths（仅 Windows COM 用；json:"-"）
	WordProgID, ExcelProgID, PowerPointProgID string
	WordExe, ExcelExe, PowerPointExe          string
	// ExcelMajor 是 Excel 主版本（csv UTF-8 需要 ≥ 16）；0 = 未知 / 没有 Excel。
	ExcelMajor int
	// WordMajor 是 Word 主版本（PDF 重排需要 ≥ 15）；0 = 未知 / 没有 Word。
	WordMajor int
	// ComponentExe 是 soffice 路径（仅 component）。
	ComponentExe string
}

// EngineInfo 对应契约 DocEngineInfo（给前端）。
type EngineInfo struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Version   string   `json:"version"`
	Source    string   `json:"source,omitempty"`
	Installed bool     `json:"installed"`
	Families  []string `json:"families"`
	Available bool     `json:"available"`
}

func (d Detected) Info() EngineInfo {
	return EngineInfo{ID: d.ID, Name: d.Name, Version: d.Version, Source: d.Source, Installed: d.Installed, Families: append([]string(nil), d.Families...), Available: d.Available}
}

// ConvertRequest 是一次转换。
type ConvertRequest struct {
	SrcExt     string // 规范化源扩展名
	Target     string // 目标扩展名（含 pdf）
	InputPath  string // 用户文件 / 副本（调用方负责；引擎内部会再拷到临时目录）
	OutputPath string
	WorkDir    string // <数据目录>/tmp/doc/<id>
	OrigDir    string // md 图片基准目录
	Name       string // 显示名（日志）
	Timeout    time.Duration
	HasMacro   bool
	Logf       func(string, ...any)
}

// PreviewPDFRequest 是预览生成 PDF。
type PreviewPDFRequest struct {
	SrcExt     string
	InputPath  string
	OutputPath string // 最终 .pdf 路径（缓存文件）
	WorkDir    string
	Timeout    time.Duration
	HasMacro   bool
	Logf       func(string, ...any)
}

// Converter 是一个能做转换的引擎后端。
type Converter interface {
	Convert(ctx context.Context, d Detected, req ConvertRequest) error
	PreviewPDF(ctx context.Context, d Detected, req PreviewPDFRequest) error
}

// SkipTracker 记录同一程序连续启动失败 / 超时（6.12.25）：连续 2 次本次运行内跳过。
type SkipTracker struct {
	mu   sync.Mutex
	fail map[string]int // key = office:word | office:excel | ... | wps:word | ...
}

func NewSkipTracker() *SkipTracker { return &SkipTracker{fail: map[string]int{}} }

func skipKey(engineID, family string) string { return engineID + ":" + family }

func (s *SkipTracker) RecordFailure(engineID, family string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := skipKey(engineID, family)
	s.fail[k]++
}

func (s *SkipTracker) RecordSuccess(engineID, family string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.fail, skipKey(engineID, family))
}

func (s *SkipTracker) Skipped(engineID, family string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fail[skipKey(engineID, family)] >= 2
}

func (s *SkipTracker) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fail = map[string]int{}
}

// busyErr 构造 DOC_PRESENTATION_BUSY / DOC_ENGINE_BUSY（detail 首行 engine=）。
func BusyErr(presentation bool, engineID string, forPreview bool) *apperr.AppError {
	var code apperr.Code
	var msg string
	if presentation {
		code = apperr.DocPresentationBusy
		if forPreview {
			msg = "请先关闭正在打开的演示文稿，再预览。"
		} else {
			msg = "请先关闭正在打开的演示文稿，再转换。"
		}
	} else {
		code = apperr.DocEngineBusy
		if forPreview {
			msg = "请先关闭正在打开的文档，再预览。"
		} else {
			msg = "请先关闭正在打开的文档，再转换。"
		}
	}
	return apperr.New(code, msg).WithDetail("engine=" + engineID)
}

// TimeoutConvert / TimeoutPreview 单次超时（6.12.29 / 6.12.32.3）。
const (
	TimeoutOfficeConvert    = 3 * time.Minute
	TimeoutComponentConvert = 5 * time.Minute
	TimeoutTaskMax          = 10 * time.Minute
	TimeoutOfficePreview    = 90 * time.Second
	TimeoutComponentPreview = 2 * time.Minute
	TimeoutPreviewMax       = 3 * time.Minute
)

// ComponentStatus 是文档组件侧的状态视图（来自 doccomp.Manager）。
type ComponentStatus = doccomp.Status

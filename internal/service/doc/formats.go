package doc

import "strings"

// ---------- 文档多格式转换的格式表（契约 v0.26，6.12.10 / 6.12.13） ----------

// 类别。
const (
	FamilyText  = "text"
	FamilySheet = "sheet"
	FamilySlide = "slide"
	// FamilyPDF：v0.28（6.12.59）PDF 作为源；不算文字类，目标单独列（pdfTargets）。
	FamilyPDF = "pdf"
)

// hintKey 与文案（6.12.13）。
const (
	HintCSVFirstSheet = "csv_first_sheet"
	HintMDLossy       = "md_lossy"
	HintSimpleMode    = "simple_mode"

	hintCSVFirstSheetText = "转成 CSV 只会保留第一个工作表。"
	hintMDLossyText       = "转成 Markdown 只保留文字和基本格式，图片和复杂表格会丢失。"
	hintSimpleModeText    = "下载文档组件后可保留图片和排版"
	disabledNeedComponent = "需要文档组件"

	// v0.28（6.12.59）：PDF 源。
	HintPDFLayout        = "pdf_layout"
	hintPDFLayoutText    = "PDF 转 Word 会尽量保留排版，复杂版式和扫描件可能会走样。"
	hintPDFSimpleModeTxt = "只提取文字，不保留排版和图片。" // simple_mode 在 PDF 源上的文案

	// WarningCSVFirstSheetOnly 是转 CSV 时多工作表的结果警告（6.12.16）。
	WarningCSVFirstSheetOnly = "csv_first_sheet_only"
)

// 引擎（params.engine）。
const (
	engineComponent = "component"
	engineGo        = "go"
)

// familyFormats 按 6.12.10 表里的顺序。
var familyFormats = map[string][]string{
	FamilyText:  {"doc", "docx", "odt", "rtf", "txt", "html", "md"},
	FamilySheet: {"xls", "xlsx", "ods", "csv"},
	FamilySlide: {"ppt", "pptx", "odp"},
	FamilyPDF:   {"pdf"},
}

var familyOrder = []string{FamilyText, FamilySheet, FamilySlide, FamilyPDF}

// pdfTargets 是 PDF 源能转成的目标（6.12.59，按文字类顺序，没有 pdf / 表格 / 演示文稿 / 图片）。
var pdfTargets = []string{"doc", "docx", "odt", "rtf", "txt", "html", "md"}

// pdfLayoutTarget：PDF → 这些目标要 Word（≥ 2013）或文档组件。
func pdfLayoutTarget(t string) bool { return t == "doc" || t == "docx" || t == "odt" || t == "rtf" }

// docInputs 是能添加的扩展名（DocFormatMatrix.inputs）。
var docInputs = []string{"doc", "docx", "odt", "rtf", "txt", "html", "htm", "md", "markdown", "xls", "xlsx", "ods", "csv", "ppt", "pptx", "odp", "pdf"}

var aliasOf = map[string]string{"htm": "html", "markdown": "md"}

// normExt 把扩展名规范化（小写、去点、htm → html、markdown → md）；不认识返回 ""。
func normExt(ext string) string {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	if a, ok := aliasOf[ext]; ok {
		ext = a
	}
	if familyOf(ext) == "" {
		return ""
	}
	return ext
}

func familyOf(ext string) string {
	for _, f := range familyOrder {
		for _, e := range familyFormats[f] {
			if e == ext {
				return f
			}
		}
	}
	return ""
}

var displayNames = map[string]string{
	"pdf": "PDF", "docx": "Word (DOCX)", "doc": "Word 97-2003 (DOC)", "odt": "ODT 文档 (ODT)", "rtf": "RTF 文档 (RTF)",
	"txt": "纯文本 (TXT)", "html": "网页 (HTML)", "md": "Markdown (MD)",
	"xlsx": "Excel (XLSX)", "xls": "Excel 97-2003 (XLS)", "ods": "ODS 表格 (ODS)", "csv": "CSV",
	"pptx": "PowerPoint (PPTX)", "ppt": "PowerPoint 97-2003 (PPT)", "odp": "ODP 演示文稿 (ODP)",
}

// shortNames 用于 paramsSummary（如 “Word → PDF”、“Markdown → 网页”）。
var shortNames = map[string]string{
	"pdf": "PDF", "docx": "Word", "doc": "Word 97-2003", "odt": "ODT", "rtf": "RTF", "txt": "纯文本", "html": "网页", "md": "Markdown",
	"xlsx": "Excel", "xls": "Excel 97-2003", "ods": "ODS", "csv": "CSV",
	"pptx": "PowerPoint", "ppt": "PowerPoint 97-2003", "odp": "ODP",
}

func paramsSummary(src, target string) string { return shortNames[src] + " → " + shortNames[target] }

// simplePDFInputs 是组件未就绪时能简易转 PDF 的源格式（6.12.21）。
var simplePDFInputs = map[string]bool{"docx": true, "odt": true, "txt": true}

// goPair 是不需要组件的转换（md ↔ html，纯 Go）。
func goPair(src, target string) bool {
	return src == "md" && target == "html" || src == "html" && target == "md"
}

// DocFormatMatrix 见契约 6.12.13。
type DocFormatMatrix struct {
	ComponentReady bool               `json:"componentReady"`
	Inputs         []string           `json:"inputs"`
	Sources        []DocSourceFormats `json:"sources"`
}

// DocSourceFormats 是一个源格式能转成的目标。
type DocSourceFormats struct {
	Ext     string      `json:"ext"`
	Aliases []string    `json:"aliases,omitempty"`
	Family  string      `json:"family"`
	Targets []DocTarget `json:"targets"`
}

// DocTarget 是一个目标格式。
type DocTarget struct {
	Ext            string `json:"ext"`
	DisplayName    string `json:"displayName"`
	NeedsComponent bool   `json:"needsComponent"`
	Simple         bool   `json:"simple"`
	Available      bool   `json:"available"`
	HintKey        string `json:"hintKey,omitempty"`
	Hint           string `json:"hint,omitempty"`
	DisabledReason string `json:"disabledReason,omitempty"`
	// Engines 是现在能做这个转换的引擎 id（v0.27，6.12.30；只供排查，界面不显示）。本版只有 component。
	Engines []string `json:"engines,omitempty"`
}

// targetFor 计算 src → target（契约 6.12.30）：engineIDs 是现在能做的引擎（已按设置排序）；anyReady = state=ready。
func targetFor(src, target string, anyReady bool, engineIDs []string) (DocTarget, bool) {
	if src == "pdf" {
		return pdfTargetFor(target, engineIDs)
	}
	fam := familyOf(src)
	if fam == "" || src == target {
		return DocTarget{}, false
	}
	if target != "pdf" && familyOf(target) != fam {
		return DocTarget{}, false
	}
	t := DocTarget{Ext: target, DisplayName: displayNames[target], NeedsComponent: !goPair(src, target)}
	hasEngine := len(engineIDs) > 0
	if !hasEngine && anyReady && t.NeedsComponent {
		// 测试 / 未接线引擎时：anyReady 视为文档组件可用
		hasEngine = true
		engineIDs = []string{engineComponent}
	}
	t.Simple = !hasEngine && target == "pdf" && simplePDFInputs[src]
	t.Available = !t.NeedsComponent || hasEngine || t.Simple
	t.Engines = engineIDs
	switch {
	case t.Simple:
		t.HintKey, t.Hint = HintSimpleMode, hintSimpleModeText
	case target == "md":
		t.HintKey, t.Hint = HintMDLossy, hintMDLossyText
	case target == "csv" && src != "csv":
		t.HintKey, t.Hint = HintCSVFirstSheet, hintCSVFirstSheetText
	}
	if !t.Available {
		t.DisabledReason = disabledNeedComponent
	}
	_ = anyReady
	return t, true
}

// pdfTargetFor 是 PDF 源的目标（6.12.59）。engineIDs 是现在能做 pdf → target 的排版引擎（office / component，
// 已按设置排序；纯 Go 不在里面）。
func pdfTargetFor(target string, engineIDs []string) (DocTarget, bool) {
	ok := false
	for _, t := range pdfTargets {
		if t == target {
			ok = true
		}
	}
	if !ok {
		return DocTarget{}, false
	}
	hasComp := false
	for _, id := range engineIDs {
		if id == engineComponent {
			hasComp = true
		}
	}
	t := DocTarget{Ext: target, DisplayName: displayNames[target]}
	switch {
	case pdfLayoutTarget(target):
		t.NeedsComponent = true
		t.Available = len(engineIDs) > 0
		t.Engines = engineIDs
		t.HintKey, t.Hint = HintPDFLayout, hintPDFLayoutText
		if !t.Available {
			t.DisabledReason = disabledNeedComponent
		}
	case target == "html":
		t.Available = true
		if hasComp {
			t.Engines = []string{engineComponent}
		} else {
			t.Simple = true
			t.Engines = []string{engineGo}
			t.HintKey, t.Hint = HintSimpleMode, hintPDFSimpleModeTxt
		}
	default: // txt / md：纯 Go，恒可用
		t.Simple, t.Available = true, true
		t.Engines = []string{engineGo}
		if hasComp {
			t.Engines = append(t.Engines, engineComponent)
		}
		if target == "md" {
			t.HintKey, t.Hint = HintMDLossy, hintMDLossyText
		} else {
			t.HintKey, t.Hint = HintSimpleMode, hintPDFSimpleModeTxt
		}
	}
	return t, true
}

// buildMatrix 生成格式表（6.12.13 / 6.12.30）。
// engineFn(src, target) 返回能做这个转换的引擎 id 列表；anyReady 是整体 state=ready。
func buildMatrix(anyReady bool, engineFn func(src, target string) []string) DocFormatMatrix {
	m := DocFormatMatrix{ComponentReady: anyReady, Inputs: append([]string(nil), docInputs...)}
	for _, fam := range familyOrder {
		for _, src := range familyFormats[fam] {
			sf := DocSourceFormats{Ext: src, Family: fam, Targets: []DocTarget{}}
			switch src {
			case "html":
				sf.Aliases = []string{"htm"}
			case "md":
				sf.Aliases = []string{"markdown"}
			}
			tgs := append([]string{"pdf"}, familyFormats[fam]...)
			if fam == FamilyPDF {
				tgs = pdfTargets
			}
			for _, tg := range tgs {
				var ids []string
				if engineFn != nil {
					ids = engineFn(src, tg)
				}
				if t, ok := targetFor(src, tg, anyReady, ids); ok {
					sf.Targets = append(sf.Targets, t)
				}
			}
			m.Sources = append(m.Sources, sf)
		}
	}
	return m
}

// convertToArg 是 --convert-to 的值（6.12.11 的表；作为单个参数传给进程，不需要 shell 引号）。
func convertToArg(fam, target string) string {
	switch target {
	case "pdf":
		switch fam {
		case FamilySheet:
			return "pdf:calc_pdf_Export"
		case FamilySlide:
			return "pdf:impress_pdf_Export"
		}
		return "pdf:writer_pdf_Export"
	case "docx":
		return "docx:MS Word 2007 XML"
	case "doc":
		return "doc:MS Word 97"
	case "odt":
		return "odt:writer8"
	case "rtf":
		return "rtf:Rich Text Format"
	case "txt":
		return "txt:Text (encoded):UTF8"
	case "html":
		return "html:XHTML Writer File:UTF8"
	case "xlsx":
		return "xlsx:Calc MS Excel 2007 XML"
	case "xls":
		return "xls:MS Excel 97"
	case "ods":
		return "ods:calc8"
	case "csv":
		return "csv:Text - txt - csv (StarCalc):44,34,76,1,,0,false,true,false,false,false,1"
	case "pptx":
		return "pptx:Impress MS PowerPoint 2007 XML"
	case "ppt":
		return "ppt:MS PowerPoint 97"
	case "odp":
		return "odp:impress8"
	}
	return ""
}

// inFilterFor 是 --infilter 的值（6.12.11）；md 的临时 HTML 按 html 处理。
func inFilterFor(src string) string {
	switch src {
	case "txt":
		return "Text (encoded):UTF8"
	case "html", "md":
		return "HTML (StarWriter)"
	case "pdf":
		return "writer_pdf_import" // 6.12.61：不加时组件用 Draw 打开 PDF，导不出文字类格式
	case "csv":
		return "CSV:44,34,76,1"
	}
	return ""
}

package doceng

// 能力表（契约 6.12.27）：格式表和挑引擎都读这里；真机验证后只改这张表。

// ID 是引擎 id。
const (
	IDOffice    = "office"
	IDWPS       = "wps"
	IDComponent = "component"
	IDGo        = "go"     // 只出现在 result.engine
	IDSimple    = "simple" // 只出现在 result.engine
)

// 显示名（界面可以出现）。
const (
	NameOffice    = "Microsoft Office"
	NameWPS       = "WPS"
	NameComponent = "文档组件"
)

// Family 类别。
const (
	FamilyText  = "text"
	FamilySheet = "sheet"
	FamilySlide = "slide"
	// FamilyPDF 是 PDF 源（v0.28，6.12.59）：只作源；目标单独列，不和 text 家族混在一起。
	FamilyPDF = "pdf"
)

// WordMinMajorForPDF：Word 2013（主版本 15）起才有 PDF 重排（6.12.59）。
const WordMinMajorForPDF = 15

// Cap 是某个引擎在某一类上的源/目标集合。
type Cap struct {
	Sources map[string]bool
	Targets map[string]bool
}

func set(ss ...string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[s] = true
	}
	return m
}

// officeCaps：Word / Excel / PowerPoint 分别检测，装了哪个算哪类（6.12.27）。
var officeCaps = map[string]Cap{
	FamilyText:  {Sources: set("doc", "docx", "odt", "rtf", "txt", "html", "md"), Targets: set("pdf", "docx", "doc", "odt", "rtf", "txt", "html", "md")},
	FamilySheet: {Sources: set("xls", "xlsx", "ods", "csv"), Targets: set("pdf", "xlsx", "xls", "ods", "csv")},
	FamilySlide: {Sources: set("ppt", "pptx", "odp"), Targets: set("pdf", "pptx", "ppt", "odp")},
	// PDF 只看 Word（≥ 15，Pick 里再按 WordMajor 过滤），只转 doc / docx / odt / rtf（6.12.59）。
	FamilyPDF: {Sources: set("pdf"), Targets: set("docx", "doc", "odt", "rtf")},
}

// wpsCaps：一期不做 ODF（6.12.27）。
var wpsCaps = map[string]Cap{
	FamilyText:  {Sources: set("doc", "docx", "rtf", "txt", "html", "md"), Targets: set("pdf", "docx", "doc", "rtf", "txt", "html", "md")},
	FamilySheet: {Sources: set("xls", "xlsx", "csv"), Targets: set("pdf", "xlsx", "xls", "csv")},
	FamilySlide: {Sources: set("ppt", "pptx"), Targets: set("pdf", "pptx", "ppt")},
}

// componentCaps：v0.26 的全部 + v0.28 PDF 源（writer_pdf_import）。
var componentCaps = map[string]Cap{
	FamilyText:  {Sources: set("doc", "docx", "odt", "rtf", "txt", "html", "md"), Targets: set("pdf", "docx", "doc", "odt", "rtf", "txt", "html", "md")},
	FamilySheet: {Sources: set("xls", "xlsx", "ods", "csv"), Targets: set("pdf", "xlsx", "xls", "ods", "csv")},
	FamilySlide: {Sources: set("ppt", "pptx", "odp"), Targets: set("pdf", "pptx", "ppt", "odp")},
	FamilyPDF:   {Sources: set("pdf"), Targets: set("docx", "doc", "odt", "rtf", "txt", "html", "md")},
}

// CapsFor 返回引擎能力表；office 的 sheet 另受 Excel 版本限制（csv 需要 ≥ 16），由调用方再过滤。
func CapsFor(id string) map[string]Cap {
	switch id {
	case IDOffice:
		return officeCaps
	case IDWPS:
		return wpsCaps
	case IDComponent:
		return componentCaps
	}
	return nil
}

// CanConvert 判断引擎能否做 src→target（跨类除 pdf 外不行；转成自己不行）。
func CanConvert(id, src, target string, families []string) bool {
	caps := CapsFor(id)
	if caps == nil || src == "" || target == "" || src == target {
		return false
	}
	fam := familyOf(src)
	if fam == "" {
		return false
	}
	if fam != FamilyPDF && target != "pdf" && familyOf(target) != fam {
		return false
	}
	okFam := false
	for _, f := range families {
		if f == fam {
			okFam = true
			break
		}
	}
	if !okFam {
		return false
	}
	c, ok := caps[fam]
	if !ok || !c.Sources[src] {
		return false
	}
	if target == "pdf" && fam != FamilyPDF {
		return true
	}
	return c.Targets[target]
}

func familyOf(ext string) string {
	switch ext {
	case "doc", "docx", "odt", "rtf", "txt", "html", "md":
		return FamilyText
	case "xls", "xlsx", "ods", "csv":
		return FamilySheet
	case "ppt", "pptx", "odp":
		return FamilySlide
	case "pdf":
		return FamilyPDF
	}
	return ""
}

// SimplePDF 是没有任何引擎时能简易转 PDF 的源格式（6.12.21 / 6.12.25）。
func SimplePDF(src string) bool {
	return src == "docx" || src == "odt" || src == "txt"
}

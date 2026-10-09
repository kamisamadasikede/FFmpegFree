// Package doc 是 DocService 的实现（契约 6.12）：Office（docx / xlsx / pptx）纯文本转 PDF、PDF 预览的字节读取、最近 PDF 列表。
//
// Office 转 PDF 是“提取文字后重排”，不是版式保真：没有图片、表格线、样式（与 v1 一致）。纯 Go，不依赖 ffmpeg。
// PDF 的渲染全在前端（pdf.js），后端只把字节交出去：≤64 MiB 前端循环 ReadPDFChunk，更大的走 /local/<token> 按 Range 加载。
package doc

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/doceng"
	"FFmpegFree/internal/localassets"
	"FFmpegFree/internal/store"
	"FFmpegFree/internal/task"
)

// 限制（契约 6.12.2 DocLimits）。
const (
	MaxInputsPerSubmit       = 50
	MaxInputBytes      int64 = 100 << 20
	MaxPages                 = 5000
	MaxPDFBytes        int64 = 512 << 20
	ChunkBytes               = 1 << 20
	WholeLoadBytes     int64 = 64 << 20
	MaxRemoveIDs             = 500

	maxEntryBytes  int64 = 256 << 20 // zip 单个条目解压后上限（防 zip 炸弹）
	maxCellRunes         = 32767     // xlsx 单元格文本上限（Excel 自身上限）
	windowsMaxPath       = 259
	nameSuffixRoom       = 24 // ".part" + ".pdf" + "(999)" 等余量（UTF-16 码元）
)

// DocCapabilities 描述 Office 转 PDF 的能力（GetDocCapabilities 的返回值）。
type DocCapabilities struct {
	Formats []DocFormat `json:"formats"`
	Font    DocFont     `json:"font"`
	Limits  DocLimits   `json:"limits"`
	// Experimental 由后端给出：Office 转 PDF 是否在界面上标“实验性”（首版恒为 true）。
	Experimental bool `json:"experimental"`
}

// DocFormat 是一种输入格式的支持情况。
type DocFormat struct {
	Ext       string `json:"ext"` // 不带点，小写
	Supported bool   `json:"supported"`
	Fidelity  string `json:"fidelity"` // "text-only"（支持的三种）| ""
	Reason    string `json:"reason"`   // 不支持时的原因
}

// DocFont 是主用字体的状态。
type DocFont struct {
	Available bool   `json:"available"`
	Name      string `json:"name"` // noto-sans-sc-embedded / simhei / msyh / simsun / arialunicode / dejavu / ""
	Cjk       bool   `json:"cjk"`
}

// DocLimits 是各项上限。
type DocLimits struct {
	MaxInputsPerSubmit int   `json:"maxInputsPerSubmit"`
	MaxInputBytes      int64 `json:"maxInputBytes"`
	MaxPages           int   `json:"maxPages"`
	MaxPDFBytes        int64 `json:"maxPdfBytes"`
	ChunkBytes         int   `json:"chunkBytes"`
	WholeLoadBytes     int64 `json:"wholeLoadBytes"`
}

// PDFSource 是 OpenPDF 的返回值。
type PDFSource struct {
	ID   string `json:"id"` // 句柄（128 位随机，进程内有效）；同一路径复用同一 id
	Path string `json:"path"`
	Name string `json:"name"`
	Size int64  `json:"size"`
	// URL 是 /local/<token>，只有 size > WholeLoadBytes（64 MiB）时才有值，前端按 Range 加载；否则为空，前端用 ReadPDFChunk。
	URL string `json:"url"`
}

// PDFChunk 是 ReadPDFChunk 的返回值。
type PDFChunk struct {
	Offset int64  `json:"offset"`
	Length int    `json:"length"` // 实际读到的字节数
	EOF    bool   `json:"eof"`    // offset+length >= 文件当前大小
	Size   int64  `json:"size"`   // 本次读取时文件的当前大小；与 OpenPDF 返回的 size 不同说明文件读取期间被改动，前端应重新 OpenPDF
	Data   string `json:"data"`   // 后端显式编码的标准 base64（含填充）；Length 是解码后的原始字节数
}

// PDFFile 是最近打开列表的一项。
type PDFFile struct {
	ID       string `json:"id"` // doc_recent.id（ULID）
	Path     string `json:"path"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	OpenedAt int64  `json:"openedAt"`
	Exists   bool   `json:"exists"`
}

// RecentStore 是最近 PDF 列表的持久化能力，*store.Store 实现它。
type RecentStore interface {
	UpsertDocRecent(ctx context.Context, pathKey string, r store.DocRecent) (store.DocRecent, error)
	ListDocRecent(ctx context.Context, limit int) ([]store.DocRecent, error)
	DeleteDocRecent(ctx context.Context, ids []string) ([]string, error)
}

// TaskSubmitter 是任务管理器的能力（*task.Manager 实现）。
type TaskSubmitter interface {
	Submit(spec task.Spec, r task.Runner) (task.Task, error)
	RegisterFactory(t task.Type, f task.Factory)
}

// LocalAssets 是 /local/<token> 登记表（*localassets.Registry 实现）。
type LocalAssets interface {
	Register(path string) (localassets.Entry, error)
	Revoke(token string)
}

// Config 是 Service 的依赖。
type Config struct {
	Recent RecentStore   // 可为 nil：OpenPDF 不记录、ListRecentPDFs 返回空
	Lister TaskLister    // 可为 nil：不做启动时的 .part 清理
	Tasks  TaskSubmitter // 可为 nil：ConvertToPDF 返回 INTERNAL
	Local  LocalAssets   // 可为 nil：PDFSource.URL 恒为空
	// DefaultOutputDir 返回设置里的默认输出目录，空表示与源文件同目录。可为 nil。
	DefaultOutputDir func(ctx context.Context) string
	// DataDir 是应用数据目录；输出目录不能在它里面。为空则不检查。
	DataDir string

	// v0.26 文档多格式转换（6.12.9~）：
	// Component 是文档组件管理器（可为 nil：一律按未就绪）；Sources 是文档页的源文件行（*convert.Service，可为 nil：文档页接口返回 INTERNAL）；
	// TempRoot 是每个任务的临时目录根 <数据目录>/tmp/doc（启动时清空）。
	Component Component
	Sources   DocSources
	TempRoot  string
	// UploadsDir 返回实际上传目录（另存为路径校验，6.12.42）；nil 时用 <DataDir>/uploads。
	UploadsDir func(ctx context.Context) string

	// v0.27 引擎与预览：
	Engines   *doceng.Registry // 可为 nil：只有文档组件（走 Component）
	DocEngine func(ctx context.Context) string
	Emit      func(event string, payload any)
	TaskGet   func(ctx context.Context, id string) (task.Task, error)

	// 以下供测试覆盖。
	//
	// EmbeddedFont 为 nil 用内嵌的 Noto Sans SC 子集；DisableEmbedded 模拟“内嵌字体加载失败”。
	EmbeddedFont    []byte
	DisableEmbedded bool
	// SystemFonts 为 nil 用平台默认候选；非 nil 的空切片表示没有系统字体。
	SystemFonts []SystemFont
	// WindowsMaxPath 是整条输出路径的 UTF-16 码元上限：0 时 Windows 为 259，其他平台不限制；负数不限制。
	WindowsMaxPath int
}

// Service 实现 DocService。
type Service struct {
	cfg   Config
	fonts *fontSet
	h     *handles
	maxP  int
	// started 是服务创建时刻：清理 .part 只删修改时间早于它的文件。
	started time.Time

	previews *previewHub
	saves    *binarySaveHub

	// 引擎 PDF 预览（v0.27）
	prevOnce  sync.Once
	prevCache *previewCache
	prevQueue chan previewJob
}

// New 创建 Service，并注册 office_pdf 的重试工厂。
func New(cfg Config) *Service {
	s := &Service{cfg: cfg, fonts: newFontSet(cfg), h: newHandles(), maxP: cfg.WindowsMaxPath, started: time.Now(),
		previews: &previewHub{}, saves: &binarySaveHub{sessions: map[string]*binarySaveSession{}, byPath: map[string]string{}}}
	if s.maxP == 0 && runtime.GOOS == "windows" {
		s.maxP = windowsMaxPath
	}
	if cfg.Tasks != nil {
		cfg.Tasks.RegisterFactory(task.TypeOfficePDF, s.retryFactory)
		cfg.Tasks.RegisterFactory(task.TypeDocConvert, s.docRetryFactory)
	}
	if cfg.Sources != nil {
		cfg.Sources.SetDocReconverter(s.docReconverter)
	}
	go s.sweepSaves()
	return s
}

var unsupportedFormats = []struct{ ext, reason string }{
	{"doc", "旧版二进制格式，请先另存为 docx"},
	{"xls", "旧版二进制格式，请先另存为 xlsx"},
	{"ppt", "旧版二进制格式，请先另存为 pptx"},
	{"odt", "暂不支持 OpenDocument 格式，请先另存为 docx"},
	{"ods", "暂不支持 OpenDocument 格式，请先另存为 xlsx"},
	{"odp", "暂不支持 OpenDocument 格式，请先另存为 pptx"},
	{"rtf", "暂不支持 RTF，请先另存为 docx"},
	{"csv", "暂不支持该格式"},
	{"txt", "暂不支持该格式"},
}

// GetDocCapabilities 返回支持的格式、主用字体状态和各项上限。不依赖 ffmpeg，不读文档，随时可调。
func (s *Service) GetDocCapabilities() DocCapabilities {
	c := DocCapabilities{Experimental: true, Formats: []DocFormat{}}
	for _, e := range []string{"docx", "xlsx", "pptx"} {
		c.Formats = append(c.Formats, DocFormat{Ext: e, Supported: true, Fidelity: "text-only"})
	}
	for _, u := range unsupportedFormats {
		c.Formats = append(c.Formats, DocFormat{Ext: u.ext, Reason: u.reason})
	}
	c.Font = s.fonts.primary()
	c.Limits = DocLimits{
		MaxInputsPerSubmit: MaxInputsPerSubmit, MaxInputBytes: MaxInputBytes, MaxPages: MaxPages,
		MaxPDFBytes: MaxPDFBytes, ChunkBytes: ChunkBytes, WholeLoadBytes: WholeLoadBytes,
	}
	return c
}

func (s *Service) internalNotReady(what string) error {
	return apperr.New(apperr.Internal, what+"尚未初始化")
}

// detail 首行的 reason 枚举（契约 2.2 / 6.12.6）：Doc 面向前端的“文件本身有问题”类错误，detail 首行严格是 `reason=<值>`，
// 其后可以有自由文本行（原因说明、出错文件路径）；code 和 message 不变（前端精确匹配 message）。
// 映射不到这六个值的错误（取消、磁盘满、读写失败、参数不合法、路径 / 句柄问题等）不带 reason 行。
const (
	reasonTooManyPages = "too_many_pages" // 超过 5000 页（含文字量超限）
	reasonFormat       = "format"         // 格式不受支持 / 不是支持的文件类型
	reasonEncrypted    = "encrypted"      // 加密文档（OLE 容器）
	reasonNoFont       = "no_font"        // 缺 Unicode 字体
	reasonInvalidOOXML = "invalid_ooxml"  // 不是有效的 OOXML（不是 zip、缺必需部件、XML 损坏、zip 目录信息无效）
	reasonTooLarge     = "too_large"      // 超大小 / 超 zip 条目数 / 超 zip 目录 / 条目解压后过大
)

// reasonErr 构造 detail 首行是 reason=<值> 的错误；extra 非空时作为第二行起的自由文本。
func reasonErr(code apperr.Code, message, reason, extra string) *apperr.AppError {
	d := "reason=" + reason
	if extra != "" {
		d += "\n" + extra
	}
	return apperr.New(code, message).WithDetail(d)
}

// readErr 把打开 / stat 文件的系统错误转成契约错误码：不存在 NOT_FOUND，其余 IO_ERROR。
func readErr(msg string, err error) *apperr.AppError {
	if os.IsNotExist(err) {
		return apperr.Wrap(apperr.NotFound, "文件不存在", err)
	}
	return apperr.Wrap(apperr.IOError, msg, err)
}

func withPath(err error, path string) error {
	ae := apperr.From(err)
	cp := *ae
	switch {
	case cp.Detail == "":
		cp.Detail = path
	case strings.HasPrefix(cp.Detail, "reason="):
		// reason 行必须留在首行，路径插在它后面
		first, rest, _ := strings.Cut(cp.Detail, "\n")
		cp.Detail = first + "\n" + path
		if rest != "" {
			cp.Detail += "\n" + rest
		}
	default:
		cp.Detail = path + "\n" + cp.Detail
	}
	return &cp
}

func extLower(p string) string { return strings.ToLower(strings.TrimPrefix(filepath.Ext(p), ".")) }

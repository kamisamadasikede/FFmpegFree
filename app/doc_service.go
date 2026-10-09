package app

import (
	"context"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/service/convert"
	"FFmpegFree/internal/service/doc"
	"FFmpegFree/internal/store"
)

// DocService 是 Office 转 PDF 与 PDF 预览的 Wails 绑定（契约 4 节、6.12）。只做转发，逻辑在 internal/service/doc。
// 底层服务依赖数据库和任务管理器，OnStartup 之后才存在，启动完成之前调用返回 INTERNAL。
//
// Office 转 PDF 是“提取文字后重排”（无图片、表格线和样式），docx / xlsx / pptx 之外的格式返回 UNSUPPORTED。
// PDF 的渲染全在前端（pdf.js）：≤64 MiB 前端循环 ReadPDFChunk 读整份，更大的用 PDFSource.url（/local/<token>）按 Range 加载。
type DocService struct {
	get func() *doc.Service
	ctx func() context.Context
}

// NewDocService 创建 DocService。get 每次调用时返回当前服务（未就绪返回 nil）；rootCtx 返回应用根 ctx。
func NewDocService(get func() *doc.Service, rootCtx func() context.Context) *DocService {
	return &DocService{get: get, ctx: rootCtx}
}

func (s *DocService) rootCtx() context.Context {
	if s.ctx != nil {
		if c := s.ctx(); c != nil {
			return c
		}
	}
	return context.Background()
}

func (s *DocService) svc() (*doc.Service, error) {
	if s.get != nil {
		if c := s.get(); c != nil {
			return c, nil
		}
	}
	return nil, apperr.New(apperr.Internal, "文档服务尚未初始化")
}

// GetDocCapabilities 返回支持的格式、字体状态、各项上限和 experimental 标志。不依赖 ffmpeg，随时可调。
func (s *DocService) GetDocCapabilities() (doc.DocCapabilities, error) {
	c, err := s.svc()
	if err != nil {
		return doc.DocCapabilities{}, err
	}
	return c.GetDocCapabilities(), nil
}

// ConvertToPDF 为每个输入文件提交一个 office_pdf 任务，返回的任务与 inputs 一一对应；进度、取消、重试走任务中心。
// 只支持 docx / xlsx / pptx，且只提取文字（无图片和样式）；其他格式与加密文档返回 UNSUPPORTED。
// 先整体校验再提交，任何一个不通过整体失败，错误 detail：有 reason 的错误首行是 reason=<枚举>、第二行是出错文件路径，其余首行是出错文件路径（契约 2.2 / 6.12.3）。
// 错误码：INVALID_ARGUMENT、NOT_FOUND、IO_ERROR、UNSUPPORTED、CANCELED；任务失败见任务的 error（含 CONVERT_DISK_FULL）。
func (s *DocService) ConvertToPDF(inputs []string, outputDir string) ([]store.Task, error) {
	c, err := s.svc()
	if err != nil {
		return nil, err
	}
	return c.ConvertToPDF(s.rootCtx(), inputs, outputDir)
}

// OpenPDF 校验并登记一个 PDF，返回句柄（同一路径复用同一 id），同时写入最近打开列表。
// size > 64 MiB 时 url 是 /local/<token>，前端按 Range 加载；否则 url 为空，前端循环 ReadPDFChunk。
func (s *DocService) OpenPDF(path string) (doc.PDFSource, error) {
	c, err := s.svc()
	if err != nil {
		return doc.PDFSource{}, err
	}
	return c.OpenPDF(s.rootCtx(), path)
}

// ReadPDFChunk 按句柄读一段 PDF 字节（length 1~1 MiB）。返回的 data 是后端显式编码的标准 base64 字符串（含填充），length 仍是原始字节数（上限 1 MiB）。
func (s *DocService) ReadPDFChunk(id string, offset int64, length int) (doc.PDFChunk, error) {
	c, err := s.svc()
	if err != nil {
		return doc.PDFChunk{}, err
	}
	return c.ReadPDFChunk(id, offset, length)
}

// ListRecentPDFs 返回最近打开的 PDF，按打开时间倒序；默认 20，最大 200。文件已删除的 exists=false。
func (s *DocService) ListRecentPDFs(limit int) ([]doc.PDFFile, error) {
	c, err := s.svc()
	if err != nil {
		return nil, err
	}
	return c.ListRecentPDFs(s.rootCtx(), limit)
}

// RemoveRecentPDFs 只删最近打开记录（不删文件），同时撤销对应的句柄和 /local/<token>。一次最多 500 个 id，不存在的忽略。
func (s *DocService) RemoveRecentPDFs(ids []string) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.RemoveRecentPDFs(s.rootCtx(), ids)
}

// ---------- v0.26 文档多格式转换（契约 6.12.9~6.12.22） ----------

// GetFormatMatrix 按当前文档组件状态生成格式表；组件在检测中时最多等 6 秒，等不到按未就绪返回。
// 组件状态变为 ready（或从 ready 变成别的）时前端重新调用（doc:component 事件）。
func (s *DocService) GetFormatMatrix() (doc.DocFormatMatrix, error) {
	c, err := s.svc()
	if err != nil {
		return doc.DocFormatMatrix{}, err
	}
	return c.GetFormatMatrix(s.rootCtx()), nil
}

// GetDocComponentStatus 返回文档组件状态（checking | ready | missing | outdated | downloading | preparing | failed）。
func (s *DocService) GetDocComponentStatus() (doc.DocComponentStatus, error) {
	c, err := s.svc()
	if err != nil {
		return doc.DocComponentStatus{}, err
	}
	return c.GetDocComponentStatus(s.rootCtx())
}

// InstallDocComponent 开始或继续下载 + 准备文档组件（mirror 只接受 "" 和 "cn"），立即返回；幂等。
// 进度走 doc:component-progress，状态走 doc:component。Linux 返回 UNSUPPORTED_PLATFORM；空间不够 CONVERT_DISK_FULL（reason=no_space）。
func (s *DocService) InstallDocComponent(mirror string) (doc.DocComponentStatus, error) {
	c, err := s.svc()
	if err != nil {
		return doc.DocComponentStatus{}, err
	}
	return c.InstallDocComponent(s.rootCtx(), mirror)
}

// CancelDocComponentInstall 取消下载 / 准备（保留已下载的部分）；没有在进行时无操作。
func (s *DocService) CancelDocComponentInstall() error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.CancelDocComponentInstall(s.rootCtx())
}

// RecheckDocComponent 重新检测文档组件（用户自己装了系统版本后）；下载 / 准备中返回当前状态，不打断。
func (s *DocService) RecheckDocComponent() (doc.DocComponentStatus, error) {
	c, err := s.svc()
	if err != nil {
		return doc.DocComponentStatus{}, err
	}
	return c.RecheckDocComponent(s.rootCtx())
}

// AddDocSources 登记文档页的源文件行（1~50 个，与入参一一对应）；被拒的文件不建行，原因在 error 里。
func (s *DocService) AddDocSources(paths []string) ([]doc.AddDocSourceResult, error) {
	c, err := s.svc()
	if err != nil {
		return nil, err
	}
	return c.AddDocSources(s.rootCtx(), paths)
}

// ListDocSources 分页列文档页的行（只有文档页添加的行）。
func (s *DocService) ListDocSources(filter convert.ConvertSourceFilter) (doc.DocSourcePage, error) {
	c, err := s.svc()
	if err != nil {
		return doc.DocSourcePage{Items: []doc.DocSourceEntry{}}, err
	}
	return c.ListDocSources(s.rootCtx(), filter)
}

// SearchDocSources 按关键字搜文档页的行。
func (s *DocService) SearchDocSources(filter convert.ConvertSearchFilter) (doc.DocSourcePage, error) {
	c, err := s.svc()
	if err != nil {
		return doc.DocSourcePage{Items: []doc.DocSourceEntry{}}, err
	}
	return c.SearchDocSources(s.rootCtx(), filter)
}

// SubmitDocConvert 一个源一个任务；返回 {tasks, skipped}，规则同 ConvertService.SubmitSources。
func (s *DocService) SubmitDocConvert(req doc.DocSubmitRequest) (convert.ConvertSubmitResult, error) {
	c, err := s.svc()
	if err != nil {
		return convert.ConvertSubmitResult{Tasks: []store.Task{}, Skipped: []convert.SkippedSource{}}, err
	}
	return c.SubmitDocConvert(s.rootCtx(), req)
}

// ---------- v0.27 文档预览（契约 6.12.32） ----------

// GetDocPreview 开始（或从缓存直接拿）一个预览。
func (s *DocService) GetDocPreview(req doc.DocPreviewRequest) (doc.DocPreview, error) {
	c, err := s.svc()
	if err != nil {
		return doc.DocPreview{}, err
	}
	return c.GetDocPreview(s.rootCtx(), req)
}

// CancelDocPreview 弹窗关掉时调用：生成中就停止；已就绪就撤销本地地址。不存在的 id 忽略。
func (s *DocService) CancelDocPreview(previewID string) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.CancelDocPreview(previewID)
}

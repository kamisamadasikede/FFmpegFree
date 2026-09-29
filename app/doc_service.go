package app

import (
	"context"

	"FFmpegFree/internal/apperr"
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
// 先整体校验再提交，任何一个不通过整体失败，错误 detail 第一行是出错文件路径。
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

// ReadPDFChunk 按句柄读一段 PDF 字节（length 1~1 MiB）。返回的 data 在生成的 TypeScript 里是 base64 字符串。
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

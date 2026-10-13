// Package office 实现办公画布组件状态与 PPTX 导出骨架（契约 6.19.16）。
package office

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/officecanvas"
)

// Config 依赖。
type Config struct {
	Canvas          *officecanvas.Manager
	Emit            func(event string, payload any)
	Logf            func(format string, args ...any)
	ActualOutputDir func(ctx context.Context) string
	TempRoot        string // <data>/tmp/office
}

// Service 是办公画布服务。
type Service struct {
	cfg Config

	artMu sync.Mutex
	arts  map[string]SlideArtifact // key = convId + "\x00" + artifactId
}

// SlideArtifact 是 Cat 演示文稿工具卡的内存登记（TODO: cat:artifact kind=slides 流式写入）。
type SlideArtifact struct {
	ConvID     string
	ArtifactID string
	Deck       officecanvas.Deck
}

// New 创建服务。
func New(cfg Config) *Service {
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	return &Service{cfg: cfg, arts: make(map[string]SlideArtifact)}
}

func artKey(convID, artifactID string) string {
	return convID + "\x00" + artifactID
}

// UpsertSlideArtifact 登记 / 更新演示文稿 IR（内部 / 测试；Cat 流式卡后续接入）。
func (s *Service) UpsertSlideArtifact(a SlideArtifact) {
	if a.ConvID == "" || a.ArtifactID == "" {
		return
	}
	s.artMu.Lock()
	s.arts[artKey(a.ConvID, a.ArtifactID)] = a
	s.artMu.Unlock()
}

func (s *Service) getArtifact(convID, artifactID string) (SlideArtifact, bool) {
	s.artMu.Lock()
	defer s.artMu.Unlock()
	a, ok := s.arts[artKey(convID, artifactID)]
	return a, ok
}

// ---------- 组件 ----------

func (s *Service) GetOfficeCanvasStatus(ctx context.Context) (officecanvas.Status, error) {
	_ = ctx
	if s.cfg.Canvas == nil {
		return officecanvas.Status{}, apperr.New(apperr.Internal, "办公画布服务尚未初始化")
	}
	return s.cfg.Canvas.Status(), nil
}

func (s *Service) InstallOfficeCanvas(ctx context.Context) error {
	_ = ctx
	if s.cfg.Canvas == nil {
		return apperr.New(apperr.Internal, "办公画布服务尚未初始化")
	}
	return s.cfg.Canvas.Install()
}

func (s *Service) CancelOfficeCanvasInstall(ctx context.Context) error {
	_ = ctx
	if s.cfg.Canvas == nil {
		return apperr.New(apperr.Internal, "办公画布服务尚未初始化")
	}
	s.cfg.Canvas.CancelInstall()
	return nil
}

func (s *Service) RecheckOfficeCanvas(ctx context.Context) (officecanvas.Status, error) {
	_ = ctx
	if s.cfg.Canvas == nil {
		return officecanvas.Status{}, apperr.New(apperr.Internal, "办公画布服务尚未初始化")
	}
	return s.cfg.Canvas.Recheck(), nil
}

// ComponentDir 返回已安装目录（OpenStorageFolder("office_canvas")）。
func (s *Service) ComponentDir() (string, error) {
	if s.cfg.Canvas == nil {
		return "", apperr.New(apperr.NotFound, "办公画布组件还没有就绪。").WithDetail("reason=component")
	}
	st := s.cfg.Canvas.Status()
	if st.State != officecanvas.StateReady || st.Path == "" {
		return "", apperr.New(apperr.NotFound, "办公画布组件还没有就绪。").WithDetail("reason=component")
	}
	return st.Path, nil
}

// ---------- 导出 ----------

// ExportOfficeCanvasPptxRequest 见契约 6.19.16.5。
type ExportOfficeCanvasPptxRequest struct {
	ConvID     string `json:"convId"`
	ArtifactID string `json:"artifactId"`
	DestPath   string `json:"destPath,omitempty"`
}

// ExportOfficeCanvasPptxResult 见契约 6.19.16.5。
type ExportOfficeCanvasPptxResult struct {
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
}

// ExportOfficeCanvasPptx 调用组件导出 PPTX；骨架阶段在组件未就绪 / 无 deck 时返回中文失败。
func (s *Service) ExportOfficeCanvasPptx(ctx context.Context, req ExportOfficeCanvasPptxRequest) (ExportOfficeCanvasPptxResult, error) {
	if s.cfg.Canvas == nil {
		return ExportOfficeCanvasPptxResult{}, apperr.New(apperr.Internal, "办公画布服务尚未初始化")
	}
	convID := strings.TrimSpace(req.ConvID)
	artID := strings.TrimSpace(req.ArtifactID)
	if convID == "" || artID == "" {
		return ExportOfficeCanvasPptxResult{}, apperr.New(apperr.InvalidArgument, "请指定对话与演示文稿").WithDetail("reason=ids")
	}
	st := s.cfg.Canvas.Status()
	if st.State != officecanvas.StateReady || st.Path == "" {
		return ExportOfficeCanvasPptxResult{}, apperr.New(apperr.Internal, officecanvas.MsgExportNotReady).WithDetail("reason=" + st.State)
	}
	art, ok := s.getArtifact(convID, artID)
	if !ok || len(art.Deck.Slides) == 0 {
		return ExportOfficeCanvasPptxResult{}, apperr.New(apperr.NotFound, officecanvas.MsgExportNoDeck).WithDetail("reason=artifact")
	}
	dest := strings.TrimSpace(req.DestPath)
	if dest == "" {
		dir := ""
		if s.cfg.ActualOutputDir != nil {
			dir = s.cfg.ActualOutputDir(ctx)
		}
		if dir == "" {
			return ExportOfficeCanvasPptxResult{}, apperr.New(apperr.InvalidArgument, "请指定保存位置")
		}
		name := "演示文稿.pptx"
		if t := strings.TrimSpace(art.Deck.Title); t != "" {
			name = sanitizeFileName(t) + ".pptx"
		}
		dest = filepath.Join(dir, name)
	}
	if !filepath.IsAbs(dest) {
		return ExportOfficeCanvasPptxResult{}, apperr.New(apperr.InvalidArgument, "保存路径必须是绝对路径").WithDetail(dest)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return ExportOfficeCanvasPptxResult{}, apperr.Wrap(apperr.IOError, "无法创建保存目录", err)
	}
	work := filepath.Join(s.cfg.TempRoot, convID, artID)
	if s.cfg.TempRoot == "" {
		work = filepath.Join(os.TempDir(), "ffmpegfree-office", convID, artID)
	}
	resp, err := officecanvas.RunExport(ctx, officecanvas.ExportOptions{
		ComponentRoot: st.Path,
		WorkDir:       work,
		Deck:          art.Deck,
		Logf:          s.cfg.Logf,
	})
	if err != nil {
		return ExportOfficeCanvasPptxResult{}, err
	}
	// 将组件写出的 pptx 挪到目标路径（同盘 Rename，跨盘复制）。
	if resp.PptxPath != dest {
		if err := moveFile(resp.PptxPath, dest); err != nil {
			return ExportOfficeCanvasPptxResult{}, apperr.Wrap(apperr.IOError, officecanvas.MsgExportStub, err)
		}
	}
	fi, err := os.Stat(dest)
	if err != nil {
		return ExportOfficeCanvasPptxResult{}, apperr.Wrap(apperr.IOError, officecanvas.MsgExportStub, err)
	}
	return ExportOfficeCanvasPptxResult{Path: dest, Bytes: fi.Size()}, nil
}

func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, in, 0o644); err != nil {
		return err
	}
	_ = os.Remove(src)
	return nil
}

func sanitizeFileName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "演示文稿"
	}
	repl := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", "*", "_", "?", "_",
		"\"", "_", "<", "_", ">", "_", "|", "_",
	)
	s = repl.Replace(s)
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}

package cat

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"FFmpegFree/internal/genoffice"
)

func (s *Service) pptManager() *genoffice.Manager {
	s.pptMu.Lock()
	defer s.pptMu.Unlock()
	if s.ppt == nil {
		s.ppt = genoffice.New(genoffice.Config{Dir: filepath.Join(s.cfg.DataRoot, "tools", "genoffice")})
	}
	return s.ppt
}
func (s *Service) ClosePptPreviews() {
	s.pptMu.Lock()
	m := s.ppt
	s.pptMu.Unlock()
	if m != nil {
		m.Close()
	}
}
func (s *Service) GetCatPptTool(retry bool) genoffice.ToolState { return s.pptManager().Tool(retry) }

func previewOfficeExt(ext string) bool {
	switch ext {
	case ".pptx", ".docx", ".xlsx", ".pdf", ".html", ".htm", ".md", ".markdown":
		return true
	default:
		return false
	}
}

// pptPath also authorizes a future file, so the preview can open before create.
// Every existing component is checked again for each read/render.
func (s *Service) pptPath(ctx context.Context, req ReadFileRequest) (string, string, error) {
	return s.scopedOfficePath(ctx, req, func(ext string) bool { return ext == ".pptx" })
}
func (s *Service) previewPath(ctx context.Context, req ReadFileRequest) (string, string, error) {
	return s.scopedOfficePath(ctx, req, previewOfficeExt)
}
func (s *Service) scopedOfficePath(ctx context.Context, req ReadFileRequest, allow func(string) bool) (string, string, error) {
	c, err := s.requireConv(ctx, req.ConvID)
	if err != nil {
		return "", "", err
	}
	root, err := s.conversationRoot(ctx, c)
	if err != nil {
		return "", "", err
	}
	rel := req.RelPath
	if filepath.IsAbs(rel) {
		rel, err = filepath.Rel(root, rel)
		if err != nil {
			return "", "", errFilePath()
		}
	}
	rel = strings.ReplaceAll(rel, `\`, "/")
	if strings.Contains(rel, ":") || strings.HasPrefix(rel, "/") || !allow(strings.ToLower(filepath.Ext(rel))) {
		return "", "", errFilePath()
	}
	cur := root
	for _, seg := range strings.Split(rel, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return "", "", errFilePath()
		}
		cur = filepath.Join(cur, seg)
		fi, e := os.Lstat(cur)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return "", "", e
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return "", "", errFilePath()
		}
		// EvalSymlinks catches Windows junctions as well as directory symlinks.
		actual, e := filepath.EvalSymlinks(cur)
		if e != nil {
			return "", "", e
		}
		base, e := filepath.EvalSymlinks(root)
		if e != nil || !lexicalInside(base, actual) {
			return "", "", errFilePath()
		}
	}
	if !lexicalInside(root, cur) {
		return "", "", errFilePath()
	}
	return cur, rel, nil
}
func (s *Service) pptRoot(ctx context.Context, req ReadFileRequest) (string, error) {
	c, err := s.requireConv(ctx, req.ConvID)
	if err != nil {
		return "", err
	}
	return s.conversationRoot(ctx, c)
}
func (s *Service) pptGuard(req ReadFileRequest, abs string) func() error {
	return s.officeGuard(req, abs, s.pptPath)
}
func (s *Service) previewGuard(req ReadFileRequest, abs string) func() error {
	return s.officeGuard(req, abs, s.previewPath)
}
func (s *Service) officeGuard(req ReadFileRequest, abs string, lookup func(context.Context, ReadFileRequest) (string, string, error)) func() error {
	return func() error {
		latest, _, e := lookup(context.Background(), req)
		if e != nil {
			return e
		}
		if latest != abs {
			return fmt.Errorf("工作区路径已改变")
		}
		return nil
	}
}
func (s *Service) StartCatPptPreview(ctx context.Context, req ReadFileRequest) (genoffice.Preview, error) {
	abs, _, err := s.previewPath(ctx, req)
	if err != nil {
		return genoffice.Preview{}, err
	}
	root, err := s.pptRoot(ctx, req)
	if err != nil {
		return genoffice.Preview{}, err
	}
	return s.pptManager().Start(ctx, abs, root, s.previewGuard(req, abs))
}
func (s *Service) ReadCatPptPreview(id string, version uint64) (genoffice.Preview, error) {
	return s.pptManager().Read(id, version)
}
func (s *Service) ReleaseCatPptPreview(id string) { s.pptManager().Release(id) }
func (s *Service) prepareCatPpt(ctx context.Context, req ReadFileRequest) error {
	_, rel, err := s.pptPath(ctx, req)
	if err != nil {
		return err
	}
	s.emit("cat:open-file", OpenFileEvent{req.ConvID, rel, filepath.Base(rel)})
	return nil
}

package cat

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/id"
	"FFmpegFree/internal/store"
)

func normalizeGroupAttachments(input []SendAttachment) ([]store.CatGroupAttachment, error) {
	files := make([]catagent.PromptFile, 0, len(input))
	for _, f := range input {
		// Match the formats supported by all native CLI image adapters.
		switch strings.ToLower(filepath.Ext(f.Path)) {
		case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		default:
			return nil, apperr.New(apperr.InvalidArgument, "请选择 PNG、JPEG、GIF 或 WebP 图片")
		}
		files = append(files, catagent.PromptFile{Name: filepath.Base(f.Path), Path: f.Path})
	}
	files, err := catagent.NormalizePromptFiles(files)
	if err != nil {
		return nil, err
	}
	var out []store.CatGroupAttachment
	for _, f := range files {
		out = append(out, store.CatGroupAttachment{Name: f.Name, Path: f.Path})
	}
	return out, nil
}

func groupPromptFiles(files []store.CatGroupAttachment) []catagent.PromptFile {
	out := make([]catagent.PromptFile, 0, len(files))
	for _, f := range files {
		out = append(out, catagent.PromptFile{Name: f.Name, Path: f.Path})
	}
	return out
}

// Keep sent images in app storage so deleting a source file or clearing the
// clipboard temp directory cannot break later task dependencies or history.
func (s *Service) persistGroupAttachments(groupID string, files []store.CatGroupAttachment) ([]store.CatGroupAttachment, error) {
	if len(files) == 0 {
		return nil, nil
	}
	dir := filepath.Join(s.cfg.DataRoot, "cat-group-media", groupID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, apperr.Wrap(apperr.IOError, "保存图片失败", err)
	}
	out := make([]store.CatGroupAttachment, 0, len(files))
	for _, f := range files {
		raw, err := os.ReadFile(f.Path)
		if err != nil {
			return nil, apperr.Wrap(apperr.InvalidArgument, "读取图片失败", err)
		}
		if len(raw) > 15<<20 {
			return nil, apperr.New(apperr.InvalidArgument, "这个文件太大")
		}
		ext := map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif", "image/webp": ".webp"}[http.DetectContentType(raw)]
		if ext == "" {
			return nil, apperr.New(apperr.InvalidArgument, "请选择有效的 PNG、JPEG、GIF 或 WebP 图片")
		}
		path := filepath.Join(dir, id.New()+ext)
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			return nil, apperr.Wrap(apperr.IOError, "保存图片失败", err)
		}
		name := strings.TrimSuffix(f.Name, filepath.Ext(f.Name)) + ext
		out = append(out, store.CatGroupAttachment{Name: name, Path: path})
	}
	return out, nil
}

// Pin the original images for every dependent task, retry and reassignment.
// A handoff can also carry a later triggering user message with more images.
func (s *Service) groupTurnFiles(ctx context.Context, groupID, chainID string, triggerSeq int64) ([]catagent.PromptFile, error) {
	var files []catagent.PromptFile
	origin, err := s.cfg.Store.GetCatGroupChainOrigin(ctx, groupID, chainID)
	if err == nil {
		files = append(files, groupPromptFiles(origin.Files)...)
	}
	if triggerSeq > 0 && triggerSeq != origin.Seq {
		if trigger, err := s.cfg.Store.GetCatGroupMessage(ctx, groupID, triggerSeq); err == nil {
			files = append(files, groupPromptFiles(trigger.Files)...)
		}
	}
	return catagent.NormalizePromptFiles(files)
}

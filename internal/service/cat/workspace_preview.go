package cat

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/service/doc"
)

type FileRevision struct {
	Revision string `json:"revision"`
	Size     int64  `json:"size"`
	Stable   bool   `json:"stable"`
}

func (s *Service) StatCatFile(ctx context.Context, req ReadFileRequest) (FileRevision, error) {
	abs, _, fi, err := s.resolveCatFile(ctx, req.ConvID, req.RelPath, msgFileReadFailed)
	if err != nil {
		return FileRevision{}, err
	}
	stable := true
	ext := strings.ToLower(filepath.Ext(abs))
	if ext == ".pptx" || ext == ".xlsx" || ext == ".docx" {
		z, e := zip.OpenReader(abs)
		stable = e == nil
		if z != nil {
			_ = z.Close()
		}
	}
	return FileRevision{Revision: fmt.Sprintf("%d:%d", fi.Size(), fi.ModTime().UnixNano()), Size: fi.Size(), Stable: stable}, nil
}
func (s *Service) PreviewCatDocument(ctx context.Context, req ReadFileRequest) (doc.DocPreview, error) {
	abs, _, _, err := s.resolveCatFile(ctx, req.ConvID, req.RelPath, msgFileReadFailed)
	if err != nil {
		return doc.DocPreview{}, err
	}
	if s.cfg.DocumentPreview == nil {
		return doc.DocPreview{}, apperr.New(apperr.Unsupported, "文档预览尚未就绪。")
	}
	return s.cfg.DocumentPreview(ctx, abs)
}
func (s *Service) ReleaseCatDocument(previewID string) error {
	if s.cfg.ReleaseDocumentPreview != nil {
		return s.cfg.ReleaseDocumentPreview(previewID)
	}
	return nil
}

type OpenFileEvent struct {
	ConvID  string `json:"convId"`
	RelPath string `json:"relPath"`
	Name    string `json:"name"`
}

func (s *Service) OpenCatWorkspaceFile(ctx context.Context, req ReadFileRequest) error {
	if filepath.IsAbs(req.RelPath) {
		c, e := s.requireConv(ctx, req.ConvID)
		if e != nil {
			return e
		}
		root, e := s.conversationRoot(ctx, c)
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(root, req.RelPath)
		if e != nil {
			return errFilePath()
		}
		req.RelPath = filepath.ToSlash(rel)
	}
	_, rel, fi, err := s.resolveCatFile(ctx, req.ConvID, req.RelPath, msgFileReadFailed)
	if err != nil {
		return err
	}
	s.emit("cat:open-file", OpenFileEvent{req.ConvID, rel, fi.Name()})
	return nil
}

// Each turn gets a private, short-lived loopback endpoint. No CORS, shell
// execution or external file access. The capability is never stored globally.
func (s *Service) startOpenBridge(ctx context.Context, t *turnState, root string, opts *catagent.TurnOptions) func() {
	if !s.cfg.FileBridge {
		return func() {}
	}
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return func() {}
	}
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		_ = l.Close()
		return func() {}
	}
	secret := hex.EncodeToString(b)
	mux := http.NewServeMux()
	mux.HandleFunc("/open-file", func(w http.ResponseWriter, r *http.Request) {
		if ctx.Err() != nil {
			http.Error(w, "Expired", http.StatusGone)
			return
		}
		if r.Method != "POST" || r.Header.Get("Origin") != "" || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			http.Error(w, "Unsupported", http.StatusForbidden)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+secret)) != 1 {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		var req struct {
			Path string `json:"path"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		if json.NewDecoder(r.Body).Decode(&req) != nil {
			http.Error(w, "Invalid request", 400)
			return
		}
		rel := req.Path
		if filepath.IsAbs(rel) {
			var pathErr error
			rel, pathErr = filepath.Rel(root, rel)
			if pathErr != nil {
				http.Error(w, "Invalid path", 400)
				return
			}
		}
		if err := s.OpenCatWorkspaceFile(r.Context(), ReadFileRequest{ConvID: t.convID, RelPath: filepath.ToSlash(rel)}); err != nil {
			http.Error(w, "File unavailable or outside workspace", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"opened":true}`))
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second, IdleTimeout: 3 * time.Second}
	go func() { _ = server.Serve(l) }()
	endpoint := "http://" + l.Addr().String() + "/open-file"
	opts.Rules += fmt.Sprintf("\n\nffc app file preview: When the user asks to open/show a workspace file or you produce an Office artifact, use this app's preview instead of launching a system application. POST JSON {\"path\":\"relative/path.pptx\"} to %s with Authorization: Bearer %s and Content-Type: application/json. Only current workspace files are allowed. PowerShell example: Invoke-RestMethod -Method Post -Uri '%s' -Headers @{ Authorization = 'Bearer %s' } -ContentType 'application/json' -Body (@{ path = 'relative/path.pptx' } | ConvertTo-Json). The preview follows complete saved versions; an incomplete Office ZIP will wait. Do not include the capability token in your reply or save it in project files.\n", endpoint, secret, endpoint, secret)
	return func() { _ = server.Close() }
}

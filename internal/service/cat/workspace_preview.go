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
	mux.HandleFunc("/office", func(w http.ResponseWriter, r *http.Request) {
		if ctx.Err() != nil || r.Method != "POST" || r.Header.Get("Origin") != "" || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+secret)) != 1 {
			http.Error(w, "Forbidden", 403)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
		var args map[string]any
		if json.NewDecoder(r.Body).Decode(&args) != nil {
			http.Error(w, "Invalid request", 400)
			return
		}
		result, err := s.officeRequest(r.Context(), t.convID, args)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if err != nil {
			w.WriteHeader(409)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": err.Error()}})
			return
		}
		_, _ = w.Write(result)
	})
	// The same turn-scoped capability authorizes tool discovery and early preview.
	mux.HandleFunc("/ppt", func(w http.ResponseWriter, r *http.Request) {
		if ctx.Err() != nil || r.Method != "POST" || r.Header.Get("Origin") != "" || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+secret)) != 1 {
			http.Error(w, "Forbidden", 403)
			return
		}
		var req struct {
			Action string `json:"action"`
			Path   string `json:"path"`
			Retry  bool   `json:"retry"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		if json.NewDecoder(r.Body).Decode(&req) != nil {
			http.Error(w, "Invalid request", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch req.Action {
		case "tool":
			_ = json.NewEncoder(w).Encode(s.GetCatPptTool(req.Retry))
		case "prepare":
			if err := s.prepareCatPpt(r.Context(), ReadFileRequest{ConvID: t.convID, RelPath: req.Path}); err != nil {
				http.Error(w, "PPT path unavailable or outside workspace", 400)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]bool{"opened": true})
		default:
			http.Error(w, "Unknown action", 400)
		}
	})
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
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 60 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 3 * time.Second}
	go func() { _ = server.Serve(l) }()
	endpoint := "http://" + l.Addr().String() + "/open-file"
	if skill, err := s.ensurePptSkill(); err == nil {
		if opts.ToolEnv == nil {
			opts.ToolEnv = map[string]string{}
		}
		opts.ToolEnv["FFC_OFFICE_ENDPOINT"] = "http://" + l.Addr().String() + "/office"
		opts.ToolEnv["FFC_OFFICE_TOKEN"] = secret
		opts.Rules += fmt.Sprintf("\n<ffc_internal_file_preview>For PowerPoint generation/editing use ffc's GenOffice skill at %q. Read SKILL.md first. Wrapper: %q. Run node <wrapper> <action> --path <workspace-relative.pptx> [--input <JSON file>] [--revision N]. It reads the private FFC_OFFICE_ENDPOINT and FFC_OFFICE_TOKEN environment; do not print or persist them. If a later note says the deck is already created and open, generate into that path. Otherwise open with --create BEFORE generating. Submit one page per call and wait for its revision before the next page; the open canvas draws that page's elements one at a time before the call returns. Do not build the whole deck elsewhere and save it once. Save after checking the result. Native shapes/text remain editable. Office tools operate inside ffc.</ffc_internal_file_preview>\n", skill, filepath.Join(filepath.Dir(skill), "scripts", "office.cjs"))
		if len(opts.Messages) > 0 {
			s.pinLivePpt(ctx, t.convID, opts.Messages[0].Content, opts)
		}
	}
	opts.Rules += fmt.Sprintf("\n\n<ffc_internal_file_preview>\nffc app file preview: When the user asks to open/show a workspace file or you produce an Office artifact, use this app's preview instead of launching a system application. POST JSON {\"path\":\"relative/path.pptx\"} to %s with Authorization: Bearer %s and Content-Type: application/json. Only current workspace files are allowed. PowerShell example: Invoke-RestMethod -Method Post -Uri '%s' -Headers @{ Authorization = 'Bearer %s' } -ContentType 'application/json' -Body (@{ path = 'relative/path.pptx' } | ConvertTo-Json). For a PowerPoint deck in this turn, do not wait for a finished file: each office page is painted when its call returns. Other Office files still follow complete saved versions; an incomplete Office ZIP will wait. Do not include the capability token in your reply or save it in project files.\n</ffc_internal_file_preview>\n", endpoint, secret, endpoint, secret)
	return func() { _ = server.Close() }
}

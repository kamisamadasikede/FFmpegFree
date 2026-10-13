package cat

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"FFmpegFree/internal/apperr"
)

const (
	maxSearchHits   = 80
	maxSearchVisit  = 4000
	maxSearchBytes  = 8 << 20
	maxSearchFile   = 256 * 1024
	maxSearchDepth  = 24
	maxSearchQuery  = 200
	maxSnippetRunes = 80
	searchTimeout   = 8 * time.Second
)

// SearchFilesRequest 是 SearchCatFiles 的入参。Kind 为 name 或 content。
type SearchFilesRequest struct {
	ConvID string `json:"convId"`
	Query  string `json:"query"`
	Kind   string `json:"kind"`
}

// SearchHit 是一条查找结果。内容查找带上第一次命中的行。
type SearchHit struct {
	Name    string `json:"name"`
	RelPath string `json:"relPath"`
	IsDir   bool   `json:"isDir"`
	Line    int    `json:"line,omitempty"`
	Text    string `json:"text,omitempty"`
}

// SearchFilesResult 是查找结果。超出上限时 Truncated 为 true。
type SearchFilesResult struct {
	Hits      []SearchHit `json:"hits"`
	Truncated bool        `json:"truncated"`
}

// SearchCatFiles 在对话根下按文件名或文件内容查找。不跟随符号链接，跳过依赖目录。
func (s *Service) SearchCatFiles(ctx context.Context, req SearchFilesRequest) (SearchFilesResult, error) {
	root, err := s.convRoot(ctx, req.ConvID)
	if err != nil {
		return SearchFilesResult{}, err
	}
	q := strings.TrimSpace(req.Query)
	if q == "" {
		return SearchFilesResult{Hits: []SearchHit{}}, nil
	}
	if utf8.RuneCountInString(q) > maxSearchQuery {
		return SearchFilesResult{}, apperrNewQuery()
	}
	kind := req.Kind
	if kind == "" {
		kind = "name"
	}
	if kind != "name" && kind != "content" {
		return SearchFilesResult{}, apperrNewKind()
	}
	cctx, cancel := context.WithTimeout(ctx, searchTimeout)
	defer cancel()
	needle := strings.ToLower(q)
	hits := make([]SearchHit, 0)
	visited := 0
	readTotal := 0
	truncated := false
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, werr error) error {
		if cctx.Err() != nil {
			truncated = true
			return fs.SkipAll
		}
		if path == root {
			if werr != nil {
				return werr
			}
			return nil
		}
		if werr != nil {
			return nil
		}
		name := d.Name()
		if hiddenFileName(name) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || !lexicalInside(root, path) {
			return nil
		}
		rel = filepath.ToSlash(rel)
		depth := strings.Count(rel, "/") + 1
		if d.IsDir() && depth > maxSearchDepth {
			return fs.SkipDir
		}
		visited++
		if visited > maxSearchVisit {
			truncated = true
			return fs.SkipAll
		}
		if kind == "name" {
			if strings.Contains(strings.ToLower(name), needle) {
				hits = append(hits, SearchHit{Name: name, RelPath: rel, IsDir: d.IsDir()})
				if len(hits) >= maxSearchHits {
					truncated = true
					return fs.SkipAll
				}
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil || !info.Mode().IsRegular() || info.Size() > maxSearchFile || info.Size() == 0 {
			return nil
		}
		if readTotal+int(info.Size()) > maxSearchBytes {
			truncated = true
			return fs.SkipAll
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		readTotal += len(b)
		if indexByte(b, 0) >= 0 {
			return nil
		}
		if line, text, ok := firstMatch(string(b), needle); ok {
			hits = append(hits, SearchHit{Name: name, RelPath: rel, Line: line, Text: text})
			if len(hits) >= maxSearchHits {
				truncated = true
				return fs.SkipAll
			}
		}
		return nil
	})
	if walkErr != nil && walkErr != fs.SkipAll {
		return SearchFilesResult{}, apperr.Wrap(apperr.IOError, msgFileListFailed, walkErr)
	}
	if hits == nil {
		hits = []SearchHit{}
	}
	return SearchFilesResult{Hits: hits, Truncated: truncated}, nil
}

func apperrNewQuery() error {
	return apperr.New(apperr.InvalidArgument, "查找内容太长。")
}

func apperrNewKind() error {
	return apperr.New(apperr.InvalidArgument, "查找方式不对。")
}

func firstMatch(body, needle string) (int, string, bool) {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if strings.Contains(strings.ToLower(line), needle) {
			text := strings.TrimSpace(line)
			r := []rune(text)
			if len(r) > maxSnippetRunes {
				text = string(r[:maxSnippetRunes]) + "…"
			}
			return i + 1, text, true
		}
	}
	return 0, "", false
}

func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}

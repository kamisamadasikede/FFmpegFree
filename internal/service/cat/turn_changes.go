package cat

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
)

const snapshotFileLimit = 3000
const snapshotTextLimit = 256 * 1024
const snapshotBudget = 32 * 1024 * 1024

type workspaceFile struct {
	hash   string
	text   string
	binary bool
}
type workspaceSnapshot struct {
	files map[string]workspaceFile
	cut   bool
}
type TurnFileChange struct {
	Path      string `json:"path"`
	Status    string `json:"status"`
	Added     int    `json:"added"`
	Deleted   int    `json:"deleted"`
	Binary    bool   `json:"binary"`
	Diff      string `json:"diff,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}
type TurnChanges struct {
	MessageID  string           `json:"messageId"`
	FinishedAt int64            `json:"finishedAt"`
	ConvID     string           `json:"convId"`
	TurnID     string           `json:"turnId"`
	Files      []TurnFileChange `json:"files"`
	FileCount  int              `json:"fileCount"`
	Added      int              `json:"added"`
	Deleted    int              `json:"deleted"`
	Finished   bool             `json:"finished"`
	Truncated  bool             `json:"truncated"`
	CreatedAt  int64            `json:"createdAt"`
}

func snapshotWorkspace(ctx context.Context, root string) workspaceSnapshot {
	out := workspaceSnapshot{files: map[string]workspaceFile{}}
	if root == "" {
		out.cut = true
		return out
	}
	var names []string
	b, _, e := runGit(ctx, 3*time.Second, root, "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	if e == nil {
		for _, n := range strings.Split(string(b), "\x00") {
			if n != "" {
				names = append(names, n)
			}
		}
	} else {
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
			if ctx.Err() != nil {
				return fs.SkipAll
			}
			if e != nil {
				out.cut = true
				return nil
			}
			if d.Type()&os.ModeSymlink != 0 {
				return nil
			}
			if d.IsDir() {
				if p != root {
					switch d.Name() {
					case ".git", "node_modules", ".gocache", ".venv", "vendor", "dist":
						return filepath.SkipDir
					}
				}
				return nil
			}
			rel, e := filepath.Rel(root, p)
			if e == nil {
				names = append(names, filepath.ToSlash(rel))
			}
			if len(names) > snapshotFileLimit {
				out.cut = true
				return fs.SkipAll
			}
			return nil
		})
	}
	sort.Strings(names)
	used := 0
	for _, name := range names {
		if ctx.Err() != nil {
			out.cut = true
			break
		}
		if len(out.files) >= snapshotFileLimit {
			out.cut = true
			break
		}
		abs, rel, fi, err := resolveCatFile(root, name, msgFileReadFailed)
		if err != nil {
			continue
		}
		f := workspaceFile{binary: true, hash: fmt.Sprintf("large:%d:%d", fi.Size(), fi.ModTime().UnixNano())}
		if fi.Size() <= snapshotTextLimit && used+int(fi.Size()) <= snapshotBudget {
			b, err := os.ReadFile(abs)
			if err != nil {
				out.cut = true
				continue
			}
			used += len(b)
			h := sha256.Sum256(b)
			f.hash = hex.EncodeToString(h[:])
			f.binary = !utf8.Valid(b) || bytes.IndexByte(b, 0) >= 0
			if !f.binary {
				f.text = string(b)
			}
		} else if fi.Size() <= snapshotTextLimit {
			out.cut = true
		}
		out.files[rel] = f
	}
	return out
}
func lineCount(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}
func changeDiff(ctx context.Context, path string, a, b workspaceFile, status string) TurnFileChange {
	c := TurnFileChange{Path: path, Status: status, Binary: a.binary || b.binary}
	if c.Binary {
		return c
	}
	if status == "added" {
		c.Added = lineCount(b.text)
		c.Diff = "--- /dev/null\n+++ b/" + path + "\n@@ -0,0 +1," + fmt.Sprint(c.Added) + " @@\n" + prefixLines(b.text, "+")
	} else if status == "deleted" {
		c.Deleted = lineCount(a.text)
		c.Diff = "--- a/" + path + "\n+++ /dev/null\n@@ -1," + fmt.Sprint(c.Deleted) + " +0,0 @@\n" + prefixLines(a.text, "-")
	} else {
		dir, e := os.MkdirTemp("", "ffc-turn-diff-")
		if e != nil {
			c.Truncated = true
			return c
		}
		defer os.RemoveAll(dir)
		p1, p2 := filepath.Join(dir, "before"), filepath.Join(dir, "after")
		if os.WriteFile(p1, []byte(a.text), 0600) != nil || os.WriteFile(p2, []byte(b.text), 0600) != nil {
			c.Truncated = true
			return c
		}
		patch, _, err := runGit(ctx, 3*time.Second, dir, "diff", "--no-index", "--no-color", "--no-ext-diff", "--no-textconv", "--unified=3", "--", p1, p2)
		if len(patch) == 0 && err != nil {
			c.Truncated = true
			return c
		}
		lines := strings.Split(string(patch), "\n")
		var body strings.Builder
		for _, l := range lines {
			if strings.HasPrefix(l, "diff --git") || strings.HasPrefix(l, "index ") {
				continue
			}
			if strings.HasPrefix(l, "--- ") {
				l = "--- a/" + path
			} else if strings.HasPrefix(l, "+++ ") {
				l = "+++ b/" + path
			} else if strings.HasPrefix(l, "+") {
				c.Added++
			} else if strings.HasPrefix(l, "-") {
				c.Deleted++
			}
			body.WriteString(l + "\n")
		}
		c.Diff = body.String()
	}
	if len(c.Diff) > 16384 {
		c.Diff = c.Diff[:16384]
		for !utf8.ValidString(c.Diff) {
			c.Diff = c.Diff[:len(c.Diff)-1]
		}
		c.Truncated = true
	}
	return c
}
func prefixLines(text, prefix string) string {
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	if text == "" {
		return ""
	}
	return prefix + strings.Join(lines, "\n"+prefix) + "\n"
}
func compareSnapshot(ctx context.Context, base, now workspaceSnapshot, t *turnState, finished bool) TurnChanges {
	t.mu.Lock()
	messageID, finishedAt := t.msgID, t.finishedAt
	t.mu.Unlock()
	out := TurnChanges{ConvID: t.convID, TurnID: t.turnID, CreatedAt: t.startedAt, Finished: finished, Truncated: base.cut || now.cut, Files: []TurnFileChange{}}
	out.MessageID, out.FinishedAt = messageID, finishedAt
	paths := map[string]bool{}
	for p := range base.files {
		paths[p] = true
	}
	for p := range now.files {
		paths[p] = true
	}
	var names []string
	for p := range paths {
		names = append(names, p)
	}
	sort.Strings(names)
	for _, p := range names {
		a, hasA := base.files[p]
		b, hasB := now.files[p]
		if a.hash == b.hash || (now.cut && !hasB) || (base.cut && !hasA) {
			continue
		}
		out.FileCount++
		if len(out.Files) >= 100 {
			out.Truncated = true
			continue
		}
		status := "modified"
		if !hasA {
			status = "added"
		} else if !hasB {
			status = "deleted"
		}
		c := changeDiff(ctx, p, a, b, status)
		out.Files = append(out.Files, c)
		out.Added += c.Added
		out.Deleted += c.Deleted
		if c.Truncated {
			out.Truncated = true
		}
	}
	return out
}
func (s *Service) GetCatTurnChanges(ctx context.Context, convID string) ([]TurnChanges, error) {
	if _, err := s.requireConv(ctx, convID); err != nil {
		return nil, err
	}
	records, err := s.cfg.Store.ListCatTurnRecords(ctx, convID)
	if err != nil {
		return nil, err
	}
	out := []TurnChanges{}
	for _, raw := range records {
		var change TurnChanges
		if err = json.Unmarshal([]byte(raw), &change); err != nil {
			return nil, err
		}
		out = append(out, change)
	}
	return out, nil
}
func (s *Service) GetCatTurnChangeDetail(ctx context.Context, convID, turnID string) (TurnChanges, error) {
	if _, err := s.requireConv(ctx, convID); err != nil {
		return TurnChanges{}, err
	}
	raw, err := s.cfg.Store.GetCatTurnRecord(ctx, convID, turnID)
	if err != nil {
		return TurnChanges{}, apperr.New(apperr.NotFound, "找不到这轮任务的改动记录。")
	}
	var out TurnChanges
	err = json.Unmarshal([]byte(raw), &out)
	return out, err
}
func (s *Service) saveTurnChanges(out TurnChanges) {
	s.workflowMu.Lock()
	defer s.workflowMu.Unlock()
	ctx := context.Background()
	if _, err := s.requireConv(ctx, out.ConvID); err != nil {
		return
	}
	detail, err := json.Marshal(out)
	if err != nil {
		return
	}
	compact := out
	compact.Files = []TurnFileChange{}
	summary, err := json.Marshal(compact)
	if err != nil {
		return
	}
	if err = s.cfg.Store.SaveCatTurnRecord(ctx, out.ConvID, out.TurnID, out.CreatedAt, string(summary), string(detail)); err != nil {
		s.cfg.Logf("cat changes save: %v", err)
	}
}
func (s *Service) watchTurnWorkspace(ctx context.Context, t *turnState, root string, opts *catagent.TurnOptions) func() {
	closeBridge := s.startOpenBridge(ctx, t, root, opts)
	base := snapshotWorkspace(ctx, root)
	autoOpened := false
	showArtifact := func(out TurnChanges) {
		if autoOpened || !s.cfg.FileBridge {
			return
		}
		for _, file := range out.Files {
			if file.Status != "added" {
				continue
			}
			switch strings.ToLower(filepath.Ext(file.Path)) {
			case ".pptx", ".docx", ".xlsx", ".pdf":
			default:
				continue
			}
			r, e := s.StatCatFile(context.Background(), ReadFileRequest{ConvID: t.convID, RelPath: file.Path})
			if e == nil && r.Stable && s.OpenCatWorkspaceFile(context.Background(), ReadFileRequest{ConvID: t.convID, RelPath: file.Path}) == nil {
				autoOpened = true
				break
			}
		}
	}
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		timer := time.NewTicker(3 * time.Second)
		defer timer.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-timer.C:
				now := snapshotWorkspace(ctx, root)
				out := compareSnapshot(ctx, base, now, t, false)
				s.emit("cat:turn-changes", out)
				showArtifact(out)
			}
		}
	}()
	return func() {
		close(done)
		wg.Wait()
		closeBridge()
		cctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		out := compareSnapshot(cctx, base, snapshotWorkspace(cctx, root), t, true)
		s.saveTurnChanges(out)
		s.emit("cat:turn-changes", out)
		showArtifact(out)
	}
}

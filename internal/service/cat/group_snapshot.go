package cat

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// v2 supplements the tracked Git tree with exact, unfiltered untracked blobs.
// Excluded paths existed before the turn but were not backed up (e.g. ignored
// directories). Absence from the tracked tree alone never proves a new file.
type revertSnapshot struct {
	Version   int                   `json:"version"`
	Tracked   string                `json:"tracked"`
	Prefix    string                `json:"prefix"`
	Untracked map[string]revertBlob `json:"untracked"`
	Excluded  []string              `json:"excluded,omitempty"`
}

type revertBlob struct {
	ID   string      `json:"id"`
	Mode os.FileMode `json:"mode"`
}

func createGitSnapshot(ctx context.Context, root string) string {
	if root == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, _, err := runGit(ctx, 15*time.Second, root, "stash", "create")
	if err != nil {
		return ""
	}
	snap := revertSnapshot{Version: 2, Tracked: strings.TrimSpace(string(out)), Untracked: map[string]revertBlob{}, Excluded: []string{".git/"}}
	if snap.Tracked == "" {
		out, _, err = runGit(ctx, 5*time.Second, root, "rev-parse", "--verify", "HEAD")
		if err != nil {
			return ""
		}
		snap.Tracked = strings.TrimSpace(string(out))
	}
	out, _, err = runGit(ctx, 5*time.Second, root, "rev-parse", "--show-prefix")
	if err != nil {
		return ""
	}
	snap.Prefix = strings.TrimSpace(string(out))
	out, _, err = runGit(ctx, 5*time.Second, root, "ls-files", "--others", "--ignored", "--exclude-standard", "--directory", "-z")
	if err != nil {
		return ""
	}
	for _, rel := range strings.Split(string(out), "\x00") {
		if rel != "" {
			snap.Excluded = append(snap.Excluded, rel)
		}
	}
	out, _, err = runGit(ctx, 5*time.Second, root, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return ""
	}
	for _, rel := range strings.Split(string(out), "\x00") {
		if rel == "" {
			continue
		}
		abs, err := safeRevertPath(root, rel)
		if err != nil {
			return ""
		}
		info, err := os.Lstat(abs)
		if err != nil {
			return ""
		}
		if !info.Mode().IsRegular() {
			snap.Excluded = append(snap.Excluded, rel)
			continue
		}
		blob, _, err := runGit(ctx, 5*time.Second, root, "hash-object", "-w", "--no-filters", "--", rel)
		if err != nil {
			return ""
		}
		snap.Untracked[rel] = revertBlob{ID: strings.TrimSpace(string(blob)), Mode: info.Mode().Perm()}
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		return ""
	}
	return string(raw)
}

// Reject traversal, Git metadata, symlinks and junctions before any hash/write.
// Missing parents are allowed, so a deleted file can be restored safely.
func safeRevertPath(root, rel string) (string, error) {
	rel = normalizeRel(rel)
	if rel == "" || filepath.IsAbs(rel) || strings.ContainsAny(rel, ":\x00") {
		return "", errFilePath()
	}
	if filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel))) != rel {
		return "", errFilePath()
	}
	for _, part := range strings.Split(rel, "/") {
		if part == ".." || strings.EqualFold(part, ".git") || strings.TrimRight(part, ". ") != part {
			return "", errFilePath()
		}
		// DOS short names can alias a different original path (including .git).
		if runtime.GOOS == "windows" && strings.Contains(part, "~") {
			return "", errFilePath()
		}
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	target := filepath.Join(root, filepath.FromSlash(rel))
	if !lexicalInside(root, target) || filepath.Clean(target) == filepath.Clean(root) {
		return "", errFilePath()
	}
	cur := root
	for _, part := range strings.Split(filepath.FromSlash(rel), string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", errFilePath()
		}
		physical, err := filepath.EvalSymlinks(cur)
		if err != nil || !lexicalInside(root, physical) {
			return "", errFilePath()
		}
	}
	return target, nil
}

func parseRevertSnapshot(raw string) (revertSnapshot, error) {
	if strings.HasPrefix(raw, "{") {
		var snap revertSnapshot
		if json.Unmarshal([]byte(raw), &snap) != nil || snap.Version != 2 || snap.Tracked == "" {
			return snap, errors.New("轮前快照无效，不能自动撤销")
		}
		return snap, nil
	}
	if raw == "" || strings.HasPrefix(raw, "-") {
		return revertSnapshot{}, errors.New("轮前快照无效，不能自动撤销")
	}
	// Legacy snapshots can restore known tracked files, but cannot prove absence.
	return revertSnapshot{Tracked: raw}, nil
}

func restoreFromSnapshot(ctx context.Context, root, raw, rel string) error {
	abs, err := safeRevertPath(root, rel)
	if err != nil {
		return err
	}
	snap, err := parseRevertSnapshot(raw)
	if err != nil {
		return err
	}
	// Validate the object separately: a missing/corrupt object is not a missing file.
	if _, _, err = runGit(ctx, 5*time.Second, root, "cat-file", "-e", snap.Tracked+"^{tree}"); err != nil {
		return errors.New("轮前快照不可用，不能自动撤销")
	}
	if b, ok := snap.untrackedBlob(rel); ok {
		content, _, err := runGit(ctx, 5*time.Second, root, "cat-file", "blob", b.ID)
		if err != nil {
			return errors.New("原文件备份不可用，不能自动撤销")
		}
		if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(abs, content, b.Mode); err != nil {
			return err
		}
		return os.Chmod(abs, b.Mode)
	}
	prefix := snap.Prefix
	if snap.Version == 0 {
		out, _, err := runGit(ctx, 5*time.Second, root, "rev-parse", "--show-prefix")
		if err != nil {
			return err
		}
		prefix = strings.TrimSpace(string(out))
	}
	out, _, err := runGit(ctx, 5*time.Second, root, "ls-tree", "-z", "--full-tree", snap.Tracked, "--", ":(literal)"+prefix+rel)
	if err != nil {
		return errors.New("读取轮前快照失败，不能自动撤销")
	}
	if len(out) > 0 {
		// Only regular blobs: never recursively restore a tree or a symlink.
		if !strings.HasPrefix(string(out), "100644 blob ") && !strings.HasPrefix(string(out), "100755 blob ") {
			return errFilePath()
		}
		_, _, err = runGit(ctx, 10*time.Second, root, "--literal-pathspecs", "restore", "--source="+snap.Tracked, "--worktree", "--", rel)
		return err
	}
	if snap.Version != 2 {
		return errors.New("旧快照没有记录文件是否存在，不能自动删除")
	}
	// ls-tree does not support icase pathspecs. Check the tree inventory before
	// authorizing deletion through a case alias on a case-insensitive worktree.
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		names, _, err := runGit(ctx, 5*time.Second, root, "ls-tree", "-r", "-z", "--full-tree", "--name-only", snap.Tracked)
		if err != nil {
			return errors.New("读取轮前文件列表失败，不能自动撤销")
		}
		for _, name := range strings.Split(string(names), "\x00") {
			if strings.EqualFold(name, prefix+rel) {
				return errors.New("文件名大小写与快照不一致，不能自动撤销")
			}
		}
	}
	compareRel := rel
	for _, excluded := range snap.Excluded {
		if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
			compareRel, excluded = strings.ToLower(rel), strings.ToLower(excluded)
		}
		if compareRel == strings.TrimSuffix(excluded, "/") || (strings.HasSuffix(excluded, "/") && strings.HasPrefix(compareRel, excluded)) {
			return errors.New("原文件未备份，不能自动撤销")
		}
	}
	if info, err := os.Lstat(abs); err == nil && !info.Mode().IsRegular() {
		return errFilePath()
	}
	if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (snap revertSnapshot) untrackedBlob(rel string) (revertBlob, bool) {
	if b, ok := snap.Untracked[rel]; ok {
		return b, true
	}
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		for path, b := range snap.Untracked {
			if strings.EqualFold(path, rel) {
				return b, true
			}
		}
	}
	return revertBlob{}, false
}

package cat

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/store"
)

func createGitSnapshot(ctx context.Context, root string) string {
	if root == "" {
		return ""
	}
	// git stash create → dangling commit object of working tree; does not touch index/HEAD.
	out, _, err := runGit(ctx, 15*time.Second, root, "stash", "create")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (s *Service) tagReportFiles(ctx context.Context, g store.CatGroup, m store.CatGroupMember, claimed []string, changes TurnChanges, members []store.CatGroupMember) []store.CatGroupFileTag {
	claimedSet := map[string]bool{}
	for _, p := range claimed {
		claimedSet[normalizeRel(p)] = true
	}
	dirs := m.Charter.Dirs
	out := []store.CatGroupFileTag{}
	seen := map[string]bool{}
	for _, f := range changes.Files {
		rel := normalizeRel(f.Path)
		if seen[rel] {
			continue
		}
		seen[rel] = true
		tag := ""
		if len(dirs) > 0 && !pathInDirs(rel, dirs) {
			if claimedSet[rel] {
				tag = "overreach"
			} else {
				tag = "unknown"
			}
		} else if !claimedSet[rel] && len(members) > 1 {
			// unclaimed in snapshot diff while others may be running
			runningOthers := false
			for _, o := range members {
				if o.ID == m.ID {
					continue
				}
				conv := groupConvID(g.ID, o.ID)
				s.mu.Lock()
				_, busy := s.turns[conv]
				s.mu.Unlock()
				if busy {
					runningOthers = true
					break
				}
			}
			if runningOthers {
				tag = "unknown"
			}
		}
		out = append(out, store.CatGroupFileTag{Path: rel, Tag: tag, Added: f.Added, Deleted: f.Deleted})
	}
	// also list claimed overreach not in changes
	for _, p := range claimed {
		rel := normalizeRel(p)
		if seen[rel] {
			continue
		}
		if len(dirs) > 0 && !pathInDirs(rel, dirs) {
			out = append(out, store.CatGroupFileTag{Path: rel, Tag: "overreach"})
			seen[rel] = true
		}
	}
	return out
}

func normalizeRel(p string) string {
	p = strings.TrimSpace(p)
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.TrimPrefix(p, "./")
	return p
}

func pathInDirs(rel string, dirs []string) bool {
	rel = normalizeRel(rel)
	for _, d := range dirs {
		d = normalizeRel(d)
		if d == "" {
			continue
		}
		if !strings.HasSuffix(d, "/") {
			d += "/"
		}
		if rel == strings.TrimSuffix(d, "/") || strings.HasPrefix(rel, d) {
			return true
		}
	}
	return false
}

func (s *Service) RevertCatOverreach(ctx context.Context, req RevertOverreachRequest) (RevertOverreachResult, error) {
	m, err := s.requireMember(ctx, req.MemberID)
	if err != nil {
		return RevertOverreachResult{}, err
	}
	meta, err := s.cfg.Store.GetCatGroupTurnMeta(ctx, req.TurnID)
	if err != nil {
		return RevertOverreachResult{}, apperr.New(apperr.NotFound, "找不到这一轮的改动记录")
	}
	if meta.MemberID != m.ID {
		return RevertOverreachResult{}, apperr.New(apperr.InvalidArgument, "回合与成员不匹配")
	}
	g, err := s.requireGroup(ctx, m.GroupID)
	if err != nil {
		return RevertOverreachResult{}, err
	}
	projectRoot := ""
	if row, err := s.cfg.Store.GetCatProject(ctx, g.ProjectID); err == nil {
		projectRoot = row.Path
	}
	if projectRoot == "" || meta.SnapshotBefore == "" {
		return RevertOverreachResult{}, apperr.New(apperr.InvalidArgument, "无法撤销：没有轮前快照")
	}
	claimed := map[string]bool{}
	for _, p := range meta.ClaimedFiles {
		claimed[normalizeRel(p)] = true
	}
	var reverted []string
	var skipped []RevertSkippedFile
	for _, f := range req.Files {
		rel := normalizeRel(f)
		if !claimed[rel] {
			skipped = append(skipped, RevertSkippedFile{Path: rel, Reason: "来源不明，不能撤销"})
			continue
		}
		abs := filepath.Join(projectRoot, filepath.FromSlash(rel))
		endHash := fileSHA256(abs)
		// end-of-round hash: prefer stored; else compare to snapshot_after blob
		wantEnd := ""
		if meta.EndHashes != nil {
			wantEnd = meta.EndHashes[rel]
		}
		if wantEnd == "" && meta.SnapshotAfter != "" {
			wantEnd = blobHashFromSnapshot(ctx, projectRoot, meta.SnapshotAfter, rel)
		}
		if wantEnd != "" && endHash != "" && endHash != wantEnd {
			skipped = append(skipped, RevertSkippedFile{Path: rel, Reason: "之后又被改过，不能自动撤销，请手动处理。"})
			continue
		}
		if err := restoreFromSnapshot(ctx, projectRoot, meta.SnapshotBefore, rel); err != nil {
			skipped = append(skipped, RevertSkippedFile{Path: rel, Reason: err.Error()})
			continue
		}
		reverted = append(reverted, rel)
	}
	res := RevertOverreachResult{Reverted: reverted, Skipped: skipped}
	if len(skipped) > 0 && len(reverted) == 0 {
		return res, apperr.New(apperr.InvalidArgument, "没有文件被撤销").WithDetail("reason=revert_conflict")
	}
	if len(skipped) > 0 {
		return res, apperr.New(apperr.InvalidArgument, "部分文件未能撤销").WithDetail("reason=revert_conflict")
	}
	return res, nil
}

func fileSHA256(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func blobHashFromSnapshot(ctx context.Context, root, snap, rel string) string {
	// git rev-parse snap:path
	out, _, err := runGit(ctx, 5*time.Second, root, "rev-parse", snap+":"+rel)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func restoreFromSnapshot(ctx context.Context, root, snap, rel string) error {
	// If file existed in snapshot, checkout; else remove.
	_, _, err := runGit(ctx, 5*time.Second, root, "cat-file", "-e", snap+":"+rel)
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err != nil {
		if removeErr := os.Remove(abs); removeErr != nil && !os.IsNotExist(removeErr) {
			return removeErr
		}
		return nil
	}
	_, _, err = runGit(ctx, 10*time.Second, root, "checkout", snap, "--", rel)
	return err
}

// persistEndHashes stores per-file hashes at end of round for revert checks.
func (s *Service) persistEndHashes(ctx context.Context, turnID, root string, files []store.CatGroupFileTag) {
	meta, err := s.cfg.Store.GetCatGroupTurnMeta(ctx, turnID)
	if err != nil {
		return
	}
	meta.EndHashes = map[string]string{}
	for _, f := range files {
		abs := filepath.Join(root, filepath.FromSlash(f.Path))
		if h := fileSHA256(abs); h != "" {
			meta.EndHashes[f.Path] = h
		}
	}
	raw, _ := json.Marshal(meta.EndHashes)
	meta.OverreachJSON = string(raw)
	_ = s.cfg.Store.SaveCatGroupTurnMeta(ctx, meta)
}

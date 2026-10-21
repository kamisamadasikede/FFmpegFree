package cat

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"

	"FFmpegFree/internal/agentexec"
	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
	"FFmpegFree/internal/id"
	"FFmpegFree/internal/store"
)

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
	var result RevertOverreachResult
	op := &workspaceExecutionAdapter{run: func(o catagent.TurnOptions) (catagent.TurnResponse, error) {
		var err error
		result, err = s.revertGroupFiles(o.Ctx, projectRoot, meta, req)
		return catagent.TurnResponse{}, err
	}}
	owner := "revert-" + id.New()
	_, err = s.cfg.Execution.Run(op, agentexec.Scope{OwnerID: owner, Workspace: projectRoot, NativeConfigKey: projectConfigKey(projectRoot)}, catagent.TurnOptions{Ctx: ctx, ConversationID: owner})
	return result, err
}

func (s *Service) revertGroupFiles(ctx context.Context, projectRoot string, meta store.CatGroupTurnMeta, req RevertOverreachRequest) (RevertOverreachResult, error) {
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
		abs, err := safeRevertPath(projectRoot, rel)
		if err != nil {
			skipped = append(skipped, RevertSkippedFile{Path: rel, Reason: "这个路径不能撤销"})
			continue
		}
		// 轮末内容：优先用轮末记下的 sha256；没有就和轮后快照里的 blob 比（同为 git blob 哈希，不能拿 sha256 去比）。
		endHash, wantEnd := "", ""
		if meta.EndHashes != nil && meta.EndHashes[rel] != "" {
			wantEnd = meta.EndHashes[rel]
			endHash = revertFileState(abs)
		} else if meta.SnapshotAfter != "" {
			wantEnd = blobHashFromSnapshot(ctx, projectRoot, meta.SnapshotAfter, rel)
			endHash = worktreeBlobHash(ctx, projectRoot, rel)
			if snapshot, err := parseRevertSnapshot(meta.SnapshotAfter); err == nil {
				if _, ok := snapshot.untrackedBlob(rel); ok {
					out, _, err := runGit(ctx, 5*time.Second, projectRoot, "hash-object", "--no-filters", "--", rel)
					endHash = ""
					if err == nil {
						endHash = strings.TrimSpace(string(out))
					}
				}
			}
		}
		if wantEnd == "" {
			skipped = append(skipped, RevertSkippedFile{Path: rel, Reason: "缺少轮末内容记录，不能自动撤销"})
			continue
		}
		if endHash != wantEnd {
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
	snapshot, err := parseRevertSnapshot(snap)
	if err != nil {
		return ""
	}
	if b, ok := snapshot.untrackedBlob(rel); ok {
		return b.ID
	}
	prefix := snapshot.Prefix
	if snapshot.Version == 0 {
		out, _, err := runGit(ctx, 5*time.Second, root, "rev-parse", "--show-prefix")
		if err != nil {
			return ""
		}
		prefix = strings.TrimSpace(string(out))
	}
	out, _, err := runGit(ctx, 5*time.Second, root, "rev-parse", "--verify", snapshot.Tracked+":"+prefix+rel)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func worktreeBlobHash(ctx context.Context, root, rel string) string {
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
		return ""
	}
	out, _, err := runGit(ctx, 5*time.Second, root, "hash-object", "--", rel)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Missing is an explicit state, not a failed read or an empty file.
func revertFileState(path string) string {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return "missing"
	}
	if err != nil || !info.Mode().IsRegular() {
		return ""
	}
	return fileSHA256(path)
}

package cat

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"FFmpegFree/internal/apperr"
)

// maxGroupFiles 是「文件」页签最多返回的条数，超出丢最旧（契约 6.21.17 ①）。
const maxGroupFiles = 500

// ListCatGroupFiles 列出本群所有汇报里出现过的文件（契约 6.21.17 ①）：
// 不建表，扫 kind=member 的 report.files（Cat 用 TurnChanges 生成的标签）按 path 聚合，按 updatedAt 降序。
func (s *Service) ListCatGroupFiles(ctx context.Context, groupID string) ([]GroupFile, error) {
	g, err := s.requireGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	msgs, err := s.cfg.Store.ListCatGroupReportMessages(ctx, g.ID)
	if err != nil {
		return nil, apperr.Wrap(apperr.IOError, "列出群消息失败", err)
	}
	root := ""
	if row, err := s.cfg.Store.GetCatProject(ctx, g.ProjectID); err == nil {
		root = row.Path
	}
	byPath := map[string]*GroupFile{}
	for _, m := range msgs { // seq 升序：后来的覆盖 last*
		if m.Report == nil {
			continue
		}
		seen := map[string]bool{} // 同一条汇报里重复列出只算一次
		for _, f := range m.Report.Files {
			p := strings.TrimSpace(filepath.ToSlash(f.Path))
			if p == "" || seen[p] {
				continue
			}
			seen[p] = true
			gf := byPath[p]
			if gf == nil {
				gf = &GroupFile{Path: p}
				byPath[p] = gf
			}
			gf.ChangeCount++
			gf.LastMemberID = m.MemberID
			gf.LastSeq = m.Seq
			gf.LastTurnID = m.TurnID
			gf.Tag = f.Tag
			gf.UpdatedAt = m.CreatedAt
			if f.Tag == "overreach" {
				gf.Overreach = true
			}
		}
	}
	out := make([]GroupFile, 0, len(byPath))
	for _, gf := range byPath {
		out = append(out, *gf)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt != out[j].UpdatedAt {
			return out[i].UpdatedAt > out[j].UpdatedAt
		}
		if out[i].LastSeq != out[j].LastSeq {
			return out[i].LastSeq > out[j].LastSeq
		}
		return out[i].Path < out[j].Path
	})
	if len(out) > maxGroupFiles {
		out = out[:maxGroupFiles]
	}
	for i := range out {
		out[i].Exists = groupFileExists(root, out[i].Path)
	}
	return out, nil
}

// groupFileExists：调用时 os.Stat 项目根 + path；路径跑出项目根的一律当不存在。
func groupFileExists(root, rel string) bool {
	if root == "" || rel == "" || filepath.IsAbs(rel) {
		return false
	}
	full := filepath.Join(root, filepath.FromSlash(rel))
	if r, err := filepath.Rel(root, full); err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return false
	}
	st, err := os.Stat(full)
	return err == nil && !st.IsDir()
}

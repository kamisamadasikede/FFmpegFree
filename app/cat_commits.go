package app

import "FFmpegFree/internal/service/cat"

// 契约 v0.53（6.19.28）：Git 面板「提交历史」。

// ListCatCommits 分页列出提交（游标分页，refs 变化时 reason=history_changed）。
func (s *CatService) ListCatCommits(req cat.ListCatCommitsRequest) (cat.CatCommitPage, error) {
	c, err := s.svc()
	if err != nil {
		return cat.CatCommitPage{}, err
	}
	return c.ListCatCommits(s.rootCtx(), req)
}

// GetCatCommitDetail 返回提交详情和改动文件（合并提交对第一父）。
func (s *CatService) GetCatCommitDetail(req cat.CatCommitRequest) (cat.CatCommitDetail, error) {
	c, err := s.svc()
	if err != nil {
		return cat.CatCommitDetail{}, err
	}
	return c.GetCatCommitDetail(s.rootCtx(), req)
}

// GetCatCommitFileDiff 返回提交里单个文件的 diff。
func (s *CatService) GetCatCommitFileDiff(req cat.CatCommitFileDiffRequest) (cat.GitDiffResult, error) {
	c, err := s.svc()
	if err != nil {
		return cat.GitDiffResult{}, err
	}
	return c.GetCatCommitFileDiff(s.rootCtx(), req)
}

// GitCatCheckoutCommit 签出某个提交（HEAD 游离）。
func (s *CatService) GitCatCheckoutCommit(req cat.CatCommitRequest) (cat.CatBranchTree, error) {
	c, err := s.svc()
	if err != nil {
		return cat.CatBranchTree{}, err
	}
	return c.GitCatCheckoutCommit(s.rootCtx(), req)
}

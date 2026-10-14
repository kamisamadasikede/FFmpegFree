package app

import "FFmpegFree/internal/service/cat"

// 契约 v0.51（6.19.26）：Git 分支树。

// ListCatBranchTree 读分支树（只读本地 refs，不 fetch）。
func (s *CatService) ListCatBranchTree(req cat.CatBranchTreeRequest) (cat.CatBranchTree, error) {
	c, err := s.svc()
	if err != nil {
		return cat.CatBranchTree{}, err
	}
	return c.ListCatBranchTree(s.rootCtx(), req)
}

// FetchCatRemotes 从远程获取（force=false 时同仓库 5 分钟最多一次）；失败不报错，写进 fetchError。
func (s *CatService) FetchCatRemotes(req cat.FetchCatRemotesRequest) (cat.CatBranchTree, error) {
	c, err := s.svc()
	if err != nil {
		return cat.CatBranchTree{}, err
	}
	return c.FetchCatRemotes(s.rootCtx(), req)
}

// SetCatBranchFavorite 收藏 / 取消收藏分支（存本机设置，按仓库分）。
func (s *CatService) SetCatBranchFavorite(req cat.SetCatBranchFavoriteRequest) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.SetCatBranchFavorite(s.rootCtx(), req)
}

// GitCatCreateBranch 从本地或远程分支新建分支，可选签出；也用于签出远程分支。
func (s *CatService) GitCatCreateBranch(req cat.GitCatCreateBranchRequest) (cat.CatBranchTree, error) {
	c, err := s.svc()
	if err != nil {
		return cat.CatBranchTree{}, err
	}
	return c.GitCatCreateBranch(s.rootCtx(), req)
}

// GitCatCompareBranch 比较分支与当前分支。
func (s *CatService) GitCatCompareBranch(req cat.GitCatCompareBranchRequest) (cat.CatBranchCompare, error) {
	c, err := s.svc()
	if err != nil {
		return cat.CatBranchCompare{}, err
	}
	return c.GitCatCompareBranch(s.rootCtx(), req)
}

// GitCatRenameBranch 重命名本地分支。
func (s *CatService) GitCatRenameBranch(req cat.GitCatRenameBranchRequest) (cat.CatBranchTree, error) {
	c, err := s.svc()
	if err != nil {
		return cat.CatBranchTree{}, err
	}
	return c.GitCatRenameBranch(s.rootCtx(), req)
}

// GitCatDeleteBranch 删除本地分支（未合并需 force）。
func (s *CatService) GitCatDeleteBranch(req cat.GitCatDeleteBranchRequest) (cat.CatBranchTree, error) {
	c, err := s.svc()
	if err != nil {
		return cat.CatBranchTree{}, err
	}
	return c.GitCatDeleteBranch(s.rootCtx(), req)
}

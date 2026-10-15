package app

import "FFmpegFree/internal/service/cat"

func (s *CatService) GitCatConflictSkip(req cat.GitStatusRequest) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.GitCatConflictSkip(s.rootCtx(), req)
}

func (s *CatService) GitCatInit(req cat.GitStatusRequest) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.GitCatInit(s.rootCtx(), req)
}

func (s *CatService) GitCatAddRemote(req cat.GitAddRemoteRequest) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.GitCatAddRemote(s.rootCtx(), req)
}

func (s *CatService) GitCatCommitAction(req cat.GitCommitActionRequest) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.GitCatCommitAction(s.rootCtx(), req)
}

func (s *CatService) ListCatStashes(req cat.GitStatusRequest) ([]cat.GitStash, error) {
	c, err := s.svc()
	if err != nil {
		return nil, err
	}
	return c.ListCatStashes(s.rootCtx(), req)
}

func (s *CatService) GetCatStashDiff(req cat.GitStashRequest) (cat.GitDiffResult, error) {
	c, err := s.svc()
	if err != nil {
		return cat.GitDiffResult{}, err
	}
	return c.GetCatStashDiff(s.rootCtx(), req)
}

func (s *CatService) GitCatStash(req cat.GitStashRequest) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.GitCatStash(s.rootCtx(), req)
}

func (s *CatService) GitCatUndoCommit(req cat.CatCommitRequest) (cat.GitUndoResult, error) {
	c, err := s.svc()
	if err != nil {
		return cat.GitUndoResult{}, err
	}
	return c.GitCatUndoCommit(s.rootCtx(), req)
}

func (s *CatService) GitCatDiscard(req cat.GitPathsRequest) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.GitCatDiscard(s.rootCtx(), req)
}

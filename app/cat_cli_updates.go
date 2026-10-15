package app

import "FFmpegFree/internal/catagent"

func (s *CatService) CheckCatCLIUpdates() ([]catagent.CLIUpdate, error) {
	c, err := s.svc()
	if err != nil {
		return nil, err
	}
	return c.CheckCatCLIUpdates(s.rootCtx())
}

func (s *CatService) UpdateCatCLI(agentKind string) (catagent.Status, error) {
	c, err := s.svc()
	if err != nil {
		return catagent.Status{}, err
	}
	return c.UpdateCatCLI(s.rootCtx(), agentKind)
}

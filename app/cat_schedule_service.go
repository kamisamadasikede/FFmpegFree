package app

import "FFmpegFree/internal/service/cat"

// ---------- 定时任务（契约 v0.35 / v0.35.1，6.19.13） ----------

// ListCatSchedules 按创建时间倒序列出定时任务。
func (s *CatService) ListCatSchedules() ([]cat.Schedule, error) {
	c, err := s.svc()
	if err != nil {
		return nil, err
	}
	return c.ListCatSchedules(s.rootCtx())
}

// CreateCatSchedule 新建定时任务（默认启用；批准方式空 = plan）。
func (s *CatService) CreateCatSchedule(req cat.ScheduleInput) (cat.Schedule, error) {
	c, err := s.svc()
	if err != nil {
		return cat.Schedule{}, err
	}
	return c.CreateCatSchedule(s.rootCtx(), req)
}

// UpdateCatSchedule 整体修改定时任务。
func (s *CatService) UpdateCatSchedule(req cat.UpdateScheduleRequest) (cat.Schedule, error) {
	c, err := s.svc()
	if err != nil {
		return cat.Schedule{}, err
	}
	return c.UpdateCatSchedule(s.rootCtx(), req)
}

// DeleteCatSchedule 删除定时任务和它的运行记录；已产生的对话保留。
func (s *CatService) DeleteCatSchedule(req cat.ScheduleIDRequest) error {
	c, err := s.svc()
	if err != nil {
		return err
	}
	return c.DeleteCatSchedule(s.rootCtx(), req)
}

// PauseCatSchedule 暂停定时任务。
func (s *CatService) PauseCatSchedule(req cat.ScheduleIDRequest) (cat.Schedule, error) {
	c, err := s.svc()
	if err != nil {
		return cat.Schedule{}, err
	}
	return c.PauseCatSchedule(s.rootCtx(), req)
}

// ResumeCatSchedule 恢复定时任务。
func (s *CatService) ResumeCatSchedule(req cat.ScheduleIDRequest) (cat.Schedule, error) {
	c, err := s.svc()
	if err != nil {
		return cat.Schedule{}, err
	}
	return c.ResumeCatSchedule(s.rootCtx(), req)
}

// RunNowCatSchedule 立即运行一次；上一次没结束返回一条 skipped。
func (s *CatService) RunNowCatSchedule(req cat.ScheduleIDRequest) (cat.ScheduleRun, error) {
	c, err := s.svc()
	if err != nil {
		return cat.ScheduleRun{}, err
	}
	return c.RunNowCatSchedule(s.rootCtx(), req)
}

// ListCatScheduleRuns 倒序列出运行记录（默认 50，最多 100）。
func (s *CatService) ListCatScheduleRuns(req cat.ListScheduleRunsRequest) ([]cat.ScheduleRun, error) {
	c, err := s.svc()
	if err != nil {
		return nil, err
	}
	return c.ListCatScheduleRuns(s.rootCtx(), req)
}

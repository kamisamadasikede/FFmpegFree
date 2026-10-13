-- Cat 定时任务可选服务商（契约 v0.40 6.19.13.6）。
-- 已有行默认 cat_build。
ALTER TABLE cat_schedules ADD COLUMN agent_kind TEXT NOT NULL DEFAULT 'cat_build';

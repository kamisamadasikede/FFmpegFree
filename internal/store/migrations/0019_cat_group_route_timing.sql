-- v0.55.6（契约 6.21.17 ③）：派活判断耗时。旧数据两列为 0，前端不显示耗时。
ALTER TABLE cat_group_routes ADD COLUMN started_at  INTEGER NOT NULL DEFAULT 0;
ALTER TABLE cat_group_routes ADD COLUMN finished_at INTEGER NOT NULL DEFAULT 0;

-- 契约 v0.23（6.14.8）：转换记录。只加表、加列、加索引，不改不删任何旧数据。
-- 旧 convert 任务的 source_id / output_name_key 由启动时的 store.BackfillConvertSources 在 Go 里回填。
CREATE TABLE convert_sources (
    id               TEXT PRIMARY KEY,          -- sourceId（ULID）
    path             TEXT NOT NULL,
    path_key         TEXT NOT NULL UNIQUE,      -- 与 media.path_key 同一规范化函数
    name             TEXT NOT NULL,
    name_key         TEXT NOT NULL,             -- Go strings.ToLower(basename)，用于搜索
    added_at         INTEGER NOT NULL,
    last_activity_at INTEGER NOT NULL
);
CREATE INDEX idx_convert_sources_activity ON convert_sources(last_activity_at DESC, id DESC);
CREATE INDEX idx_convert_sources_name_key ON convert_sources(name_key);

ALTER TABLE tasks ADD COLUMN source_id TEXT;                                   -- 可空；只有 convert 任务有
ALTER TABLE tasks ADD COLUMN hidden_in_task_center INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN result TEXT;                                      -- TaskResult JSON，可空
ALTER TABLE tasks ADD COLUMN output_name_key TEXT NOT NULL DEFAULT '';         -- Go strings.ToLower(basename(output_path))
CREATE INDEX idx_tasks_source_created ON tasks(source_id, created_at DESC) WHERE source_id IS NOT NULL;
CREATE INDEX idx_tasks_source_status ON tasks(source_id, status) WHERE source_id IS NOT NULL;   -- v0.23.1：ListSources 的 status 筛选
CREATE INDEX idx_tasks_convert_output_name ON tasks(output_name_key, source_id) WHERE type = 'convert';
CREATE INDEX idx_tasks_hidden_created ON tasks(hidden_in_task_center, created_at DESC);

-- 剪辑工程列表要显示时长和片段数，不想为此反序列化整个工程 JSON（最大 1 MiB）：保存时一并写入。
-- 旧行（0001 时代没有写入路径，理论上为空表）默认 0。
ALTER TABLE edit_projects ADD COLUMN duration_sec REAL NOT NULL DEFAULT 0;
ALTER TABLE edit_projects ADD COLUMN clip_count INTEGER NOT NULL DEFAULT 0;

-- Cat 助手项目（契约 v0.31，6.19.10.7）。项目 = 用户选的一个本机文件夹。
-- 删除项目用显式事务（先消息、再会话、再项目），不依赖外键级联；project_id 不加外键。
-- 旧列 cat_conversations.project_path 保留不删，v0.31 起不再读写。
CREATE TABLE cat_projects (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    path       TEXT NOT NULL,
    path_key   TEXT NOT NULL UNIQUE,
    created_at INTEGER NOT NULL DEFAULT 0,
    updated_at INTEGER NOT NULL DEFAULT 0
);
ALTER TABLE cat_conversations ADD COLUMN project_id TEXT;
CREATE INDEX idx_cat_conversations_project ON cat_conversations (project_id, updated_at DESC);

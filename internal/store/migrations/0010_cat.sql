-- Cat 助手会话与消息（契约 v0.30，6.19）。agent_kind 创建后不可变。
CREATE TABLE cat_conversations (
    id          TEXT PRIMARY KEY,
    title       TEXT NOT NULL DEFAULT '',
    agent_kind  TEXT NOT NULL,
    access_mode TEXT NOT NULL DEFAULT 'ask',
    project_path TEXT NOT NULL DEFAULT '',
    created_at  INTEGER NOT NULL DEFAULT 0,
    updated_at  INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_cat_conversations_updated ON cat_conversations (updated_at DESC);

CREATE TABLE cat_messages (
    id              TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL,
    role            TEXT NOT NULL,
    content         TEXT NOT NULL DEFAULT '',
    created_at      INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (conversation_id) REFERENCES cat_conversations(id) ON DELETE CASCADE
);
CREATE INDEX idx_cat_messages_conv ON cat_messages (conversation_id, created_at ASC);

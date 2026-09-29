-- DocService 最近打开的 PDF（契约 6.12.2）。时间一律 Unix 毫秒。
CREATE TABLE doc_recent (
    id        TEXT PRIMARY KEY,
    path      TEXT NOT NULL,
    path_key  TEXT NOT NULL UNIQUE,
    name      TEXT NOT NULL DEFAULT '',
    size      INTEGER NOT NULL DEFAULT 0,
    opened_at INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_doc_recent_opened ON doc_recent (opened_at DESC);

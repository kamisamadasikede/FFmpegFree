-- 契约 v0.24（6.15.3）：源文件副本。只加表、加列、加索引，不改旧数据（旧行 copy_id = NULL，即 copyState=none）。
CREATE TABLE convert_copies (
    id                TEXT PRIMARY KEY,
    owner_source_id   TEXT NOT NULL,
    original_path     TEXT NOT NULL,
    original_path_key TEXT NOT NULL,
    original_size     INTEGER NOT NULL,
    original_mtime_ns INTEGER NOT NULL,
    stored_path       TEXT NOT NULL,
    state             TEXT NOT NULL,
    total_bytes       INTEGER NOT NULL,
    copied_bytes      INTEGER NOT NULL DEFAULT 0,
    error             TEXT,
    ref_count         INTEGER NOT NULL DEFAULT 0,
    pending_delete    INTEGER NOT NULL DEFAULT 0,
    created_at        INTEGER NOT NULL,
    finished_at       INTEGER
);
CREATE INDEX idx_convert_copies_identity ON convert_copies(original_path_key, original_size, original_mtime_ns);
CREATE INDEX idx_convert_copies_state ON convert_copies(state);
ALTER TABLE convert_sources ADD COLUMN copy_id TEXT;
CREATE INDEX idx_convert_sources_copy ON convert_sources(copy_id) WHERE copy_id IS NOT NULL;

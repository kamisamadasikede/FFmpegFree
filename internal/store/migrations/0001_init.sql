-- v2 初始表结构，见 docs/architecture/contract.md 第 6 节。
-- 时间一律 Unix 毫秒；JSON 列存 TEXT。

CREATE TABLE media (
    id          TEXT PRIMARY KEY,
    path        TEXT NOT NULL,
    path_key    TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL DEFAULT '',
    size        INTEGER NOT NULL DEFAULT 0,
    duration    REAL NOT NULL DEFAULT 0,
    width       INTEGER NOT NULL DEFAULT 0,
    height      INTEGER NOT NULL DEFAULT 0,
    video_codec TEXT NOT NULL DEFAULT '',
    audio_codec TEXT NOT NULL DEFAULT '',
    bitrate     INTEGER NOT NULL DEFAULT 0,
    probed_at   INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_media_probed_at ON media (probed_at DESC);

CREATE TABLE tasks (
    id          TEXT PRIMARY KEY,
    type        TEXT NOT NULL,
    status      TEXT NOT NULL,
    title       TEXT NOT NULL DEFAULT '',
    input_paths TEXT NOT NULL DEFAULT '[]',
    output_path TEXT NOT NULL DEFAULT '',
    params      TEXT NOT NULL DEFAULT '{}',
    progress    REAL NOT NULL DEFAULT 0,
    error       TEXT,
    log_path    TEXT NOT NULL DEFAULT '',
    version     INTEGER NOT NULL DEFAULT 1,
    created_at  INTEGER NOT NULL,
    started_at  INTEGER NOT NULL DEFAULT 0,
    finished_at INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_tasks_status ON tasks (status);
CREATE INDEX idx_tasks_type_created ON tasks (type, created_at DESC);
CREATE INDEX idx_tasks_created ON tasks (created_at DESC);

CREATE TABLE presets (
    id       TEXT PRIMARY KEY,
    name     TEXT NOT NULL,
    built_in INTEGER NOT NULL DEFAULT 0,
    options  TEXT NOT NULL DEFAULT '{}',
    sort     INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE edit_projects (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    project    TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);
CREATE INDEX idx_edit_projects_updated ON edit_projects (updated_at DESC);

CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

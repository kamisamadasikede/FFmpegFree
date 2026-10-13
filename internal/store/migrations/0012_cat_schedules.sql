-- Cat 定时任务（契约 v0.35 6.19.13.6；v0.35.1 加 manual / missed_reason）。
-- project_id 不加外键：删除项目时由代码把任务暂停并标 project_deleted。
-- weekdays 是 JSON 数组（1~7，周一=1）；error 是 AppError 的 JSON。
CREATE TABLE cat_schedules (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    prompt          TEXT NOT NULL,
    project_id      TEXT NOT NULL,
    frequency       TEXT NOT NULL,
    time            TEXT NOT NULL DEFAULT '',
    weekdays        TEXT NOT NULL DEFAULT '[]',
    run_at          INTEGER NOT NULL DEFAULT 0,
    model           TEXT NOT NULL DEFAULT '',
    effort          TEXT NOT NULL DEFAULT '',
    approval_mode   TEXT NOT NULL DEFAULT 'plan',
    enabled         INTEGER NOT NULL DEFAULT 1,
    project_deleted INTEGER NOT NULL DEFAULT 0,
    next_run_at     INTEGER NOT NULL DEFAULT 0,
    created_at      INTEGER NOT NULL DEFAULT 0,
    updated_at      INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_cat_schedules_project ON cat_schedules (project_id);
CREATE TABLE cat_schedule_runs (
    id              TEXT PRIMARY KEY,
    schedule_id     TEXT NOT NULL,
    conversation_id TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL,
    error           TEXT NOT NULL DEFAULT '',
    scheduled_for   INTEGER NOT NULL DEFAULT 0,
    started_at      INTEGER NOT NULL DEFAULT 0,
    finished_at     INTEGER NOT NULL DEFAULT 0,
    manual          INTEGER NOT NULL DEFAULT 0,
    missed_reason   TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_cat_schedule_runs_schedule ON cat_schedule_runs (schedule_id, scheduled_for DESC);

-- 项目群聊（契约 v0.55，6.21.2）。群消息由 Cat 自存，是「库内只留元数据」的唯一例外。
CREATE TABLE cat_groups (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL,
    name        TEXT NOT NULL,
    auto_route  INTEGER NOT NULL DEFAULT 1,
    created_at  INTEGER NOT NULL DEFAULT 0,
    updated_at  INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_cat_groups_project ON cat_groups (project_id, updated_at DESC);

CREATE TABLE cat_group_members (
    id            TEXT PRIMARY KEY,
    group_id      TEXT NOT NULL REFERENCES cat_groups(id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    title         TEXT NOT NULL DEFAULT '',
    agent_kind    TEXT NOT NULL,
    model_id      TEXT NOT NULL DEFAULT '',
    think_level   TEXT NOT NULL DEFAULT '',
    sort_order    INTEGER NOT NULL DEFAULT 0,
    session_id    TEXT NOT NULL DEFAULT '',
    last_seen_seq INTEGER NOT NULL DEFAULT 0,
    charter_json  TEXT NOT NULL DEFAULT '{}',
    created_at    INTEGER NOT NULL DEFAULT 0,
    updated_at    INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX idx_cat_group_members_name ON cat_group_members (group_id, name COLLATE NOCASE);
CREATE INDEX idx_cat_group_members_group ON cat_group_members (group_id, sort_order, id);

CREATE TABLE cat_group_messages (
    group_id   TEXT NOT NULL REFERENCES cat_groups(id) ON DELETE CASCADE,
    seq        INTEGER NOT NULL,
    kind       TEXT NOT NULL,
    member_id  TEXT NOT NULL DEFAULT '',
    text       TEXT NOT NULL DEFAULT '',
    mentions   TEXT NOT NULL DEFAULT '[]',
    chain_id   TEXT NOT NULL DEFAULT '',
    hop        INTEGER NOT NULL DEFAULT 0,
    turn_id    TEXT NOT NULL DEFAULT '',
    route_json TEXT NOT NULL DEFAULT '',
    report_json TEXT NOT NULL DEFAULT '',
    system_kind TEXT NOT NULL DEFAULT '',
    from_member_id TEXT NOT NULL DEFAULT '',
    to_member_id TEXT NOT NULL DEFAULT '',
    fail_reason TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (group_id, seq)
);
CREATE INDEX idx_cat_group_messages_chain ON cat_group_messages (group_id, chain_id, hop);

CREATE TABLE cat_group_routes (
    route_seq  INTEGER PRIMARY KEY,
    group_id   TEXT NOT NULL,
    chain_id   TEXT NOT NULL,
    mode       TEXT NOT NULL,
    member_id  TEXT NOT NULL DEFAULT '',
    reason     TEXT NOT NULL DEFAULT '',
    input_text TEXT NOT NULL DEFAULT '',
    output_text TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE cat_group_turn_meta (
    turn_id         TEXT PRIMARY KEY,
    group_id        TEXT NOT NULL,
    member_id       TEXT NOT NULL,
    chain_id        TEXT NOT NULL DEFAULT '',
    hop             INTEGER NOT NULL DEFAULT 0,
    snapshot_before TEXT NOT NULL DEFAULT '',
    snapshot_after  TEXT NOT NULL DEFAULT '',
    claimed_files   TEXT NOT NULL DEFAULT '[]',
    overreach_json  TEXT NOT NULL DEFAULT '',
    end_hashes      TEXT NOT NULL DEFAULT '{}',
    created_at      INTEGER NOT NULL DEFAULT 0
);

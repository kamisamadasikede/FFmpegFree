CREATE TABLE cat_turn_changes (
    turn_id TEXT PRIMARY KEY,
    conv_id TEXT NOT NULL REFERENCES cat_conversations(id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    summary TEXT NOT NULL,
    detail TEXT NOT NULL
);
CREATE INDEX idx_cat_turn_changes_conv ON cat_turn_changes(conv_id, created_at);

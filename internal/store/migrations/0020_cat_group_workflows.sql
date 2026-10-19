CREATE TABLE cat_group_workflows (
    chain_id TEXT PRIMARY KEY,
    group_id TEXT NOT NULL REFERENCES cat_groups(id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    data TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);
CREATE INDEX cat_group_workflows_group ON cat_group_workflows(group_id, updated_at);
CREATE INDEX cat_group_messages_chain ON cat_group_messages(group_id, chain_id, seq);

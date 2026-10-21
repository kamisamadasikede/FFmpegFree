-- Requests and immutable assignments form the durable dispatch outbox.
CREATE TABLE cat_group_requests (
 id TEXT PRIMARY KEY, group_id TEXT NOT NULL REFERENCES cat_groups(id) ON DELETE CASCADE, client_id TEXT NOT NULL,
 status TEXT NOT NULL, input_json TEXT NOT NULL, decision_json TEXT NOT NULL DEFAULT '',
 updated_at INTEGER NOT NULL, first_reply_at INTEGER NOT NULL DEFAULT 0,
 UNIQUE(group_id, client_id)
);
CREATE INDEX idx_cat_group_requests_status ON cat_group_requests(group_id,status);
CREATE TABLE cat_group_assignments (
 id TEXT PRIMARY KEY, request_id TEXT NOT NULL REFERENCES cat_group_requests(id) ON DELETE CASCADE,
 member_id TEXT NOT NULL, status TEXT NOT NULL, data TEXT NOT NULL,
 turn_id TEXT NOT NULL DEFAULT '', updated_at INTEGER NOT NULL
);
CREATE INDEX idx_cat_group_assignments_request ON cat_group_assignments(request_id,status);

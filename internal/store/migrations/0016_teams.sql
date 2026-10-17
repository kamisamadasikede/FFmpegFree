CREATE TABLE team_profiles (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL CHECK(kind IN ('human','agent')),
    data TEXT NOT NULL
);
CREATE TABLE team_rooms (
    id TEXT PRIMARY KEY,
    data TEXT NOT NULL,
    event_seq INTEGER NOT NULL DEFAULT 0,
    room_seq INTEGER NOT NULL DEFAULT 0,
    updated_at INTEGER NOT NULL
);
CREATE TABLE team_members (
    id TEXT PRIMARY KEY,
    room_id TEXT NOT NULL REFERENCES team_rooms(id),
    profile_id TEXT NOT NULL REFERENCES team_profiles(id),
    active INTEGER NOT NULL,
    data TEXT NOT NULL,
    UNIQUE(room_id,profile_id)
);
CREATE INDEX team_members_room ON team_members(room_id,active);
CREATE TABLE team_messages (
    id TEXT PRIMARY KEY,
    room_id TEXT NOT NULL REFERENCES team_rooms(id),
    room_seq INTEGER NOT NULL,
    data TEXT NOT NULL,
    UNIQUE(room_id,room_seq)
);
CREATE TABLE team_tasks (
    id TEXT PRIMARY KEY,
    room_id TEXT NOT NULL REFERENCES team_rooms(id),
    member_id TEXT NOT NULL REFERENCES team_members(id),
    status TEXT NOT NULL,
    parent_id TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    data TEXT NOT NULL
);
CREATE INDEX team_tasks_queue ON team_tasks(status,created_at);
CREATE INDEX team_tasks_member ON team_tasks(room_id,member_id,status);
CREATE INDEX team_tasks_parent ON team_tasks(parent_id);
CREATE TABLE team_inbox (
    id TEXT PRIMARY KEY,
    room_id TEXT NOT NULL REFERENCES team_rooms(id),
    message_id TEXT NOT NULL REFERENCES team_messages(id),
    recipient_id TEXT NOT NULL REFERENCES team_members(id),
    read_at INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    data TEXT NOT NULL,
    UNIQUE(message_id,recipient_id)
);
CREATE INDEX team_inbox_unread ON team_inbox(read_at,created_at);
CREATE TABLE team_events (
    room_id TEXT NOT NULL REFERENCES team_rooms(id),
    event_seq INTEGER NOT NULL,
    kind TEXT NOT NULL,
    data TEXT NOT NULL,
    PRIMARY KEY(room_id,event_seq)
);
CREATE TABLE team_commands (
    source_id TEXT NOT NULL,
    request_id TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    result TEXT NOT NULL,
    PRIMARY KEY(source_id,request_id)
);
CREATE TABLE team_notification_outbox (
    id TEXT PRIMARY KEY REFERENCES team_inbox(id),
    status TEXT NOT NULL DEFAULT 'pending',
    data TEXT NOT NULL
);

CREATE TABLE team_artifacts (
    id TEXT PRIMARY KEY,
    room_id TEXT NOT NULL REFERENCES team_rooms(id),
    path TEXT NOT NULL,
    data TEXT NOT NULL,
    UNIQUE(room_id,path)
);
CREATE TABLE team_runs (
    id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL REFERENCES team_tasks(id),
    room_id TEXT NOT NULL REFERENCES team_rooms(id),
    data TEXT NOT NULL
);
CREATE INDEX team_runs_task ON team_runs(task_id);

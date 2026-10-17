package team

import (
	"context"
	"encoding/json"
)

// RunRecord is immutable execution identity plus final outcome. Credentials and
// hidden native prompts are never returned to the group or written here.
type RunRecord struct {
	ID         string   `json:"id"`
	TaskID     string   `json:"taskId"`
	RoomID     string   `json:"roomId"`
	SessionID  string   `json:"sessionId"`
	Employee   Employee `json:"employee"`
	Transport  string   `json:"transport"`
	InputMode  string   `json:"inputMode"`
	Status     string   `json:"status"`
	Error      string   `json:"error"`
	StartedAt  int64    `json:"startedAt"`
	FinishedAt int64    `json:"finishedAt"`
}

func (t *Tx) SaveRun(v RunRecord) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	return t.exec("INSERT INTO team_runs(id,task_id,room_id,data) VALUES(?,?,?,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data", v.ID, v.TaskID, v.RoomID, string(b))
}
func (s *Service) Runs(ctx context.Context, taskID string) ([]RunRecord, error) {
	return list[RunRecord](ctx, s.cfg.Repository.DB, "SELECT data FROM team_runs WHERE task_id=? ORDER BY rowid DESC LIMIT 40", taskID)
}

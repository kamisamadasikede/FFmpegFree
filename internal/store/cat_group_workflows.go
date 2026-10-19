package store

import (
	"context"
	"encoding/json"
	"time"
)

// CatGroupWorkflow is one development request, including its dependency graph.
// A successful CLI exit alone never establishes that the request passed review.
type CatGroupWorkflow struct {
	ChainID   string             `json:"chainId"`
	GroupID   string             `json:"groupId"`
	OriginSeq int64              `json:"originSeq"`
	Request   string             `json:"request"`
	Status    string             `json:"status"`
	Tasks     []CatGroupWorkTask `json:"tasks"`
	UpdatedAt int64              `json:"updatedAt"`
	Revision  int64              `json:"revision"`
}

type CatGroupWorkTask struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	MemberID    string   `json:"memberId"`
	Instruction string   `json:"instruction"`
	DependsOn   []string `json:"dependsOn"`
	Kind        string   `json:"kind"`   // develop | integrate | verify
	Status      string   `json:"status"` // pending | queued | running | done | blocked | failed | stopped
	Attempts    int      `json:"attempts"`
	Result      string   `json:"result,omitempty"`
	Evidence    string   `json:"evidence,omitempty"`
	TurnID      string   `json:"turnId,omitempty"`
}

func (s *Store) SaveCatGroupWorkflow(ctx context.Context, w *CatGroupWorkflow) error {
	w.UpdatedAt = time.Now().UnixMilli()
	w.Revision++
	b, err := json.Marshal(w)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO cat_group_workflows(chain_id,group_id,status,data,updated_at) VALUES(?,?,?,?,?)
		ON CONFLICT(chain_id) DO UPDATE SET status=excluded.status,data=excluded.data,updated_at=excluded.updated_at`, w.ChainID, w.GroupID, w.Status, string(b), w.UpdatedAt)
	return err
}

func (s *Store) GetCatGroupWorkflow(ctx context.Context, chainID string) (CatGroupWorkflow, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT data FROM cat_group_workflows WHERE chain_id=?`, chainID).Scan(&raw)
	var w CatGroupWorkflow
	if err == nil {
		err = json.Unmarshal([]byte(raw), &w)
	}
	return w, err
}

func (s *Store) ListCatGroupWorkflows(ctx context.Context, groupID string) ([]CatGroupWorkflow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT data FROM cat_group_workflows WHERE group_id=? AND
		(status NOT IN ('completed','stopped') OR chain_id IN
		 (SELECT chain_id FROM cat_group_workflows WHERE group_id=? AND status IN ('completed','stopped') ORDER BY updated_at DESC LIMIT 10))
		ORDER BY updated_at DESC`, groupID, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CatGroupWorkflow{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var w CatGroupWorkflow
		if err := json.Unmarshal([]byte(raw), &w); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var ErrCatGroupAssignmentInactive = errors.New("group assignment is stopped or superseded")

// The request id is also the chain id. Snapshot is captured at reception and
// never rebuilt from a member's unread cursor. Progress does not version input.
type CatGroupRequest struct {
	ID           string            `json:"id"`
	GroupID      string            `json:"groupId"`
	ClientID     string            `json:"clientMessageId"`
	Status       string            `json:"status"`
	Message      CatGroupMessage   `json:"message"`
	Snapshot     []CatGroupMessage `json:"snapshot"`
	FirstReplyAt int64             `json:"firstReplyAt"`
}

type CatGroupOperation struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"`
	TargetIDs []string `json:"targetIds"`
	DependsOn []string `json:"dependsOn"`
}

type CatGroupDecision struct {
	Operations  []CatGroupOperation  `json:"operations"`
	Assignments []CatGroupAssignment `json:"assignments"`
}

type CatGroupAssignment struct {
	ID          string `json:"id"`
	RequestID   string `json:"requestId"`
	MemberID    string `json:"memberId"`
	TaskID      string `json:"taskId"`
	Instruction string `json:"instruction"`
	Capability  string `json:"capability"`
	Status      string `json:"status"`
	TurnID      string `json:"-"`
}

// ReceiveCatGroupRequest atomically accepts a message and its reply obligation.
// A retried client id returns the original receipt without another dispatch.
func (s *Store) ReceiveCatGroupRequest(ctx context.Context, r CatGroupRequest) (CatGroupRequest, bool, error) {
	groupMsgSeqMu.Lock()
	defer groupMsgSeqMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return r, false, err
	}
	defer tx.Rollback()
	var raw, status string
	var first int64
	err = tx.QueryRowContext(ctx, `SELECT input_json,status,first_reply_at FROM cat_group_requests WHERE group_id=? AND client_id=?`, r.GroupID, r.ClientID).Scan(&raw, &status, &first)
	if err == nil {
		err = json.Unmarshal([]byte(raw), &r)
		r.Status = status
		r.FirstReplyAt = first
		return r, false, err
	}
	if err != sql.ErrNoRows {
		return r, false, err
	}
	m := r.Message
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq),0)+1 FROM cat_group_messages WHERE group_id=?`, r.GroupID).Scan(&m.Seq); err != nil {
		return r, false, err
	}
	m.CreatedAt = time.Now().UnixMilli()
	r.Message = m
	r.Status = "received"
	_, err = tx.ExecContext(ctx, `INSERT INTO cat_group_messages(group_id,seq,kind,text,mentions,chain_id,created_at,files_json) VALUES(?,?,?,?,?,?,?,?)`, m.GroupID, m.Seq, "user", m.Text, marshalJSONString(m.Mentions), r.ID, m.CreatedAt, marshalJSONString(m.Files))
	if err != nil {
		return r, false, err
	}
	raw = marshalJSONString(r)
	_, err = tx.ExecContext(ctx, `INSERT INTO cat_group_requests(id,group_id,client_id,status,input_json,updated_at) VALUES(?,?,?,?,?,?)`, r.ID, r.GroupID, r.ClientID, r.Status, raw, m.CreatedAt)
	if err != nil {
		return r, false, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE cat_groups SET updated_at=? WHERE id=?`, m.CreatedAt, r.GroupID); err != nil {
		return r, false, err
	}
	return r, true, tx.Commit()
}

func (s *Store) GetCatGroupRequest(ctx context.Context, id string) (CatGroupRequest, error) {
	var r CatGroupRequest
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT input_json,status,first_reply_at FROM cat_group_requests WHERE id=?`, id).Scan(&raw, &r.Status, &r.FirstReplyAt)
	if err != nil {
		return r, err
	}
	status, first := r.Status, r.FirstReplyAt
	err = json.Unmarshal([]byte(raw), &r)
	r.Status, r.FirstReplyAt = status, first
	return r, err
}

func (s *Store) ClaimCatGroupRequest(ctx context.Context, id string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE cat_group_requests SET status='routing',updated_at=? WHERE id=? AND status='received'`, time.Now().UnixMilli(), id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// CommitCatGroupDecision is the only decision -> dispatch boundary. Every
// assignment is saved in the same transaction, including pending DAG nodes.
func (s *Store) CommitCatGroupDecision(ctx context.Context, id string, d CatGroupDecision, w *CatGroupWorkflow) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UnixMilli()
	res, err := tx.ExecContext(ctx, `UPDATE cat_group_requests SET status='assigned',decision_json=?,updated_at=? WHERE id=? AND status='routing'`, marshalJSONString(d), now, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("request no longer awaiting decision: %s", id)
	}
	for _, a := range d.Assignments {
		_, err = tx.ExecContext(ctx, `INSERT INTO cat_group_assignments(id,request_id,member_id,status,data,updated_at) VALUES(?,?,?,'pending',?,?)`, a.ID, id, a.MemberID, marshalJSONString(a), now)
		if err != nil {
			return err
		}
	}
	if w != nil {
		w.UpdatedAt = now
		w.Revision++
		_, err = tx.ExecContext(ctx, `INSERT INTO cat_group_workflows(chain_id,group_id,status,data,updated_at) VALUES(?,?,?,?,?)`, w.ChainID, w.GroupID, w.Status, marshalJSONString(w), now)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) GetCatGroupAssignment(ctx context.Context, id string) (CatGroupAssignment, error) {
	var a CatGroupAssignment
	var raw, status string
	err := s.db.QueryRowContext(ctx, `SELECT data,status,turn_id FROM cat_group_assignments WHERE id=?`, id).Scan(&raw, &status, &a.TurnID)
	if err == nil {
		turnID := a.TurnID
		err = json.Unmarshal([]byte(raw), &a)
		a.Status, a.TurnID = status, turnID
	}
	return a, err
}

// CAS claims stop duplicate wakeups without holding a database lock while a
// process runs. A retry retains the same assignment but obtains a new turn id.
func (s *Store) ClaimCatGroupAssignment(ctx context.Context, id, from, to, turnID string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE cat_group_assignments SET status=?,turn_id=?,updated_at=? WHERE id=? AND status=? AND EXISTS(SELECT 1 FROM cat_group_requests r WHERE r.id=request_id AND r.status IN ('assigned','answered'))`, to, turnID, time.Now().UnixMilli(), id, from)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

func (s *Store) FinishCatGroupAssignment(ctx context.Context, id, turnID, status string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UnixMilli()
	res, err := tx.ExecContext(ctx, `UPDATE cat_group_assignments SET status=?,updated_at=? WHERE id=? AND turn_id=? AND status='running'`, status, now, id, turnID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 1 && status == "done" {
		_, err = tx.ExecContext(ctx, `UPDATE cat_group_requests SET status='answered',first_reply_at=CASE WHEN first_reply_at=0 THEN ? ELSE first_reply_at END,updated_at=? WHERE id=(SELECT request_id FROM cat_group_assignments WHERE id=?) AND status IN ('assigned','answered')`, now, now, id)
	}
	if err != nil {
		return err
	}
	if n == 1 && status != "done" {
		_, err = tx.ExecContext(ctx, `UPDATE cat_group_requests SET status=?,updated_at=? WHERE id=(SELECT request_id FROM cat_group_assignments WHERE id=?) AND status='assigned' AND NOT EXISTS(SELECT 1 FROM cat_group_assignments a WHERE a.request_id=cat_group_requests.id AND a.status IN('pending','enqueued','running','retrying','held'))`, status, now, id)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) StopCatGroupRequests(ctx context.Context, groupID, memberID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UnixMilli()
	if memberID == "" {
		_, err = tx.ExecContext(ctx, `UPDATE cat_group_requests SET status='stopped',updated_at=? WHERE group_id=? AND status IN ('received','routing','assigned')`, now, groupID)
		if err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE cat_group_assignments SET status='stopped',updated_at=? WHERE request_id IN(SELECT id FROM cat_group_requests WHERE group_id=?) AND (?='' OR member_id=?) AND status IN('pending','enqueued','running','retrying','held')`, now, groupID, memberID, memberID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE cat_group_requests SET status='stopped',updated_at=? WHERE group_id=? AND status='assigned' AND NOT EXISTS(SELECT 1 FROM cat_group_assignments a WHERE a.request_id=cat_group_requests.id AND a.status IN('pending','enqueued','running','retrying','held'))`, now, groupID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// Recovery replays only work known not to have started. An unknown running
// process may already have written files, so its assignment requires inspection.
func (s *Store) RecoverCatGroupRequests(ctx context.Context) ([]CatGroupRequest, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE cat_group_requests SET status='received' WHERE status='routing'`); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE cat_group_assignments SET status='pending' WHERE status='enqueued'`); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE cat_group_assignments SET status='interrupted' WHERE status IN('running','retrying','held')`); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE cat_group_requests SET status='interrupted' WHERE status IN('assigned','answered') AND id IN(SELECT request_id FROM cat_group_assignments WHERE status='interrupted')`); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT input_json,status,first_reply_at FROM cat_group_requests WHERE status IN('received','assigned','interrupted') OR (status='answered' AND EXISTS(SELECT 1 FROM cat_group_assignments a WHERE a.request_id=cat_group_requests.id AND a.status='pending')) ORDER BY updated_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CatGroupRequest
	for rows.Next() {
		var raw, status string
		var r CatGroupRequest
		var first int64
		if err = rows.Scan(&raw, &status, &first); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &r); err != nil {
			return nil, err
		}
		r.Status = status
		r.FirstReplyAt = first
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ListCatGroupAssignments(ctx context.Context, requestID string) ([]CatGroupAssignment, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT data,status,turn_id FROM cat_group_assignments WHERE request_id=?`, requestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CatGroupAssignment
	for rows.Next() {
		var raw, status string
		var a CatGroupAssignment
		if err = rows.Scan(&raw, &status, &a.TurnID); err != nil {
			return nil, err
		}
		turnID := a.TurnID
		if err = json.Unmarshal([]byte(raw), &a); err != nil {
			return nil, err
		}
		a.Status, a.TurnID = status, turnID
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) LatestCatGroupRouteForChain(ctx context.Context, groupID, chainID string) (CatGroupRouteDetail, error) {
	var seq int64
	err := s.db.QueryRowContext(ctx, `SELECT route_seq FROM cat_group_routes WHERE group_id=? AND chain_id=? ORDER BY route_seq DESC LIMIT 1`, groupID, chainID).Scan(&seq)
	if err != nil {
		return CatGroupRouteDetail{}, err
	}
	return s.GetCatGroupRoute(ctx, seq)
}

// CompleteCatGroupAssignment commits delivery and its reply obligation together.
// The conditional claim also fences late results from stopped/replaced turns.
func (s *Store) CompleteCatGroupAssignment(ctx context.Context, id, turnID string, m CatGroupMessage) (CatGroupMessage, error) {
	groupMsgSeqMu.Lock()
	defer groupMsgSeqMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CatGroupMessage{}, err
	}
	defer tx.Rollback()
	now := time.Now().UnixMilli()
	res, err := tx.ExecContext(ctx, `UPDATE cat_group_assignments SET status='done',updated_at=? WHERE id=? AND turn_id=? AND status='running' AND EXISTS(SELECT 1 FROM cat_group_requests r WHERE r.id=request_id AND r.status IN('assigned','answered'))`, now, id, turnID)
	if err != nil {
		return CatGroupMessage{}, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return CatGroupMessage{}, err
	}
	if n != 1 {
		return CatGroupMessage{}, fmt.Errorf("%w: %s", ErrCatGroupAssignmentInactive, id)
	}
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq),0)+1 FROM cat_group_messages WHERE group_id=?`, m.GroupID).Scan(&m.Seq); err != nil {
		return CatGroupMessage{}, err
	}
	m.CreatedAt = now
	route, report := "", ""
	if m.Route != nil {
		route = marshalJSONString(m.Route)
	}
	if m.Report != nil {
		report = marshalJSONString(m.Report)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO cat_group_messages(group_id,seq,kind,member_id,text,mentions,chain_id,hop,turn_id,route_json,report_json,system_kind,from_member_id,to_member_id,fail_reason,created_at,files_json) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, m.GroupID, m.Seq, m.Kind, m.MemberID, m.Text, marshalJSONString(m.Mentions), m.ChainID, m.Hop, m.TurnID, route, report, m.SystemKind, m.FromMemberID, m.ToMemberID, m.FailReason, now, marshalJSONString(m.Files))
	if err != nil {
		return CatGroupMessage{}, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE cat_group_requests SET status='answered',first_reply_at=CASE WHEN first_reply_at=0 THEN ? ELSE first_reply_at END,updated_at=? WHERE id=(SELECT request_id FROM cat_group_assignments WHERE id=?)`, now, now, id)
	if err != nil {
		return CatGroupMessage{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE cat_groups SET updated_at=? WHERE id=?`, now, m.GroupID); err != nil {
		return CatGroupMessage{}, err
	}
	return m, tx.Commit()
}

// Explicit retries can reopen terminal assignments. A running/queued assignment
// is never reset; its process must finish or be stopped before another starts.
func (s *Store) ResetCatGroupAssignment(ctx context.Context, a CatGroupAssignment) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UnixMilli()
	a.Status = "pending"
	_, err = tx.ExecContext(ctx, `INSERT INTO cat_group_assignments(id,request_id,member_id,status,data,updated_at) VALUES(?,?,?,'pending',?,?) ON CONFLICT(id) DO UPDATE SET status='pending',turn_id='',data=excluded.data,updated_at=excluded.updated_at WHERE cat_group_assignments.status IN('done','failed','stopped','interrupted','superseded')`, a.ID, a.RequestID, a.MemberID, marshalJSONString(a), now)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE cat_group_requests SET status=CASE WHEN first_reply_at>0 THEN 'answered' ELSE 'assigned' END,updated_at=? WHERE id=? AND status IN('assigned','answered','failed','stopped','interrupted')`, now, a.RequestID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) FailCatGroupRequest(ctx context.Context, id string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE cat_group_requests SET status='failed',updated_at=? WHERE id=? AND status IN('received','routing')`, time.Now().UnixMilli(), id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// MoveCatGroupAssignment saves the handoff and fences the old execution together.
// Held work is released only after the previous process exits; on restart it is
// interrupted for inspection rather than replayed over possibly running work.
func (s *Store) MoveCatGroupAssignment(ctx context.Context, oldID string, next CatGroupAssignment, w *CatGroupWorkflow) error {
	if w != nil && w.ChainID != next.RequestID {
		return fmt.Errorf("handoff workflow does not match request")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UnixMilli()
	res, err := tx.ExecContext(ctx, `UPDATE cat_group_assignments SET status='superseded',updated_at=? WHERE id=? AND request_id=? AND status!='superseded'`, now, oldID, next.RequestID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("assignment no longer available for handoff: %s", oldID)
	}
	next.Status, next.TurnID = "held", ""
	res, err = tx.ExecContext(ctx, `INSERT INTO cat_group_assignments(id,request_id,member_id,status,data,turn_id,updated_at) VALUES(?,?,?,'held',?,'',?) ON CONFLICT(id) DO UPDATE SET status='held',data=excluded.data,turn_id='',updated_at=excluded.updated_at WHERE cat_group_assignments.status IN('superseded','done','failed','stopped','interrupted')`, next.ID, next.RequestID, next.MemberID, marshalJSONString(next), now)
	if err != nil {
		return err
	}
	n, err = res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("handoff target already active: %s", next.ID)
	}
	_, err = tx.ExecContext(ctx, `UPDATE cat_group_requests SET status=CASE WHEN first_reply_at>0 THEN 'answered' ELSE 'assigned' END,updated_at=? WHERE id=?`, now, next.RequestID)
	if err != nil {
		return err
	}
	if w != nil {
		if err = saveGroupHandoffWorkflow(ctx, tx, w, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ReleaseCatGroupAssignment publishes a held handoff and its graph node together.
// A stop or another reservation wins without changing either record.
func (s *Store) ReleaseCatGroupAssignment(ctx context.Context, id string, w *CatGroupWorkflow) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	chainID := ""
	if w != nil {
		chainID = w.ChainID
	}
	now := time.Now().UnixMilli()
	res, err := tx.ExecContext(ctx, `UPDATE cat_group_assignments SET status='pending',turn_id='',updated_at=? WHERE id=? AND status='held' AND (?='' OR request_id=?) AND EXISTS(SELECT 1 FROM cat_group_requests r WHERE r.id=request_id AND r.status IN('assigned','answered'))`, now, id, chainID, chainID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil || n != 1 {
		return false, err
	}
	if w != nil {
		if err = saveGroupHandoffWorkflow(ctx, tx, w, now); err != nil {
			return false, err
		}
	}
	return true, tx.Commit()
}

func saveGroupHandoffWorkflow(ctx context.Context, tx *sql.Tx, w *CatGroupWorkflow, now int64) error {
	previousRevision := w.Revision
	w.UpdatedAt, w.Revision = now, previousRevision+1
	res, err := tx.ExecContext(ctx, `UPDATE cat_group_workflows SET status=?,data=?,updated_at=? WHERE chain_id=? AND group_id=? AND COALESCE(json_extract(data,'$.revision'),0)=?`, w.Status, marshalJSONString(w), now, w.ChainID, w.GroupID, previousRevision)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("workflow changed during handoff: %s", w.ChainID)
	}
	return nil
}

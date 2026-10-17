package team

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type Repository struct{ DB *sql.DB }
type Tx struct {
	ctx    context.Context
	tx     *sql.Tx
	Events []Event
}
type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func encode(v any) (string, error) { b, e := json.Marshal(v); return string(b), e }
func list[T any](ctx context.Context, q queryer, query string, args ...any) ([]T, error) {
	rows, e := q.QueryContext(ctx, query, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []T{}
	for rows.Next() {
		var raw string
		if e = rows.Scan(&raw); e != nil {
			return nil, e
		}
		var v T
		if e = json.Unmarshal([]byte(raw), &v); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func one[T any](ctx context.Context, q queryer, query string, args ...any) (T, error) {
	var v T
	var raw string
	e := q.QueryRowContext(ctx, query, args...).Scan(&raw)
	if e != nil {
		return v, e
	}
	e = json.Unmarshal([]byte(raw), &v)
	return v, e
}
func (r *Repository) Write(ctx context.Context, fn func(*Tx) error) ([]Event, error) {
	t, e := r.DB.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer t.Rollback()
	u := &Tx{ctx: ctx, tx: t}
	if e = fn(u); e != nil {
		return nil, e
	}
	if e = t.Commit(); e != nil {
		return nil, e
	}
	return u.Events, nil
}
func (t *Tx) exec(query string, args ...any) error {
	_, e := t.tx.ExecContext(t.ctx, query, args...)
	return e
}
func (t *Tx) Human() (Human, error) {
	return one[Human](t.ctx, t.tx, "SELECT data FROM team_profiles WHERE kind='human' LIMIT 1")
}
func (t *Tx) Employee(id string) (Employee, error) {
	return one[Employee](t.ctx, t.tx, "SELECT data FROM team_profiles WHERE id=? AND kind='agent'", id)
}
func (t *Tx) Employees() ([]Employee, error) {
	return list[Employee](t.ctx, t.tx, "SELECT data FROM team_profiles WHERE kind='agent' ORDER BY id")
}
func (t *Tx) SaveHuman(v Human) error {
	b, e := encode(v)
	if e != nil {
		return e
	}
	return t.exec("INSERT INTO team_profiles(id,kind,data) VALUES(?,'human',?) ON CONFLICT(id) DO UPDATE SET data=excluded.data", v.ID, b)
}
func (t *Tx) SaveEmployee(v Employee) error {
	b, e := encode(v)
	if e != nil {
		return e
	}
	return t.exec("INSERT INTO team_profiles(id,kind,data) VALUES(?,'agent',?) ON CONFLICT(id) DO UPDATE SET data=excluded.data", v.ID, b)
}
func (t *Tx) Room(id string) (Room, error) {
	return one[Room](t.ctx, t.tx, "SELECT data FROM team_rooms WHERE id=?", id)
}
func (t *Tx) Rooms() ([]Room, error) {
	return list[Room](t.ctx, t.tx, "SELECT data FROM team_rooms ORDER BY updated_at DESC")
}
func (t *Tx) SaveRoom(v Room) error {
	b, e := encode(v)
	if e != nil {
		return e
	}
	return t.exec("INSERT INTO team_rooms(id,data,updated_at) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data,updated_at=excluded.updated_at", v.ID, b, v.UpdatedAt)
}
func (t *Tx) Members(roomID string) ([]Member, error) {
	return list[Member](t.ctx, t.tx, "SELECT data FROM team_members WHERE room_id=? ORDER BY id", roomID)
}
func (t *Tx) Member(id string) (Member, error) {
	return one[Member](t.ctx, t.tx, "SELECT data FROM team_members WHERE id=?", id)
}
func (t *Tx) SaveMember(v Member) error {
	b, e := encode(v)
	if e != nil {
		return e
	}
	return t.exec("INSERT INTO team_members(id,room_id,profile_id,active,data) VALUES(?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET active=excluded.active,data=excluded.data", v.ID, v.RoomID, v.ProfileID, v.Active, b)
}
func (t *Tx) Message(id string) (Message, error) {
	return one[Message](t.ctx, t.tx, "SELECT data FROM team_messages WHERE id=?", id)
}
func (t *Tx) Messages(roomID string, before int64, limit int) ([]Message, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	if before <= 0 {
		before = 1 << 62
	}
	return list[Message](t.ctx, t.tx, "SELECT data FROM (SELECT data,room_seq FROM team_messages WHERE room_id=? AND room_seq<? ORDER BY room_seq DESC LIMIT ?) ORDER BY room_seq", roomID, before, limit)
}
func (t *Tx) SaveMessage(v *Message) error {
	var previous Message
	if v.RoomSeq != 0 {
		previous, _ = t.Message(v.ID)
	}
	if v.RoomSeq == 0 {
		if e := t.tx.QueryRowContext(t.ctx, "UPDATE team_rooms SET room_seq=room_seq+1,updated_at=? WHERE id=? RETURNING room_seq", time.Now().UnixMilli(), v.RoomID).Scan(&v.RoomSeq); e != nil {
			return e
		}
	}
	v.Revision++
	if v.Mentions == nil {
		v.Mentions = []string{}
	}
	b, e := encode(v)
	if e != nil {
		return e
	}
	if e = t.exec("INSERT INTO team_messages(id,room_id,room_seq,data) VALUES(?,?,?,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data", v.ID, v.RoomID, v.RoomSeq, b); e != nil {
		return e
	}
	if v.Status == "streaming" && previous.ID != "" && strings.HasPrefix(v.Content, previous.Content) {
		return t.Event(v.RoomID, "message_delta", map[string]any{"id": v.ID, "revision": v.Revision, "contentDelta": strings.TrimPrefix(v.Content, previous.Content)})
	}
	return t.Event(v.RoomID, "message", v)
}
func (t *Tx) Task(id string) (Task, error) {
	return one[Task](t.ctx, t.tx, "SELECT data FROM team_tasks WHERE id=?", id)
}
func (t *Tx) Tasks(roomID string) ([]Task, error) {
	return list[Task](t.ctx, t.tx, "SELECT data FROM team_tasks WHERE room_id=? ORDER BY CASE WHEN status IN ('queued','running','starting','stopping','waiting_user','waiting_agent') THEN 0 ELSE 1 END,created_at DESC LIMIT 600", roomID)
}
func (t *Tx) AllPending() ([]Task, error) {
	return list[Task](t.ctx, t.tx, "SELECT data FROM team_tasks WHERE status IN ('queued','waiting_agent','waiting_user','running','starting','stopping') ORDER BY created_at LIMIT 2000")
}
func (t *Tx) SaveTask(v *Task) error {
	v.Revision++
	v.UpdatedAt = time.Now().UnixMilli()
	if v.Acceptance == nil {
		v.Acceptance = []string{}
	}
	if v.DependsOn == nil {
		v.DependsOn = []string{}
	}
	if v.WaitFor == nil {
		v.WaitFor = []string{}
	}
	b, e := encode(v)
	if e != nil {
		return e
	}
	if e = t.exec("INSERT INTO team_tasks(id,room_id,member_id,status,parent_id,created_at,data) VALUES(?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET status=excluded.status,data=excluded.data", v.ID, v.RoomID, v.MemberID, v.Status, v.ParentID, v.CreatedAt, b); e != nil {
		return e
	}
	return t.Event(v.RoomID, "task", v)
}
func (t *Tx) Inbox(roomID string) ([]InboxItem, error) {
	if roomID == "" {
		return list[InboxItem](t.ctx, t.tx, "SELECT data FROM team_inbox ORDER BY created_at DESC LIMIT 500")
	}
	return list[InboxItem](t.ctx, t.tx, "SELECT data FROM team_inbox WHERE room_id=? ORDER BY created_at DESC LIMIT 500", roomID)
}
func (t *Tx) SaveInbox(v InboxItem) error {
	b, e := encode(v)
	if e != nil {
		return e
	}
	if e = t.exec("INSERT INTO team_inbox(id,room_id,message_id,recipient_id,read_at,created_at,data) VALUES(?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET read_at=excluded.read_at,data=excluded.data", v.ID, v.RoomID, v.MessageID, v.RecipientID, v.ReadAt, v.CreatedAt, b); e != nil {
		return e
	}
	return t.Event(v.RoomID, "inbox", v)
}
func (t *Tx) Event(roomID, kind string, v any) error {
	var seq int64
	if e := t.tx.QueryRowContext(t.ctx, "UPDATE team_rooms SET event_seq=event_seq+1 WHERE id=? RETURNING event_seq", roomID).Scan(&seq); e != nil {
		return e
	}
	b, e := encode(v)
	if e != nil {
		return e
	}
	if e = t.exec("INSERT INTO team_events(room_id,event_seq,kind,data) VALUES(?,?,?,?)", roomID, seq, kind, b); e != nil {
		return e
	}
	if seq > 20000 && seq%500 == 0 {
		if e = t.exec("DELETE FROM team_events WHERE room_id=? AND event_seq<=?", roomID, seq-20000); e != nil {
			return e
		}
	}
	// Copy mutable pointers before the transaction caller reuses them.
	var frozen any
	if e = json.Unmarshal([]byte(b), &frozen); e != nil {
		return e
	}
	t.Events = append(t.Events, Event{roomID, seq, kind, frozen})
	return nil
}
func (t *Tx) Command(source, id, hash string) (CommandResult, bool, error) {
	var oldHash, raw string
	e := t.tx.QueryRowContext(t.ctx, "SELECT content_hash,result FROM team_commands WHERE source_id=? AND request_id=?", source, id).Scan(&oldHash, &raw)
	if errors.Is(e, sql.ErrNoRows) {
		return CommandResult{}, false, nil
	}
	if e != nil {
		return CommandResult{}, false, e
	}
	if oldHash != hash {
		return CommandResult{}, false, errors.New("请求 ID 已用于不同内容")
	}
	var v CommandResult
	e = json.Unmarshal([]byte(raw), &v)
	return v, true, e
}
func (t *Tx) SaveCommand(source, id, hash string, v CommandResult) error {
	b, e := encode(v)
	if e != nil {
		return e
	}
	return t.exec("INSERT INTO team_commands(source_id,request_id,content_hash,result) VALUES(?,?,?,?)", source, id, hash, b)
}
func (t *Tx) Outbox(id string, v Notification) error {
	b, e := encode(v)
	if e != nil {
		return e
	}
	return t.exec("INSERT INTO team_notification_outbox(id,data) VALUES(?,?) ON CONFLICT(id) DO NOTHING", id, b)
}
func (r *Repository) Bootstrap(ctx context.Context) (Bootstrap, error) {
	v := Bootstrap{}
	_, e := r.Write(ctx, func(t *Tx) (e error) {
		if v.Human, e = t.Human(); e != nil {
			return
		}
		if v.Employees, e = t.Employees(); e != nil {
			return
		}
		if v.Rooms, e = t.Rooms(); e != nil {
			return
		}
		v.Inbox, e = t.Inbox("")
		return
	})
	return v, e
}
func (r *Repository) Snapshot(ctx context.Context, roomID string, before int64) (Snapshot, error) {
	v := Snapshot{}
	_, e := r.Write(ctx, func(t *Tx) (e error) {
		if v.Room, e = t.Room(roomID); e != nil {
			return
		}
		if v.Members, e = t.Members(roomID); e != nil {
			return
		}
		if v.Messages, e = t.Messages(roomID, before, 100); e != nil {
			return
		}
		if v.Tasks, e = t.Tasks(roomID); e != nil {
			return
		}
		if v.Inbox, e = t.Inbox(roomID); e != nil {
			return
		}
		if v.Artifacts, e = list[Artifact](ctx, t.tx, "SELECT data FROM team_artifacts WHERE room_id=? LIMIT 500", roomID); e != nil {
			return
		}
		if len(v.Messages) > 0 {
			var n int
			e = t.tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM team_messages WHERE room_id=? AND room_seq<?)", roomID, v.Messages[0].RoomSeq).Scan(&n)
			v.HasEarlier = n != 0
			if e != nil {
				return
			}
		}
		return t.tx.QueryRowContext(ctx, "SELECT event_seq FROM team_rooms WHERE id=?", roomID).Scan(&v.EventSeq)
	})
	return v, e
}
func (r *Repository) Events(ctx context.Context, roomID string, after int64) ([]Event, error) {
	rows, e := r.DB.QueryContext(ctx, "SELECT event_seq,kind,data FROM team_events WHERE room_id=? AND event_seq>? ORDER BY event_seq LIMIT 250", roomID, after)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		v := Event{RoomID: roomID}
		var raw string
		if e = rows.Scan(&v.EventSeq, &v.Kind, &raw); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(raw), &v.Data); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

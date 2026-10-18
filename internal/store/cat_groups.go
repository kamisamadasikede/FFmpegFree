package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"FFmpegFree/internal/id"
)

// CatGroup is a project group chat (契约 6.21.2).
type CatGroup struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	Name      string `json:"name"`
	AutoRoute bool   `json:"autoRoute"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
	// Members 只读成员摘要（契约 6.21.17 ②）：按 order 升序，不含 charter / session；新建群为空数组。
	Members []CatGroupMemberBrief `json:"members"`
}

// CatGroupMemberBrief 是侧栏头像用的成员摘要（契约 6.21.17 ②）。
type CatGroupMemberBrief struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Title       string `json:"title"`
	AgentKind   string `json:"agentKind"`
	Order       int    `json:"order"`
	AvatarIndex int    `json:"avatarIndex"` // (order % 5) + 1 → --cat-member-{avatarIndex}
}

// CatMemberAvatarIndex 与前端 catGroupChat.ts 同一公式：(order % 5) + 1。
func CatMemberAvatarIndex(order int) int {
	if order < 0 {
		order = -order
	}
	return order%5 + 1
}

// CatGroupQueuedTask 是成员排队中的一条任务（契约 6.21.17 ④，不落库）。
type CatGroupQueuedTask struct {
	ChainID       string `json:"chainId"`
	Hop           int    `json:"hop"`
	FromSeq       int64  `json:"fromSeq"`
	QueuePosition int    `json:"queuePosition"`
}

// CatCharter is a member's role spec.
type CatCharter struct {
	Status     string   `json:"status"`
	Owns       string   `json:"owns"`
	NotTouch   string   `json:"notTouch"`
	Handoff    string   `json:"handoff"`
	Dirs       []string `json:"dirs"`
	Edited     []string `json:"edited"`
	FailReason string   `json:"failReason,omitempty"`
	UpdatedAt  int64    `json:"updatedAt"`
}

// CatGroupMember is one employee in a group.
type CatGroupMember struct {
	ID          string     `json:"id"`
	GroupID     string     `json:"groupId"`
	Name        string     `json:"name"`
	Title       string     `json:"title"`
	AgentKind   string     `json:"agentKind"`
	ModelID     string     `json:"modelId"`
	ThinkLevel  string     `json:"thinkLevelId,omitempty"`
	Order       int        `json:"order"`
	SessionID   string     `json:"-"`
	LastSeenSeq int64      `json:"-"`
	Charter     CatCharter `json:"charter"`
	CreatedAt   int64      `json:"createdAt,omitempty"`
	UpdatedAt   int64      `json:"updatedAt,omitempty"`
	// 运行时只读（契约 6.21.17 ④，不落库）：由 service 层按内存队列填。
	Status      string               `json:"status"`      // idle | running | queued
	QueueLength int                  `json:"queueLength"` // 不含正在跑的这一条
	Queue       []CatGroupQueuedTask `json:"queue"`
}

// CatRouteInfo is structured source for a system gray line / 协调气泡 (v0.57).
type CatRouteInfo struct {
	Mode     string `json:"mode"`               // mention|followup|ai|fallback|undecided|off|reassign|done|hop_limit|stopped|failed|retry
	MemberID string `json:"memberId,omitempty"` // 兼容旧读；写入时优先 MemberIDs
	// MemberIDs：v0.57 mode=ai 成功时的目标成员（1～N）；与协调气泡 mentions 一致。
	MemberIDs []string `json:"memberIds,omitempty"`
	// Everyone：判为 @全体 时为 true（后端已展开为全员 MemberIDs）。
	Everyone bool   `json:"everyone,omitempty"`
	Reason   string `json:"reason,omitempty"`
	RouteSeq int64  `json:"routeSeq,omitempty"`
	// DurationMs 派活判断耗时（契约 6.21.17 ③ / 6.21.18）；ai 成功与判断失败落的 undecided 有；旧 fallback 灰字也可能有。
	DurationMs int64 `json:"durationMs,omitempty"`
	// Queued 入队灰字标记（契约 6.21.17 ④ 5）：「交给 {成员}：正在干活，排在后面」。
	Queued bool `json:"queued,omitempty"`
}

// CatMessageFooter reuses ordinary conversation message-tail fields on a report card.
type CatMessageFooter struct {
	FinishedAt   int64    `json:"finishedAt,omitempty"`
	FirstTokenMs *int64   `json:"firstTokenMs,omitempty"`
	TotalMs      *int64   `json:"totalMs,omitempty"`
	TokenTotal   *int64   `json:"tokenTotal,omitempty"`
	OutputTokens *int64   `json:"outputTokens,omitempty"`
	TokensPerSec *float64 `json:"tokensPerSec,omitempty"`
}

// CatGroupReport is the parsed report card (契约 6.21.15).
type CatGroupReport struct {
	Did    string            `json:"did"`
	Attn   string            `json:"attn,omitempty"`
	Give   string            `json:"give,omitempty"`
	Files  []CatGroupFileTag `json:"files"`
	Parsed bool              `json:"parsed"`
	Footer *CatMessageFooter `json:"footer,omitempty"`
}

// CatGroupFileTag is a file listed on a report, with overreach tags.
type CatGroupFileTag struct {
	Path    string `json:"path"`
	Tag     string `json:"tag,omitempty"` // "" | "overreach" | "unknown"
	Added   int    `json:"added,omitempty"`
	Deleted int    `json:"deleted,omitempty"`
}

// CatGroupMessage is one row in the group stream.
type CatGroupMessage struct {
	Seq          int64           `json:"seq"`
	GroupID      string          `json:"groupId"`
	Kind         string          `json:"kind"` // user|member|system
	MemberID     string          `json:"memberId,omitempty"`
	Text         string          `json:"text"`
	Mentions     []string        `json:"mentions,omitempty"`
	ChainID      string          `json:"chainId"`
	Hop          int             `json:"hop"`
	TurnID       string          `json:"turnId,omitempty"`
	Route        *CatRouteInfo   `json:"route,omitempty"`
	SystemKind   string          `json:"systemKind,omitempty"` // route|hop_limit|no_handoff|reassign|failed|stopped|charter|coord
	FromMemberID string          `json:"fromMemberId,omitempty"`
	ToMemberID   string          `json:"toMemberId,omitempty"`
	FailReason   string          `json:"failReason,omitempty"`
	Report       *CatGroupReport `json:"report,omitempty"`
	// ReplyKind：v0.56 短复 vs 汇报卡（契约 6.21.19；FE #346）。
	// "short" | "report"；短复亦可省略 Report。不落库——重载靠「无 report + 有 text」识别。
	ReplyKind string `json:"replyKind,omitempty"`
	ShortText string `json:"shortText,omitempty"`
	CreatedAt int64  `json:"createdAt"`
}

// CatGroupRouteDetail is GetCatRouteDetail payload.
type CatGroupRouteDetail struct {
	RouteSeq   int64  `json:"routeSeq"`
	GroupID    string `json:"groupId"`
	ChainID    string `json:"chainId"`
	Mode       string `json:"mode"`
	MemberID   string `json:"memberId"`
	Reason     string `json:"reason"`
	InputText  string `json:"inputText"`
	OutputText string `json:"outputText"`
	CreatedAt  int64  `json:"createdAt"`
	StartedAt  int64  `json:"startedAt"`  // 发「正在安排谁来接…」时（unix ms），旧数据 0
	FinishedAt int64  `json:"finishedAt"` // 拿到结果 / 超时 / 出错时（unix ms），旧数据 0
}

// CatGroupTurnMeta persists overreach / snapshot state for a member turn.
type CatGroupTurnMeta struct {
	TurnID         string
	GroupID        string
	MemberID       string
	ChainID        string
	Hop            int
	SnapshotBefore string
	SnapshotAfter  string
	ClaimedFiles   []string
	OverreachJSON  string
	EndHashes      map[string]string
	CreatedAt      int64
}

func marshalJSONString(v any) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func decodeStringSlice(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	var out []string
	if json.Unmarshal([]byte(raw), &out) != nil {
		return nil
	}
	return out
}

func decodeCharter(raw string) CatCharter {
	c := CatCharter{Status: "none", Dirs: []string{}, Edited: []string{}}
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return c
	}
	_ = json.Unmarshal([]byte(raw), &c)
	if c.Dirs == nil {
		c.Dirs = []string{}
	}
	if c.Edited == nil {
		c.Edited = []string{}
	}
	if c.Status == "" {
		c.Status = "none"
	}
	return c
}

func encodeCharter(c CatCharter) string {
	if c.Dirs == nil {
		c.Dirs = []string{}
	}
	if c.Edited == nil {
		c.Edited = []string{}
	}
	return marshalJSONString(c)
}

func decodeRoute(raw string) *CatRouteInfo {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	var r CatRouteInfo
	if json.Unmarshal([]byte(raw), &r) != nil || r.Mode == "" {
		return nil
	}
	// v0.57：旧单字段 memberId 兼容读入 → memberIds
	if len(r.MemberIDs) == 0 && strings.TrimSpace(r.MemberID) != "" {
		// mention 模式可能用逗号拼接多人；拆开
		parts := strings.Split(r.MemberID, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				r.MemberIDs = append(r.MemberIDs, p)
			}
		}
	}
	return &r
}

func decodeReport(raw string) *CatGroupReport {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	var r CatGroupReport
	if json.Unmarshal([]byte(raw), &r) != nil {
		return nil
	}
	if r.Files == nil {
		r.Files = []CatGroupFileTag{}
	}
	return &r
}

// ---------- groups ----------

func (s *Store) InsertCatGroup(ctx context.Context, g CatGroup) (CatGroup, error) {
	if g.ID == "" {
		g.ID = id.New()
	}
	now := time.Now().UnixMilli()
	if g.CreatedAt == 0 {
		g.CreatedAt = now
	}
	if g.UpdatedAt == 0 {
		g.UpdatedAt = g.CreatedAt
	}
	if g.Members == nil {
		g.Members = []CatGroupMemberBrief{}
	}
	auto := 1
	if !g.AutoRoute {
		auto = 0
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO cat_groups (id, project_id, name, auto_route, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		g.ID, g.ProjectID, g.Name, auto, g.CreatedAt, g.UpdatedAt)
	if err != nil {
		return CatGroup{}, fmt.Errorf("写入群聊失败: %w", err)
	}
	return g, nil
}

func (s *Store) ListCatGroups(ctx context.Context, projectID string) ([]CatGroup, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, name, auto_route, created_at, updated_at
		FROM cat_groups WHERE project_id = ? ORDER BY updated_at DESC, created_at DESC, id DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("列出群聊失败: %w", err)
	}
	defer rows.Close()
	out := []CatGroup{}
	for rows.Next() {
		var g CatGroup
		var auto int
		if err := rows.Scan(&g.ID, &g.ProjectID, &g.Name, &auto, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		g.AutoRoute = auto != 0
		g.Members = []CatGroupMemberBrief{}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) GetCatGroup(ctx context.Context, groupID string) (CatGroup, error) {
	var g CatGroup
	var auto int
	err := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, name, auto_route, created_at, updated_at
		FROM cat_groups WHERE id = ?`, groupID).Scan(&g.ID, &g.ProjectID, &g.Name, &auto, &g.CreatedAt, &g.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return CatGroup{}, err
	}
	if err != nil {
		return CatGroup{}, fmt.Errorf("读取群聊失败: %w", err)
	}
	g.AutoRoute = auto != 0
	g.Members = []CatGroupMemberBrief{}
	return g, nil
}

func (s *Store) RenameCatGroup(ctx context.Context, groupID, name string) error {
	now := time.Now().UnixMilli()
	res, err := s.db.ExecContext(ctx, `UPDATE cat_groups SET name = ?, updated_at = ? WHERE id = ?`, name, now, groupID)
	if err != nil {
		return fmt.Errorf("重命名群聊失败: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) SetCatGroupAutoRoute(ctx context.Context, groupID string, on bool) error {
	now := time.Now().UnixMilli()
	auto := 0
	if on {
		auto = 1
	}
	res, err := s.db.ExecContext(ctx, `UPDATE cat_groups SET auto_route = ?, updated_at = ? WHERE id = ?`, auto, now, groupID)
	if err != nil {
		return fmt.Errorf("更新群聊开关失败: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) TouchCatGroup(ctx context.Context, groupID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE cat_groups SET updated_at = ? WHERE id = ?`, time.Now().UnixMilli(), groupID)
	return err
}

func (s *Store) DeleteCatGroup(ctx context.Context, groupID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM cat_group_turn_meta WHERE group_id = ?`, groupID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM cat_group_routes WHERE group_id = ?`, groupID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM cat_group_messages WHERE group_id = ?`, groupID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM cat_group_members WHERE group_id = ?`, groupID); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM cat_groups WHERE id = ?`, groupID)
	if err != nil {
		return fmt.Errorf("删除群聊失败: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return tx.Commit()
}

// ---------- members ----------

func (s *Store) InsertCatGroupMember(ctx context.Context, m CatGroupMember) (CatGroupMember, error) {
	if m.ID == "" {
		m.ID = id.New()
	}
	now := time.Now().UnixMilli()
	if m.CreatedAt == 0 {
		m.CreatedAt = now
	}
	if m.UpdatedAt == 0 {
		m.UpdatedAt = m.CreatedAt
	}
	if m.Charter.Status == "" {
		m.Charter.Status = "none"
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO cat_group_members (
			id, group_id, name, title, agent_kind, model_id, think_level, sort_order,
			session_id, last_seen_seq, charter_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.GroupID, m.Name, m.Title, m.AgentKind, m.ModelID, m.ThinkLevel, m.Order,
		m.SessionID, m.LastSeenSeq, encodeCharter(m.Charter), m.CreatedAt, m.UpdatedAt)
	if err != nil {
		if isUniqueNameErr(err) {
			return CatGroupMember{}, errMemberNameTaken
		}
		return CatGroupMember{}, fmt.Errorf("写入群成员失败: %w", err)
	}
	_ = s.TouchCatGroup(ctx, m.GroupID)
	return m, nil
}

var errMemberNameTaken = errors.New("member_name_taken")

func IsMemberNameTaken(err error) bool {
	return errors.Is(err, errMemberNameTaken)
}

func isUniqueNameErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") || strings.Contains(msg, "constraint")
}

func scanMember(r interface{ Scan(...any) error }) (CatGroupMember, error) {
	var m CatGroupMember
	var charter string
	err := r.Scan(
		&m.ID, &m.GroupID, &m.Name, &m.Title, &m.AgentKind, &m.ModelID, &m.ThinkLevel, &m.Order,
		&m.SessionID, &m.LastSeenSeq, &charter, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return CatGroupMember{}, err
	}
	m.Charter = decodeCharter(charter)
	m.Status = "idle"
	m.Queue = []CatGroupQueuedTask{}
	return m, nil
}

const memberCols = `id, group_id, name, title, agent_kind, model_id, think_level, sort_order, session_id, last_seen_seq, charter_json, created_at, updated_at`

func (s *Store) ListCatGroupMembers(ctx context.Context, groupID string) ([]CatGroupMember, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+memberCols+` FROM cat_group_members
		WHERE group_id = ? ORDER BY sort_order ASC, created_at ASC, id ASC`, groupID)
	if err != nil {
		return nil, fmt.Errorf("列出群成员失败: %w", err)
	}
	defer rows.Close()
	out := []CatGroupMember{}
	for rows.Next() {
		m, err := scanMember(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) GetCatGroupMember(ctx context.Context, memberID string) (CatGroupMember, error) {
	m, err := scanMember(s.db.QueryRowContext(ctx, `SELECT `+memberCols+` FROM cat_group_members WHERE id = ?`, memberID))
	if errors.Is(err, sql.ErrNoRows) {
		return CatGroupMember{}, err
	}
	if err != nil {
		return CatGroupMember{}, fmt.Errorf("读取群成员失败: %w", err)
	}
	return m, nil
}

func (s *Store) UpdateCatGroupMember(ctx context.Context, m CatGroupMember) error {
	now := time.Now().UnixMilli()
	res, err := s.db.ExecContext(ctx, `
		UPDATE cat_group_members SET
			name = ?, title = ?, agent_kind = ?, model_id = ?, think_level = ?,
			sort_order = ?, session_id = ?, last_seen_seq = ?, charter_json = ?, updated_at = ?
		WHERE id = ?`,
		m.Name, m.Title, m.AgentKind, m.ModelID, m.ThinkLevel,
		m.Order, m.SessionID, m.LastSeenSeq, encodeCharter(m.Charter), now, m.ID)
	if err != nil {
		if isUniqueNameErr(err) {
			return errMemberNameTaken
		}
		return fmt.Errorf("更新群成员失败: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	_ = s.TouchCatGroup(ctx, m.GroupID)
	return nil
}

func (s *Store) UpdateCatGroupMemberCharter(ctx context.Context, memberID string, c CatCharter) error {
	now := time.Now().UnixMilli()
	c.UpdatedAt = now
	res, err := s.db.ExecContext(ctx, `
		UPDATE cat_group_members SET charter_json = ?, updated_at = ? WHERE id = ?`,
		encodeCharter(c), now, memberID)
	if err != nil {
		return fmt.Errorf("更新职责规范失败: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) UpdateCatGroupMemberLastSeen(ctx context.Context, memberID string, seq int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE cat_group_members SET last_seen_seq = ? WHERE id = ?`, seq, memberID)
	return err
}

func (s *Store) UpdateCatGroupMemberSession(ctx context.Context, memberID, sessionID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE cat_group_members SET session_id = ? WHERE id = ?`, sessionID, memberID)
	return err
}

func (s *Store) RemoveCatGroupMember(ctx context.Context, memberID string) error {
	var groupID string
	_ = s.db.QueryRowContext(ctx, `SELECT group_id FROM cat_group_members WHERE id = ?`, memberID).Scan(&groupID)
	res, err := s.db.ExecContext(ctx, `DELETE FROM cat_group_members WHERE id = ?`, memberID)
	if err != nil {
		return fmt.Errorf("删除群成员失败: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	if groupID != "" {
		_ = s.TouchCatGroup(ctx, groupID)
	}
	return nil
}

func (s *Store) ReorderCatGroupMembers(ctx context.Context, groupID string, ids []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, idv := range ids {
		res, err := tx.ExecContext(ctx, `UPDATE cat_group_members SET sort_order = ? WHERE id = ? AND group_id = ?`, i, idv, groupID)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return sql.ErrNoRows
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE cat_groups SET updated_at = ? WHERE id = ?`, time.Now().UnixMilli(), groupID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) NextCatGroupMemberOrder(ctx context.Context, groupID string) (int, error) {
	var n sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT MAX(sort_order) FROM cat_group_members WHERE group_id = ?`, groupID).Scan(&n)
	if err != nil {
		return 0, err
	}
	if !n.Valid {
		return 0, nil
	}
	return int(n.Int64) + 1, nil
}

// ---------- messages ----------

func (s *Store) NextCatGroupMessageSeq(ctx context.Context, groupID string) (int64, error) {
	var n sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT MAX(seq) FROM cat_group_messages WHERE group_id = ?`, groupID).Scan(&n)
	if err != nil {
		return 0, err
	}
	if !n.Valid {
		return 1, nil
	}
	return n.Int64 + 1, nil
}

// groupMsgSeqMu 把「取下一个 seq + 插入」串起来：成员并行跑完同时写群消息时，
// 两条拿到同一个 seq 会主键冲突，汇报卡被静默丢掉（e2e @ 两人并行复现）。
var groupMsgSeqMu sync.Mutex

func (s *Store) InsertCatGroupMessage(ctx context.Context, m CatGroupMessage) (CatGroupMessage, error) {
	groupMsgSeqMu.Lock()
	defer groupMsgSeqMu.Unlock()
	if m.Seq == 0 {
		seq, err := s.NextCatGroupMessageSeq(ctx, m.GroupID)
		if err != nil {
			return CatGroupMessage{}, err
		}
		m.Seq = seq
	}
	if m.CreatedAt == 0 {
		m.CreatedAt = time.Now().UnixMilli()
	}
	mentions := marshalJSONString(m.Mentions)
	if m.Mentions == nil {
		mentions = "[]"
	}
	route := ""
	if m.Route != nil {
		route = marshalJSONString(m.Route)
	}
	report := ""
	if m.Report != nil {
		report = marshalJSONString(m.Report)
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO cat_group_messages (
			group_id, seq, kind, member_id, text, mentions, chain_id, hop, turn_id, route_json, report_json,
			system_kind, from_member_id, to_member_id, fail_reason, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.GroupID, m.Seq, m.Kind, m.MemberID, m.Text, mentions, m.ChainID, m.Hop, m.TurnID, route, report,
		m.SystemKind, m.FromMemberID, m.ToMemberID, m.FailReason, m.CreatedAt)
	if err != nil {
		return CatGroupMessage{}, fmt.Errorf("写入群消息失败: %w", err)
	}
	_ = s.TouchCatGroup(ctx, m.GroupID)
	return m, nil
}

func scanGroupMessage(r interface{ Scan(...any) error }) (CatGroupMessage, error) {
	var m CatGroupMessage
	var mentions, route, report string
	err := r.Scan(
		&m.GroupID, &m.Seq, &m.Kind, &m.MemberID, &m.Text, &mentions,
		&m.ChainID, &m.Hop, &m.TurnID, &route, &report, &m.SystemKind,
		&m.FromMemberID, &m.ToMemberID, &m.FailReason, &m.CreatedAt,
	)
	if err != nil {
		return CatGroupMessage{}, err
	}
	m.Mentions = decodeStringSlice(mentions)
	m.Route = decodeRoute(route)
	m.Report = decodeReport(report)
	return m, nil
}

const msgCols = `group_id, seq, kind, member_id, text, mentions, chain_id, hop, turn_id, route_json, report_json, system_kind, from_member_id, to_member_id, fail_reason, created_at`

func (s *Store) ListCatGroupMessages(ctx context.Context, groupID string, beforeSeq int64, limit int) ([]CatGroupMessage, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	var rows *sql.Rows
	var err error
	if beforeSeq > 0 {
		rows, err = s.db.QueryContext(ctx, `
			SELECT `+msgCols+` FROM cat_group_messages
			WHERE group_id = ? AND seq < ? ORDER BY seq DESC LIMIT ?`, groupID, beforeSeq, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT `+msgCols+` FROM cat_group_messages
			WHERE group_id = ? ORDER BY seq DESC LIMIT ?`, groupID, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("列出群消息失败: %w", err)
	}
	defer rows.Close()
	out := []CatGroupMessage{}
	for rows.Next() {
		m, err := scanGroupMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// reverse to ascending
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func (s *Store) ListCatGroupMessagesAfter(ctx context.Context, groupID string, afterSeq int64, limit int) ([]CatGroupMessage, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+msgCols+` FROM cat_group_messages
		WHERE group_id = ? AND seq > ? ORDER BY seq ASC LIMIT ?`, groupID, afterSeq, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CatGroupMessage{}
	for rows.Next() {
		m, err := scanGroupMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) GetCatGroupMessage(ctx context.Context, groupID string, seq int64) (CatGroupMessage, error) {
	m, err := scanGroupMessage(s.db.QueryRowContext(ctx, `SELECT `+msgCols+` FROM cat_group_messages WHERE group_id = ? AND seq = ?`, groupID, seq))
	if errors.Is(err, sql.ErrNoRows) {
		return CatGroupMessage{}, err
	}
	if err != nil {
		return CatGroupMessage{}, err
	}
	return m, nil
}

// UpdateCatGroupMessage replaces text/systemKind/mentions/route on an existing row (same seq).
// Used by v0.57 to replace loading grey「正在安排谁来接…」with 协调 bubble / undecided.
func (s *Store) UpdateCatGroupMessage(ctx context.Context, m CatGroupMessage) (CatGroupMessage, error) {
	if m.GroupID == "" || m.Seq == 0 {
		return CatGroupMessage{}, fmt.Errorf("更新群消息失败: 缺少 groupId/seq")
	}
	mentions := marshalJSONString(m.Mentions)
	if m.Mentions == nil {
		mentions = "[]"
	}
	route := ""
	if m.Route != nil {
		route = marshalJSONString(m.Route)
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE cat_group_messages SET
			kind = ?, member_id = ?, text = ?, mentions = ?, hop = ?,
			route_json = ?, system_kind = ?, from_member_id = ?, to_member_id = ?, fail_reason = ?
		WHERE group_id = ? AND seq = ?`,
		m.Kind, m.MemberID, m.Text, mentions, m.Hop,
		route, m.SystemKind, m.FromMemberID, m.ToMemberID, m.FailReason,
		m.GroupID, m.Seq)
	if err != nil {
		return CatGroupMessage{}, fmt.Errorf("更新群消息失败: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return CatGroupMessage{}, fmt.Errorf("更新群消息失败: 找不到 seq=%d", m.Seq)
	}
	_ = s.TouchCatGroup(ctx, m.GroupID)
	return m, nil
}

func (s *Store) CountCatGroupChainHops(ctx context.Context, groupID, chainID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM cat_group_messages
		WHERE group_id = ? AND chain_id = ? AND kind = 'member'`, groupID, chainID).Scan(&n)
	return n, err
}

// ---------- routes ----------

func (s *Store) InsertCatGroupRoute(ctx context.Context, d CatGroupRouteDetail) (CatGroupRouteDetail, error) {
	if d.CreatedAt == 0 {
		d.CreatedAt = time.Now().UnixMilli()
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO cat_group_routes (group_id, chain_id, mode, member_id, reason, input_text, output_text, created_at, started_at, finished_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.GroupID, d.ChainID, d.Mode, d.MemberID, d.Reason, d.InputText, d.OutputText, d.CreatedAt, d.StartedAt, d.FinishedAt)
	if err != nil {
		return CatGroupRouteDetail{}, err
	}
	seq, err := res.LastInsertId()
	if err != nil {
		return CatGroupRouteDetail{}, err
	}
	d.RouteSeq = seq
	return d, nil
}

func (s *Store) GetCatGroupRoute(ctx context.Context, routeSeq int64) (CatGroupRouteDetail, error) {
	var d CatGroupRouteDetail
	err := s.db.QueryRowContext(ctx, `
		SELECT route_seq, group_id, chain_id, mode, member_id, reason, input_text, output_text, created_at, started_at, finished_at
		FROM cat_group_routes WHERE route_seq = ?`, routeSeq).Scan(
		&d.RouteSeq, &d.GroupID, &d.ChainID, &d.Mode, &d.MemberID, &d.Reason, &d.InputText, &d.OutputText, &d.CreatedAt,
		&d.StartedAt, &d.FinishedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return CatGroupRouteDetail{}, err
	}
	return d, err
}

// ---------- turn meta ----------

func (s *Store) SaveCatGroupTurnMeta(ctx context.Context, m CatGroupTurnMeta) error {
	if m.CreatedAt == 0 {
		m.CreatedAt = time.Now().UnixMilli()
	}
	claimed := marshalJSONString(m.ClaimedFiles)
	if m.ClaimedFiles == nil {
		claimed = "[]"
	}
	hashes := marshalJSONString(m.EndHashes)
	if m.EndHashes == nil {
		hashes = "{}"
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO cat_group_turn_meta (
			turn_id, group_id, member_id, chain_id, hop, snapshot_before, snapshot_after,
			claimed_files, overreach_json, end_hashes, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(turn_id) DO UPDATE SET
			snapshot_after = excluded.snapshot_after,
			claimed_files = excluded.claimed_files,
			overreach_json = excluded.overreach_json,
			end_hashes = excluded.end_hashes`,
		m.TurnID, m.GroupID, m.MemberID, m.ChainID, m.Hop, m.SnapshotBefore, m.SnapshotAfter,
		claimed, m.OverreachJSON, hashes, m.CreatedAt)
	return err
}

func (s *Store) GetCatGroupTurnMeta(ctx context.Context, turnID string) (CatGroupTurnMeta, error) {
	var m CatGroupTurnMeta
	var claimed, hashes string
	err := s.db.QueryRowContext(ctx, `
		SELECT turn_id, group_id, member_id, chain_id, hop, snapshot_before, snapshot_after,
			claimed_files, overreach_json, end_hashes, created_at
		FROM cat_group_turn_meta WHERE turn_id = ?`, turnID).Scan(
		&m.TurnID, &m.GroupID, &m.MemberID, &m.ChainID, &m.Hop, &m.SnapshotBefore, &m.SnapshotAfter,
		&claimed, &m.OverreachJSON, &hashes, &m.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return CatGroupTurnMeta{}, err
	}
	if err != nil {
		return CatGroupTurnMeta{}, err
	}
	m.ClaimedFiles = decodeStringSlice(claimed)
	m.EndHashes = map[string]string{}
	_ = json.Unmarshal([]byte(hashes), &m.EndHashes)
	return m, nil
}

// IsCatGroupMemberConv reports whether convID is a group member's backing conversation id (same as member id).
func (s *Store) IsCatGroupMemberConv(ctx context.Context, convID string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM cat_group_members WHERE id = ?`, convID).Scan(&n)
	return n > 0, err
}

// ListCatGroupMemberIDs returns all member ids (for filtering conversation lists).
func (s *Store) ListCatGroupMemberIDs(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM cat_group_members`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var idv string
		if err := rows.Scan(&idv); err != nil {
			return nil, err
		}
		out[idv] = true
	}
	return out, rows.Err()
}

// ListCatGroupMemberBriefs 一条 SQL 带出多个群的成员摘要（契约 6.21.17 ②），按 order 升序。
// 返回 map 里每个请求的 groupID 都有值（没成员为空数组）。
func (s *Store) ListCatGroupMemberBriefs(ctx context.Context, groupIDs []string) (map[string][]CatGroupMemberBrief, error) {
	out := make(map[string][]CatGroupMemberBrief, len(groupIDs))
	if len(groupIDs) == 0 {
		return out, nil
	}
	args := make([]any, 0, len(groupIDs))
	for _, g := range groupIDs {
		out[g] = []CatGroupMemberBrief{}
		args = append(args, g)
	}
	q := `SELECT group_id, id, name, title, agent_kind, sort_order FROM cat_group_members
		WHERE group_id IN (?` + strings.Repeat(",?", len(groupIDs)-1) + `)
		ORDER BY group_id, sort_order ASC, created_at ASC, id ASC`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("列出群成员失败: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var gid string
		var b CatGroupMemberBrief
		if err := rows.Scan(&gid, &b.ID, &b.Name, &b.Title, &b.AgentKind, &b.Order); err != nil {
			return nil, err
		}
		b.AvatarIndex = CatMemberAvatarIndex(b.Order)
		out[gid] = append(out[gid], b)
	}
	return out, rows.Err()
}

// ListCatGroupReportMessages 返回本群所有带汇报的 kind=member 消息（seq 升序），供「文件」页签聚合（契约 6.21.17 ①）。
func (s *Store) ListCatGroupReportMessages(ctx context.Context, groupID string) ([]CatGroupMessage, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+msgCols+` FROM cat_group_messages
		WHERE group_id = ? AND kind = 'member' AND report_json != '' ORDER BY seq ASC`, groupID)
	if err != nil {
		return nil, fmt.Errorf("列出群消息失败: %w", err)
	}
	defer rows.Close()
	out := []CatGroupMessage{}
	for rows.Next() {
		m, err := scanGroupMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"FFmpegFree/internal/id"
)

// CatConversation 是契约 6.19.4 的会话行。
type CatConversation struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	AgentKind  string `json:"agentKind"`
	AccessMode string `json:"accessMode"` // ask | full（一期只允许 ask）
	// ProjectPath 自 v0.31 作废（6.19.10.1 第 3 条）：不再读写，恒为空，字段只为绑定兼容保留。
	ProjectPath string `json:"projectPath"`
	// ProjectID 是所属项目（v0.31）；空 = 不属于任何项目，库里 NULL。创建时写入，之后不可变。
	ProjectID string `json:"projectId,omitempty"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

// CatMessage 是契约 6.19.4 的消息行。
type CatMessage struct {
	ID        string `json:"id"`
	Role      string `json:"role"` // user | assistant | system
	Content   string `json:"content"`
	CreatedAt int64  `json:"createdAt"`
}

// InsertCatConversation 写入新会话；agentKind 之后不可变。
func (s *Store) InsertCatConversation(ctx context.Context, c CatConversation) (CatConversation, error) {
	if c.ID == "" {
		c.ID = id.New()
	}
	now := time.Now().UnixMilli()
	if c.CreatedAt == 0 {
		c.CreatedAt = now
	}
	if c.UpdatedAt == 0 {
		c.UpdatedAt = c.CreatedAt
	}
	if c.AccessMode == "" {
		c.AccessMode = "ask"
	}
	c.ProjectPath = ""
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO cat_conversations (id, title, agent_kind, access_mode, project_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.Title, c.AgentKind, c.AccessMode, nullIfEmpty(c.ProjectID), c.CreatedAt, c.UpdatedAt)
	if err != nil {
		return CatConversation{}, fmt.Errorf("写入 Cat 会话失败: %w", err)
	}
	return c, nil
}

const catConvColumns = `id, title, agent_kind, access_mode, COALESCE(project_id, ''), created_at, updated_at`

func scanCatConversation(r interface{ Scan(...any) error }) (CatConversation, error) {
	var c CatConversation
	err := r.Scan(&c.ID, &c.Title, &c.AgentKind, &c.AccessMode, &c.ProjectID, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// ListCatConversations 按 updatedAt 倒序，相同按 createdAt 倒序，再相同按 id（契约 6.19.10.6）。
func (s *Store) ListCatConversations(ctx context.Context) ([]CatConversation, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+catConvColumns+`
		FROM cat_conversations ORDER BY updated_at DESC, created_at DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("列出 Cat 会话失败: %w", err)
	}
	defer rows.Close()
	var out []CatConversation
	for rows.Next() {
		c, err := scanCatConversation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if out == nil {
		out = []CatConversation{}
	}
	return out, rows.Err()
}

// GetCatConversation 按 id 取会话。
func (s *Store) GetCatConversation(ctx context.Context, convID string) (CatConversation, error) {
	c, err := scanCatConversation(s.db.QueryRowContext(ctx, `
		SELECT `+catConvColumns+`
		FROM cat_conversations WHERE id = ?`, convID))
	if errors.Is(err, sql.ErrNoRows) {
		return CatConversation{}, err
	}
	if err != nil {
		return CatConversation{}, fmt.Errorf("读取 Cat 会话失败: %w", err)
	}
	return c, nil
}

// DeleteCatConversation 删除会话及其消息。
func (s *Store) DeleteCatConversation(ctx context.Context, convID string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM cat_conversations WHERE id = ?`, convID)
	if err != nil {
		return fmt.Errorf("删除 Cat 会话失败: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// TouchCatConversation 更新标题 / updated_at（不改 agent_kind、project_id；v0.31 起不写 project_path）。
func (s *Store) TouchCatConversation(ctx context.Context, convID, title string) error {
	now := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx, `
		UPDATE cat_conversations
		SET title = CASE WHEN ? != '' THEN ? ELSE title END,
		    updated_at = ?
		WHERE id = ?`, title, title, now, convID)
	if err != nil {
		return fmt.Errorf("更新 Cat 会话失败: %w", err)
	}
	return nil
}

// InsertCatMessage 追加一条消息，并刷新会话 updated_at。
func (s *Store) InsertCatMessage(ctx context.Context, conversationID string, m CatMessage) (CatMessage, error) {
	if m.ID == "" {
		m.ID = id.New()
	}
	if m.CreatedAt == 0 {
		m.CreatedAt = time.Now().UnixMilli()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CatMessage{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO cat_messages (id, conversation_id, role, content, created_at)
		VALUES (?, ?, ?, ?, ?)`, m.ID, conversationID, m.Role, m.Content, m.CreatedAt); err != nil {
		return CatMessage{}, fmt.Errorf("写入 Cat 消息失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE cat_conversations SET updated_at = ? WHERE id = ?`, m.CreatedAt, conversationID); err != nil {
		return CatMessage{}, fmt.Errorf("刷新 Cat 会话时间失败: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return CatMessage{}, err
	}
	return m, nil
}

// ListCatMessages 按创建时间升序。
func (s *Store) ListCatMessages(ctx context.Context, conversationID string) ([]CatMessage, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, role, content, created_at FROM cat_messages
		WHERE conversation_id = ? ORDER BY created_at ASC, id ASC`, conversationID)
	if err != nil {
		return nil, fmt.Errorf("列出 Cat 消息失败: %w", err)
	}
	defer rows.Close()
	var out []CatMessage
	for rows.Next() {
		var m CatMessage
		if err := rows.Scan(&m.ID, &m.Role, &m.Content, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if out == nil {
		out = []CatMessage{}
	}
	return out, rows.Err()
}

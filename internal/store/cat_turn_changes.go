package store

import "context"

func (s *Store) SaveCatTurnRecord(ctx context.Context, convID, turnID string, createdAt int64, summary, detail string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO cat_turn_changes(turn_id,conv_id,created_at,summary,detail) VALUES(?,?,?,?,?) ON CONFLICT(turn_id) DO UPDATE SET summary=excluded.summary,detail=excluded.detail`, turnID, convID, createdAt, summary, detail)
	return err
}
func (s *Store) ListCatTurnRecords(ctx context.Context, convID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT summary FROM cat_turn_changes WHERE conv_id=? ORDER BY created_at,turn_id`, convID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		out = append(out, raw)
	}
	return out, rows.Err()
}
func (s *Store) GetCatTurnRecord(ctx context.Context, convID, turnID string) (string, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT detail FROM cat_turn_changes WHERE conv_id=? AND turn_id=?`, convID, turnID).Scan(&raw)
	return raw, err
}

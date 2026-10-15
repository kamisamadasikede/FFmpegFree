package store

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

func TestCatTurnRecordsKeepHistoryAndCascade(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	// Use a minimal conversation row to verify the actual database relationship.
	_, err = st.db.ExecContext(ctx, `INSERT INTO cat_conversations(id,agent_kind) VALUES('conv','cat_build')`)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 15; i++ {
		if err = st.SaveCatTurnRecord(ctx, "conv", fmt.Sprint(i), int64(i), fmt.Sprintf(`{"fileCount":%d}`, i), `{"files":[{"diff":"patch"}]}`); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := st.ListCatTurnRecords(ctx, "conv")
	if err != nil || len(rows) != 15 {
		t.Fatalf("%d %v", len(rows), err)
	}
	detail, err := st.GetCatTurnRecord(ctx, "conv", "0")
	if err != nil || detail != `{"files":[{"diff":"patch"}]}` {
		t.Fatalf("%s %v", detail, err)
	}
	if _, err = st.GetCatTurnRecord(ctx, "other", "0"); err == nil {
		t.Fatal("cross-conversation detail exposed")
	}
	_, err = st.db.ExecContext(ctx, `DELETE FROM cat_conversations WHERE id='conv'`)
	if err != nil {
		t.Fatal(err)
	}
	rows, err = st.ListCatTurnRecords(ctx, "conv")
	if err != nil || len(rows) != 0 {
		t.Fatalf("records survived deletion: %v %v", rows, err)
	}
}

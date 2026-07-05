package store

import (
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "board.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestOpenCreatesSchema(t *testing.T) {
	st := newTestStore(t)
	var n int
	err := st.db.QueryRow(
		`SELECT count(*) FROM sqlite_master WHERE type='table' AND name IN ('tasks','tags','notes')`,
	).Scan(&n)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if n != 3 {
		t.Fatalf("expected 3 tables, got %d", n)
	}
}

func TestValidators(t *testing.T) {
	if !validStatus("todo") || !validStatus("in_progress") || !validStatus("done") {
		t.Fatal("valid statuses rejected")
	}
	if validStatus("backlog") {
		t.Fatal("invalid status accepted")
	}
	if !validPriority("low") || validPriority("urgent") {
		t.Fatal("priority validation wrong")
	}
}

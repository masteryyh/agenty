package storage

import (
	"path/filepath"
	"testing"
)

func TestOpenDBInitializesSchema(t *testing.T) {
	t.Parallel()

	db, err := OpenDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	wantColumns := map[string]bool{
		"id": false, "title": false, "agent_slug": false,
		"last_provider_slug": false, "last_model_slug": false,
		"context_window": false, "last_thinking_effort": false,
		"created_at": false, "updated_at": false,
	}
	rows, err := db.Query("SELECT name FROM pragma_table_info('sessions')")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			t.Fatal(err)
		}
		if _, ok := wantColumns[column]; ok {
			wantColumns[column] = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for column, found := range wantColumns {
		if !found {
			t.Errorf("sessions column %q not found", column)
		}
	}
}

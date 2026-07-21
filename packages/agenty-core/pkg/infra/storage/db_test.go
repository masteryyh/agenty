package storage

import (
	"path/filepath"
	"testing"
)

func TestOpenDB(t *testing.T) {
	defer CloseDB()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db1, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}

	// Second call ignores path and returns the same instance.
	db2, err := OpenDB(dbPath + "-ignored")
	if err != nil {
		t.Fatalf("second OpenDB: %v", err)
	}
	if db1 != db2 {
		t.Error("OpenDB should return the same instance on subsequent calls")
	}
	if GetDB() != db1 {
		t.Error("GetDB should return the singleton")
	}

	// Schema initialized on first open.
	var name string
	if err := db1.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='sessions'").Scan(&name); err != nil {
		t.Fatalf("query sessions table: %v", err)
	}
	if name != "sessions" {
		t.Errorf("sessions table not created, got name=%q", name)
	}

	var column string
	if err := db1.QueryRow("SELECT name FROM pragma_table_info('sessions') WHERE name = 'context_window'").Scan(&column); err != nil {
		t.Fatalf("query context_window column: %v", err)
	}
	if column != "context_window" {
		t.Errorf("context column = %q, want context_window", column)
	}
}

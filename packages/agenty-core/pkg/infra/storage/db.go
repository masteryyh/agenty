package storage

import (
	"database/sql"
	"sync"

	_ "github.com/mattn/go-sqlite3"
)

const schema = `
CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY NOT NULL,
	title TEXT NOT NULL DEFAULT '',
	agent_slug TEXT NOT NULL,
	last_provider_slug TEXT NOT NULL DEFAULT '',
	last_model_slug TEXT NOT NULL DEFAULT '',
	context_window INTEGER NOT NULL DEFAULT 0,
	last_thinking_effort TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_sessions_agent_slug ON sessions(agent_slug);
CREATE INDEX IF NOT EXISTS idx_sessions_updated_at ON sessions(updated_at DESC);
`

var (
	openOnce sync.Once
	db       *sql.DB
	openErr  error
)

func OpenDB(path string) (*sql.DB, error) {
	openOnce.Do(func() {
		d, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_timeout=5000")
		if err != nil {
			openErr = err
			return
		}

		if _, err := d.Exec(schema); err != nil {
			d.Close()
			openErr = err
			return
		}
		db = d
	})
	return db, openErr
}

func GetDB() *sql.DB {
	return db
}

func CloseDB() error {
	if db == nil {
		return nil
	}
	return db.Close()
}

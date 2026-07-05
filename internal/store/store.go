package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("task not found")

// Store owns all data access against a WAL-mode SQLite database.
type Store struct {
	db *sql.DB
}

type Task struct {
	ID            int64    `json:"id"`
	Title         string   `json:"title"`
	Description   string   `json:"description,omitempty"`
	Status        string   `json:"status"`
	Project       *string  `json:"project,omitempty"`
	Priority      *string  `json:"priority,omitempty"`
	DueDate       *string  `json:"due_date,omitempty"`
	Archived      bool     `json:"archived"`
	HandoffTo     *string  `json:"handoff_to,omitempty"`
	HandoffReason *string  `json:"handoff_reason,omitempty"`
	Tags          []string `json:"tags"`
	Notes         []Note   `json:"notes,omitempty"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
}

type Note struct {
	ID        int64  `json:"id"`
	TaskID    int64  `json:"task_id"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

func validStatus(s string) bool {
	switch s {
	case "todo", "in_progress", "done":
		return true
	}
	return false
}

func validPriority(p string) bool {
	switch p {
	case "low", "medium", "high":
		return true
	}
	return false
}

const schema = `
CREATE TABLE IF NOT EXISTS tasks (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  title       TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status      TEXT NOT NULL CHECK(status IN ('todo','in_progress','done')),
  project     TEXT,
  priority    TEXT CHECK(priority IN ('low','medium','high')),
  due_date    TEXT,
  archived    INTEGER NOT NULL DEFAULT 0,
  created_at  TEXT NOT NULL,
  updated_at  TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS tags (
  task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  tag     TEXT NOT NULL,
  PRIMARY KEY (task_id, tag)
);
CREATE TABLE IF NOT EXISTS notes (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id    INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  body       TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS events (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id    INTEGER,
  kind       TEXT NOT NULL,
  detail     TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_tasks_project ON tasks(project);
CREATE INDEX IF NOT EXISTS idx_tasks_status  ON tasks(status);
`

// Open opens (creating parent dirs as needed) the SQLite database at path in
// WAL mode with foreign keys on, and applies the schema.
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	for _, col := range []string{"handoff_to", "handoff_reason"} {
		// ADD COLUMN is a no-op-safe migration; ignore "duplicate column" errors.
		_, _ = db.Exec("ALTER TABLE tasks ADD COLUMN " + col + " TEXT")
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

// schemaVersion is the latest migration step applied by Open via PRAGMA user_version.
// Bump when adding a new step; never reuse or renumber prior versions.
const schemaVersion = 8

var ErrNotFound = errors.New("task not found")

// ValidationError is a client input error. The web layer maps it to HTTP 400.
type ValidationError struct {
	Msg string
}

func (e *ValidationError) Error() string { return e.Msg }

// Invalid returns a ValidationError with the given message.
func Invalid(msg string) error {
	return &ValidationError{Msg: msg}
}

// Invalidf returns a ValidationError with a formatted message.
func Invalidf(format string, args ...any) error {
	return &ValidationError{Msg: fmt.Sprintf(format, args...)}
}

// Store owns all data access against a WAL-mode SQLite database.
type Store struct {
	db *sql.DB

	// eventSig closes (and is replaced) whenever this process writes an event.
	// SSE hubs wait on EventSignal so same-process emits wake immediately;
	// a backup poll still covers events written by other board processes.
	eventMu  sync.Mutex
	eventSig chan struct{}
}

type Task struct {
	ID               int64    `json:"id"`
	Title            string   `json:"title"`
	Description      string   `json:"description,omitempty"`
	Status           string   `json:"status"`
	Project          *string  `json:"project,omitempty"`
	Priority         *string  `json:"priority,omitempty"`
	DueDate          *string  `json:"due_date,omitempty"`
	Archived         bool     `json:"archived"`
	HandoffTo        *string  `json:"handoff_to,omitempty"`
	HandoffReason    *string  `json:"handoff_reason,omitempty"`
	Tags             []string `json:"tags"`
	Notes            []Note   `json:"notes,omitempty"`
	RecentAgentNotes []Note   `json:"recent_agent_notes,omitempty"`
	CreatedAt        string   `json:"created_at"`
	UpdatedAt        string   `json:"updated_at"`
}

type Note struct {
	ID        int64  `json:"id"`
	TaskID    int64  `json:"task_id"`
	Body      string `json:"body"`
	Author    string `json:"author,omitempty"` // "" | human | agent | system
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
	// modernc.org/sqlite allows only one writer at a time; cap the pool at a
	// single connection so concurrent write handlers serialize instead of
	// racing separate connections into SQLITE_BUSY.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	s := &Store{db: db, eventSig: make(chan struct{})}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// migrate applies pending schema/data steps gated on PRAGMA user_version.
// Each step bumps the version on success so it never re-runs.
func (s *Store) migrate() error {
	ver, err := userVersion(s.db)
	if err != nil {
		return fmt.Errorf("user_version: %w", err)
	}
	if ver > schemaVersion {
		return fmt.Errorf("database schema version %d is newer than this binary (%d)", ver, schemaVersion)
	}

	// v1: handoff columns on tasks.
	if ver < 1 {
		for _, col := range []string{"handoff_to", "handoff_reason"} {
			if err := addColumn(s.db, "tasks", col, "TEXT"); err != nil {
				return fmt.Errorf("migrate v1: %w", err)
			}
		}
		if err := setUserVersion(s.db, 1); err != nil {
			return err
		}
		ver = 1
	}
	// v2: project_paths / questions / runs / run_files (never edit the base schema string).
	if ver < 2 {
		if _, err := s.db.Exec(extraTables); err != nil {
			return fmt.Errorf("migrate v2: %w", err)
		}
		if err := setUserVersion(s.db, 2); err != nil {
			return err
		}
		ver = 2
	}
	// v3: runs.message (CREATE already has it on fresh DBs; ALTER for older ones).
	if ver < 3 {
		if err := addColumn(s.db, "runs", "message", "TEXT NOT NULL DEFAULT ''"); err != nil {
			return fmt.Errorf("migrate v3: %w", err)
		}
		if err := setUserVersion(s.db, 3); err != nil {
			return err
		}
		ver = 3
	}
	// v4: notes.author
	if ver < 4 {
		if err := addColumn(s.db, "notes", "author", "TEXT NOT NULL DEFAULT ''"); err != nil {
			return fmt.Errorf("migrate v4: %w", err)
		}
		if err := setUserVersion(s.db, 4); err != nil {
			return err
		}
		ver = 4
	}
	// v5: runs.wait
	if ver < 5 {
		if err := addColumn(s.db, "runs", "wait", "TEXT NOT NULL DEFAULT ''"); err != nil {
			return fmt.Errorf("migrate v5: %w", err)
		}
		if err := setUserVersion(s.db, 5); err != nil {
			return err
		}
		ver = 5
	}
	// v6: runs.message_source
	if ver < 6 {
		if err := addColumn(s.db, "runs", "message_source", "TEXT NOT NULL DEFAULT ''"); err != nil {
			return fmt.Errorf("migrate v6: %w", err)
		}
		if err := setUserVersion(s.db, 6); err != nil {
			return err
		}
		ver = 6
	}
	// v7: one-shot purge of legacy PostToolUse "tool" events.
	if ver < 7 {
		if _, err := s.db.Exec(`DELETE FROM events WHERE kind = 'tool'`); err != nil {
			return fmt.Errorf("migrate v7: %w", err)
		}
		if err := setUserVersion(s.db, 7); err != nil {
			return err
		}
		ver = 7
	}
	// v8: rewrite absolute project_paths / run_files to relative forms.
	if ver < 8 {
		if err := s.migratePathsRelative(); err != nil {
			return fmt.Errorf("migrate v8: %w", err)
		}
		if err := setUserVersion(s.db, 8); err != nil {
			return err
		}
	}
	return nil
}

func userVersion(db *sql.DB) (int, error) {
	var v int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		return 0, err
	}
	return v, nil
}

func setUserVersion(db *sql.DB, v int) error {
	// PRAGMA user_version does not accept bound parameters.
	_, err := db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, v))
	if err != nil {
		return fmt.Errorf("set user_version %d: %w", v, err)
	}
	return nil
}

// addColumn runs ALTER TABLE … ADD COLUMN, treating duplicate-column as success
// so legacy DBs (user_version 0 with columns already present) upgrade cleanly.
// Any other error is returned.
func addColumn(db *sql.DB, table, col, decl string) error {
	_, err := db.Exec("ALTER TABLE " + table + " ADD COLUMN " + col + " " + decl)
	if err != nil && !isDuplicateColumn(err) {
		return err
	}
	return nil
}

func isDuplicateColumn(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "duplicate column")
}

// migratePathsRelative rewrites legacy absolute project_paths to ~/… and
// run_files under a mapped project root to project-relative paths.
func (s *Store) migratePathsRelative() error {
	rows, err := s.db.Query(`SELECT project, path FROM project_paths`)
	if err != nil {
		return err
	}
	type ppRow struct{ project, path string }
	var pps []ppRow
	for rows.Next() {
		var r ppRow
		if err := rows.Scan(&r.project, &r.path); err != nil {
			rows.Close()
			return err
		}
		pps = append(pps, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, r := range pps {
		next := RelativizeToHome(ExpandUserPath(r.path))
		if next == r.path {
			continue
		}
		if _, err := s.db.Exec(`UPDATE project_paths SET path = ? WHERE project = ?`, next, r.project); err != nil {
			return err
		}
	}

	frows, err := s.db.Query(`
		SELECT rf.run_id, rf.path, rf.first_seen_at, COALESCE(t.project, '') AS project
		FROM run_files rf
		JOIN runs r ON r.id = rf.run_id
		JOIN tasks t ON t.id = r.task_id`)
	if err != nil {
		return err
	}
	type fr struct {
		runID       int64
		path        string
		firstSeenAt string
		project     string
	}
	var files []fr
	for frows.Next() {
		var f fr
		if err := frows.Scan(&f.runID, &f.path, &f.firstSeenAt, &f.project); err != nil {
			frows.Close()
			return err
		}
		files = append(files, f)
	}
	frows.Close()
	if err := frows.Err(); err != nil {
		return err
	}
	for _, f := range files {
		projKey := f.project
		if projKey == "" {
			projKey = GlobalProjectKey
		}
		root, err := s.ResolveProjectPath(projKey)
		if err != nil {
			continue
		}
		next := RelativizeToRoot(f.path, root)
		if next == f.path {
			continue
		}
		if _, err := s.db.Exec(
			`INSERT OR IGNORE INTO run_files (run_id, path, first_seen_at) VALUES (?, ?, ?)`,
			f.runID, next, f.firstSeenAt,
		); err != nil {
			return err
		}
		if _, err := s.db.Exec(`DELETE FROM run_files WHERE run_id = ? AND path = ?`, f.runID, f.path); err != nil {
			return err
		}
	}

	// Unduplicate absolute prefixes in historical run_file activity details.
	erows, err := s.db.Query(`SELECT id, task_id, detail FROM events WHERE kind = 'run_file' AND detail LIKE '/%'`)
	if err != nil {
		return err
	}
	type ev struct {
		id     int64
		taskID sql.NullInt64
		detail string
	}
	var evs []ev
	for erows.Next() {
		var e ev
		if err := erows.Scan(&e.id, &e.taskID, &e.detail); err != nil {
			erows.Close()
			return err
		}
		evs = append(evs, e)
	}
	erows.Close()
	if err := erows.Err(); err != nil {
		return err
	}
	for _, e := range evs {
		if !e.taskID.Valid {
			continue
		}
		next := s.relativizeRunFilePath(e.taskID.Int64, e.detail)
		if next == e.detail {
			continue
		}
		if _, err := s.db.Exec(`UPDATE events SET detail = ? WHERE id = ?`, truncate(next, 80), e.id); err != nil {
			return err
		}
	}
	return nil
}

const extraTables = `
CREATE TABLE IF NOT EXISTS project_paths (
  project    TEXT PRIMARY KEY,
  path       TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS questions (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id     INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  question    TEXT NOT NULL,
  answer      TEXT,
  status      TEXT NOT NULL CHECK(status IN ('pending','answered','cancelled')),
  created_at  TEXT NOT NULL,
  answered_at TEXT
);
CREATE TABLE IF NOT EXISTS runs (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id    INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  agent      TEXT NOT NULL,
  pid        INTEGER,
  status     TEXT NOT NULL CHECK(status IN ('running','exited','failed','killed')),
  started_at TEXT NOT NULL,
  ended_at   TEXT,
  exit_code  INTEGER,
  message    TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_questions_task ON questions(task_id);
CREATE INDEX IF NOT EXISTS idx_questions_status ON questions(status);
CREATE INDEX IF NOT EXISTS idx_runs_task ON runs(task_id);
CREATE INDEX IF NOT EXISTS idx_runs_status ON runs(status);
CREATE TABLE IF NOT EXISTS run_files (
  run_id INTEGER NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  path TEXT NOT NULL,
  first_seen_at TEXT NOT NULL,
  PRIMARY KEY (run_id, path)
);
CREATE INDEX IF NOT EXISTS idx_run_files_run ON run_files(run_id);
`

func (s *Store) Close() error { return s.db.Close() }

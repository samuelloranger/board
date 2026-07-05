package store

import "errors"

type Event struct {
	ID        int64  `json:"id"`
	TaskID    *int64 `json:"task_id"`
	Kind      string `json:"kind"`
	Detail    string `json:"detail"`
	CreatedAt string `json:"created_at"`
}

// emit is best-effort: activity logging must never fail a real mutation.
func (s *Store) emit(taskID *int64, kind, detail string) {
	_, _ = s.db.Exec(
		`INSERT INTO events (task_id, kind, detail, created_at) VALUES (?, ?, ?, ?)`,
		taskID, kind, detail, now(),
	)
}

func (s *Store) Events(sinceID int64, limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.Query(
		`SELECT id, task_id, kind, detail, created_at FROM events WHERE id > ? ORDER BY id ASC LIMIT ?`,
		sinceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.TaskID, &e.Kind, &e.Detail, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// LogEvent appends a free-form activity event not tied to a specific task.
func (s *Store) LogEvent(kind, detail string) error {
	if kind == "" {
		return errors.New("event kind is required")
	}
	_, err := s.db.Exec(
		`INSERT INTO events (task_id, kind, detail, created_at) VALUES (NULL, ?, ?, ?)`,
		kind, detail, now())
	return err
}

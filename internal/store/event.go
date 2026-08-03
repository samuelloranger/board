package store

import "database/sql"

type Event struct {
	ID        int64  `json:"id"`
	TaskID    *int64 `json:"task_id"`
	Kind      string `json:"kind"`
	Detail    string `json:"detail"`
	CreatedAt string `json:"created_at"`
}

// emit is best-effort: activity logging must never fail a real mutation.
func (s *Store) emit(taskID *int64, kind, detail string) {
	_, err := s.db.Exec(
		`INSERT INTO events (task_id, kind, detail, created_at) VALUES (?, ?, ?, ?)`,
		taskID, kind, detail, now(),
	)
	if err == nil {
		s.wakeEventListeners()
	}
}

// wakeEventListeners closes the current EventSignal channel so waiters wake,
// then installs a fresh one for the next writer.
func (s *Store) wakeEventListeners() {
	s.eventMu.Lock()
	defer s.eventMu.Unlock()
	if s.eventSig == nil {
		s.eventSig = make(chan struct{})
		return
	}
	select {
	case <-s.eventSig:
		// already closed — replace below
	default:
		close(s.eventSig)
	}
	s.eventSig = make(chan struct{})
}

// EventSignal returns a channel that closes the next time this process writes
// an activity event. Callers should re-fetch the signal after it closes.
func (s *Store) EventSignal() <-chan struct{} {
	s.eventMu.Lock()
	defer s.eventMu.Unlock()
	if s.eventSig == nil {
		s.eventSig = make(chan struct{})
	}
	return s.eventSig
}

// MaxEventID returns the highest events.id, or 0 when the table is empty.
func (s *Store) MaxEventID() (int64, error) {
	var id sql.NullInt64
	if err := s.db.QueryRow(`SELECT MAX(id) FROM events`).Scan(&id); err != nil {
		return 0, err
	}
	if !id.Valid {
		return 0, nil
	}
	return id.Int64, nil
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

// RecentEvents returns the newest `limit` activity events in ascending id order.
// Used to seed a fresh SSE connection so a page load replays a short tail instead
// of the entire events table. Skips tool/run_progress noise (hooks / live agent
// chatter) so the Activity drawer opens with useful history.
func (s *Store) RecentEvents(limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 30
	}
	rows, err := s.db.Query(
		`SELECT id, task_id, kind, detail, created_at FROM (
		   SELECT id, task_id, kind, detail, created_at FROM events
		   WHERE kind NOT IN ('tool', 'run_progress')
		   ORDER BY id DESC LIMIT ?
		 ) ORDER BY id ASC`, limit)
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
// Kind "tool" is ignored: PostToolUse hooks used to log every tool name and
// flooded the activity feed / events table.
func (s *Store) LogEvent(kind, detail string) error {
	if kind == "" {
		return Invalid("event kind is required")
	}
	if kind == "tool" {
		return nil
	}
	_, err := s.db.Exec(
		`INSERT INTO events (task_id, kind, detail, created_at) VALUES (NULL, ?, ?, ?)`,
		kind, detail, now())
	if err == nil {
		s.wakeEventListeners()
	}
	return err
}

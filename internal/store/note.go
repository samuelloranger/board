package store

import "errors"

func (s *Store) AddNote(taskID int64, body string) (*Note, error) {
	if body == "" {
		return nil, errors.New("note body is required")
	}
	if _, err := s.GetTask(taskID); err != nil {
		return nil, err
	}
	ts := now()
	res, err := s.db.Exec(`INSERT INTO notes (task_id, body, created_at) VALUES (?, ?, ?)`, taskID, body, ts)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	s.emit(&taskID, "note", body)
	// Mirror onto the active agent run so the web UI can show live progress.
	if run, err := s.ActiveRunForTask(taskID); err == nil {
		_ = s.ReportRunProgress(run.ID, taskID, body)
	}
	return &Note{ID: id, TaskID: taskID, Body: body, CreatedAt: ts}, nil
}

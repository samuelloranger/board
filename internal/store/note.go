package store

import (
	"errors"
	"fmt"
)

func validNoteAuthor(a string) bool {
	switch a {
	case "", "human", "agent", "system":
		return true
	}
	return false
}

// AddNote appends a note. author must be "", "human", "agent", or "system"
// (empty = unlabeled / legacy).
func (s *Store) AddNote(taskID int64, body, author string) (*Note, error) {
	if body == "" {
		return nil, errors.New("note body is required")
	}
	if !validNoteAuthor(author) {
		return nil, fmt.Errorf("invalid note author %q", author)
	}
	if _, err := s.GetTask(taskID); err != nil {
		return nil, err
	}
	ts := now()
	res, err := s.db.Exec(
		`INSERT INTO notes (task_id, body, author, created_at) VALUES (?, ?, ?, ?)`,
		taskID, body, author, ts,
	)
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
	return &Note{ID: id, TaskID: taskID, Body: body, Author: author, CreatedAt: ts}, nil
}

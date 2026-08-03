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

// RecentNotes returns up to limit notes for taskID with the given author
// (empty author = any). SQL is newest-first; the returned slice is oldest→newest
// for UI mini-threads. limit <= 0 yields an empty slice.
func (s *Store) RecentNotes(taskID int64, limit int, author string) ([]Note, error) {
	if limit <= 0 {
		return []Note{}, nil
	}
	q := `SELECT id, task_id, body, author, created_at FROM notes WHERE task_id = ?`
	args := []any{taskID}
	if author != "" {
		q += ` AND author = ?`
		args = append(args, author)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	desc := []Note{}
	for rows.Next() {
		var n Note
		if err := rows.Scan(&n.ID, &n.TaskID, &n.Body, &n.Author, &n.CreatedAt); err != nil {
			return nil, err
		}
		desc = append(desc, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Note, len(desc))
	for i := range desc {
		out[len(desc)-1-i] = desc[i]
	}
	return out, nil
}

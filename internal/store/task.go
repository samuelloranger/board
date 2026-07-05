package store

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"
)

func now() string { return time.Now().UTC().Format(time.RFC3339) }

type CreateTaskParams struct {
	Title       string
	Description string
	Status      string
	Project     *string
	Priority    *string
	DueDate     *string
	Tags        []string
}

func (s *Store) CreateTask(p CreateTaskParams) (*Task, error) {
	if p.Title == "" {
		return nil, errors.New("title is required")
	}
	if p.Status == "" {
		p.Status = "todo"
	}
	if !validStatus(p.Status) {
		return nil, fmt.Errorf("invalid status %q", p.Status)
	}
	if p.Priority != nil && !validPriority(*p.Priority) {
		return nil, fmt.Errorf("invalid priority %q", *p.Priority)
	}
	ts := now()
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(
		`INSERT INTO tasks (title, description, status, project, priority, due_date, archived, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?)`,
		p.Title, p.Description, p.Status, p.Project, p.Priority, p.DueDate, ts, ts,
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	if err := insertTags(tx, id, p.Tags); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetTask(id)
}

func insertTags(tx *sql.Tx, taskID int64, tags []string) error {
	for _, tag := range tags {
		if tag == "" {
			continue
		}
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO tags (task_id, tag) VALUES (?, ?)`, taskID, tag,
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) GetTask(id int64) (*Task, error) {
	var tk Task
	err := s.db.QueryRow(
		`SELECT id, title, description, status, project, priority, due_date, archived, created_at, updated_at
		 FROM tasks WHERE id = ?`, id,
	).Scan(&tk.ID, &tk.Title, &tk.Description, &tk.Status, &tk.Project, &tk.Priority,
		&tk.DueDate, &tk.Archived, &tk.CreatedAt, &tk.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if tk.Tags, err = s.taskTags(id); err != nil {
		return nil, err
	}
	if tk.Notes, err = s.taskNotes(id); err != nil {
		return nil, err
	}
	return &tk, nil
}

func (s *Store) taskTags(id int64) ([]string, error) {
	rows, err := s.db.Query(`SELECT tag FROM tags WHERE task_id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tags := []string{}
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	sort.Strings(tags)
	return tags, rows.Err()
}

func (s *Store) taskNotes(id int64) ([]Note, error) {
	rows, err := s.db.Query(
		`SELECT id, task_id, body, created_at FROM notes WHERE task_id = ? ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	notes := []Note{}
	for rows.Next() {
		var n Note
		if err := rows.Scan(&n.ID, &n.TaskID, &n.Body, &n.CreatedAt); err != nil {
			return nil, err
		}
		notes = append(notes, n)
	}
	return notes, rows.Err()
}

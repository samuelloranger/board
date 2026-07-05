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
	s.emit(&id, "created", p.Title)
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
		`SELECT id, title, description, status, project, priority, due_date, archived,
		        handoff_to, handoff_reason, created_at, updated_at
		 FROM tasks WHERE id = ?`, id,
	).Scan(&tk.ID, &tk.Title, &tk.Description, &tk.Status, &tk.Project, &tk.Priority,
		&tk.DueDate, &tk.Archived, &tk.HandoffTo, &tk.HandoffReason, &tk.CreatedAt, &tk.UpdatedAt)
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

type ListFilter struct {
	Project         *string
	Status          *string
	Priority        *string
	Tag             *string
	IncludeArchived bool
}

func (s *Store) ListTasks(f ListFilter) ([]*Task, error) {
	q := `SELECT id FROM tasks WHERE 1=1`
	var args []any
	if f.Project != nil {
		// Global (NULL-project) tasks are pinned into every project-scoped
		// view, so a task created outside any repo is never invisible.
		q += ` AND (project = ? OR project IS NULL)`
		args = append(args, *f.Project)
	}
	if f.Status != nil {
		q += ` AND status = ?`
		args = append(args, *f.Status)
	}
	if f.Priority != nil {
		q += ` AND priority = ?`
		args = append(args, *f.Priority)
	}
	if f.Tag != nil {
		q += ` AND id IN (SELECT task_id FROM tags WHERE tag = ?)`
		args = append(args, *f.Tag)
	}
	if !f.IncludeArchived {
		q += ` AND archived = 0`
	}
	q += ` ORDER BY CASE priority WHEN 'high' THEN 0 WHEN 'medium' THEN 1 WHEN 'low' THEN 2 ELSE 3 END, created_at ASC, id ASC`

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := []*Task{}
	for _, id := range ids {
		tk, err := s.GetTask(id)
		if err != nil {
			return nil, err
		}
		out = append(out, tk)
	}
	return out, nil
}

type UpdateTaskParams struct {
	Title       *string
	Description *string
	Priority    *string // "" clears to NULL
	DueDate     *string // "" clears to NULL
	Tags        *[]string
}

func (s *Store) UpdateTask(id int64, p UpdateTaskParams) (*Task, error) {
	if _, err := s.GetTask(id); err != nil {
		return nil, err
	}
	if p.Priority != nil && *p.Priority != "" && !validPriority(*p.Priority) {
		return nil, fmt.Errorf("invalid priority %q", *p.Priority)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	set := []string{"updated_at = ?"}
	args := []any{now()}
	if p.Title != nil {
		set = append(set, "title = ?")
		args = append(args, *p.Title)
	}
	if p.Description != nil {
		set = append(set, "description = ?")
		args = append(args, *p.Description)
	}
	if p.Priority != nil {
		if *p.Priority == "" {
			set = append(set, "priority = NULL")
		} else {
			set = append(set, "priority = ?")
			args = append(args, *p.Priority)
		}
	}
	if p.DueDate != nil {
		if *p.DueDate == "" {
			set = append(set, "due_date = NULL")
		} else {
			set = append(set, "due_date = ?")
			args = append(args, *p.DueDate)
		}
	}
	args = append(args, id)
	if _, err := tx.Exec("UPDATE tasks SET "+joinComma(set)+" WHERE id = ?", args...); err != nil {
		return nil, err
	}
	if p.Tags != nil {
		if _, err := tx.Exec(`DELETE FROM tags WHERE task_id = ?`, id); err != nil {
			return nil, err
		}
		if err := insertTags(tx, id, *p.Tags); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	s.emit(&id, "updated", "")
	return s.GetTask(id)
}

func joinComma(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}

func (s *Store) MoveTask(id int64, status string) (*Task, error) {
	if !validStatus(status) {
		return nil, fmt.Errorf("invalid status %q", status)
	}
	var current string
	err := s.db.QueryRow(`SELECT status FROM tasks WHERE id = ?`, id).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	// Only a real status change consumes a handoff (the receiver picking the
	// task up). A no-op re-move must not silently wipe handoff context.
	if current == status {
		if _, err := s.db.Exec(`UPDATE tasks SET updated_at = ? WHERE id = ?`, now(), id); err != nil {
			return nil, err
		}
		return s.GetTask(id)
	}
	if _, err := s.db.Exec(
		`UPDATE tasks SET status = ?, handoff_to = NULL, handoff_reason = NULL, updated_at = ? WHERE id = ?`,
		status, now(), id); err != nil {
		return nil, err
	}
	s.emit(&id, "moved", "→ "+status)
	return s.GetTask(id)
}

func (s *Store) SetArchived(id int64, archived bool) (*Task, error) {
	res, err := s.db.Exec(`UPDATE tasks SET archived = ?, updated_at = ? WHERE id = ?`, boolToInt(archived), now(), id)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	kind := "archived"
	if !archived {
		kind = "unarchived"
	}
	s.emit(&id, kind, "")
	return s.GetTask(id)
}

func (s *Store) DeleteTask(id int64) error {
	res, err := s.db.Exec(`DELETE FROM tasks WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	s.emit(&id, "deleted", "")
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

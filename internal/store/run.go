package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// Run tracks a spawned agent process for a task.
type Run struct {
	ID        int64   `json:"id"`
	TaskID    int64   `json:"task_id"`
	Agent     string  `json:"agent"`
	PID       *int    `json:"pid,omitempty"`
	Status    string  `json:"status"`
	StartedAt string  `json:"started_at"`
	EndedAt   *string `json:"ended_at,omitempty"`
	ExitCode  *int    `json:"exit_code,omitempty"`
	Message   string  `json:"message,omitempty"`
}

var ErrRunActive = errors.New("task already has a running agent")

func (s *Store) CreateRun(taskID int64, agent string, pid int) (*Run, error) {
	if agent == "" {
		return nil, errors.New("agent is required")
	}
	if _, err := s.GetTask(taskID); err != nil {
		return nil, err
	}
	if _, err := s.ActiveRunForTask(taskID); err == nil {
		return nil, ErrRunActive
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	ts := now()
	res, err := s.db.Exec(
		`INSERT INTO runs (task_id, agent, pid, status, started_at, message) VALUES (?, ?, ?, 'running', ?, '')`,
		taskID, agent, pid, ts,
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	s.emit(&taskID, "run", agent)
	tk, err := s.GetTask(taskID)
	if err != nil {
		return nil, err
	}
	if tk.Status != "in_progress" {
		if _, err := s.MoveTask(taskID, "in_progress"); err != nil {
			return nil, err
		}
	}
	return s.GetRun(id)
}

func (s *Store) GetRun(id int64) (*Run, error) {
	var r Run
	var pid, exitCode sql.NullInt64
	var endedAt sql.NullString
	err := s.db.QueryRow(
		`SELECT id, task_id, agent, pid, status, started_at, ended_at, exit_code, message FROM runs WHERE id = ?`, id,
	).Scan(&r.ID, &r.TaskID, &r.Agent, &pid, &r.Status, &r.StartedAt, &endedAt, &exitCode, &r.Message)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	scanRunExtras(&r, pid, endedAt, exitCode)
	return &r, nil
}

func scanRunExtras(r *Run, pid sql.NullInt64, endedAt sql.NullString, exitCode sql.NullInt64) {
	if pid.Valid {
		p := int(pid.Int64)
		r.PID = &p
	}
	if endedAt.Valid {
		r.EndedAt = &endedAt.String
	}
	if exitCode.Valid {
		c := int(exitCode.Int64)
		r.ExitCode = &c
	}
}

func (s *Store) ListRuns(taskID *int64, status string) ([]Run, error) {
	q := `SELECT id, task_id, agent, pid, status, started_at, ended_at, exit_code, message FROM runs WHERE 1=1`
	args := []any{}
	if taskID != nil {
		q += ` AND task_id = ?`
		args = append(args, *taskID)
	}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY id DESC`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Run{}
	for rows.Next() {
		var r Run
		var pid, exitCode sql.NullInt64
		var endedAt sql.NullString
		if err := rows.Scan(&r.ID, &r.TaskID, &r.Agent, &pid, &r.Status, &r.StartedAt, &endedAt, &exitCode, &r.Message); err != nil {
			return nil, err
		}
		scanRunExtras(&r, pid, endedAt, exitCode)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ActiveRunForTask(taskID int64) (*Run, error) {
	var r Run
	var pid, exitCode sql.NullInt64
	var endedAt sql.NullString
	err := s.db.QueryRow(
		`SELECT id, task_id, agent, pid, status, started_at, ended_at, exit_code, message
		 FROM runs WHERE task_id = ? AND status = 'running' ORDER BY id DESC LIMIT 1`, taskID,
	).Scan(&r.ID, &r.TaskID, &r.Agent, &pid, &r.Status, &r.StartedAt, &endedAt, &exitCode, &r.Message)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	scanRunExtras(&r, pid, endedAt, exitCode)
	return &r, nil
}

// LatestRunForTask returns the most recent run for a task (any status).
func (s *Store) LatestRunForTask(taskID int64) (*Run, error) {
	runs, err := s.ListRuns(&taskID, "")
	if err != nil {
		return nil, err
	}
	if len(runs) == 0 {
		return nil, ErrNotFound
	}
	return &runs[0], nil
}

// SetRunMessage updates the live progress text on a run (while running or after).
func (s *Store) SetRunMessage(id int64, message string) error {
	_, err := s.db.Exec(`UPDATE runs SET message = ? WHERE id = ?`, message, id)
	return err
}

// ReportRunProgress sets the run message and emits a run_progress event for the UI.
func (s *Store) ReportRunProgress(runID, taskID int64, message string) error {
	if err := s.SetRunMessage(runID, message); err != nil {
		return err
	}
	s.emit(&taskID, "run_progress", truncate(message, 80))
	return nil
}

func (s *Store) FinishRun(id int64, status string, exitCode *int, message string) (*Run, error) {
	switch status {
	case "exited", "failed", "killed":
	default:
		return nil, fmt.Errorf("invalid run status %q", status)
	}
	r, err := s.GetRun(id)
	if err != nil {
		return nil, err
	}
	if r.Status != "running" {
		return r, nil // idempotent
	}
	ts := now()
	_, err = s.db.Exec(
		`UPDATE runs SET status = ?, ended_at = ?, exit_code = ?, message = ? WHERE id = ?`,
		status, ts, exitCode, message, id,
	)
	if err != nil {
		return nil, err
	}
	if _, err := s.CancelPendingQuestions(r.TaskID); err != nil {
		return nil, err
	}
	detail := status
	if message != "" {
		detail = status + ": " + truncate(message, 60)
	} else if exitCode != nil {
		detail = fmt.Sprintf("%s (%d)", status, *exitCode)
	}
	s.emit(&r.TaskID, "run_done", detail)
	return s.GetRun(id)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ReconcileOrphanRuns marks running rows whose process is dead as failed.
func (s *Store) ReconcileOrphanRuns(alive func(pid int) bool) (int, error) {
	running := "running"
	runs, err := s.ListRuns(nil, running)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range runs {
		live := false
		if r.PID != nil && alive != nil {
			live = alive(*r.PID)
		}
		if live {
			continue
		}
		code := -1
		if _, err := s.FinishRun(r.ID, "failed", &code, "process gone after serve restart"); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

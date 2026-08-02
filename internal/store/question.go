package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Question is a pending or resolved ask_user prompt tied to a task.
type Question struct {
	ID         int64   `json:"id"`
	TaskID     int64   `json:"task_id"`
	Question   string  `json:"question"`
	Answer     *string `json:"answer,omitempty"`
	Status     string  `json:"status"`
	CreatedAt  string  `json:"created_at"`
	AnsweredAt *string `json:"answered_at,omitempty"`
}

var ErrQuestionClosed = errors.New("question is not pending")

func (s *Store) CreateQuestion(taskID int64, question string) (*Question, error) {
	if question == "" {
		return nil, errors.New("question is required")
	}
	if _, err := s.GetTask(taskID); err != nil {
		return nil, err
	}
	ts := now()
	res, err := s.db.Exec(
		`INSERT INTO questions (task_id, question, status, created_at) VALUES (?, ?, 'pending', ?)`,
		taskID, question, ts,
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	detail := question
	if len(detail) > 80 {
		detail = detail[:80] + "…"
	}
	s.emit(&taskID, "question", detail)
	return s.GetQuestion(id)
}

func (s *Store) GetQuestion(id int64) (*Question, error) {
	var q Question
	var answer, answeredAt sql.NullString
	err := s.db.QueryRow(
		`SELECT id, task_id, question, answer, status, created_at, answered_at
		 FROM questions WHERE id = ?`, id,
	).Scan(&q.ID, &q.TaskID, &q.Question, &answer, &q.Status, &q.CreatedAt, &answeredAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if answer.Valid {
		q.Answer = &answer.String
	}
	if answeredAt.Valid {
		q.AnsweredAt = &answeredAt.String
	}
	return &q, nil
}

func (s *Store) ListQuestions(taskID *int64, status string) ([]Question, error) {
	q := `SELECT id, task_id, question, answer, status, created_at, answered_at FROM questions WHERE 1=1`
	args := []any{}
	if taskID != nil {
		q += ` AND task_id = ?`
		args = append(args, *taskID)
	}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY id ASC`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Question{}
	for rows.Next() {
		var item Question
		var answer, answeredAt sql.NullString
		if err := rows.Scan(&item.ID, &item.TaskID, &item.Question, &answer, &item.Status, &item.CreatedAt, &answeredAt); err != nil {
			return nil, err
		}
		if answer.Valid {
			item.Answer = &answer.String
		}
		if answeredAt.Valid {
			item.AnsweredAt = &answeredAt.String
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) AnswerQuestion(id int64, answer string) (*Question, error) {
	q, err := s.GetQuestion(id)
	if err != nil {
		return nil, err
	}
	if q.Status != "pending" {
		return nil, ErrQuestionClosed
	}
	ts := now()
	_, err = s.db.Exec(
		`UPDATE questions SET answer = ?, status = 'answered', answered_at = ? WHERE id = ? AND status = 'pending'`,
		answer, ts, id,
	)
	if err != nil {
		return nil, err
	}
	s.emit(&q.TaskID, "answered", fmt.Sprintf("#%d", id))
	return s.GetQuestion(id)
}

func (s *Store) CancelPendingQuestions(taskID int64) (int64, error) {
	res, err := s.db.Exec(
		`UPDATE questions SET status = 'cancelled', answered_at = ? WHERE task_id = ? AND status = 'pending'`,
		now(), taskID,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// WaitForAnswer polls until the question is answered or cancelled.
// Each poll is a separate QueryRow — never hold a transaction across sleeps
// (MaxOpenConns(1) would otherwise deadlock AnswerQuestion from another process).
func (s *Store) WaitForAnswer(ctx context.Context, id int64, poll time.Duration) (string, error) {
	if poll <= 0 {
		poll = 200 * time.Millisecond
	}
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		q, err := s.GetQuestion(id)
		if err != nil {
			return "", err
		}
		switch q.Status {
		case "answered":
			if q.Answer == nil {
				return "", errors.New("answered question has no answer")
			}
			return *q.Answer, nil
		case "cancelled":
			return "", errors.New("question cancelled")
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
		}
	}
}

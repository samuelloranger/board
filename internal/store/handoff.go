package store

import "errors"

func (s *Store) Handoff(id int64, to, reason string) (*Task, error) {
	if to == "" {
		return nil, errors.New("handoff target (to) is required")
	}
	res, err := s.db.Exec(
		`UPDATE tasks SET handoff_to = ?, handoff_reason = ?, updated_at = ? WHERE id = ?`,
		to, reason, now(), id)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	s.emit(&id, "handoff", "→ "+to+": "+reason)
	return s.GetTask(id)
}

type ResumeResult struct {
	Project    *string `json:"project"`
	InProgress []*Task `json:"in_progress"`
	Handoffs   []*Task `json:"handoffs"`
}

func (s *Store) Resume(project *string) (*ResumeResult, error) {
	inProg := "in_progress"
	ip, err := s.ListTasks(ListFilter{Project: project, Status: &inProg})
	if err != nil {
		return nil, err
	}
	// Handoffs: any non-archived, non-done task in scope with handoff_to set.
	// Done tasks never represent pending work to pick up.
	all, err := s.ListTasks(ListFilter{Project: project})
	if err != nil {
		return nil, err
	}
	handoffs := []*Task{}
	for _, tk := range all {
		if tk.HandoffTo != nil && tk.Status != "done" {
			handoffs = append(handoffs, tk)
		}
	}
	return &ResumeResult{Project: project, InProgress: ip, Handoffs: handoffs}, nil
}

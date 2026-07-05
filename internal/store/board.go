package store

type Board struct {
	Project    *string `json:"project"`
	Todo       []*Task `json:"todo"`
	InProgress []*Task `json:"in_progress"`
	Done       []*Task `json:"done"`
}

func (s *Store) GetBoard(project *string) (*Board, error) {
	tasks, err := s.ListTasks(ListFilter{Project: project})
	if err != nil {
		return nil, err
	}
	b := &Board{Project: project, Todo: []*Task{}, InProgress: []*Task{}, Done: []*Task{}}
	for _, tk := range tasks {
		switch tk.Status {
		case "todo":
			b.Todo = append(b.Todo, tk)
		case "in_progress":
			b.InProgress = append(b.InProgress, tk)
		case "done":
			b.Done = append(b.Done, tk)
		}
	}
	return b, nil
}

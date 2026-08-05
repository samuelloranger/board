package store

import "sort"

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
	// Todo: newly added first. In progress / Done: most recently updated first.
	sort.SliceStable(b.Todo, func(i, j int) bool {
		if b.Todo[i].CreatedAt != b.Todo[j].CreatedAt {
			return b.Todo[i].CreatedAt > b.Todo[j].CreatedAt
		}
		return b.Todo[i].ID > b.Todo[j].ID
	})
	sortByUpdatedDesc(b.InProgress)
	sortByUpdatedDesc(b.Done)
	return b, nil
}

func sortByUpdatedDesc(ts []*Task) {
	sort.SliceStable(ts, func(i, j int) bool {
		if ts[i].UpdatedAt != ts[j].UpdatedAt {
			return ts[i].UpdatedAt > ts[j].UpdatedAt
		}
		return ts[i].ID > ts[j].ID
	})
}

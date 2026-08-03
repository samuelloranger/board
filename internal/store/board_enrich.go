package store

// AttachRecentAgentNotes sets Task.RecentAgentNotes (max n) on tasks that
// currently have a status=running run. No-op if b is nil or n <= 0.
func (s *Store) AttachRecentAgentNotes(b *Board, n int) error {
	if b == nil || n <= 0 {
		return nil
	}
	running, err := s.ListRuns(nil, "running")
	if err != nil {
		return err
	}
	active := make(map[int64]struct{}, len(running))
	for _, r := range running {
		active[r.TaskID] = struct{}{}
	}
	attach := func(ts []*Task) error {
		for _, tk := range ts {
			if _, ok := active[tk.ID]; !ok {
				continue
			}
			notes, err := s.RecentNotes(tk.ID, n, "agent")
			if err != nil {
				return err
			}
			tk.RecentAgentNotes = notes
		}
		return nil
	}
	if err := attach(b.Todo); err != nil {
		return err
	}
	if err := attach(b.InProgress); err != nil {
		return err
	}
	return attach(b.Done)
}

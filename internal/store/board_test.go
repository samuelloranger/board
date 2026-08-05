package store

import "testing"

func TestGetBoard(t *testing.T) {
	st := newTestStore(t)
	p := "proj"
	st.CreateTask(CreateTaskParams{Title: "a", Status: "todo", Project: &p})
	st.CreateTask(CreateTaskParams{Title: "b", Status: "in_progress", Project: &p})
	st.CreateTask(CreateTaskParams{Title: "c", Status: "done", Project: &p})
	other := "other"
	st.CreateTask(CreateTaskParams{Title: "d", Status: "todo", Project: &other})

	b, err := st.GetBoard(&p)
	if err != nil {
		t.Fatalf("GetBoard: %v", err)
	}
	if len(b.Todo) != 1 || len(b.InProgress) != 1 || len(b.Done) != 1 {
		t.Fatalf("scoped board wrong: %+v", b)
	}
	all, _ := st.GetBoard(nil)
	if len(all.Todo) != 2 {
		t.Fatalf("global board should see both todos, got %d", len(all.Todo))
	}
}

func TestGetBoardColumnSort(t *testing.T) {
	st := newTestStore(t)
	p := "sortproj"

	oldTodo, _ := st.CreateTask(CreateTaskParams{Title: "old-todo", Status: "todo", Project: &p})
	newTodo, _ := st.CreateTask(CreateTaskParams{Title: "new-todo", Status: "todo", Project: &p})
	staleIP, _ := st.CreateTask(CreateTaskParams{Title: "stale-ip", Status: "in_progress", Project: &p})
	freshIP, _ := st.CreateTask(CreateTaskParams{Title: "fresh-ip", Status: "in_progress", Project: &p})
	oldDone, _ := st.CreateTask(CreateTaskParams{Title: "old-done", Status: "done", Project: &p})
	newDone, _ := st.CreateTask(CreateTaskParams{Title: "new-done", Status: "done", Project: &p})

	// Force timestamps so order is independent of insert timing.
	setTS := func(id int64, created, updated string) {
		t.Helper()
		if _, err := st.db.Exec(
			`UPDATE tasks SET created_at = ?, updated_at = ? WHERE id = ?`, created, updated, id,
		); err != nil {
			t.Fatalf("set timestamps: %v", err)
		}
	}
	setTS(oldTodo.ID, "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z")
	setTS(newTodo.ID, "2026-06-01T00:00:00Z", "2026-06-01T00:00:00Z")
	setTS(staleIP.ID, "2026-01-01T00:00:00Z", "2026-02-01T00:00:00Z")
	setTS(freshIP.ID, "2026-01-02T00:00:00Z", "2026-05-01T00:00:00Z")
	setTS(oldDone.ID, "2026-01-01T00:00:00Z", "2026-03-01T00:00:00Z")
	setTS(newDone.ID, "2026-01-02T00:00:00Z", "2026-04-01T00:00:00Z")

	b, err := st.GetBoard(&p)
	if err != nil {
		t.Fatalf("GetBoard: %v", err)
	}
	if len(b.Todo) != 2 || b.Todo[0].Title != "new-todo" || b.Todo[1].Title != "old-todo" {
		t.Fatalf("todo should be newest-created first, got %+v", titles(b.Todo))
	}
	if len(b.InProgress) != 2 || b.InProgress[0].Title != "fresh-ip" || b.InProgress[1].Title != "stale-ip" {
		t.Fatalf("in_progress should be newest-updated first, got %+v", titles(b.InProgress))
	}
	if len(b.Done) != 2 || b.Done[0].Title != "new-done" || b.Done[1].Title != "old-done" {
		t.Fatalf("done should be newest-updated first, got %+v", titles(b.Done))
	}
}

func titles(ts []*Task) []string {
	out := make([]string, len(ts))
	for i, tk := range ts {
		out[i] = tk.Title
	}
	return out
}

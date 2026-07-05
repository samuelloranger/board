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

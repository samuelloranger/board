package store

import (
	"errors"
	"testing"
)

func TestCreateAndGetTask(t *testing.T) {
	st := newTestStore(t)
	proj := "demo"
	pr := "high"
	created, err := st.CreateTask(CreateTaskParams{
		Title:       "Write plan",
		Description: "the whole thing",
		Project:     &proj,
		Priority:    &pr,
		Tags:        []string{"docs", "urgent"},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if created.ID == 0 || created.Status != "todo" {
		t.Fatalf("bad created task: %+v", created)
	}
	got, err := st.GetTask(created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Title != "Write plan" || got.Project == nil || *got.Project != "demo" {
		t.Fatalf("mismatch: %+v", got)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "docs" || got.Tags[1] != "urgent" {
		t.Fatalf("tags wrong: %v", got.Tags)
	}
	if got.CreatedAt == "" || got.UpdatedAt == "" {
		t.Fatal("timestamps not set")
	}
}

func TestCreateTaskRejectsBadStatus(t *testing.T) {
	st := newTestStore(t)
	if _, err := st.CreateTask(CreateTaskParams{Title: "x", Status: "nope"}); err == nil {
		t.Fatal("expected error for bad status")
	}
}

func TestGetTaskNotFound(t *testing.T) {
	st := newTestStore(t)
	if _, err := st.GetTask(999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestListTasksFilters(t *testing.T) {
	st := newTestStore(t)
	a := "proja"
	b := "projb"
	mk := func(title, status string, proj *string, tags ...string) int64 {
		tk, err := st.CreateTask(CreateTaskParams{Title: title, Status: status, Project: proj, Tags: tags})
		if err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
		return tk.ID
	}
	mk("t1", "todo", &a, "x")
	mk("t2", "done", &a)
	mk("t3", "todo", &b)
	mk("g1", "todo", nil)

	all, _ := st.ListTasks(ListFilter{})
	if len(all) != 4 {
		t.Fatalf("want 4 got %d", len(all))
	}
	byProj, _ := st.ListTasks(ListFilter{Project: &a})
	if len(byProj) != 2 {
		t.Fatalf("project filter: want 2 got %d", len(byProj))
	}
	todo := "todo"
	byStatus, _ := st.ListTasks(ListFilter{Project: &a, Status: &todo})
	if len(byStatus) != 1 || byStatus[0].Title != "t1" {
		t.Fatalf("status filter wrong: %+v", byStatus)
	}
	tag := "x"
	byTag, _ := st.ListTasks(ListFilter{Tag: &tag})
	if len(byTag) != 1 || byTag[0].Title != "t1" {
		t.Fatalf("tag filter wrong: %+v", byTag)
	}
}

func TestListExcludesArchived(t *testing.T) {
	st := newTestStore(t)
	tk, _ := st.CreateTask(CreateTaskParams{Title: "arch me"})
	if _, err := st.db.Exec(`UPDATE tasks SET archived=1 WHERE id=?`, tk.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.ListTasks(ListFilter{}); len(got) != 0 {
		t.Fatalf("archived should be hidden, got %d", len(got))
	}
	if got, _ := st.ListTasks(ListFilter{IncludeArchived: true}); len(got) != 1 {
		t.Fatalf("include archived failed, got %d", len(got))
	}
}

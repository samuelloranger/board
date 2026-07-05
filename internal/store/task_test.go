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
	// Global (NULL-project) tasks are pinned into every project view, so
	// project a returns t1, t2 AND g1.
	byProj, _ := st.ListTasks(ListFilter{Project: &a})
	if len(byProj) != 3 {
		t.Fatalf("project filter: want 3 (incl. global) got %d", len(byProj))
	}
	todo := "todo"
	byStatus, _ := st.ListTasks(ListFilter{Project: &a, Status: &todo})
	if len(byStatus) != 2 || byStatus[0].Title != "t1" {
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

func TestUpdateTask(t *testing.T) {
	st := newTestStore(t)
	tk, _ := st.CreateTask(CreateTaskParams{Title: "orig", Tags: []string{"a"}})
	newTitle := "updated"
	pr := "low"
	newTags := []string{"b", "c"}
	got, err := st.UpdateTask(tk.ID, UpdateTaskParams{Title: &newTitle, Priority: &pr, Tags: &newTags})
	if err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if got.Title != "updated" || got.Priority == nil || *got.Priority != "low" {
		t.Fatalf("patch failed: %+v", got)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "b" || got.Tags[1] != "c" {
		t.Fatalf("tags not replaced: %v", got.Tags)
	}
	// Clearing priority with empty-string sentinel.
	empty := ""
	got2, _ := st.UpdateTask(tk.ID, UpdateTaskParams{Priority: &empty})
	if got2.Priority != nil {
		t.Fatalf("priority should be cleared, got %v", got2.Priority)
	}
}

func TestUpdateTaskNotFound(t *testing.T) {
	st := newTestStore(t)
	title := "x"
	if _, err := st.UpdateTask(42, UpdateTaskParams{Title: &title}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound got %v", err)
	}
}

func TestMoveArchiveDelete(t *testing.T) {
	st := newTestStore(t)
	tk, _ := st.CreateTask(CreateTaskParams{Title: "x"})

	moved, err := st.MoveTask(tk.ID, "in_progress")
	if err != nil || moved.Status != "in_progress" {
		t.Fatalf("MoveTask: %v %+v", err, moved)
	}
	if _, err := st.MoveTask(tk.ID, "bogus"); err == nil {
		t.Fatal("expected invalid status error")
	}

	arch, err := st.SetArchived(tk.ID, true)
	if err != nil || !arch.Archived {
		t.Fatalf("SetArchived: %v %+v", err, arch)
	}

	if err := st.DeleteTask(tk.ID); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	if _, err := st.GetTask(tk.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("task should be gone, got %v", err)
	}
	if err := st.DeleteTask(tk.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete want ErrNotFound got %v", err)
	}
}

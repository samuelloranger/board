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

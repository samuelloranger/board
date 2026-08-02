package store

import (
	"errors"
	"testing"
)

func TestHandoffAndResume(t *testing.T) {
	st := newTestStore(t)
	p := "proj"
	a, _ := st.CreateTask(CreateTaskParams{Title: "wip", Status: "in_progress", Project: &p})
	b, _ := st.CreateTask(CreateTaskParams{Title: "todo", Status: "todo", Project: &p})

	handed, err := st.Handoff(b.ID, "human", "need creds")
	if err != nil || handed.HandoffTo == nil || *handed.HandoffTo != "human" {
		t.Fatalf("Handoff: %v %+v", err, handed)
	}

	res, err := st.Resume(&p)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if len(res.InProgress) != 1 || res.InProgress[0].ID != a.ID {
		t.Fatalf("resume in_progress wrong: %+v", res.InProgress)
	}
	if len(res.Handoffs) != 1 || res.Handoffs[0].ID != b.ID {
		t.Fatalf("resume handoffs wrong: %+v", res.Handoffs)
	}

	// Moving a handed-off task clears the handoff (ownership taken).
	moved, _ := st.MoveTask(b.ID, "in_progress")
	if moved.HandoffTo != nil {
		t.Fatalf("move should clear handoff, got %v", moved.HandoffTo)
	}

	if _, err := st.Handoff(9999, "x", "y"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound got %v", err)
	}
}

func TestClearHandoff(t *testing.T) {
	st := newTestStore(t)
	tk, _ := st.CreateTask(CreateTaskParams{Title: "parked"})
	if _, err := st.Handoff(tk.ID, "cursor", "parked for later"); err != nil {
		t.Fatal(err)
	}
	cleared, err := st.ClearHandoff(tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.HandoffTo != nil || cleared.HandoffReason != nil {
		t.Fatalf("expected cleared, got %+v", cleared)
	}
	again, err := st.ClearHandoff(tk.ID)
	if err != nil || again.HandoffTo != nil {
		t.Fatalf("idempotent clear: %v %+v", err, again)
	}
	if _, err := st.ClearHandoff(9999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound got %v", err)
	}
}

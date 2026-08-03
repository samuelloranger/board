package store

import (
	"errors"
	"testing"
)

func TestAddNote(t *testing.T) {
	st := newTestStore(t)
	tk, _ := st.CreateTask(CreateTaskParams{Title: "x"})
	n, err := st.AddNote(tk.ID, "found the bug", "agent")
	if err != nil || n.ID == 0 || n.Body != "found the bug" {
		t.Fatalf("AddNote: %v %+v", err, n)
	}
	got, _ := st.GetTask(tk.ID)
	if len(got.Notes) != 1 || got.Notes[0].Body != "found the bug" {
		t.Fatalf("note not attached: %+v", got.Notes)
	}
	if _, err := st.AddNote(9999, "x", "agent"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound got %v", err)
	}
	if _, err := st.AddNote(tk.ID, "", "agent"); err == nil {
		t.Fatal("empty body should error")
	}
}

func TestAddNoteAuthor(t *testing.T) {
	st := newTestStore(t)
	tk, _ := st.CreateTask(CreateTaskParams{Title: "t"})
	n, err := st.AddNote(tk.ID, "hello", "human")
	if err != nil {
		t.Fatal(err)
	}
	if n.Author != "human" {
		t.Fatalf("author=%q", n.Author)
	}
	got, _ := st.GetTask(tk.ID)
	if len(got.Notes) != 1 || got.Notes[0].Author != "human" {
		t.Fatalf("GetTask notes: %+v", got.Notes)
	}
	if _, err := st.AddNote(tk.ID, "x", "nope"); err == nil {
		t.Fatal("want invalid author error")
	}
	empty, err := st.AddNote(tk.ID, "legacy", "")
	if err != nil || empty.Author != "" {
		t.Fatalf("empty author: %v %+v", err, empty)
	}
}

func TestRecentNotesAgentLimit(t *testing.T) {
	st := newTestStore(t)
	tk, _ := st.CreateTask(CreateTaskParams{Title: "t"})
	_, _ = st.AddNote(tk.ID, "h1", "human")
	_, _ = st.AddNote(tk.ID, "a1", "agent")
	_, _ = st.AddNote(tk.ID, "a2", "agent")
	_, _ = st.AddNote(tk.ID, "a3", "agent")
	_, _ = st.AddNote(tk.ID, "a4", "agent")
	notes, err := st.RecentNotes(tk.ID, 3, "agent")
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 3 || notes[0].Body != "a2" || notes[2].Body != "a4" {
		t.Fatalf("%+v", notes)
	}
}

func TestAttachRecentAgentNotesOnlyRunning(t *testing.T) {
	st := newTestStore(t)
	tk, _ := st.CreateTask(CreateTaskParams{Title: "t", Status: "in_progress"})
	_, _ = st.AddNote(tk.ID, "note", "agent")
	b, _ := st.GetBoard(nil)
	_ = st.AttachRecentAgentNotes(b, 3)
	for _, tsk := range b.InProgress {
		if tsk.ID == tk.ID && len(tsk.RecentAgentNotes) != 0 {
			t.Fatal("expected no notes without running run")
		}
	}
	_, _ = st.CreateRun(tk.ID, "cursor", 1)
	b, _ = st.GetBoard(nil)
	_ = st.AttachRecentAgentNotes(b, 3)
	for _, tsk := range b.InProgress {
		if tsk.ID == tk.ID {
			if len(tsk.RecentAgentNotes) != 1 || tsk.RecentAgentNotes[0].Body != "note" {
				t.Fatalf("%+v", tsk.RecentAgentNotes)
			}
			return
		}
	}
	t.Fatal("task missing")
}

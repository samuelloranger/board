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

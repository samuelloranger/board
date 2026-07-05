package store

import (
	"errors"
	"testing"
)

func TestAddNote(t *testing.T) {
	st := newTestStore(t)
	tk, _ := st.CreateTask(CreateTaskParams{Title: "x"})
	n, err := st.AddNote(tk.ID, "found the bug")
	if err != nil || n.ID == 0 || n.Body != "found the bug" {
		t.Fatalf("AddNote: %v %+v", err, n)
	}
	got, _ := st.GetTask(tk.ID)
	if len(got.Notes) != 1 || got.Notes[0].Body != "found the bug" {
		t.Fatalf("note not attached: %+v", got.Notes)
	}
	if _, err := st.AddNote(9999, "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound got %v", err)
	}
	if _, err := st.AddNote(tk.ID, ""); err == nil {
		t.Fatal("empty body should error")
	}
}

package store

import (
	"errors"
	"testing"
)

func TestCreateRunRejectsSecond(t *testing.T) {
	st := newTestStore(t)
	tk, _ := st.CreateTask(CreateTaskParams{Title: "t"})
	if _, err := st.CreateRun(tk.ID, "cursor", 1234); err != nil {
		t.Fatal(err)
	}
	_, err := st.CreateRun(tk.ID, "cursor", 5678)
	if !errors.Is(err, ErrRunActive) {
		t.Fatalf("got %v", err)
	}
	tk2, _ := st.GetTask(tk.ID)
	if tk2.Status != "in_progress" {
		t.Fatalf("expected in_progress, got %s", tk2.Status)
	}
}

func TestFinishRunCancelsQuestions(t *testing.T) {
	st := newTestStore(t)
	tk, _ := st.CreateTask(CreateTaskParams{Title: "t"})
	r, _ := st.CreateRun(tk.ID, "cursor", 1)
	q, _ := st.CreateQuestion(tk.ID, "q?")
	code := 1
	if _, err := st.FinishRun(r.ID, "failed", &code, "boom"); err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetQuestion(q.ID)
	if got.Status != "cancelled" {
		t.Fatalf("%+v", got)
	}
	fin, _ := st.GetRun(r.ID)
	if fin.Message != "boom" || fin.Status != "failed" {
		t.Fatalf("finish: %+v", fin)
	}
}

func TestReconcileOrphanRuns(t *testing.T) {
	st := newTestStore(t)
	tk, _ := st.CreateTask(CreateTaskParams{Title: "t"})
	st.CreateRun(tk.ID, "cursor", 99999)
	n, err := st.ReconcileOrphanRuns(func(pid int) bool { return false })
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	_, err = st.ActiveRunForTask(tk.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("still active: %v", err)
	}
}

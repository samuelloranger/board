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

func TestFinishRunCancelsQuestionsOnlyWhenKilled(t *testing.T) {
	st := newTestStore(t)
	tk, _ := st.CreateTask(CreateTaskParams{Title: "t"})
	r, _ := st.CreateRun(tk.ID, "cursor", 1)
	q, _ := st.CreateQuestion(tk.ID, "q?")
	code := 1
	if _, err := st.FinishRun(r.ID, "failed", &code, "boom"); err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetQuestion(q.ID)
	if got.Status != "pending" {
		t.Fatalf("failed run should leave ask pending, got %+v", got)
	}
	fin, _ := st.GetRun(r.ID)
	if fin.Message != "boom" || fin.Status != "failed" {
		t.Fatalf("finish: %+v", fin)
	}

	tk2, _ := st.CreateTask(CreateTaskParams{Title: "t2"})
	r2, _ := st.CreateRun(tk2.ID, "cursor", 2)
	q2, _ := st.CreateQuestion(tk2.ID, "q2?")
	if _, err := st.FinishRun(r2.ID, "killed", nil, "cancelled from board UI"); err != nil {
		t.Fatal(err)
	}
	got2, _ := st.GetQuestion(q2.ID)
	if got2.Status != "cancelled" {
		t.Fatalf("killed run should cancel ask, got %+v", got2)
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

func TestSetRunWait(t *testing.T) {
	st := newTestStore(t)
	tk, _ := st.CreateTask(CreateTaskParams{Title: "t"})
	run, err := st.CreateRun(tk.ID, "cursor", 1)
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.SetRunWait(tk.ID, "ci")
	if err != nil || got.Wait != "ci" {
		t.Fatalf("set ci: %v %+v", err, got)
	}
	got, err = st.SetRunWait(tk.ID, "")
	if err != nil || got.Wait != "" {
		t.Fatalf("clear: %v %+v", err, got)
	}
	if _, err := st.SetRunWait(tk.ID, "nope"); err == nil {
		t.Fatal("want invalid wait")
	}
	_, _ = st.FinishRun(run.ID, "exited", nil, "done")
	finished, _ := st.GetRun(run.ID)
	if finished.Wait != "" {
		t.Fatalf("finish should clear wait, got %q", finished.Wait)
	}
	if _, err := st.SetRunWait(tk.ID, "ci"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no active run: %v", err)
	}
}

func TestReportRunProgressStdoutDoesNotClobberNote(t *testing.T) {
	st := newTestStore(t)
	tk, _ := st.CreateTask(CreateTaskParams{Title: "t"})
	run, err := st.CreateRun(tk.ID, "cursor", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ReportRunProgress(run.ID, tk.ID, "stdout-1", MessageSourceStdout); err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetRun(run.ID)
	if got.Message != "stdout-1" || got.MessageSource != MessageSourceStdout {
		t.Fatalf("after stdout: %+v", got)
	}
	if err := st.ReportRunProgress(run.ID, tk.ID, "agent note", MessageSourceNote); err != nil {
		t.Fatal(err)
	}
	got, _ = st.GetRun(run.ID)
	if got.Message != "agent note" || got.MessageSource != MessageSourceNote {
		t.Fatalf("after note: %+v", got)
	}
	if err := st.ReportRunProgress(run.ID, tk.ID, "stdout-2", MessageSourceStdout); err != nil {
		t.Fatal(err)
	}
	got, _ = st.GetRun(run.ID)
	if got.Message != "agent note" || got.MessageSource != MessageSourceNote {
		t.Fatalf("stdout must not clobber note: %+v", got)
	}
	// Matching text still must not overwrite — provenance, not string equality.
	if err := st.ReportRunProgress(run.ID, tk.ID, "agent note", MessageSourceStdout); err != nil {
		t.Fatal(err)
	}
	got, _ = st.GetRun(run.ID)
	if got.MessageSource != MessageSourceNote {
		t.Fatalf("stdout matching note text must not flip source: %+v", got)
	}
}

func TestAddNoteMirrorsWithNoteSource(t *testing.T) {
	st := newTestStore(t)
	tk, _ := st.CreateTask(CreateTaskParams{Title: "t"})
	run, _ := st.CreateRun(tk.ID, "cursor", 1)
	_, err := st.AddNote(tk.ID, "live progress", "agent")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetRun(run.ID)
	if got.Message != "live progress" || got.MessageSource != MessageSourceNote {
		t.Fatalf("%+v", got)
	}
}

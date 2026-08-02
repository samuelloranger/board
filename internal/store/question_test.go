package store

import (
	"context"
	"testing"
	"time"
)

func TestQuestionAskAnswer(t *testing.T) {
	st := newTestStore(t)
	tk, _ := st.CreateTask(CreateTaskParams{Title: "t"})
	q, err := st.CreateQuestion(tk.ID, "Ship it?")
	if err != nil || q.Status != "pending" {
		t.Fatalf("CreateQuestion: %+v %v", q, err)
	}
	done := make(chan string, 1)
	go func() {
		ans, err := st.WaitForAnswer(context.Background(), q.ID, 20*time.Millisecond)
		if err != nil {
			t.Errorf("Wait: %v", err)
			return
		}
		done <- ans
	}()
	time.Sleep(40 * time.Millisecond)
	if _, err := st.AnswerQuestion(q.ID, "yes"); err != nil {
		t.Fatalf("Answer: %v", err)
	}
	select {
	case ans := <-done:
		if ans != "yes" {
			t.Fatalf("got %q", ans)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WaitForAnswer timed out")
	}
}

func TestCancelPendingQuestions(t *testing.T) {
	st := newTestStore(t)
	tk, _ := st.CreateTask(CreateTaskParams{Title: "t"})
	q, _ := st.CreateQuestion(tk.ID, "q?")
	n, err := st.CancelPendingQuestions(tk.ID)
	if err != nil || n != 1 {
		t.Fatalf("cancel: n=%d err=%v", n, err)
	}
	got, _ := st.GetQuestion(q.ID)
	if got.Status != "cancelled" {
		t.Fatalf("%+v", got)
	}
	_, err = st.WaitForAnswer(context.Background(), q.ID, time.Millisecond)
	if err == nil {
		t.Fatal("expected error on cancelled")
	}
}

func TestAnswerQuestionNotPending(t *testing.T) {
	st := newTestStore(t)
	tk, _ := st.CreateTask(CreateTaskParams{Title: "t"})
	q, _ := st.CreateQuestion(tk.ID, "q?")
	st.AnswerQuestion(q.ID, "a")
	_, err := st.AnswerQuestion(q.ID, "b")
	if err == nil {
		t.Fatal("expected error re-answering")
	}
}

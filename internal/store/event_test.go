package store

import "testing"

func TestEventsEmittedOnCreateAndMove(t *testing.T) {
	st := newTestStore(t)
	tk, _ := st.CreateTask(CreateTaskParams{Title: "x"})
	st.MoveTask(tk.ID, "done")

	evs, err := st.Events(0, 100)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if len(evs) < 2 {
		t.Fatalf("expected >=2 events, got %d", len(evs))
	}
	if evs[0].Kind != "created" || evs[1].Kind != "moved" {
		t.Fatalf("wrong event kinds: %+v", evs)
	}
	// since filter
	after, _ := st.Events(evs[0].ID, 100)
	if len(after) != len(evs)-1 {
		t.Fatalf("since filter wrong: %d", len(after))
	}
}

func TestLogEvent(t *testing.T) {
	st := newTestStore(t)
	if err := st.LogEvent("tool", "Edit main.go"); err != nil {
		t.Fatalf("LogEvent: %v", err)
	}
	evs, _ := st.Events(0, 10)
	if len(evs) != 1 || evs[0].Kind != "tool" || evs[0].TaskID != nil {
		t.Fatalf("LogEvent wrong: %+v", evs)
	}
}

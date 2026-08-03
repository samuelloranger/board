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

func TestRecentEvents(t *testing.T) {
	st := newTestStore(t)
	for i := 0; i < 10; i++ {
		if err := st.LogEvent("tool", "x"); err != nil {
			t.Fatalf("LogEvent: %v", err)
		}
	}
	evs, err := st.RecentEvents(3)
	if err != nil {
		t.Fatalf("RecentEvents: %v", err)
	}
	if len(evs) != 3 {
		t.Fatalf("want 3, got %d", len(evs))
	}
	// Ascending id order, and the newest three overall.
	if evs[0].ID >= evs[1].ID || evs[1].ID >= evs[2].ID {
		t.Fatalf("not ascending: %+v", evs)
	}
	all, _ := st.Events(0, 100)
	want := all[len(all)-3:]
	for i := range evs {
		if evs[i].ID != want[i].ID {
			t.Fatalf("tail mismatch at %d: got %d want %d", i, evs[i].ID, want[i].ID)
		}
	}
}

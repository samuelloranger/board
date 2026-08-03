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
	if err := st.LogEvent("session", "started"); err != nil {
		t.Fatalf("LogEvent: %v", err)
	}
	evs, _ := st.Events(0, 10)
	if len(evs) != 1 || evs[0].Kind != "session" || evs[0].TaskID != nil {
		t.Fatalf("LogEvent wrong: %+v", evs)
	}
}

func TestEventSignalWakes(t *testing.T) {
	st := newTestStore(t)
	sig := st.EventSignal()
	select {
	case <-sig:
		t.Fatal("signal should not be closed yet")
	default:
	}
	if err := st.LogEvent("session", "ping"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-sig:
	default:
		t.Fatal("expected EventSignal to close after LogEvent")
	}
	id, err := st.MaxEventID()
	if err != nil || id == 0 {
		t.Fatalf("MaxEventID: id=%d err=%v", id, err)
	}
}

func TestLogEventIgnoresTool(t *testing.T) {
	st := newTestStore(t)
	if err := st.LogEvent("tool", "Read"); err != nil {
		t.Fatalf("LogEvent tool: %v", err)
	}
	evs, err := st.Events(0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 0 {
		t.Fatalf("tool events must not be stored, got %+v", evs)
	}
}

func TestOpenPurgesToolEvents(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/board.db"
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// Bypass LogEvent so we can plant legacy rows the way old hooks did.
	if _, err := st.db.Exec(
		`INSERT INTO events (task_id, kind, detail, created_at) VALUES (NULL, 'tool', 'Bash', ?)`,
		now(),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(
		`INSERT INTO events (task_id, kind, detail, created_at) VALUES (NULL, 'session', 'ended', ?)`,
		now(),
	); err != nil {
		t.Fatal(err)
	}
	// Rewind past the tool-purge step so the next Open re-runs v7 once.
	if err := setUserVersion(st.db, 6); err != nil {
		t.Fatal(err)
	}
	st.Close()

	st2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	evs, err := st2.Events(0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 || evs[0].Kind != "session" {
		t.Fatalf("want only session after purge, got %+v", evs)
	}
	ver, err := userVersion(st2.db)
	if err != nil {
		t.Fatal(err)
	}
	if ver != schemaVersion {
		t.Fatalf("user_version after purge: want %d, got %d", schemaVersion, ver)
	}
}

func TestOpenDoesNotRepurgeToolEvents(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/board.db"
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// Fully migrated DB: planting a tool row must survive a subsequent Open.
	if _, err := st.db.Exec(
		`INSERT INTO events (task_id, kind, detail, created_at) VALUES (NULL, 'tool', 'Bash', ?)`,
		now(),
	); err != nil {
		t.Fatal(err)
	}
	st.Close()

	st2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	evs, err := st2.Events(0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 || evs[0].Kind != "tool" {
		t.Fatalf("tool cleanup must be one-shot, got %+v", evs)
	}
}

func TestRecentEvents(t *testing.T) {
	st := newTestStore(t)
	for i := 0; i < 10; i++ {
		if err := st.LogEvent("session", "x"); err != nil {
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

func TestRecentEventsSkipsNoise(t *testing.T) {
	st := newTestStore(t)
	tk, _ := st.CreateTask(CreateTaskParams{Title: "t"})
	// Plant legacy tool + live run_progress noise alongside a real session event.
	_, _ = st.db.Exec(
		`INSERT INTO events (task_id, kind, detail, created_at) VALUES (NULL, 'tool', 'Bash', ?)`, now())
	st.emit(&tk.ID, "run_progress", "thinking…")
	if err := st.LogEvent("session", "ended"); err != nil {
		t.Fatal(err)
	}
	evs, err := st.RecentEvents(10)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range evs {
		if e.Kind == "tool" || e.Kind == "run_progress" {
			t.Fatalf("noise in RecentEvents: %+v", e)
		}
	}
	found := false
	for _, e := range evs {
		if e.Kind == "session" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected session event, got %+v", evs)
	}
}

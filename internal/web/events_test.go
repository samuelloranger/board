package web

import (
	"sync"
	"testing"
	"time"

	"github.com/samuelloranger/board/internal/store"
)

// With the backup poll disabled, every event must still arrive via the
// same-process signal. Fetching EventSignal *after* polling would drop the
// wake for any event written in between, and this test would time out.
func TestEventHubDeliversWithoutBackupPoll(t *testing.T) {
	st := newStore(t)
	hub := newEventHub(st)
	hub.pollInterval = time.Hour
	ch, unsub, ok := hub.subscribe()
	if !ok {
		t.Fatal("subscribe refused")
	}
	defer unsub()

	// Emit strictly one at a time and require delivery before the next write.
	// That pins each write into the loop's poll/subscribe window: taking the
	// signal after polling loses that wake, and with no backup tick the read
	// below blocks forever.
	const n = 50
	for i := 0; i < n; i++ {
		if _, err := st.CreateTask(store.CreateTaskParams{Title: "t"}); err != nil {
			t.Fatal(err)
		}
		select {
		case batch, open := <-ch:
			if !open {
				t.Fatalf("hub dropped the subscriber at event %d", i)
			}
			if len(batch) == 0 {
				t.Fatalf("empty batch at event %d", i)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("event %d never arrived — a signal wake was missed", i)
		}
	}
}

// The wake-loss window is between a poll and the wait that follows it. afterPoll
// puts a write exactly there: with the signal taken before the poll it is already
// held and the wake lands, whereas taking it afterwards grabs a channel the write
// has already replaced and the event is stranded until the backup tick.
func TestEventHubSignalTakenBeforePoll(t *testing.T) {
	st := newStore(t)
	var once sync.Once
	written := make(chan struct{})
	hookedHub := newEventHub(st)
	hookedHub.pollInterval = time.Hour
	hookedHub.afterPoll = func() {
		once.Do(func() {
			if _, err := st.CreateTask(store.CreateTaskParams{Title: "in-window"}); err != nil {
				t.Error(err)
			}
			close(written)
		})
	}

	ch, unsub, ok := hookedHub.subscribe()
	if !ok {
		t.Fatal("subscribe refused")
	}
	defer unsub()

	<-written
	select {
	case batch, open := <-ch:
		if !open {
			t.Fatal("hub dropped the subscriber")
		}
		if len(batch) == 0 {
			t.Fatal("empty batch")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("event written inside the poll/wait window never arrived")
	}
}

func TestEventHubCapsClients(t *testing.T) {
	st := newStore(t)
	hub := newEventHub(st)
	unsubs := make([]func(), 0, maxSSEClients)
	for i := 0; i < maxSSEClients; i++ {
		_, unsub, ok := hub.subscribe()
		if !ok {
			t.Fatalf("subscribe %d refused below the cap", i)
		}
		unsubs = append(unsubs, unsub)
	}
	if _, _, ok := hub.subscribe(); ok {
		t.Fatal("subscribe past the cap was allowed")
	}
	unsubs[0]()
	if _, unsub, ok := hub.subscribe(); !ok {
		t.Fatal("subscribe refused after a slot freed")
	} else {
		unsub()
	}
	for _, u := range unsubs[1:] {
		u()
	}
}

package web

import (
	"sync"
	"time"

	"github.com/samuelloranger/board/internal/store"
)

// maxSSEClients caps concurrent /api/events streams. Beyond this, new
// connections get HTTP 503. Local board; a handful of tabs is normal.
const maxSSEClients = 32

// eventPollInterval is the backup poll cadence for events written by other
// board processes (MCP / CLI) that share the DB but not this process's wake.
const eventPollInterval = time.Second

// eventHub fans store events out to SSE clients with one shared DB poller.
type eventHub struct {
	st *store.Store

	// pollInterval is the backup poll cadence; afterPoll runs between a poll
	// and the wait that follows it. Both are set once at construction and only
	// varied by tests, which disable the safety net and write inside that
	// window to prove the signal-before-poll ordering holds.
	pollInterval time.Duration
	afterPoll    func()

	mu      sync.Mutex
	clients map[chan []store.Event]struct{}
	n       int
	running bool
	stop    chan struct{}
}

func newEventHub(st *store.Store) *eventHub {
	return &eventHub{
		st:           st,
		clients:      make(map[chan []store.Event]struct{}),
		pollInterval: eventPollInterval,
		afterPoll:    func() {},
	}
}

// subscribe registers a client. ok is false when the connection cap is hit.
func (h *eventHub) subscribe() (ch <-chan []store.Event, unsubscribe func(), ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.n >= maxSSEClients {
		return nil, nil, false
	}
	c := make(chan []store.Event, 16)
	h.clients[c] = struct{}{}
	h.n++
	if !h.running {
		h.running = true
		h.stop = make(chan struct{})
		go h.loop(h.stop)
	}
	unsub := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.removeLocked(c)
	}
	return c, unsub, true
}

func (h *eventHub) removeLocked(c chan []store.Event) {
	if _, ok := h.clients[c]; !ok {
		return
	}
	delete(h.clients, c)
	close(c)
	h.n--
	if h.n == 0 && h.running {
		close(h.stop)
		h.running = false
		h.stop = nil
	}
}

func (h *eventHub) loop(stop <-chan struct{}) {
	since, err := h.st.MaxEventID()
	if err != nil {
		since = 0
	}
	ticker := time.NewTicker(h.pollInterval)
	defer ticker.Stop()
	for {
		// Grab the signal channel BEFORE polling. Taken after, an emit landing
		// between poll and EventSignal would close the old channel and install
		// a fresh one, so that wake is missed until the backup tick.
		sig := h.st.EventSignal()
		h.poll(&since)
		h.afterPoll()
		select {
		case <-stop:
			return
		case <-sig:
			// same-process emit — poll immediately
		case <-ticker.C:
			// cross-process writers / missed wakes
		}
	}
}

func (h *eventHub) poll(since *int64) {
	evs, err := h.st.Events(*since, 200)
	if err != nil || len(evs) == 0 {
		return
	}
	*since = evs[len(evs)-1].ID
	h.broadcast(evs)
}

func (h *eventHub) broadcast(evs []store.Event) {
	h.mu.Lock()
	clients := make([]chan []store.Event, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.Unlock()

	for _, c := range clients {
		select {
		case c <- evs:
		default:
			// Slow consumer: drop them. EventSource reconnects and seeds again.
			h.mu.Lock()
			h.removeLocked(c)
			h.mu.Unlock()
		}
	}
}

package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/samuelloranger/board/internal/store"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "b.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestAPICreateAndBoard(t *testing.T) {
	st := newStore(t)
	srv := httptest.NewServer(Handler(st))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/tasks", "application/json",
		strings.NewReader(`{"title":"web task","status":"todo"}`))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("create: %v status=%v", err, resp.StatusCode)
	}
	r2, _ := http.Get(srv.URL + "/api/board?project=*")
	var b store.Board
	json.NewDecoder(r2.Body).Decode(&b)
	if len(b.Todo) != 1 || b.Todo[0].Title != "web task" {
		t.Fatalf("board wrong: %+v", b)
	}
}

func TestResumeEndpoint(t *testing.T) {
	st := newStore(t)
	p := "proj"
	st.CreateTask(store.CreateTaskParams{Title: "wip", Status: "in_progress", Project: &p})
	srv := httptest.NewServer(Handler(st))
	defer srv.Close()

	r, _ := http.Get(srv.URL + "/api/resume?project=proj")
	var res store.ResumeResult
	json.NewDecoder(r.Body).Decode(&res)
	if len(res.InProgress) != 1 {
		t.Fatalf("resume endpoint wrong: %+v", res)
	}
}

func TestEventsSSEEmitsExisting(t *testing.T) {
	st := newStore(t)
	st.CreateTask(store.CreateTaskParams{Title: "x"}) // produces a 'created' event
	srv := httptest.NewServer(Handler(st))
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/api/events?since=0", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		t.Fatalf("sse connect: %v", err)
	}
	defer resp.Body.Close()
	buf := make([]byte, 256)
	n, _ := resp.Body.Read(buf)
	if !strings.Contains(string(buf[:n]), "created") {
		t.Fatalf("expected a created event in SSE stream, got %q", string(buf[:n]))
	}
}

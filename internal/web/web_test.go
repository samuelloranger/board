package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

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

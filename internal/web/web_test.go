package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/samuelloranger/board/internal/agent"
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

func TestGetTaskAPI(t *testing.T) {
	st := newStore(t)
	tk, _ := st.CreateTask(store.CreateTaskParams{Title: "full", Description: "desc"})
	st.AddNote(tk.ID, "n1")
	srv := httptest.NewServer(Handler(st))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/tasks/" + strconv.FormatInt(tk.ID, 10))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("%v %d", err, resp.StatusCode)
	}
	var got store.Task
	json.NewDecoder(resp.Body).Decode(&got)
	if got.Title != "full" || got.Description != "desc" || len(got.Notes) != 1 {
		t.Fatalf("%+v", got)
	}
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

func TestProjectPathAPI(t *testing.T) {
	st := newStore(t)
	srv := httptest.NewServer(Handler(st))
	defer srv.Close()
	dir := t.TempDir()
	body, _ := json.Marshal(map[string]string{"path": dir})
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/projects/board/path", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("status=%d err=%v", resp.StatusCode, err)
	}
	r2, _ := http.Get(srv.URL + "/api/projects/paths")
	var list []store.ProjectPath
	json.NewDecoder(r2.Body).Decode(&list)
	if len(list) != 1 || list[0].Path != dir {
		t.Fatalf("%+v", list)
	}
}

func TestAnswerQuestionAPI(t *testing.T) {
	st := newStore(t)
	tk, _ := st.CreateTask(store.CreateTaskParams{Title: "t"})
	q, _ := st.CreateQuestion(tk.ID, "q?")
	srv := httptest.NewServer(Handler(st))
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/api/questions/"+strconv.FormatInt(q.ID, 10)+"/answer",
		"application/json", strings.NewReader(`{"answer":"a"}`))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("%v %d", err, resp.StatusCode)
	}
}

func TestRunNeedPath(t *testing.T) {
	st := newStore(t)
	p := "board"
	tk, _ := st.CreateTask(store.CreateTaskParams{Title: "t", Project: &p})
	fake := &agent.FakeRunner{}
	srv := httptest.NewServer(HandlerConfig(Config{Store: st, Runner: fake}))
	defer srv.Close()
	resp, _ := http.Post(srv.URL+"/api/tasks/"+strconv.FormatInt(tk.ID, 10)+"/run",
		"application/json", strings.NewReader(`{"agent":"cursor"}`))
	if resp.StatusCode != 409 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["need_path"] != true {
		t.Fatalf("%v", body)
	}
}

func TestRunStartsAgent(t *testing.T) {
	st := newStore(t)
	p := "board"
	tk, _ := st.CreateTask(store.CreateTaskParams{Title: "t", Project: &p})
	dir := t.TempDir()
	st.SetProjectPath("board", dir)
	fake := &agent.FakeRunner{ExitCode: 0}
	srv := httptest.NewServer(HandlerConfig(Config{Store: st, Runner: fake}))
	defer srv.Close()
	resp, _ := http.Post(srv.URL+"/api/tasks/"+strconv.FormatInt(tk.ID, 10)+"/run",
		"application/json", strings.NewReader(`{"agent":"cursor"}`))
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if fake.LastCwd != dir || !strings.Contains(fake.LastPrompt, "board web UI") {
		t.Fatalf("cwd=%q prompt=%q", fake.LastCwd, fake.LastPrompt)
	}
	tk2, _ := st.GetTask(tk.ID)
	if tk2.Status != "in_progress" {
		t.Fatalf("status %s", tk2.Status)
	}
	// Allow Wait goroutine to finish.
	time.Sleep(50 * time.Millisecond)
}

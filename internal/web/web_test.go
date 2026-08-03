package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
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

const testCSRF = "test-csrf-token-bbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func newStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "b.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func newHandler(st *store.Store) http.Handler {
	return HandlerConfig(Config{Store: st, CSRFToken: testCSRF})
}

func newHandlerCfg(cfg Config) http.Handler {
	if cfg.CSRFToken == "" {
		cfg.CSRFToken = testCSRF
	}
	return HandlerConfig(cfg)
}

func doMutate(t *testing.T, method, urlStr, contentType string, body io.Reader) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, urlStr, body)
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set(csrfHeader, testCSRF)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestGetTaskAPI(t *testing.T) {
	st := newStore(t)
	tk, _ := st.CreateTask(store.CreateTaskParams{Title: "full", Description: "desc"})
	st.AddNote(tk.ID, "n1", "agent")
	srv := httptest.NewServer(newHandler(st))
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
	srv := httptest.NewServer(newHandler(st))
	defer srv.Close()

	resp := doMutate(t, http.MethodPost, srv.URL+"/api/tasks", "application/json",
		strings.NewReader(`{"title":"web task","status":"todo"}`))
	if resp.StatusCode != 200 {
		t.Fatalf("create: status=%v", resp.StatusCode)
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
	srv := httptest.NewServer(newHandler(st))
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
	srv := httptest.NewServer(newHandler(st))
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

func TestEventsSSECap(t *testing.T) {
	st := newStore(t)
	srv := httptest.NewServer(newHandler(st))
	defer srv.Close()

	type conn struct {
		cancel context.CancelFunc
		resp   *http.Response
	}
	conns := make([]conn, 0, maxSSEClients)
	defer func() {
		for _, c := range conns {
			c.resp.Body.Close()
			c.cancel()
		}
	}()

	for i := 0; i < maxSSEClients; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/api/events?since=0", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			cancel()
			t.Fatalf("connect %d: %v", i, err)
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			cancel()
			t.Fatalf("connect %d: status %d", i, resp.StatusCode)
		}
		conns = append(conns, conn{cancel: cancel, resp: resp})
	}
	// Let handlers finish subscribe before probing the cap.
	time.Sleep(100 * time.Millisecond)

	resp, err := http.Get(srv.URL + "/api/events?since=0")
	if err != nil {
		t.Fatalf("overflow connect: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("want 503 at cap, got %d", resp.StatusCode)
	}
}

func TestEventsSSELiveBroadcast(t *testing.T) {
	st := newStore(t)
	srv := httptest.NewServer(newHandler(st))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/api/events?since=0", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("sse connect: %v", err)
	}
	defer resp.Body.Close()

	got := make(chan struct{}, 1)
	go func() {
		buf := make([]byte, 4096)
		var acc string
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				acc += string(buf[:n])
				if strings.Contains(acc, "synced") && strings.Contains(acc, "live") {
					got <- struct{}{}
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// Give the handler time to seed + park on the hub channel.
	time.Sleep(100 * time.Millisecond)
	if err := st.LogEvent("session", "live"); err != nil {
		t.Fatal(err)
	}

	select {
	case <-got:
	case <-ctx.Done():
		t.Fatal("timed out waiting for live SSE event")
	}
}

func TestProjectPathAPI(t *testing.T) {
	st := newStore(t)
	srv := httptest.NewServer(newHandler(st))
	defer srv.Close()
	dir := t.TempDir()
	body, _ := json.Marshal(map[string]string{"path": dir})
	resp := doMutate(t, http.MethodPut, srv.URL+"/api/projects/board/path", "application/json", bytes.NewReader(body))
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
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
	srv := httptest.NewServer(newHandler(st))
	defer srv.Close()
	resp := doMutate(t, http.MethodPost, srv.URL+"/api/questions/"+strconv.FormatInt(q.ID, 10)+"/answer",
		"application/json", strings.NewReader(`{"answer":"a"}`))
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestRunNeedPath(t *testing.T) {
	st := newStore(t)
	p := "board"
	tk, _ := st.CreateTask(store.CreateTaskParams{Title: "t", Project: &p})
	fake := &agent.FakeRunner{}
	srv := httptest.NewServer(newHandlerCfg(Config{Store: st, Runner: fake}))
	defer srv.Close()
	resp := doMutate(t, http.MethodPost, srv.URL+"/api/tasks/"+strconv.FormatInt(tk.ID, 10)+"/run",
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
	srv := httptest.NewServer(newHandlerCfg(Config{Store: st, Runner: fake}))
	defer srv.Close()
	resp := doMutate(t, http.MethodPost, srv.URL+"/api/tasks/"+strconv.FormatInt(tk.ID, 10)+"/run",
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

func TestRunAcceptsClaudeAndCodex(t *testing.T) {
	st := newStore(t)
	p := "board"
	dir := t.TempDir()
	st.SetProjectPath("board", dir)
	for _, name := range []string{"claude", "codex"} {
		tk, _ := st.CreateTask(store.CreateTaskParams{Title: name, Project: &p})
		fake := &agent.FakeRunner{ExitCode: 0}
		srv := httptest.NewServer(newHandlerCfg(Config{Store: st, Runner: fake}))
		resp := doMutate(t, http.MethodPost, srv.URL+"/api/tasks/"+strconv.FormatInt(tk.ID, 10)+"/run",
			"application/json", strings.NewReader(`{"agent":"`+name+`"}`))
		if resp.StatusCode != 200 {
			t.Fatalf("%s status %d", name, resp.StatusCode)
		}
		var body map[string]any
		json.NewDecoder(resp.Body).Decode(&body)
		srv.Close()
		if body["agent"] != name {
			t.Fatalf("%s body=%v", name, body)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestRunRejectsUnknownAgent(t *testing.T) {
	st := newStore(t)
	p := "board"
	tk, _ := st.CreateTask(store.CreateTaskParams{Title: "t", Project: &p})
	st.SetProjectPath("board", t.TempDir())
	srv := httptest.NewServer(newHandler(st))
	defer srv.Close()
	resp := doMutate(t, http.MethodPost, srv.URL+"/api/tasks/"+strconv.FormatInt(tk.ID, 10)+"/run",
		"application/json", strings.NewReader(`{"agent":"windsurf"}`))
	if resp.StatusCode != 400 {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestMutationRequiresCSRF(t *testing.T) {
	st := newStore(t)
	srv := httptest.NewServer(newHandler(st))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/tasks", "application/json",
		strings.NewReader(`{"title":"no token"}`))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("missing csrf: got %d want 403", resp.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/tasks",
		strings.NewReader(`{"title":"cross"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfHeader, testCSRF)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-site: got %d want 403", resp.StatusCode)
	}

	html, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer html.Body.Close()
	raw, _ := io.ReadAll(html.Body)
	if !bytes.Contains(raw, []byte(`window.__BOARD_CSRF__=`+strconv.Quote(testCSRF))) {
		t.Fatalf("index missing csrf inject: %s", raw[:min(200, len(raw))])
	}
}

func TestWriteJSONErrorStatus(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code int
	}{
		{"not found", store.ErrNotFound, http.StatusNotFound},
		{"validation", store.Invalid("title is required"), http.StatusBadRequest},
		{"validation fmt", store.Invalidf("invalid status %q", "nope"), http.StatusBadRequest},
		{"other", errors.New("sqlite: disk I/O error"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			writeJSON(rec, nil, tc.err)
			if rec.Code != tc.code {
				t.Fatalf("got %d want %d body=%q", rec.Code, tc.code, rec.Body.String())
			}
		})
	}
}

func TestAPIValidationAndNotFoundStatus(t *testing.T) {
	st := newStore(t)
	srv := httptest.NewServer(newHandler(st))
	defer srv.Close()

	resp := doMutate(t, http.MethodPost, srv.URL+"/api/tasks", "application/json",
		strings.NewReader(`{"title":""}`))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty title: got %d want 400", resp.StatusCode)
	}

	resp = doMutate(t, http.MethodPost, srv.URL+"/api/tasks/9999/move", "application/json",
		strings.NewReader(`{"status":"done"}`))
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing task: got %d want 404", resp.StatusCode)
	}
}

// A rebound attacker origin must not be able to read index.html — that page
// carries the CSRF token, so leaking it would unlock every mutation.
func TestHandlerRejectsReboundHost(t *testing.T) {
	st := newStore(t)
	h := newHandler(st)

	for _, path := range []string{"/", "/api/board"} {
		req := httptest.NewRequest(http.MethodGet, "http://attacker.example:7420"+path, nil)
		req.Host = "attacker.example:7420"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("GET %s from rebound host: status = %d, want 403", path, rec.Code)
		}
		if strings.Contains(rec.Body.String(), testCSRF) {
			t.Fatalf("GET %s leaked the CSRF token to a rebound host", path)
		}
	}

	// Same request over loopback still works and does carry the token.
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:7420/", nil)
	req.Host = "127.0.0.1:7420"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("loopback GET /: status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), testCSRF) {
		t.Fatal("loopback GET / did not carry the CSRF token")
	}
}

func TestHandlerAllowsConfiguredHost(t *testing.T) {
	st := newStore(t)
	h := newHandlerCfg(Config{Store: st, AllowedHosts: []string{"board.lan:9000"}})

	req := httptest.NewRequest(http.MethodGet, "http://board.lan:9000/api/board", nil)
	req.Host = "board.lan:9000"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("configured host: status = %d, want 200", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "http://other.lan:9000/api/board", nil)
	req.Host = "other.lan:9000"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unconfigured host: status = %d, want 403", rec.Code)
	}
}

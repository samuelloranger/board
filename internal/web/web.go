package web

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/samuelloranger/board/internal/agent"
	"github.com/samuelloranger/board/internal/store"
)

//go:embed all:ui/dist
var uiFS embed.FS

// Config configures the web handler.
type Config struct {
	Store *store.Store
	// Runner, if set, is used for every agent (tests). Otherwise DefaultRunner(name).
	Runner agent.Runner
	// Runners optionally overrides individual agents when Runner is nil.
	Runners map[string]agent.Runner
	// CSRFToken, if set, is the per-process mutation token (tests). Otherwise random.
	CSRFToken string
	// AllowedHosts are extra Host header values accepted beyond loopback.
	// Needed only when binding a non-loopback --addr. Bare host or host:port.
	// "*" disables the Host check entirely (an unspecified bind like 0.0.0.0
	// can be reached under any name, so there is nothing to match against).
	AllowedHosts []string
}

// procEntry holds a kill func for an in-flight agent process.
type procEntry struct {
	kill func() error
}

// Handler serves the JSON API and embedded UI.
func Handler(st *store.Store) http.Handler {
	return HandlerConfig(Config{Store: st})
}

// HandlerConfig serves the JSON API and embedded UI.
func HandlerConfig(cfg Config) http.Handler {
	st := cfg.Store
	csrf := cfg.CSRFToken
	if csrf == "" {
		csrf = newCSRFToken()
	}

	var (
		mu    sync.Mutex
		procs = map[int64]procEntry{}
		hub   = newEventHub(st)
	)

	resolveRunner := func(name string) (agent.Runner, error) {
		if cfg.Runner != nil {
			return cfg.Runner, nil
		}
		if cfg.Runners != nil {
			if r, ok := cfg.Runners[name]; ok {
				return r, nil
			}
		}
		return agent.DefaultRunner(name)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/board", func(w http.ResponseWriter, r *http.Request) {
		var proj *string
		if p := r.URL.Query().Get("project"); p != "" && p != "*" {
			proj = &p
		}
		b, err := st.GetBoard(proj)
		if err == nil {
			_ = st.AttachRecentAgentNotes(b, 3)
		}
		writeJSON(w, b, err)
	})

	mux.HandleFunc("POST /api/tasks", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Title, Description, Status, Project, Priority, DueDate string
			Tags                                                   []string
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		tk, err := st.CreateTask(store.CreateTaskParams{
			Title: body.Title, Description: body.Description, Status: body.Status,
			Project: ptrIfSet(body.Project), Priority: ptrIfSet(body.Priority),
			DueDate: ptrIfSet(body.DueDate), Tags: body.Tags,
		})
		writeJSON(w, tk, err)
	})

	mux.HandleFunc("GET /api/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		tk, err := st.GetTask(id)
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, tk, err)
	})

	mux.HandleFunc("POST /api/tasks/{id}/move", func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var body struct{ Status string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		tk, err := st.MoveTask(id, body.Status)
		writeJSON(w, tk, err)
	})

	mux.HandleFunc("POST /api/tasks/{id}/archive", func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		tk, err := st.SetArchived(id, true)
		writeJSON(w, tk, err)
	})

	mux.HandleFunc("POST /api/tasks/{id}/update", func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var body struct {
			Title       *string   `json:"title"`
			Description *string   `json:"description"`
			Priority    *string   `json:"priority"`
			DueDate     *string   `json:"due_date"`
			Project     *string   `json:"project"`
			Tags        *[]string `json:"tags"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		tk, err := st.UpdateTask(id, store.UpdateTaskParams{
			Title: body.Title, Description: body.Description,
			Priority: body.Priority, DueDate: body.DueDate,
			Project: body.Project, Tags: body.Tags,
		})
		writeJSON(w, tk, err)
	})

	mux.HandleFunc("POST /api/tasks/{id}/note", func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var body struct {
			Body string `json:"body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		n, err := st.AddNote(id, body.Body, "human")
		writeJSON(w, n, err)
	})

	mux.HandleFunc("POST /api/tasks/{id}/handoff", func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var body struct {
			To, Reason string
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		tk, err := st.Handoff(id, body.To, body.Reason)
		writeJSON(w, tk, err)
	})

	mux.HandleFunc("POST /api/tasks/{id}/clear_handoff", func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		tk, err := st.ClearHandoff(id)
		writeJSON(w, tk, err)
	})

	mux.HandleFunc("POST /api/tasks/{id}/run", func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		handleRun(w, r, st, resolveRunner, id, &mu, procs)
	})

	mux.HandleFunc("POST /api/tasks/{id}/run/cancel", func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		handleRunCancel(w, r, st, id, &mu, procs)
	})

	mux.HandleFunc("GET /api/resume", func(w http.ResponseWriter, r *http.Request) {
		var proj *string
		if p := r.URL.Query().Get("project"); p != "" && p != "*" {
			proj = &p
		}
		res, err := st.Resume(proj)
		writeJSON(w, res, err)
	})

	mux.HandleFunc("GET /api/projects/paths", func(w http.ResponseWriter, r *http.Request) {
		list, err := st.ListProjectPaths()
		writeJSON(w, list, err)
	})

	mux.HandleFunc("PUT /api/projects/{name}/path", func(w http.ResponseWriter, r *http.Request) {
		name := projectPathName(r)
		var body struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		abs := store.ExpandUserPath(body.Path)
		fi, err := os.Stat(abs)
		if err != nil || !fi.IsDir() {
			http.Error(w, "path must be an existing directory", http.StatusBadRequest)
			return
		}
		pp, err := st.SetProjectPath(name, abs)
		writeJSON(w, pp, err)
	})

	mux.HandleFunc("DELETE /api/projects/{name}/path", func(w http.ResponseWriter, r *http.Request) {
		name := projectPathName(r)
		err := st.DeleteProjectPath(name)
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]any{"ok": true}, err)
	})

	mux.HandleFunc("GET /api/questions", func(w http.ResponseWriter, r *http.Request) {
		var taskID *int64
		if s := r.URL.Query().Get("task_id"); s != "" {
			id, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				http.Error(w, "bad task_id", http.StatusBadRequest)
				return
			}
			taskID = &id
		}
		status := r.URL.Query().Get("status")
		list, err := st.ListQuestions(taskID, status)
		writeJSON(w, list, err)
	})

	mux.HandleFunc("POST /api/questions/{id}/answer", func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var body struct {
			Answer string `json:"answer"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		q, err := st.AnswerQuestion(id, body.Answer)
		if errors.Is(err, store.ErrQuestionClosed) || errors.Is(err, store.ErrNotFound) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, q, err)
	})

	mux.HandleFunc("GET /api/runs", func(w http.ResponseWriter, r *http.Request) {
		var taskID *int64
		if s := r.URL.Query().Get("task_id"); s != "" {
			id, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				http.Error(w, "bad task_id", http.StatusBadRequest)
				return
			}
			taskID = &id
		}
		status := r.URL.Query().Get("status")
		list, err := st.ListRuns(taskID, status)
		writeJSON(w, list, err)
	})

	mux.HandleFunc("GET /api/runs/{id}/files", func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		files, err := st.ListRunFiles(id)
		writeJSON(w, files, err)
	})

	mux.HandleFunc("POST /api/runs/{id}/files", func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var body struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		if err := st.AddRunFile(id, body.Path); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]any{"ok": true}, nil)
	})

	mux.HandleFunc("GET /api/events", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		ch, unsub, ok := hub.subscribe()
		if !ok {
			http.Error(w, "too many event streams", http.StatusServiceUnavailable)
			return
		}
		defer unsub()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		writeEvent := func(e store.Event) {
			b, _ := json.Marshal(e)
			fmt.Fprintf(w, "data: %s\n\n", b)
		}

		// No explicit `since` means a fresh page load: seed with a short tail of
		// recent activity instead of replaying the whole events table.
		since := int64(-1)
		if s := r.URL.Query().Get("since"); s != "" {
			if v, err := strconv.ParseInt(s, 10, 64); err == nil {
				since = v
			}
		}
		if since < 0 {
			if evs, err := st.RecentEvents(30); err == nil {
				for _, e := range evs {
					writeEvent(e)
					since = e.ID
				}
			}
			if since < 0 {
				since = 0
			}
		} else if evs, err := st.Events(since, 200); err == nil {
			for _, e := range evs {
				writeEvent(e)
				since = e.ID
			}
		}
		// Close the subscribe race: events may have landed between catch-up
		// and joining the hub broadcast.
		if evs, err := st.Events(since, 200); err == nil {
			for _, e := range evs {
				writeEvent(e)
				since = e.ID
			}
		}
		fmt.Fprint(w, "event: synced\ndata: {}\n\n")
		flusher.Flush()

		for {
			select {
			case <-r.Context().Done():
				return
			case batch, open := <-ch:
				if !open {
					return
				}
				for _, e := range batch {
					if e.ID <= since {
						continue
					}
					writeEvent(e)
					since = e.ID
				}
				flusher.Flush()
			}
		}
	})

	dist, _ := fs.Sub(uiFS, "ui/dist")
	indexHTML := injectCSRF(mustReadFile(dist, "index.html"), csrf)
	mux.Handle("/", http.FileServer(http.FS(dist)))

	allowedHosts := allowedHostSet(cfg.AllowedHosts)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Host is checked on every request, including GETs: serving the CSRF
		// token in index.html to a rebound attacker origin is the leak.
		if err := checkHost(r, allowedHosts); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		if isMutating(r.Method) && strings.HasPrefix(r.URL.Path, "/api/") {
			if err := checkMutation(r, csrf); err != nil {
				http.Error(w, err.Error(), http.StatusForbidden)
				return
			}
		}
		if r.Method == http.MethodGet && (r.URL.Path == "/" || r.URL.Path == "/index.html") {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write(indexHTML)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func mustReadFile(fsys fs.FS, name string) []byte {
	b, err := fs.ReadFile(fsys, name)
	if err != nil {
		panic("web: read " + name + ": " + err.Error())
	}
	return b
}

func injectCSRF(html []byte, token string) []byte {
	snippet := []byte(`<script>window.__BOARD_CSRF__=` + strconv.Quote(token) + `;</script>`)
	if i := bytes.Index(html, []byte("</head>")); i >= 0 {
		out := make([]byte, 0, len(html)+len(snippet))
		out = append(out, html[:i]...)
		out = append(out, snippet...)
		out = append(out, html[i:]...)
		return out
	}
	return append(snippet, html...)
}

func handleRun(w http.ResponseWriter, r *http.Request, st *store.Store, resolveRunner func(string) (agent.Runner, error), taskID int64, mu *sync.Mutex, procs map[int64]procEntry) {
	var body struct {
		Agent string `json:"agent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if body.Agent == "" {
		body.Agent = agent.AgentCursor
	}
	runner, err := resolveRunner(body.Agent)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	tk, err := st.GetTask(taskID)
	if err != nil {
		writeJSON(w, nil, err)
		return
	}
	projKey := store.GlobalProjectKey
	if tk.Project != nil && *tk.Project != "" {
		projKey = *tk.Project
	}
	cwd, err := st.ResolveProjectPath(projKey)
	if errors.Is(err, store.ErrNotFound) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]any{"need_path": true, "project": projKey})
		return
	}
	if err != nil {
		writeJSON(w, nil, err)
		return
	}
	var runID atomic.Int64
	run, err := st.CreateRun(taskID, body.Agent, 0)
	if errors.Is(err, store.ErrRunActive) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	if err != nil {
		writeJSON(w, nil, err)
		return
	}
	runID.Store(run.ID)
	env := append(os.Environ(),
		fmt.Sprintf("BOARD_TASK_ID=%d", taskID),
		fmt.Sprintf("BOARD_RUN_ID=%d", run.ID),
	)
	started, err := runner.Start(agent.StartOpts{
		Cwd:    cwd,
		Prompt: agent.BuildPrompt(tk),
		Env:    env,
		OnProgress: func(output string) {
			id := runID.Load()
			if id == 0 || output == "" {
				return
			}
			// Store only overwrites when message_source is empty/stdout —
			// never clobbers an add_note-sourced message (atomic in SQL).
			_ = st.ReportRunProgress(id, taskID, output, store.MessageSourceStdout)
		},
	})
	if err != nil {
		code := -1
		_, _ = st.FinishRun(run.ID, "failed", &code, err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := st.SetRunPID(run.ID, started.PID); err != nil {
		_ = started.Kill()
		code := -1
		_, _ = st.FinishRun(run.ID, "failed", &code, err.Error())
		writeJSON(w, nil, err)
		return
	}
	mu.Lock()
	procs[run.ID] = procEntry{kill: started.Kill}
	mu.Unlock()
	go func(rid int64) {
		code, output, waitErr := started.Wait()
		mu.Lock()
		delete(procs, rid)
		mu.Unlock()
		status := "exited"
		if waitErr != nil || code != 0 {
			status = "failed"
		}
		ec := code
		_, _ = st.FinishRun(rid, status, &ec, output)
	}(run.ID)
	writeJSON(w, map[string]any{
		"run_id":  run.ID,
		"task_id": taskID,
		"agent":   body.Agent,
		"status":  "running",
	}, nil)
}

func handleRunCancel(w http.ResponseWriter, r *http.Request, st *store.Store, taskID int64, mu *sync.Mutex, procs map[int64]procEntry) {
	run, err := st.ActiveRunForTask(taskID)
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "no active run", http.StatusBadRequest)
		return
	}
	if err != nil {
		writeJSON(w, nil, err)
		return
	}
	mu.Lock()
	entry, ok := procs[run.ID]
	mu.Unlock()
	if ok && entry.kill != nil {
		_ = entry.kill()
	} else if run.PID != nil {
		if p, err := os.FindProcess(*run.PID); err == nil {
			_ = p.Kill()
		}
	}
	finished, err := st.FinishRun(run.ID, "killed", nil, "cancelled from board UI")
	mu.Lock()
	delete(procs, run.ID)
	mu.Unlock()
	writeJSON(w, finished, err)
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

func projectPathName(r *http.Request) string {
	name := r.PathValue("name")
	if name == "" || name == "*" {
		return store.GlobalProjectKey
	}
	return name
}

func ptrIfSet(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func writeJSON(w http.ResponseWriter, v any, err error) {
	if err != nil {
		http.Error(w, err.Error(), statusFor(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func statusFor(err error) int {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return http.StatusNotFound
	case isValidation(err):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

func isValidation(err error) bool {
	var ve *store.ValidationError
	return errors.As(err, &ve)
}

package web

import (
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
	"time"

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

	var (
		mu    sync.Mutex
		procs = map[int64]procEntry{}
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

	mux.HandleFunc("/api/board", func(w http.ResponseWriter, r *http.Request) {
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

	mux.HandleFunc("/api/tasks", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
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

	mux.HandleFunc("/api/tasks/", func(w http.ResponseWriter, r *http.Request) {
		trimmed := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/tasks/"), "/")
		parts := strings.Split(trimmed, "/")
		if len(parts) < 1 || len(parts) > 3 || parts[0] == "" {
			http.NotFound(w, r)
			return
		}
		id, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		if len(parts) == 1 {
			if r.Method != http.MethodGet {
				http.Error(w, "GET only", http.StatusMethodNotAllowed)
				return
			}
			tk, err := st.GetTask(id)
			if errors.Is(err, store.ErrNotFound) {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			writeJSON(w, tk, err)
			return
		}
		action := parts[1]
		if len(parts) == 3 {
			action = parts[1] + "/" + parts[2]
		}
		switch action {
		case "move":
			var body struct{ Status string }
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid JSON body", http.StatusBadRequest)
				return
			}
			tk, err := st.MoveTask(id, body.Status)
			writeJSON(w, tk, err)
		case "archive":
			tk, err := st.SetArchived(id, true)
			writeJSON(w, tk, err)
		case "update":
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
		case "note":
			var body struct {
				Body string `json:"body"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid JSON body", http.StatusBadRequest)
				return
			}
			n, err := st.AddNote(id, body.Body, "human")
			writeJSON(w, n, err)
		case "handoff":
			var body struct {
				To, Reason string
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid JSON body", http.StatusBadRequest)
				return
			}
			tk, err := st.Handoff(id, body.To, body.Reason)
			writeJSON(w, tk, err)
		case "clear_handoff":
			tk, err := st.ClearHandoff(id)
			writeJSON(w, tk, err)
		case "run":
			handleRun(w, r, st, resolveRunner, id, &mu, procs)
		case "run/cancel":
			handleRunCancel(w, r, st, id, &mu, procs)
		default:
			http.NotFound(w, r)
		}
	})

	mux.HandleFunc("/api/resume", func(w http.ResponseWriter, r *http.Request) {
		var proj *string
		if p := r.URL.Query().Get("project"); p != "" && p != "*" {
			proj = &p
		}
		res, err := st.Resume(proj)
		writeJSON(w, res, err)
	})

	mux.HandleFunc("/api/projects/paths", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "GET only", http.StatusMethodNotAllowed)
			return
		}
		list, err := st.ListProjectPaths()
		writeJSON(w, list, err)
	})

	mux.HandleFunc("/api/projects/", func(w http.ResponseWriter, r *http.Request) {
		trimmed := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/projects/"), "/")
		parts := strings.Split(trimmed, "/")
		if len(parts) != 2 || parts[1] != "path" {
			http.NotFound(w, r)
			return
		}
		name := parts[0]
		if name == "" || name == "*" {
			name = store.GlobalProjectKey
		}
		switch r.Method {
		case http.MethodPut:
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
		case http.MethodDelete:
			err := st.DeleteProjectPath(name)
			if errors.Is(err, store.ErrNotFound) {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			writeJSON(w, map[string]any{"ok": true}, err)
		default:
			http.Error(w, "PUT or DELETE only", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/questions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "GET only", http.StatusMethodNotAllowed)
			return
		}
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

	mux.HandleFunc("/api/questions/", func(w http.ResponseWriter, r *http.Request) {
		trimmed := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/questions/"), "/")
		parts := strings.Split(trimmed, "/")
		if len(parts) != 2 || parts[1] != "answer" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		id, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
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

	mux.HandleFunc("/api/runs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "GET only", http.StatusMethodNotAllowed)
			return
		}
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

	mux.HandleFunc("/api/runs/", func(w http.ResponseWriter, r *http.Request) {
		trimmed := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/runs/"), "/")
		parts := strings.Split(trimmed, "/")
		if len(parts) != 2 || parts[1] != "files" {
			http.NotFound(w, r)
			return
		}
		id, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		switch r.Method {
		case http.MethodGet:
			files, err := st.ListRunFiles(id)
			writeJSON(w, files, err)
		case http.MethodPost:
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
		default:
			http.Error(w, "GET or POST only", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/events", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

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
					b, _ := json.Marshal(e)
					fmt.Fprintf(w, "data: %s\n\n", b)
					since = e.ID
				}
			}
			if since < 0 {
				since = 0
			}
		}
		send := func() {
			evs, err := st.Events(since, 200)
			if err != nil {
				return
			}
			for _, e := range evs {
				b, _ := json.Marshal(e)
				fmt.Fprintf(w, "data: %s\n\n", b)
				since = e.ID
			}
			flusher.Flush()
		}
		send()
		fmt.Fprint(w, "event: synced\ndata: {}\n\n")
		flusher.Flush()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				send()
			}
		}
	})

	dist, _ := fs.Sub(uiFS, "ui/dist")
	mux.Handle("/", http.FileServer(http.FS(dist)))
	return mux
}

func handleRun(w http.ResponseWriter, r *http.Request, st *store.Store, resolveRunner func(string) (agent.Runner, error), taskID int64, mu *sync.Mutex, procs map[int64]procEntry) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
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
	var (
		runID      atomic.Int64
		lastStdout atomic.Value // string
	)
	lastStdout.Store("")
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
			// Don't clobber a human-readable note the agent posted via add_note.
			if run, err := st.GetRun(id); err == nil {
				prev, _ := lastStdout.Load().(string)
				if run.Message != "" && run.Message != prev {
					return
				}
			}
			lastStdout.Store(output)
			_ = st.ReportRunProgress(id, taskID, output)
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
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
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

func ptrIfSet(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func writeJSON(w http.ResponseWriter, v any, err error) {
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

package web

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/samuelloranger/board/internal/store"
)

//go:embed all:ui/dist
var uiFS embed.FS

func Handler(st *store.Store) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/board", func(w http.ResponseWriter, r *http.Request) {
		var proj *string
		if p := r.URL.Query().Get("project"); p != "" && p != "*" {
			proj = &p
		}
		b, err := st.GetBoard(proj)
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
		// /api/tasks/{id}/{action}; tolerate a trailing slash.
		trimmed := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/tasks/"), "/")
		parts := strings.Split(trimmed, "/")
		if len(parts) != 2 {
			http.NotFound(w, r)
			return
		}
		id, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		switch parts[1] {
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
			// Pointer fields: absent (nil) means "leave unchanged"; a present
			// empty string on priority/due_date clears the column.
			var body struct {
				Title       *string   `json:"title"`
				Description *string   `json:"description"`
				Priority    *string   `json:"priority"`
				DueDate     *string   `json:"due_date"`
				Tags        *[]string `json:"tags"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid JSON body", http.StatusBadRequest)
				return
			}
			tk, err := st.UpdateTask(id, store.UpdateTaskParams{
				Title: body.Title, Description: body.Description,
				Priority: body.Priority, DueDate: body.DueDate, Tags: body.Tags,
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
			n, err := st.AddNote(id, body.Body)
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

	mux.HandleFunc("/api/events", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		since := int64(0)
		if s := r.URL.Query().Get("since"); s != "" {
			if v, err := strconv.ParseInt(s, 10, 64); err == nil {
				since = v
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
		send() // flush existing (backlog) immediately
		// Mark the end of the replayed backlog so the client can distinguish
		// historical events (don't bump the unseen badge) from live ones.
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

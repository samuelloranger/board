package web

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"strconv"
	"strings"

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
		json.NewDecoder(r.Body).Decode(&body)
		tk, err := st.CreateTask(store.CreateTaskParams{
			Title: body.Title, Description: body.Description, Status: body.Status,
			Project: ptrIfSet(body.Project), Priority: ptrIfSet(body.Priority),
			DueDate: ptrIfSet(body.DueDate), Tags: body.Tags,
		})
		writeJSON(w, tk, err)
	})

	mux.HandleFunc("/api/tasks/", func(w http.ResponseWriter, r *http.Request) {
		// /api/tasks/{id}/{action}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/tasks/"), "/")
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
			json.NewDecoder(r.Body).Decode(&body)
			tk, err := st.MoveTask(id, body.Status)
			writeJSON(w, tk, err)
		case "archive":
			tk, err := st.SetArchived(id, true)
			writeJSON(w, tk, err)
		default:
			http.NotFound(w, r)
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

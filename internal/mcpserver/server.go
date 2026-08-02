package mcpserver

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/samuelloranger/board/internal/store"
)

// resolveProject returns the explicit arg if non-empty, else the server default.
func resolveProject(arg string, def *string) *string {
	if arg != "" {
		return &arg
	}
	return def
}

func ptrIfSet(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ack is the minimal write confirmation: enough to identify the task, nothing
// the caller already knows. Write tools echoing back descriptions and note
// bodies was the single largest source of wasted tokens in agent transcripts.
func ack(tk *store.Task) map[string]any {
	return map[string]any{"id": tk.ID, "title": tk.Title, "status": tk.Status}
}

// slim projects a task down to what a caller needs to decide what to do next.
// Descriptions, notes, timestamps and tags are fetched on demand via get_task.
func slim(tk *store.Task) map[string]any {
	m := map[string]any{"id": tk.ID, "title": tk.Title, "status": tk.Status}
	if tk.Priority != nil {
		m["priority"] = *tk.Priority
	}
	if tk.DueDate != nil {
		m["due_date"] = *tk.DueDate
	}
	if tk.HandoffTo != nil {
		m["handoff_to"] = *tk.HandoffTo
	}
	if tk.Project != nil {
		m["project"] = *tk.Project
	}
	return m
}

// doneLimit caps how many completed tasks get_board returns by default;
// listLimit caps list_tasks. Both are overridable by the caller.
const (
	doneLimit = 10
	listLimit = 50
)

// recentDone returns the newest tasks by id, newest first, plus the full count.
func recentDone(ts []*store.Task, limit int) ([]*store.Task, int) {
	sorted := make([]*store.Task, len(ts))
	copy(sorted, ts)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID > sorted[j].ID })
	if len(sorted) > limit {
		sorted = sorted[:limit]
	}
	return sorted, len(ts)
}

func slimAll(ts []*store.Task) []map[string]any {
	out := make([]map[string]any, 0, len(ts))
	for _, tk := range ts {
		out = append(out, slim(tk))
	}
	return out
}

func BuildServer(st *store.Store, def *string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "board", Version: "v1"}, nil)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "create_task",
		Description: "Create a kanban task. status defaults to 'todo'. project scopes the task (omit to use the current project). priority is low|medium|high. tags is a list. due_date is ISO8601.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, a struct {
		Title       string   `json:"title"`
		Description string   `json:"description,omitempty"`
		Status      string   `json:"status,omitempty"`
		Project     string   `json:"project,omitempty"`
		Priority    string   `json:"priority,omitempty"`
		DueDate     string   `json:"due_date,omitempty"`
		Tags        []string `json:"tags,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		tk, err := st.CreateTask(store.CreateTaskParams{
			Title: a.Title, Description: a.Description, Status: a.Status,
			Project: resolveProject(a.Project, def), Priority: ptrIfSet(a.Priority),
			DueDate: ptrIfSet(a.DueDate), Tags: a.Tags,
		})
		if err != nil {
			return nil, nil, err
		}
		return nil, ack(tk), nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_tasks",
		Description: "List tasks (id, title, status, priority, due_date, handoff_to). Filters: project (omit for current project; pass '*' for all projects), status (todo|in_progress|done), priority, tag, include_archived. Returns at most 50 tasks unless limit is set (0 for all). Set verbose for descriptions, notes and timestamps.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, a struct {
		Project         string `json:"project,omitempty"`
		Status          string `json:"status,omitempty"`
		Priority        string `json:"priority,omitempty"`
		Tag             string `json:"tag,omitempty"`
		IncludeArchived bool   `json:"include_archived,omitempty"`
		Verbose         bool   `json:"verbose,omitempty"`
		Limit           *int   `json:"limit,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		f := store.ListFilter{
			Status: ptrIfSet(a.Status), Priority: ptrIfSet(a.Priority),
			Tag: ptrIfSet(a.Tag), IncludeArchived: a.IncludeArchived,
		}
		if a.Project != "*" {
			f.Project = resolveProject(a.Project, def)
		}
		out, err := st.ListTasks(f)
		if err != nil {
			return nil, nil, err
		}
		limit := listLimit
		if a.Limit != nil {
			limit = *a.Limit
		}
		total := len(out)
		if limit > 0 && total > limit {
			out = out[:limit]
		}
		res := map[string]any{"tasks": slimAll(out)}
		if a.Verbose {
			res["tasks"] = out
		}
		if total > len(out) {
			res["total"] = total
			res["truncated"] = "showing " + strconv.Itoa(len(out)) + " of " + strconv.Itoa(total) + "; raise limit or filter further"
		}
		return nil, res, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "get_task", Description: "Get one task by id, including tags and notes.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, a struct {
		ID int64 `json:"id"`
	}) (*mcp.CallToolResult, any, error) {
		tk, err := st.GetTask(a.ID)
		if err != nil {
			return nil, nil, err
		}
		return nil, tk, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "update_task",
		Description: "Patch a task. Only provided fields change. tags (if provided) replaces the whole tag set. Set priority or due_date to '' to clear them.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, a struct {
		ID          int64    `json:"id"`
		Title       *string  `json:"title,omitempty"`
		Description *string  `json:"description,omitempty"`
		Priority    *string  `json:"priority,omitempty"`
		DueDate     *string  `json:"due_date,omitempty"`
		Tags        []string `json:"tags,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		p := store.UpdateTaskParams{Title: a.Title, Description: a.Description, Priority: a.Priority, DueDate: a.DueDate}
		if a.Tags != nil {
			p.Tags = &a.Tags
		}
		tk, err := st.UpdateTask(a.ID, p)
		if err != nil {
			return nil, nil, err
		}
		return nil, ack(tk), nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "move_task", Description: "Move a task to a new status: todo, in_progress, or done.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, a struct {
		ID     int64  `json:"id"`
		Status string `json:"status"`
	}) (*mcp.CallToolResult, any, error) {
		// Read the prior status so the confirmation can report the transition
		// without shipping the whole task back.
		var from string
		if prev, err := st.GetTask(a.ID); err == nil {
			from = prev.Status
		}
		tk, err := st.MoveTask(a.ID, a.Status)
		if err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"id": tk.ID, "title": tk.Title, "from": from, "to": tk.Status}, nil
	})

	archive := func(name, desc string, val bool) {
		mcp.AddTool(s, &mcp.Tool{Name: name, Description: desc},
			func(ctx context.Context, req *mcp.CallToolRequest, a struct {
				ID int64 `json:"id"`
			}) (*mcp.CallToolResult, any, error) {
				tk, err := st.SetArchived(a.ID, val)
				if err != nil {
					return nil, nil, err
				}
				return nil, ack(tk), nil
			})
	}
	archive("archive_task", "Archive a task (hides it from default views; keeps its status).", true)
	archive("unarchive_task", "Restore an archived task to normal views.", false)

	mcp.AddTool(s, &mcp.Tool{
		Name: "delete_task", Description: "Permanently delete a task and its tags/notes.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, a struct {
		ID int64 `json:"id"`
	}) (*mcp.CallToolResult, any, error) {
		if err := st.DeleteTask(a.ID); err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"deleted": a.ID}, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "add_note", Description: "Append a note (progress/finding) to a task's activity log.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, a struct {
		ID   int64  `json:"id"`
		Body string `json:"body"`
	}) (*mcp.CallToolResult, any, error) {
		n, err := st.AddNote(a.ID, a.Body)
		if err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"note_id": n.ID, "task_id": n.TaskID}, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_board",
		Description: "Return tasks grouped into todo/in_progress/done columns for a project (omit for current project; pass '*' for all projects). Each task is id, title, status, priority, due_date, handoff_to — call get_task for a description or notes. Set verbose to return full tasks instead.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, a struct {
		Project string `json:"project,omitempty"`
		Verbose bool   `json:"verbose,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		var proj *string
		if a.Project != "*" {
			proj = resolveProject(a.Project, def)
		}
		b, err := st.GetBoard(proj)
		if err != nil {
			return nil, nil, err
		}
		if a.Verbose {
			return nil, b, nil
		}
		// The done column is append-only and grows without bound, so it
		// dominated the payload on long-lived boards. Show the newest few and
		// state the real count rather than silently truncating.
		done, total := recentDone(b.Done, doneLimit)
		out := map[string]any{
			"project":     b.Project,
			"todo":        slimAll(b.Todo),
			"in_progress": slimAll(b.InProgress),
			"done":        slimAll(done),
		}
		if total > len(done) {
			out["done_total"] = total
			out["done_note"] = "showing newest " + strconv.Itoa(len(done)) + " of " + strconv.Itoa(total) + "; use list_tasks with status=done for the rest"
		}
		return nil, out, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "handoff",
		Description: "Hand a task to another agent or a human, with context. 'to' is a free label (e.g. 'codex','cursor','human'). The receiver picks it up via move_task, which clears the handoff.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, a struct {
		ID     int64  `json:"id"`
		To     string `json:"to"`
		Reason string `json:"reason,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		tk, err := st.Handoff(a.ID, a.To, a.Reason)
		if err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"id": tk.ID, "title": tk.Title, "handoff_to": a.To}, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "resume",
		Description: "Restore working context in one call: returns in_progress tasks (with notes) and any tasks handed off to you, for the current project (omit project) or a named one (pass '*' for all). Call this at the start of a session.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, a struct {
		Project string `json:"project,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		var proj *string
		if a.Project != "*" {
			proj = resolveProject(a.Project, def)
		}
		r, err := st.Resume(proj)
		if err != nil {
			return nil, nil, err
		}
		return nil, r, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "ask_user",
		Description: "Ask the human a question via the board web UI and block until they answer. " +
			"Use when this session was launched from the board UI (or you need human input mid-task). " +
			"Pass the board task_id. Do not use terminal prompts.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, a struct {
		TaskID   int64  `json:"task_id"`
		Question string `json:"question"`
	}) (*mcp.CallToolResult, any, error) {
		out, err := askUser(ctx, st, a.TaskID, a.Question)
		if err != nil {
			return nil, nil, err
		}
		return nil, out, nil
	})

	return s
}

func askUser(ctx context.Context, st *store.Store, taskID int64, question string) (map[string]any, error) {
	if taskID == 0 || question == "" {
		return nil, fmt.Errorf("task_id and question are required")
	}
	q, err := st.CreateQuestion(taskID, question)
	if err != nil {
		return nil, err
	}
	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	ans, err := st.WaitForAnswer(waitCtx, q.ID, 200*time.Millisecond)
	if err != nil {
		return nil, err
	}
	return map[string]any{"answer": ans}, nil
}

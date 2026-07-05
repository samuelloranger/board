package mcpserver

import (
	"context"

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
		return nil, tk, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_tasks",
		Description: "List tasks. Filters: project (omit for current project; pass '*' for all projects), status (todo|in_progress|done), priority, tag, include_archived.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, a struct {
		Project         string `json:"project,omitempty"`
		Status          string `json:"status,omitempty"`
		Priority        string `json:"priority,omitempty"`
		Tag             string `json:"tag,omitempty"`
		IncludeArchived bool   `json:"include_archived,omitempty"`
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
		return nil, map[string]any{"tasks": out}, nil
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
		return nil, tk, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "move_task", Description: "Move a task to a new status: todo, in_progress, or done.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, a struct {
		ID     int64  `json:"id"`
		Status string `json:"status"`
	}) (*mcp.CallToolResult, any, error) {
		tk, err := st.MoveTask(a.ID, a.Status)
		if err != nil {
			return nil, nil, err
		}
		return nil, tk, nil
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
				return nil, tk, nil
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
		return nil, n, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_board",
		Description: "Return tasks grouped into todo/in_progress/done columns for a project (omit for current project; pass '*' for all projects).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, a struct {
		Project string `json:"project,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		var proj *string
		if a.Project != "*" {
			proj = resolveProject(a.Project, def)
		}
		b, err := st.GetBoard(proj)
		if err != nil {
			return nil, nil, err
		}
		return nil, b, nil
	})

	return s
}

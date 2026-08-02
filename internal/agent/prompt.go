package agent

import (
	"fmt"
	"strings"

	"github.com/samuelloranger/board/internal/store"
)

// BuildPrompt returns the full cursor-agent -p prompt with board provenance.
func BuildPrompt(tk *store.Task) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You were launched from the board web UI to work on task #%d.\n", tk.ID)
	b.WriteString("Use the board MCP tools to update this task as you work.\n")
	b.WriteString("If you need human input, call ask_user with this task_id — do not use terminal prompts.\n\n")
	fmt.Fprintf(&b, "Task #%d [%s]: %s\n", tk.ID, tk.Status, tk.Title)
	if tk.Description != "" {
		fmt.Fprintf(&b, "\nDescription:\n%s\n", tk.Description)
	}
	if tk.Project != nil {
		fmt.Fprintf(&b, "\nProject: %s\n", *tk.Project)
	}
	if len(tk.Notes) > 0 {
		b.WriteString("\nRecent notes:\n")
		start := 0
		if len(tk.Notes) > 5 {
			start = len(tk.Notes) - 5
		}
		for _, n := range tk.Notes[start:] {
			fmt.Fprintf(&b, "- %s\n", n.Body)
		}
	}
	b.WriteString("\nDo the work described by this task. When finished, add a note summarizing what changed and move the task to done if appropriate.\n")
	return b.String()
}

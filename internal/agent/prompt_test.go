package agent

import (
	"strings"
	"testing"

	"github.com/samuelloranger/board/internal/store"
)

func TestBuildPromptProvenance(t *testing.T) {
	tk := &store.Task{ID: 42, Title: "Fix bug", Description: "desc", Status: "todo"}
	p := BuildPrompt(tk)
	for _, want := range []string{"board web UI", "#42", "ask_user", "Fix bug", "desc", "LIVE PROGRESS", "add_note", "set_run_wait", "repo-relative"} {
		if !strings.Contains(p, want) {
			t.Fatalf("missing %q in %s", want, p)
		}
	}
}

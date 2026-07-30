package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const cursorBoardAllow = "Mcp(board:*)"

// Cursor skill — when/how to use the board (loaded from ~/.cursor/skills/).
const cursorSkillMarkdown = "---\n" +
	"name: board\n" +
	"description: >-\n" +
	"  Use to track multi-step work on a persistent kanban board. Create a task when\n" +
	"  starting non-trivial work, move it across todo/in_progress/done as you progress,\n" +
	"  and record findings as notes. Call resume at session start; use handoff to park\n" +
	"  work for another agent or a human.\n" +
	"---\n" +
	"\n" +
	"# Using the board\n" +
	"\n" +
	"You have a kanban board via the `board` MCP server. Use it to persist work across sessions and coordinate with other agents.\n" +
	"\n" +
	"## When to use\n" +
	"- Starting multi-step work → `create_task` (title + short description). It auto-scopes to the current project.\n" +
	"- Beginning a task → `move_task` to `in_progress`.\n" +
	"- Finishing → `add_note` (what changed + how verified), then `move_task` to `done`.\n" +
	"- Learning something worth remembering → `add_note` on the task.\n" +
	"- Reviewing state → `get_board` (current project) or `list_tasks`.\n" +
	"- **Starting a session** → call `resume` first to restore in-progress work and pick up anything handed to you.\n" +
	"- **Blocked / needs another agent or a human** → `handoff(id, to, reason)` with `to` = the receiver (`human`, `codex`, `claude`, …) and a clear reason.\n" +
	"\n" +
	"## Rules\n" +
	"- One task per meaningful unit of work. Don't create tasks for trivial one-liners.\n" +
	"- Keep exactly one task `in_progress` at a time when possible.\n" +
	"- Archive (not delete) completed work you want to keep a record of: `archive_task`.\n"

// Cursor always-on rule — injected every session (editor + Agent CLI).
const cursorRuleMarkdown = "---\n" +
	"description: Keep the board MCP updated as the source of truth for tasks\n" +
	"alwaysApply: true\n" +
	"---\n" +
	"\n" +
	"# Board / Kanban (ALWAYS ON)\n" +
	"\n" +
	"The `board` MCP server is the source of truth for task tracking. In every session, keep the board continuously updated:\n" +
	"\n" +
	"- Starting a task → `move_task` to `in_progress` before touching code.\n" +
	"- Finishing a task → `add_note` (what changed + how verified), then `move_task` to `done`.\n" +
	"- New work surfaced mid-session → `create_task` immediately.\n" +
	"- Progress/findings mid-task → `add_note` as you go.\n" +
	"- Before deciding what's next → `get_board` / `list_tasks`.\n" +
	"- Session start → `resume` to restore in-progress work and handoffs.\n" +
	"\n" +
	"Never let the board drift from reality.\n"

// InstallCursorIntegration installs the board skill, always-on rule, and Agent CLI
// MCP allowlist under ~/.cursor. All three are idempotent.
func InstallCursorIntegration(home string) error {
	dir := filepath.Join(home, ".cursor")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := InstallCursorSkill(filepath.Join(dir, "skills", "board", "SKILL.md")); err != nil {
		return err
	}
	if err := InstallCursorRule(filepath.Join(dir, "rules", "board.mdc")); err != nil {
		return err
	}
	return InstallCursorCLIAllowlist(filepath.Join(dir, "cli-config.json"))
}

// InstallCursorSkill writes (or overwrites) the board SKILL.md.
func InstallCursorSkill(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeFileAtomic(path, []byte(cursorSkillMarkdown))
}

// InstallCursorRule writes (or overwrites) the always-on board.mdc rule.
func InstallCursorRule(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeFileAtomic(path, []byte(cursorRuleMarkdown))
}

// InstallCursorCLIAllowlist upserts Mcp(board:*) into permissions.allow in
// ~/.cursor/cli-config.json so Agent CLI can call board tools under allowlist mode.
// Preserves all other fields. Creates a minimal config when the file is missing.
func InstallCursorCLIAllowlist(path string) error {
	m := map[string]any{}
	existed := false
	if raw, err := os.ReadFile(path); err == nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, &m); err != nil {
			return fmt.Errorf("%s: existing file is not valid JSON: %w", path, err)
		}
		existed = true
	} else {
		m["version"] = 1
	}

	perms, _ := m["permissions"].(map[string]any)
	if perms == nil {
		perms = map[string]any{}
	}
	allow, _ := perms["allow"].([]any)
	if hasString(allow, cursorBoardAllow) {
		if existed {
			return nil // already allowlisted; leave file untouched
		}
	} else {
		allow = append(allow, cursorBoardAllow)
	}
	perms["allow"] = allow
	if _, ok := perms["deny"]; !ok && !existed {
		perms["deny"] = []any{}
	}
	m["permissions"] = perms

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeFileAtomic(path, mustJSON(m))
}

func hasString(list []any, want string) bool {
	for _, v := range list {
		if s, ok := v.(string); ok && s == want {
			return true
		}
	}
	return false
}

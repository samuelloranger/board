package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Client struct {
	Name string
	Kind string // "json" or "toml"
	Path string
}

// WriteJSONServer upserts the board MCP server into a JSON config using the
// standard {"mcpServers":{...}} shape, preserving all other content.
func WriteJSONServer(path, binPath string) error {
	m := map[string]any{}
	if raw, err := os.ReadFile(path); err == nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, &m); err != nil {
			return fmt.Errorf("%s: existing file is not valid JSON: %w", path, err)
		}
	}
	servers, _ := m["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	servers["board"] = map[string]any{
		"command": binPath,
		"args":    []string{"mcp"},
	}
	m["mcpServers"] = servers
	return writeFileAtomic(path, mustJSON(m))
}

// WriteTOMLServer upserts a [mcp_servers.board] block into a Codex config.toml,
// preserving other lines. Line-based to avoid a TOML dependency.
func WriteTOMLServer(path, binPath string) error {
	var existing string
	if raw, err := os.ReadFile(path); err == nil {
		existing = string(raw)
	}
	block := fmt.Sprintf("[mcp_servers.board]\ncommand = %q\nargs = [\"mcp\"]\n", binPath)
	out := removeTOMLBlock(existing, "[mcp_servers.board]")
	if out != "" && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	if out != "" {
		out += "\n"
	}
	out += block
	return writeFileAtomic(path, []byte(out))
}

// removeTOMLBlock strips an existing table block (header + its body lines up to
// the next table header or EOF) so re-runs stay idempotent.
func removeTOMLBlock(content, header string) string {
	lines := strings.Split(content, "\n")
	var kept []string
	skipping := false
	for _, ln := range lines {
		trimmed := strings.TrimSpace(ln)
		if trimmed == header {
			skipping = true
			continue
		}
		if skipping {
			if strings.HasPrefix(trimmed, "[") { // next table starts
				skipping = false
			} else {
				continue
			}
		}
		kept = append(kept, ln)
	}
	return strings.TrimRight(strings.Join(kept, "\n"), "\n")
}

// --- Auto-update rules (Claude Code) -------------------------------------
//
// `board setup` also installs the "always keep the board updated" behavior into
// Claude Code so every session is reminded automatically:
//   1. a SessionStart hook in ~/.claude/settings.json (the automatic trigger)
//   2. a PostToolUse hook that records write paths on UI Runs (not every tool)
//   3. a marker-delimited rules block in ~/.claude/CLAUDE.md (the instruction)
// All are idempotent — re-running setup replaces the board entries in place.

const boardHookMarker = "BOARD (always on)"

const claudePostToolUseMarker = "board-post-tool-use.sh"

const claudePostToolUseScript = `#!/bin/sh
# Claude Code PostToolUse: best-effort record write paths on the active UI Run
# (BOARD_RUN_ID). Does not log every tool name into the activity feed.
input=$(cat)
tool=$(printf '%s' "$input" | sed -n 's/.*"tool_name"[ ]*:[ ]*"\([^"]*\)".*/\1/p' | head -1)
case "$tool" in
  Write|Edit|MultiEdit|NotebookEdit)
    path=$(printf '%s' "$input" | sed -n 's/.*"file_path"[ ]*:[ ]*"\([^"]*\)".*/\1/p' | head -1)
    [ -z "$path" ] && path=$(printf '%s' "$input" | sed -n 's/.*"path"[ ]*:[ ]*"\([^"]*\)".*/\1/p' | head -1)
    [ -n "$path" ] && board run file "$path" >/dev/null 2>&1
    ;;
esac
exit 0
`

const (
	boardMdStart = "<!-- BOARD_RULES_START -->"
	boardMdEnd   = "<!-- BOARD_RULES_END -->"
)

const boardRulesMarkdown = "## Board / Kanban (ALWAYS ON)\n\n" +
	"The `board` MCP server is the source of truth for task tracking. In every session, keep the board continuously updated:\n\n" +
	"- Starting a task → `move_task` to `in_progress` before touching code.\n" +
	"- Finishing a task → `add_note` (what changed + how verified), then `move_task` to `done`.\n" +
	"- New work surfaced mid-session → `create_task` immediately.\n" +
	"- Progress/findings mid-task → `add_note` as you go (repo-relative paths only — never absolute or home paths).\n" +
	"- Before deciding what's next → `get_board` / `list_tasks`.\n\n" +
	"If a board tool isn't loaded, load it via ToolSearch (`mcp__board__*`). Never let the board drift from reality.\n"

// boardHookCommand builds the SessionStart hook shell command. The JSON is
// marshaled (not hand-written) so quoting is always correct, then single-quoted
// for the shell.
func boardHookCommand() string {
	ctx := "BOARD (always on): the `board` MCP is the source of truth for tasks. " +
		"Keep it updated in real time — move_task to in_progress when starting a task, " +
		"add_note + move_task to done when finishing, create_task for new work surfaced mid-session. " +
		"Check get_board before deciding next steps. If board tools are not loaded, load them via ToolSearch (mcp__board__*)."
	payload := map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":     "SessionStart",
			"additionalContext": ctx,
		},
	}
	b, _ := json.Marshal(payload)
	return "echo " + shellSingleQuote(string(b))
}

func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// InstallClaudeRules installs the SessionStart hook, PostToolUse path hook, and
// CLAUDE.md rules block under the given home directory's ~/.claude.
func InstallClaudeRules(home string) error {
	dir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := InstallSettingsHook(filepath.Join(dir, "settings.json")); err != nil {
		return err
	}
	if err := InstallClaudePostToolUse(dir); err != nil {
		return err
	}
	return InstallClaudeMd(filepath.Join(dir, "CLAUDE.md"))
}

// InstallClaudePostToolUse writes hooks/board-post-tool-use.sh and upserts a
// PostToolUse entry in settings.json. Preserves other hooks; idempotent.
func InstallClaudePostToolUse(claudeDir string) error {
	scriptPath := filepath.Join(claudeDir, "hooks", claudePostToolUseMarker)
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		return err
	}
	tmp := scriptPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(claudePostToolUseScript), 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmp, scriptPath); err != nil {
		return err
	}
	_ = os.Chmod(scriptPath, 0o755)

	settingsPath := filepath.Join(claudeDir, "settings.json")
	m := map[string]any{}
	if raw, err := os.ReadFile(settingsPath); err == nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, &m); err != nil {
			return fmt.Errorf("%s: existing file is not valid JSON: %w", settingsPath, err)
		}
	}
	hooks, _ := m["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	var groups []any
	if existing, ok := hooks["PostToolUse"].([]any); ok {
		for _, g := range existing {
			if !groupHasMarker(g, claudePostToolUseMarker) {
				groups = append(groups, g)
			}
		}
	}
	groups = append(groups, map[string]any{
		"matcher": "*",
		"hooks": []any{
			map[string]any{"type": "command", "command": scriptPath},
		},
	})
	hooks["PostToolUse"] = groups
	m["hooks"] = hooks
	return writeFileAtomic(settingsPath, mustJSON(m))
}

// InstallSettingsHook upserts the board SessionStart hook into a settings.json,
// preserving all other settings and any other SessionStart hooks.
func InstallSettingsHook(path string) error {
	m := map[string]any{}
	if raw, err := os.ReadFile(path); err == nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, &m); err != nil {
			return fmt.Errorf("%s: existing file is not valid JSON: %w", path, err)
		}
	}
	hooks, _ := m["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	var groups []any
	if existing, ok := hooks["SessionStart"].([]any); ok {
		for _, g := range existing {
			if !groupHasMarker(g, boardHookMarker) { // drop our prior hook, keep others
				groups = append(groups, g)
			}
		}
	}
	groups = append(groups, map[string]any{
		"hooks": []any{
			map[string]any{"type": "command", "command": boardHookCommand()},
		},
	})
	hooks["SessionStart"] = groups
	m["hooks"] = hooks
	return writeFileAtomic(path, mustJSON(m))
}

func groupHasMarker(g any, marker string) bool {
	gm, ok := g.(map[string]any)
	if !ok {
		return false
	}
	hs, ok := gm["hooks"].([]any)
	if !ok {
		return false
	}
	for _, h := range hs {
		hm, ok := h.(map[string]any)
		if !ok {
			continue
		}
		if cmd, ok := hm["command"].(string); ok && strings.Contains(cmd, marker) {
			return true
		}
	}
	return false
}

// InstallClaudeMd inserts (or replaces in place) the board rules block, delimited
// by HTML comment markers so re-runs and surrounding user content are preserved.
func InstallClaudeMd(path string) error {
	block := boardMdStart + "\n" + boardRulesMarkdown + boardMdEnd
	var content string
	if raw, err := os.ReadFile(path); err == nil {
		content = string(raw)
	}
	if i := strings.Index(content, boardMdStart); i >= 0 {
		if j := strings.Index(content, boardMdEnd); j > i {
			j += len(boardMdEnd)
			return writeFileAtomic(path, []byte(content[:i]+block+content[j:]))
		}
	}
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	if content != "" {
		content += "\n"
	}
	content += block + "\n"
	return writeFileAtomic(path, []byte(content))
}

func Register(binPath string, c Client) error {
	if err := os.MkdirAll(filepath.Dir(c.Path), 0o755); err != nil {
		return err
	}
	switch c.Kind {
	case "toml":
		return WriteTOMLServer(c.Path, binPath)
	default:
		return WriteJSONServer(c.Path, binPath)
	}
}

func mustJSON(m map[string]any) []byte {
	b, _ := json.MarshalIndent(m, "", "  ")
	return append(b, '\n')
}

func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

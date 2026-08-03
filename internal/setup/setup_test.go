package setup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteJSONServerRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "mcp.json")
	// Pre-existing unrelated content must survive.
	os.WriteFile(p, []byte(`{"mcpServers":{"other":{"command":"x"}}}`), 0o644)
	if err := WriteJSONServer(p, "/usr/local/bin/board"); err != nil {
		t.Fatal(err)
	}
	// Idempotent second write.
	if err := WriteJSONServer(p, "/usr/local/bin/board"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p)
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	servers := m["mcpServers"].(map[string]any)
	if _, ok := servers["other"]; !ok {
		t.Fatal("clobbered existing server")
	}
	board := servers["board"].(map[string]any)
	if board["command"] != "/usr/local/bin/board" {
		t.Fatalf("wrong command: %v", board["command"])
	}
}

func TestInstallSettingsHookMergesAndIsIdempotent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	// Pre-existing unrelated settings + an unrelated SessionStart hook must survive.
	os.WriteFile(p, []byte(`{"theme":"dark","hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"echo keep-me"}]}]}}`), 0o644)

	if err := InstallSettingsHook(p); err != nil {
		t.Fatal(err)
	}
	if err := InstallSettingsHook(p); err != nil { // idempotent
		t.Fatal(err)
	}

	raw, _ := os.ReadFile(p)
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	if m["theme"] != "dark" {
		t.Fatal("clobbered unrelated setting")
	}
	groups := m["hooks"].(map[string]any)["SessionStart"].([]any)
	// Exactly: the pre-existing hook + one board hook (not two after re-run).
	if len(groups) != 2 {
		t.Fatalf("want 2 SessionStart groups, got %d", len(groups))
	}
	s := string(raw)
	if !strings.Contains(s, "echo keep-me") {
		t.Fatal("clobbered pre-existing SessionStart hook")
	}
	if strings.Count(s, boardHookMarker) != 1 {
		t.Fatalf("board hook should appear exactly once, got %d", strings.Count(s, boardHookMarker))
	}
}

func TestInstallClaudeMdInsertsAndReplacesInPlace(t *testing.T) {
	p := filepath.Join(t.TempDir(), "CLAUDE.md")
	os.WriteFile(p, []byte("# My rules\n\nkeep this line\n"), 0o644)

	if err := InstallClaudeMd(p); err != nil {
		t.Fatal(err)
	}
	if err := InstallClaudeMd(p); err != nil { // idempotent — replaces in place
		t.Fatal(err)
	}

	raw, _ := os.ReadFile(p)
	s := string(raw)
	if !strings.Contains(s, "keep this line") {
		t.Fatal("clobbered pre-existing content")
	}
	if strings.Count(s, boardMdStart) != 1 || strings.Count(s, boardMdEnd) != 1 {
		t.Fatalf("markers should appear exactly once:\n%s", s)
	}
	if !strings.Contains(s, "Board / Kanban (ALWAYS ON)") {
		t.Fatal("rules block missing")
	}
}

func TestInstallClaudeRulesInstallsPostToolUse(t *testing.T) {
	home := t.TempDir()
	settings := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(settings, []byte(`{"theme":"dark","hooks":{"PostToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"echo keep-me"}]}]}}`), 0o644)

	if err := InstallClaudeRules(home); err != nil {
		t.Fatal(err)
	}
	if err := InstallClaudeRules(home); err != nil { // idempotent
		t.Fatal(err)
	}

	script := filepath.Join(home, ".claude", "hooks", "board-post-tool-use.sh")
	fi, err := os.Stat(script)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&0o111 == 0 {
		t.Fatalf("hook script not executable: %v", fi.Mode())
	}
	body, _ := os.ReadFile(script)
	if !strings.Contains(string(body), "board run file") {
		t.Fatalf("script missing board run file:\n%s", body)
	}
	if strings.Contains(string(body), "board event tool") {
		t.Fatalf("script must not log every tool use:\n%s", body)
	}

	raw, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("settings not valid JSON: %v", err)
	}
	if m["theme"] != "dark" {
		t.Fatal("clobbered unrelated setting")
	}
	post := m["hooks"].(map[string]any)["PostToolUse"].([]any)
	nBoard, keep := 0, false
	for _, g := range post {
		gm := g.(map[string]any)
		if matcher, _ := gm["matcher"].(string); matcher == "Bash" {
			keep = true
		}
		for _, h := range gm["hooks"].([]any) {
			cmd, _ := h.(map[string]any)["command"].(string)
			if strings.Contains(cmd, claudePostToolUseMarker) {
				nBoard++
			}
			if strings.Contains(cmd, "echo keep-me") {
				keep = true
			}
		}
	}
	if !keep {
		t.Fatal("clobbered pre-existing PostToolUse hook")
	}
	if nBoard != 1 {
		t.Fatalf("want exactly one board PostToolUse hook, got %d in %s", nBoard, raw)
	}
	if !strings.Contains(string(raw), boardHookMarker) {
		t.Fatal("SessionStart board hook missing")
	}
}

func TestWriteTOMLServer(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(p, []byte("model = \"gpt-5.5\"\n"), 0o644)
	if err := WriteTOMLServer(p, "/usr/local/bin/board"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p)
	s := string(raw)
	if !strings.Contains(s, "[mcp_servers.board]") || !strings.Contains(s, `command = "/usr/local/bin/board"`) {
		t.Fatalf("toml missing board block:\n%s", s)
	}
	if !strings.Contains(s, `model = "gpt-5.5"`) {
		t.Fatal("clobbered existing toml content")
	}
}

func TestInstallCursorIntegration(t *testing.T) {
	home := t.TempDir()
	if err := InstallCursorIntegration(home); err != nil {
		t.Fatal(err)
	}
	if err := InstallCursorIntegration(home); err != nil { // idempotent
		t.Fatal(err)
	}

	skill := filepath.Join(home, ".cursor", "skills", "board", "SKILL.md")
	raw, err := os.ReadFile(skill)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.Contains(s, "name: board") || !strings.Contains(s, "resume") {
		t.Fatalf("skill missing expected content:\n%s", s)
	}

	rule := filepath.Join(home, ".cursor", "rules", "board.mdc")
	raw, err = os.ReadFile(rule)
	if err != nil {
		t.Fatal(err)
	}
	s = string(raw)
	if !strings.Contains(s, "alwaysApply: true") || !strings.Contains(s, "Board / Kanban") {
		t.Fatalf("rule missing expected content:\n%s", s)
	}

	cli := filepath.Join(home, ".cursor", "cli-config.json")
	raw, err = os.ReadFile(cli)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("cli-config not valid JSON: %v", err)
	}
	allow := m["permissions"].(map[string]any)["allow"].([]any)
	if !hasString(allow, cursorBoardAllow) {
		t.Fatalf("missing %s in allow: %v", cursorBoardAllow, allow)
	}

	script := filepath.Join(home, ".cursor", "hooks", "board-after-file-edit.sh")
	fi, err := os.Stat(script)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&0o111 == 0 {
		t.Fatalf("hook script not executable: %v", fi.Mode())
	}
	hooksPath := filepath.Join(home, ".cursor", "hooks.json")
	raw, err = os.ReadFile(hooksPath)
	if err != nil {
		t.Fatal(err)
	}
	var hooksFile map[string]any
	if err := json.Unmarshal(raw, &hooksFile); err != nil {
		t.Fatal(err)
	}
	after := hooksFile["hooks"].(map[string]any)["afterFileEdit"].([]any)
	nBoard := 0
	for _, e := range after {
		em := e.(map[string]any)
		if em["command"] == cursorBoardHookCommand {
			nBoard++
		}
	}
	if nBoard != 1 {
		t.Fatalf("want exactly one board afterFileEdit hook, got %d in %v", nBoard, after)
	}
}

func TestInstallCursorCLIAllowlistPreservesAndIsIdempotent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cli-config.json")
	os.WriteFile(p, []byte(`{"version":1,"editor":{"vimMode":true},"permissions":{"allow":["Shell(ls)"],"deny":[]}}`), 0o644)

	if err := InstallCursorCLIAllowlist(p); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(p)
	if err := InstallCursorCLIAllowlist(p); err != nil { // second run must not rewrite
		t.Fatal(err)
	}
	after, _ := os.ReadFile(p)
	if string(before) != string(after) {
		t.Fatal("idempotent re-run rewrote cli-config")
	}

	var m map[string]any
	if err := json.Unmarshal(before, &m); err != nil {
		t.Fatal(err)
	}
	if m["editor"].(map[string]any)["vimMode"] != true {
		t.Fatal("clobbered unrelated cli-config field")
	}
	allow := m["permissions"].(map[string]any)["allow"].([]any)
	if len(allow) != 2 || !hasString(allow, "Shell(ls)") || !hasString(allow, cursorBoardAllow) {
		t.Fatalf("unexpected allow list: %v", allow)
	}
}

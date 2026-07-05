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

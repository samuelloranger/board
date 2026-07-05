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

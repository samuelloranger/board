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

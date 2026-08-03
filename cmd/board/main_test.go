package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withDB(t *testing.T) {
	t.Helper()
	os.Setenv("BOARD_DB", filepath.Join(t.TempDir(), "b.db"))
	t.Cleanup(func() { os.Unsetenv("BOARD_DB") })
}

func TestCLIAddAndList(t *testing.T) {
	withDB(t)
	if err := run([]string{"add", "buy milk"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("add: %v", err)
	}
	var out bytes.Buffer
	if err := run([]string{"list"}, &out); err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out.String(), "buy milk") {
		t.Fatalf("list output missing task: %q", out.String())
	}
}

func TestCLIUnknownCommand(t *testing.T) {
	withDB(t)
	if err := run([]string{"frobnicate"}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error for unknown command")
	}
}

func TestCLIEvent(t *testing.T) {
	withDB(t)
	if err := run([]string{"event", "tool", "Edit"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("event: %v", err)
	}
}

func TestAllowedHostFor(t *testing.T) {
	cases := map[string]string{
		"127.0.0.1:7420":   "127.0.0.1",
		"localhost:7420":   "localhost",
		"192.168.1.5:9000": "192.168.1.5",
		// Unspecified binds are reachable under any name, so there is nothing
		// to pin the Host header to.
		"0.0.0.0:9000": "*",
		"[::]:9000":    "*",
		":7420":        "*",
	}
	for addr, want := range cases {
		if got := allowedHostFor(addr); got != want {
			t.Errorf("allowedHostFor(%q) = %q, want %q", addr, got, want)
		}
	}
}

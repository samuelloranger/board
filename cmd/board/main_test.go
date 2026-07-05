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

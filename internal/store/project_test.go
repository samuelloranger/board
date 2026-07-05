package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectProject(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "myrepo")
	sub := filepath.Join(repo, "a", "b")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got := DetectProject(sub)
	if got == nil || *got != "myrepo" {
		t.Fatalf("want myrepo, got %v", got)
	}
	if outside := DetectProject(root); outside != nil {
		t.Fatalf("expected nil outside a repo, got %v", *outside)
	}
}

package store

import (
	"os"
	"path/filepath"
)

// DetectProject walks up from startDir to find a git repository root and
// returns its folder name. Returns nil (global scope) if none is found.
func DetectProject(startDir string) *string {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return nil
	}
	for {
		if fi, err := os.Stat(filepath.Join(dir, ".git")); err == nil && (fi.IsDir() || fi.Mode().IsRegular()) {
			name := filepath.Base(dir)
			return &name
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil
		}
		dir = parent
	}
}

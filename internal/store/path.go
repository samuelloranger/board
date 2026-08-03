package store

import (
	"os"
	"path/filepath"
	"strings"
)

// RelativizeToHome returns path as ~/… when under the user home directory.
// Paths outside home (or when home is unknown) are returned cleaned absolute.
func RelativizeToHome(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		return filepath.ToSlash(path)
	}
	abs := path
	if !filepath.IsAbs(abs) {
		if a, err := filepath.Abs(abs); err == nil {
			abs = a
		}
	}
	abs = filepath.Clean(abs)
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.ToSlash(abs)
	}
	home = filepath.Clean(home)
	if abs == home {
		return "~"
	}
	prefix := home + string(os.PathSeparator)
	if strings.HasPrefix(abs, prefix) {
		return "~/" + filepath.ToSlash(abs[len(prefix):])
	}
	return filepath.ToSlash(abs)
}

// ExpandUserPath expands a leading ~/ (or bare ~) to the user home directory.
// Absolute paths are cleaned and returned; other relative paths are cleaned as-is.
func ExpandUserPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	switch {
	case path == "~":
		if home != "" {
			return home
		}
		return path
	case strings.HasPrefix(path, "~/"):
		if home != "" {
			return filepath.Clean(filepath.Join(home, path[2:]))
		}
		return path
	case filepath.IsAbs(path):
		return filepath.Clean(path)
	default:
		return filepath.Clean(path)
	}
}

// RelativizeToRoot strips root from path when path is under root.
// Already-relative paths are cleaned to slash form. Paths outside root stay absolute.
func RelativizeToRoot(path, root string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	root = ExpandUserPath(root)
	if root == "" {
		return filepath.ToSlash(filepath.Clean(path))
	}
	root = filepath.Clean(root)

	candidate := path
	if strings.HasPrefix(path, "~/") || path == "~" {
		candidate = ExpandUserPath(path)
	}
	if filepath.IsAbs(candidate) {
		rel, err := filepath.Rel(root, filepath.Clean(candidate))
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			if rel == "." {
				return "."
			}
			return filepath.ToSlash(rel)
		}
		return filepath.ToSlash(filepath.Clean(candidate))
	}
	return filepath.ToSlash(filepath.Clean(path))
}

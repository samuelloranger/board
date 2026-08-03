package store

import (
	"strings"
)

// RunFile is a path touched during an agent run.
// Path is project-root-relative when a project_paths mapping exists.
type RunFile struct {
	RunID       int64  `json:"run_id"`
	Path        string `json:"path"`
	FirstSeenAt string `json:"first_seen_at"`
}

// AddRunFile records path for runID. Empty/whitespace path → error.
// Absolute paths under the task's project root are stored relative (unduplicated).
// Duplicate (run_id, path) is ignored (no error, no second event).
// On first insert, emits event kind "run_file" with truncated path.
func (s *Store) AddRunFile(runID int64, path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return Invalid("path is required")
	}
	r, err := s.GetRun(runID)
	if err != nil {
		return err
	}
	path = s.relativizeRunFilePath(r.TaskID, path)
	ts := now()
	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO run_files (run_id, path, first_seen_at) VALUES (?, ?, ?)`,
		runID, path, ts,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n > 0 {
		tid := r.TaskID
		s.emit(&tid, "run_file", truncate(path, 80))
	}
	return nil
}

func (s *Store) relativizeRunFilePath(taskID int64, path string) string {
	tk, err := s.GetTask(taskID)
	if err != nil {
		return filepathToSlashClean(path)
	}
	projKey := GlobalProjectKey
	if tk.Project != nil && *tk.Project != "" {
		projKey = *tk.Project
	}
	absRoot, err := s.ResolveProjectPath(projKey)
	if err != nil {
		return filepathToSlashClean(path)
	}
	return RelativizeToRoot(path, absRoot)
}

func filepathToSlashClean(path string) string {
	return RelativizeToRoot(path, "")
}

// ListRunFiles returns paths newest-first (first_seen_at DESC, path ASC tiebreak).
func (s *Store) ListRunFiles(runID int64) ([]RunFile, error) {
	rows, err := s.db.Query(
		`SELECT run_id, path, first_seen_at FROM run_files WHERE run_id = ? ORDER BY rowid DESC`,
		runID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RunFile{}
	for rows.Next() {
		var f RunFile
		if err := rows.Scan(&f.RunID, &f.Path, &f.FirstSeenAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

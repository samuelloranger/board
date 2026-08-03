package store

import (
	"database/sql"
	"errors"
)

// GlobalProjectKey is the project_paths key for tasks with a null project.
const GlobalProjectKey = "_"

// ProjectPath maps a board project name to a working directory.
// Path is stored home-relative (~/…) when under the user home; absolute otherwise.
type ProjectPath struct {
	Project   string `json:"project"`
	Path      string `json:"path"`
	UpdatedAt string `json:"updated_at"`
}

func normalizeProjectKey(project string) string {
	if project == "" {
		return GlobalProjectKey
	}
	return project
}

func (s *Store) SetProjectPath(project, path string) (*ProjectPath, error) {
	if path == "" {
		return nil, Invalid("path is required")
	}
	project = normalizeProjectKey(project)
	stored := RelativizeToHome(ExpandUserPath(path))
	ts := now()
	_, err := s.db.Exec(
		`INSERT INTO project_paths (project, path, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(project) DO UPDATE SET path = excluded.path, updated_at = excluded.updated_at`,
		project, stored, ts,
	)
	if err != nil {
		return nil, err
	}
	return s.GetProjectPath(project)
}

// ResolveProjectPath returns the absolute filesystem path for a project mapping.
func (s *Store) ResolveProjectPath(project string) (string, error) {
	pp, err := s.GetProjectPath(project)
	if err != nil {
		return "", err
	}
	return ExpandUserPath(pp.Path), nil
}

func (s *Store) GetProjectPath(project string) (*ProjectPath, error) {
	project = normalizeProjectKey(project)
	var pp ProjectPath
	err := s.db.QueryRow(
		`SELECT project, path, updated_at FROM project_paths WHERE project = ?`, project,
	).Scan(&pp.Project, &pp.Path, &pp.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &pp, nil
}

func (s *Store) ListProjectPaths() ([]ProjectPath, error) {
	rows, err := s.db.Query(`SELECT project, path, updated_at FROM project_paths ORDER BY project`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProjectPath{}
	for rows.Next() {
		var pp ProjectPath
		if err := rows.Scan(&pp.Project, &pp.Path, &pp.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, pp)
	}
	return out, rows.Err()
}

func (s *Store) DeleteProjectPath(project string) error {
	project = normalizeProjectKey(project)
	res, err := s.db.Exec(`DELETE FROM project_paths WHERE project = ?`, project)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRelativizeToHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home")
	}
	got := RelativizeToHome(filepath.Join(home, "sites", "board"))
	if got != "~/sites/board" {
		t.Fatalf("got %q", got)
	}
	if RelativizeToHome(home) != "~" {
		t.Fatalf("home itself: %q", RelativizeToHome(home))
	}
	outside := "/var/tmp/elsewhere"
	if RelativizeToHome(outside) != outside {
		t.Fatalf("outside home should stay abs: %q", RelativizeToHome(outside))
	}
}

func TestExpandUserPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home")
	}
	got := ExpandUserPath("~/sites/board")
	want := filepath.Join(home, "sites", "board")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if ExpandUserPath("~") != home {
		t.Fatalf("~: %q", ExpandUserPath("~"))
	}
	abs := filepath.Join(home, "x")
	if ExpandUserPath(abs) != abs {
		t.Fatalf("abs passthrough: %q", ExpandUserPath(abs))
	}
}

func TestRelativizeToRoot(t *testing.T) {
	root := "/home/u/sites/board"
	got := RelativizeToRoot(filepath.Join(root, "internal", "store", "a.go"), root)
	if got != "internal/store/a.go" {
		t.Fatalf("got %q", got)
	}
	// Already relative: leave cleaned.
	if RelativizeToRoot("cmd/board/main.go", root) != "cmd/board/main.go" {
		t.Fatalf("rel: %q", RelativizeToRoot("cmd/board/main.go", root))
	}
	// Outside root: keep absolute (no false relativize).
	out := RelativizeToRoot("/tmp/smoke.txt", root)
	if out != "/tmp/smoke.txt" {
		t.Fatalf("outside: %q", out)
	}
}

func TestSetProjectPathStoresHomeRelative(t *testing.T) {
	st := newTestStore(t)
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home")
	}
	abs := filepath.Join(home, "sites", "board-path-test-"+t.Name())
	if err := os.MkdirAll(abs, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(abs) })

	pp, err := st.SetProjectPath("board", abs)
	if err != nil {
		t.Fatal(err)
	}
	if pp.Path != "~/sites/board-path-test-"+t.Name() {
		t.Fatalf("stored path = %q", pp.Path)
	}
	// Idempotent when already home-relative.
	pp2, err := st.SetProjectPath("board", pp.Path)
	if err != nil || pp2.Path != pp.Path {
		t.Fatalf("re-set: %+v %v", pp2, err)
	}
	absOut, err := st.ResolveProjectPath("board")
	if err != nil || absOut != abs {
		t.Fatalf("Resolve: %q %v", absOut, err)
	}
}

func TestAddRunFileStripsProjectRoot(t *testing.T) {
	st := newTestStore(t)
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home")
	}
	root := filepath.Join(home, "sites", "runfile-rel-"+t.Name())
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })

	proj := "board"
	if _, err := st.SetProjectPath(proj, root); err != nil {
		t.Fatal(err)
	}
	tk, err := st.CreateTask(CreateTaskParams{Title: "t", Project: &proj})
	if err != nil {
		t.Fatal(err)
	}
	r, err := st.CreateRun(tk.ID, "cursor", 1)
	if err != nil {
		t.Fatal(err)
	}
	absFile := filepath.Join(root, "internal", "store", "x.go")
	if err := st.AddRunFile(r.ID, absFile); err != nil {
		t.Fatal(err)
	}
	files, err := st.ListRunFiles(r.ID)
	if err != nil || len(files) != 1 {
		t.Fatalf("%+v %v", files, err)
	}
	if files[0].Path != "internal/store/x.go" {
		t.Fatalf("want project-relative, got %q", files[0].Path)
	}
	// Dedupe: same file via relative path.
	if err := st.AddRunFile(r.ID, "internal/store/x.go"); err != nil {
		t.Fatal(err)
	}
	files, _ = st.ListRunFiles(r.ID)
	if len(files) != 1 {
		t.Fatalf("dedupe failed: %+v", files)
	}
}

func TestMigratePathsOnOpen(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home")
	}
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "board.db")

	// Seed with legacy absolute paths using a raw open, then reopen via Open.
	st1, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(home, "sites", "migrate-path-"+t.Name())
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })

	// Bypass SetProjectPath relativization: insert abs directly.
	_, err = st1.db.Exec(
		`INSERT INTO project_paths (project, path, updated_at) VALUES (?, ?, ?)`,
		"board", root, "2026-01-01T00:00:00Z",
	)
	if err != nil {
		t.Fatal(err)
	}
	proj := "board"
	tk, _ := st1.CreateTask(CreateTaskParams{Title: "t", Project: &proj})
	r, _ := st1.CreateRun(tk.ID, "cursor", 1)
	absFile := filepath.Join(root, "a.go")
	_, err = st1.db.Exec(
		`INSERT INTO run_files (run_id, path, first_seen_at) VALUES (?, ?, ?)`,
		r.ID, absFile, "2026-01-01T00:00:00Z",
	)
	if err != nil {
		t.Fatal(err)
	}
	st1.Close()

	st2, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()

	pp, err := st2.GetProjectPath("board")
	if err != nil {
		t.Fatal(err)
	}
	wantPP := "~/sites/migrate-path-" + t.Name()
	if pp.Path != wantPP {
		t.Fatalf("project_paths migrated: got %q want %q", pp.Path, wantPP)
	}
	files, err := st2.ListRunFiles(r.ID)
	if err != nil || len(files) != 1 || files[0].Path != "a.go" {
		t.Fatalf("run_files migrated: %+v %v", files, err)
	}
}

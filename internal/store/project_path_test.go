package store

import (
	"errors"
	"testing"
)

func TestSetGetProjectPath(t *testing.T) {
	st := newTestStore(t)
	dir := t.TempDir()
	pp, err := st.SetProjectPath("board", dir)
	if err != nil {
		t.Fatalf("SetProjectPath: %v", err)
	}
	if pp.Project != "board" || pp.Path != dir || pp.UpdatedAt == "" {
		t.Fatalf("bad row: %+v", pp)
	}
	got, err := st.GetProjectPath("board")
	if err != nil || got.Path != dir {
		t.Fatalf("Get: %+v %v", got, err)
	}
}

func TestGetProjectPathMissing(t *testing.T) {
	st := newTestStore(t)
	_, err := st.GetProjectPath("nope")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestSetProjectPathUpsertAndGlobalKey(t *testing.T) {
	st := newTestStore(t)
	a, b := t.TempDir(), t.TempDir()
	if _, err := st.SetProjectPath("", a); err != nil {
		t.Fatalf("Set empty project: %v", err)
	}
	if _, err := st.SetProjectPath(GlobalProjectKey, b); err != nil {
		t.Fatalf("Set global: %v", err)
	}
	got, err := st.GetProjectPath(GlobalProjectKey)
	if err != nil || got.Path != b {
		t.Fatalf("upsert/global: %+v %v", got, err)
	}
	list, err := st.ListProjectPaths()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list: %+v", list)
	}
}

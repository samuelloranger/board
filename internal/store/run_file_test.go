package store

import (
	"testing"
)

func TestAddRunFileDedupeAndOrder(t *testing.T) {
	st := newTestStore(t)
	tk, _ := st.CreateTask(CreateTaskParams{Title: "t"})
	r, _ := st.CreateRun(tk.ID, "cursor", 1)
	if err := st.AddRunFile(r.ID, "a.go"); err != nil {
		t.Fatal(err)
	}
	if err := st.AddRunFile(r.ID, "b.go"); err != nil {
		t.Fatal(err)
	}
	if err := st.AddRunFile(r.ID, "a.go"); err != nil {
		t.Fatal(err)
	}
	files, err := st.ListRunFiles(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("len=%d %+v", len(files), files)
	}
	if files[0].Path != "b.go" || files[1].Path != "a.go" {
		t.Fatalf("%+v", files)
	}
	code := 0
	if _, err := st.FinishRun(r.ID, "exited", &code, ""); err != nil {
		t.Fatal(err)
	}
	files, _ = st.ListRunFiles(r.ID)
	if len(files) != 2 {
		t.Fatalf("files should survive FinishRun: %d", len(files))
	}
	if err := st.AddRunFile(r.ID, "  "); err == nil {
		t.Fatal("empty path should error")
	}
}

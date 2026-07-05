package mcpserver

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/samuelloranger/board/internal/store"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "b.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// connect wires an in-memory client/server pair over the SDK's in-process transport.
func connect(t *testing.T, st *store.Store, def *string) *mcp.ClientSession {
	t.Helper()
	srv := BuildServer(st, def)
	c := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v1"}, nil)
	ct, st2 := mcp.NewInMemoryTransports()
	if _, err := srv.Connect(context.Background(), st2, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := c.Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

func TestCreateTaskToolPersists(t *testing.T) {
	st := newStore(t)
	cs := connect(t, st, nil)
	_, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "create_task",
		Arguments: map[string]any{"title": "from mcp", "status": "todo"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	tasks, _ := st.ListTasks(store.ListFilter{})
	if len(tasks) != 1 || tasks[0].Title != "from mcp" {
		t.Fatalf("tool did not persist task: %+v", tasks)
	}
}

func TestDefaultProjectApplied(t *testing.T) {
	st := newStore(t)
	def := "autoproj"
	cs := connect(t, st, &def)
	if _, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "create_task",
		Arguments: map[string]any{"title": "scoped"},
	}); err != nil {
		t.Fatal(err)
	}
	tasks, _ := st.ListTasks(store.ListFilter{})
	if tasks[0].Project == nil || *tasks[0].Project != "autoproj" {
		t.Fatalf("default project not applied: %+v", tasks[0].Project)
	}
}

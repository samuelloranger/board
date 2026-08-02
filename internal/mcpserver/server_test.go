package mcpserver

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

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

// structOut decodes a tool call's structured result into a generic map.
func structOut(t *testing.T, res *mcp.CallToolResult) map[string]any {
	t.Helper()
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal %s: %v", b, err)
	}
	return m
}

func TestMoveTaskReportsTransitionOnly(t *testing.T) {
	st := newStore(t)
	cs := connect(t, st, nil)
	ctx := context.Background()

	tk, err := st.CreateTask(store.CreateTaskParams{Title: "t", Description: "a long description that must not be echoed"})
	if err != nil {
		t.Fatal(err)
	}
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "move_task",
		Arguments: map[string]any{"id": tk.ID, "status": "in_progress"}})
	if err != nil {
		t.Fatalf("move_task: %v", err)
	}
	m := structOut(t, res)
	if m["from"] != "todo" || m["to"] != "in_progress" {
		t.Fatalf("want todo->in_progress, got %+v", m)
	}
	if _, ok := m["description"]; ok {
		t.Fatalf("move_task leaked description: %+v", m)
	}
	if got, _ := st.GetTask(tk.ID); got.Status != "in_progress" {
		t.Fatalf("status not persisted: %q", got.Status)
	}
}

func TestAddNoteReturnsIdsOnly(t *testing.T) {
	st := newStore(t)
	cs := connect(t, st, nil)
	tk, _ := st.CreateTask(store.CreateTaskParams{Title: "t"})

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "add_note",
		Arguments: map[string]any{"id": tk.ID, "body": "verbose finding text"}})
	if err != nil {
		t.Fatalf("add_note: %v", err)
	}
	m := structOut(t, res)
	if _, ok := m["note_id"]; !ok {
		t.Fatalf("missing note_id: %+v", m)
	}
	if _, ok := m["body"]; ok {
		t.Fatalf("add_note echoed the body back: %+v", m)
	}
	got, _ := st.GetTask(tk.ID)
	if len(got.Notes) != 1 || got.Notes[0].Body != "verbose finding text" {
		t.Fatalf("note not persisted: %+v", got.Notes)
	}
}

func TestGetBoardSlimByDefaultAndVerbose(t *testing.T) {
	st := newStore(t)
	cs := connect(t, st, nil)
	ctx := context.Background()
	st.CreateTask(store.CreateTaskParams{Title: "t", Description: "heavy description"})

	m := structOut(t, mustCall(t, cs, ctx, "get_board", map[string]any{}))
	todo := m["todo"].([]any)
	if len(todo) != 1 {
		t.Fatalf("want 1 todo, got %+v", todo)
	}
	if _, ok := todo[0].(map[string]any)["description"]; ok {
		t.Fatalf("slim board leaked description: %+v", todo[0])
	}

	v := structOut(t, mustCall(t, cs, ctx, "get_board", map[string]any{"verbose": true}))
	vt := v["todo"].([]any)
	if vt[0].(map[string]any)["description"] != "heavy description" {
		t.Fatalf("verbose board dropped description: %+v", vt[0])
	}
}

func TestGetBoardCapsDoneColumn(t *testing.T) {
	st := newStore(t)
	cs := connect(t, st, nil)
	for i := 0; i < doneLimit+5; i++ {
		st.CreateTask(store.CreateTaskParams{Title: "d", Status: "done"})
	}
	m := structOut(t, mustCall(t, cs, context.Background(), "get_board", map[string]any{}))
	done := m["done"].([]any)
	if len(done) != doneLimit {
		t.Fatalf("want %d done, got %d", doneLimit, len(done))
	}
	if m["done_total"].(float64) != float64(doneLimit+5) {
		t.Fatalf("done_total wrong: %+v", m["done_total"])
	}
}

func mustCall(t *testing.T, cs *mcp.ClientSession, ctx context.Context, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return res
}

func TestHandoffAndResumeTools(t *testing.T) {
	st := newStore(t)
	def := "proj"
	cs := connect(t, st, &def)
	ctx := context.Background()

	// Create + hand off.
	cs.CallTool(ctx, &mcp.CallToolParams{Name: "create_task",
		Arguments: map[string]any{"title": "needs human"}})
	tasks, _ := st.ListTasks(store.ListFilter{})
	id := tasks[0].ID
	if _, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "handoff",
		Arguments: map[string]any{"id": id, "to": "human", "reason": "approve deploy"}}); err != nil {
		t.Fatalf("handoff tool: %v", err)
	}
	got, _ := st.GetTask(id)
	if got.HandoffTo == nil || *got.HandoffTo != "human" {
		t.Fatalf("handoff not applied: %+v", got.HandoffTo)
	}
	// resume returns without error.
	if _, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "resume",
		Arguments: map[string]any{}}); err != nil {
		t.Fatalf("resume tool: %v", err)
	}
}

func TestAskUserHelper(t *testing.T) {
	st := newStore(t)
	tk, _ := st.CreateTask(store.CreateTaskParams{Title: "t"})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() {
		time.Sleep(50 * time.Millisecond)
		qs, _ := st.ListQuestions(&tk.ID, "pending")
		if len(qs) == 1 {
			st.AnswerQuestion(qs[0].ID, "ok")
		}
	}()
	out, err := askUser(ctx, st, tk.ID, "go?")
	if err != nil || out["answer"] != "ok" {
		t.Fatalf("%v %#v", err, out)
	}
}

func TestAskUserTool(t *testing.T) {
	st := newStore(t)
	tk, _ := st.CreateTask(store.CreateTaskParams{Title: "t"})
	cs := connect(t, st, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go func() {
		time.Sleep(80 * time.Millisecond)
		qs, _ := st.ListQuestions(&tk.ID, "pending")
		if len(qs) == 1 {
			st.AnswerQuestion(qs[0].ID, "blue")
		}
	}()
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "ask_user",
		Arguments: map[string]any{"task_id": tk.ID, "question": "color?"},
	})
	if err != nil {
		t.Fatalf("ask_user: %v", err)
	}
	m := structOut(t, res)
	if m["answer"] != "blue" {
		t.Fatalf("got %#v", m)
	}
}

func TestSetRunWaitTool(t *testing.T) {
	st := newStore(t)
	tk, _ := st.CreateTask(store.CreateTaskParams{Title: "t"})
	cs := connect(t, st, nil)
	ctx := context.Background()
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "set_run_wait",
		Arguments: map[string]any{"task_id": tk.ID, "wait": "ci"},
	})
	if err != nil {
		t.Fatalf("CallTool transport: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatal("expected tool error with no active run")
	}
	if _, err := st.CreateRun(tk.ID, "cursor", 1); err != nil {
		t.Fatal(err)
	}
	res, err = cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "set_run_wait",
		Arguments: map[string]any{"task_id": tk.ID, "wait": "ci"},
	})
	if err != nil {
		t.Fatalf("set_run_wait: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %+v", res)
	}
	m := structOut(t, res)
	if m["wait"] != "ci" {
		t.Fatalf("got %#v", m)
	}
	run, _ := st.ActiveRunForTask(tk.ID)
	if run.Wait != "ci" {
		t.Fatalf("store wait=%q", run.Wait)
	}
	if _, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "set_run_wait",
		Arguments: map[string]any{"task_id": tk.ID, "wait": ""},
	}); err != nil {
		t.Fatalf("clear: %v", err)
	}
}

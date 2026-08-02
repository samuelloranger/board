# FE trigger agent + ask-back Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** From the board web UI, Run a task with Cursor Agent CLI; the agent knows the run came from board and can ask the human questions via a blocking `ask_user` MCP tool that surfaces in the UI.

**Architecture:** `board serve` spawns `cursor-agent` and owns Run/path/answer HTTP APIs. `board mcp` gains `ask_user`, which writes a pending question to SQLite and polls until answered. Serve and MCP already share `~/.board/board.db`; SSE `/api/events` notifies the UI. No new daemon.

**Tech Stack:** Go 1.26, `modernc.org/sqlite`, MCP go-sdk, Svelte 5 + Vite (Bun build → embedded `ui/dist`).

**Spec:** `docs/superpowers/specs/2026-08-02-fe-trigger-agent-ask-back-design.md`

## Global Constraints

- Additive migrations only — never edit the base `schema` string in `store.go`; new tables via `CREATE TABLE IF NOT EXISTS` in `Open` after the existing schema exec.
- Keep `db.SetMaxOpenConns(1)`. `ask_user` poll must use short queries (no long-held transaction) or Answer from serve will deadlock.
- MCP write/ask responses stay token-small (`{answer}` only for `ask_user`).
- v1 agent is `cursor` only; API may accept `"agent":"cursor"` and reject others with 400.
- Global/null-project path key is `"_"` (literal underscore).
- Serve stays loopback-default; no auth.
- After any `internal/web/ui/src/` edit: `cd internal/web/ui && bun run build` and commit regenerated `dist/`.
- Tests: `go test ./...`; format: `gofmt`; no FE unit runner.

## File map

| Path | Responsibility |
|------|----------------|
| `internal/store/store.go` | Create `project_paths`, `questions`, `runs` tables in `Open` |
| `internal/store/project_path.go` | Upsert/Get/List/Delete project → path |
| `internal/store/project_path_test.go` | Path map tests |
| `internal/store/question.go` | Ask / Answer / Cancel / List pending / WaitForAnswer |
| `internal/store/question_test.go` | Question lifecycle tests |
| `internal/store/run.go` | CreateRun / FinishRun / ActiveRun / CancelPending on finish / ReconcileOrphans |
| `internal/store/run_test.go` | Run invariants + reconcile |
| `internal/agent/prompt.go` | Board provenance prompt builder |
| `internal/agent/prompt_test.go` | Prompt contains provenance + task id |
| `internal/agent/runner.go` | `Runner` interface + `CursorRunner` (`cursor-agent -p --force`) |
| `internal/agent/runner_test.go` | Fake runner / lookPath error |
| `internal/mcpserver/server.go` | Register `ask_user` |
| `internal/mcpserver/server_test.go` | ask_user happy path + cancel |
| `internal/web/web.go` | Paths, questions, runs, Run/Cancel handlers; inject Runner |
| `internal/web/web_test.go` | HTTP coverage with fake runner |
| `internal/web/ui/src/App.svelte` | Run / path prompt / cancel / ask modal / projects list |
| `internal/web/ui/dist/*` | Regenerated embed assets |
| `cmd/board/main.go` | Call `ReconcileOrphanRuns` on serve start; pass runner into Handler |
| `README.md` / `CLAUDE.md` | Document Run + `ask_user` |

---

### Task 1: Schema + `project_paths` store

**Files:**
- Modify: `internal/store/store.go` (`Open`)
- Create: `internal/store/project_path.go`
- Create: `internal/store/project_path_test.go`
- Modify: `internal/store/store_test.go` (optional: assert new tables exist)

**Interfaces:**
- Consumes: `Store`, `now()`
- Produces:
  ```go
  const GlobalProjectKey = "_"

  type ProjectPath struct {
      Project   string `json:"project"`
      Path      string `json:"path"`
      UpdatedAt string `json:"updated_at"`
  }

  func (s *Store) SetProjectPath(project, absPath string) (*ProjectPath, error)
  func (s *Store) GetProjectPath(project string) (*ProjectPath, error) // ErrNotFound if missing
  func (s *Store) ListProjectPaths() ([]ProjectPath, error)
  func (s *Store) DeleteProjectPath(project string) error
  ```
  - `SetProjectPath`: reject empty path; empty project → `GlobalProjectKey`.
  - Path existence is validated at the HTTP layer, not in the store.

- [ ] **Step 1: Write the failing test**

```go
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
	st.SetProjectPath("", a) // → "_"
	st.SetProjectPath(GlobalProjectKey, b)
	got, _ := st.GetProjectPath(GlobalProjectKey)
	if got.Path != b {
		t.Fatalf("upsert/global: %+v", got)
	}
	list, _ := st.ListProjectPaths()
	if len(list) != 1 {
		t.Fatalf("list: %+v", list)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store -run TestSetGetProjectPath -count=1`

Expected: FAIL (undefined `SetProjectPath` / compile error)

- [ ] **Step 3: Implement schema + methods**

In `Open`, after the existing `schema` exec and ALTER loop, exec:

```go
const extraTables = `
CREATE TABLE IF NOT EXISTS project_paths (
  project    TEXT PRIMARY KEY,
  path       TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS questions (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id     INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  question    TEXT NOT NULL,
  answer      TEXT,
  status      TEXT NOT NULL CHECK(status IN ('pending','answered','cancelled')),
  created_at  TEXT NOT NULL,
  answered_at TEXT
);
CREATE TABLE IF NOT EXISTS runs (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id    INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  agent      TEXT NOT NULL,
  pid        INTEGER,
  status     TEXT NOT NULL CHECK(status IN ('running','exited','failed','killed')),
  started_at TEXT NOT NULL,
  ended_at   TEXT,
  exit_code  INTEGER
);
CREATE INDEX IF NOT EXISTS idx_questions_task ON questions(task_id);
CREATE INDEX IF NOT EXISTS idx_questions_status ON questions(status);
CREATE INDEX IF NOT EXISTS idx_runs_task ON runs(task_id);
CREATE INDEX IF NOT EXISTS idx_runs_status ON runs(status);
`
```

Creating `questions`/`runs` tables here avoids a second migration pass; leave their Go APIs for Tasks 2–3.

Implement `project_path.go` as specified.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/store -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/store/store.go internal/store/project_path.go internal/store/project_path_test.go
git commit -m "$(cat <<'EOF'
feat(store): add project_paths table and CRUD

Remember git-folder project name → absolute cwd for agent Run.
EOF
)"
```

---

### Task 2: Questions store + wait/answer

**Files:**
- Create: `internal/store/question.go`
- Create: `internal/store/question_test.go`

**Interfaces:**
- Consumes: `Store`, `emit`, `now`, tables from Task 1
- Produces:
  ```go
  type Question struct {
      ID         int64   `json:"id"`
      TaskID     int64   `json:"task_id"`
      Question   string  `json:"question"`
      Answer     *string `json:"answer,omitempty"`
      Status     string  `json:"status"`
      CreatedAt  string  `json:"created_at"`
      AnsweredAt *string `json:"answered_at,omitempty"`
  }

  var ErrQuestionClosed = errors.New("question is not pending")

  func (s *Store) CreateQuestion(taskID int64, question string) (*Question, error)
  func (s *Store) GetQuestion(id int64) (*Question, error)
  func (s *Store) ListQuestions(taskID *int64, status string) ([]Question, error) // status "" = any
  func (s *Store) AnswerQuestion(id int64, answer string) (*Question, error)
  func (s *Store) CancelPendingQuestions(taskID int64) (int64, error) // rows affected
  // WaitForAnswer polls GetQuestion until answered/cancelled/ctx done.
  // CRITICAL: each poll is a separate QueryRow — never hold a transaction across sleeps.
  func (s *Store) WaitForAnswer(ctx context.Context, id int64, poll time.Duration) (string, error)
  ```
  - `CreateQuestion`: require non-empty question; task must exist; status `pending`; `emit(&taskID, "question", short detail)`.
  - `AnswerQuestion`: only if `pending`; set answer, status `answered`, answered_at; emit `answered`.
  - `WaitForAnswer`: on `answered` return answer text; on `cancelled` return error; honor `ctx`.

- [ ] **Step 1: Write the failing test**

```go
func TestQuestionAskAnswer(t *testing.T) {
	st := newTestStore(t)
	tk, _ := st.CreateTask(CreateTaskParams{Title: "t"})
	q, err := st.CreateQuestion(tk.ID, "Ship it?")
	if err != nil || q.Status != "pending" {
		t.Fatalf("CreateQuestion: %+v %v", q, err)
	}
	done := make(chan string, 1)
	go func() {
		ans, err := st.WaitForAnswer(context.Background(), q.ID, 20*time.Millisecond)
		if err != nil {
			t.Errorf("Wait: %v", err)
			return
		}
		done <- ans
	}()
	time.Sleep(40 * time.Millisecond)
	if _, err := st.AnswerQuestion(q.ID, "yes"); err != nil {
		t.Fatalf("Answer: %v", err)
	}
	select {
	case ans := <-done:
		if ans != "yes" {
			t.Fatalf("got %q", ans)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WaitForAnswer timed out")
	}
}

func TestCancelPendingQuestions(t *testing.T) {
	st := newTestStore(t)
	tk, _ := st.CreateTask(CreateTaskParams{Title: "t"})
	q, _ := st.CreateQuestion(tk.ID, "q?")
	n, err := st.CancelPendingQuestions(tk.ID)
	if err != nil || n != 1 {
		t.Fatalf("cancel: n=%d err=%v", n, err)
	}
	got, _ := st.GetQuestion(q.ID)
	if got.Status != "cancelled" {
		t.Fatalf("%+v", got)
	}
	_, err = st.WaitForAnswer(context.Background(), q.ID, time.Millisecond)
	if err == nil {
		t.Fatal("expected error on cancelled")
	}
}
```

- [ ] **Step 2: Run test — expect FAIL**

Run: `go test ./internal/store -run 'TestQuestion|TestCancelPending' -count=1`

- [ ] **Step 3: Implement `question.go`**

- [ ] **Step 4: Run `go test ./internal/store -count=1` — PASS**

- [ ] **Step 5: Commit**

```bash
git add internal/store/question.go internal/store/question_test.go
git commit -m "$(cat <<'EOF'
feat(store): questions table with blocking WaitForAnswer

Bridge ask_user MCP to the web UI via SQLite poll + answer.
EOF
)"
```

---

### Task 3: Runs store + orphan reconcile

**Files:**
- Create: `internal/store/run.go`
- Create: `internal/store/run_test.go`

**Interfaces:**
- Consumes: `CancelPendingQuestions`, `MoveTask`, `GetTask`
- Produces:
  ```go
  type Run struct {
      ID        int64   `json:"id"`
      TaskID    int64   `json:"task_id"`
      Agent     string  `json:"agent"`
      PID       *int    `json:"pid,omitempty"`
      Status    string  `json:"status"`
      StartedAt string  `json:"started_at"`
      EndedAt   *string `json:"ended_at,omitempty"`
      ExitCode  *int    `json:"exit_code,omitempty"`
  }

  var ErrRunActive = errors.New("task already has a running agent")

  func (s *Store) CreateRun(taskID int64, agent string, pid int) (*Run, error)
  func (s *Store) GetRun(id int64) (*Run, error)
  func (s *Store) ListRuns(taskID *int64, status string) ([]Run, error)
  func (s *Store) ActiveRunForTask(taskID int64) (*Run, error) // ErrNotFound if none running
  func (s *Store) FinishRun(id int64, status string, exitCode *int) (*Run, error) // exited|failed|killed
  // ReconcileOrphanRuns marks running rows whose pid is dead as failed and cancels
  // pending questions. alive(pid) is injected for tests.
  func (s *Store) ReconcileOrphanRuns(alive func(pid int) bool) (int, error)
  ```
  - `CreateRun`: if active run exists → `ErrRunActive`. Insert `running`. Emit `run`. Then if task status != `in_progress`, call `MoveTask(taskID, "in_progress")`.
  - `FinishRun`: set status/ended_at/exit_code; `CancelPendingQuestions(taskID)`; emit `run_done`.

- [ ] **Step 1: Failing tests**

```go
func TestCreateRunRejectsSecond(t *testing.T) {
	st := newTestStore(t)
	tk, _ := st.CreateTask(CreateTaskParams{Title: "t"})
	if _, err := st.CreateRun(tk.ID, "cursor", 1234); err != nil {
		t.Fatal(err)
	}
	_, err := st.CreateRun(tk.ID, "cursor", 5678)
	if !errors.Is(err, ErrRunActive) {
		t.Fatalf("got %v", err)
	}
}

func TestFinishRunCancelsQuestions(t *testing.T) {
	st := newTestStore(t)
	tk, _ := st.CreateTask(CreateTaskParams{Title: "t"})
	r, _ := st.CreateRun(tk.ID, "cursor", 1)
	q, _ := st.CreateQuestion(tk.ID, "q?")
	code := 1
	st.FinishRun(r.ID, "failed", &code)
	got, _ := st.GetQuestion(q.ID)
	if got.Status != "cancelled" {
		t.Fatalf("%+v", got)
	}
}

func TestReconcileOrphanRuns(t *testing.T) {
	st := newTestStore(t)
	tk, _ := st.CreateTask(CreateTaskParams{Title: "t"})
	st.CreateRun(tk.ID, "cursor", 99999)
	n, err := st.ReconcileOrphanRuns(func(pid int) bool { return false })
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	_, err = st.ActiveRunForTask(tk.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("still active: %v", err)
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement `run.go`**

- [ ] **Step 4: `go test ./internal/store -count=1` — PASS**

- [ ] **Step 5: Commit**

```bash
git add internal/store/run.go internal/store/run_test.go
git commit -m "$(cat <<'EOF'
feat(store): agent runs with one-active guard and orphan reconcile

Track cursor-agent PIDs and clear pending asks when a run ends.
EOF
)"
```

---

### Task 4: MCP `ask_user`

**Files:**
- Modify: `internal/mcpserver/server.go`
- Modify: `internal/mcpserver/server_test.go`

**Interfaces:**
- Consumes: `CreateQuestion`, `WaitForAnswer`
- Produces: tool `ask_user` with args `task_id` (int64), `question` (string); result `{ "answer": "..." }` only.
- Extract helper for testability:

  ```go
  func askUser(ctx context.Context, st *store.Store, taskID int64, question string) (map[string]any, error) {
      if taskID == 0 || question == "" {
          return nil, fmt.Errorf("task_id and question are required")
      }
      q, err := st.CreateQuestion(taskID, question)
      if err != nil {
          return nil, err
      }
      waitCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
      defer cancel()
      ans, err := st.WaitForAnswer(waitCtx, q.ID, 200*time.Millisecond)
      if err != nil {
          return nil, err
      }
      return map[string]any{"answer": ans}, nil
  }
  ```

- Tool description: use when launched from the board web UI and you need human input; pass the board task id; do not use terminal prompts.

- [ ] **Step 1: Failing test**

```go
func TestAskUserHelper(t *testing.T) {
	st := openTestStore(t) // use existing helper name from server_test.go
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
```

(If the existing helper is named differently, match `server_test.go`.)

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement helper + `mcp.AddTool` registration**

- [ ] **Step 4: `go test ./internal/mcpserver -count=1` — PASS**

- [ ] **Step 5: Commit**

```bash
git add internal/mcpserver/server.go internal/mcpserver/server_test.go
git commit -m "$(cat <<'EOF'
feat(mcp): add ask_user tool for board UI ask-back

Blocks until the human answers via the web UI (SQLite bridge).
EOF
)"
```

---

### Task 5: Agent prompt + Cursor runner

**Files:**
- Create: `internal/agent/prompt.go`
- Create: `internal/agent/prompt_test.go`
- Create: `internal/agent/runner.go`
- Create: `internal/agent/runner_test.go`

**Interfaces:**
- Consumes: `*store.Task`
- Produces:
  ```go
  package agent

  func BuildPrompt(tk *store.Task) string

  type StartResult struct {
      PID  int
      Wait func() (exitCode int, err error)
      Kill func() error
  }

  type Runner interface {
      Start(cwd, prompt string) (*StartResult, error)
  }

  type CursorRunner struct{}

  var lookPath = exec.LookPath // injectable for tests

  func (CursorRunner) Start(cwd, prompt string) (*StartResult, error)

  // FakeRunner for web tests
  type FakeRunner struct {
      LastCwd, LastPrompt string
      StartErr            error
      ExitCode            int
  }

  func (f *FakeRunner) Start(cwd, prompt string) (*StartResult, error)
  ```
  - Prompt MUST contain: `launched from the board web UI`, `#`+id, `ask_user`, title, description, recent notes (last few).
  - Cursor argv: `cursor-agent`, `-p`, `--force`, `--output-format`, `text`, then the prompt string.
  - `Start` sets `cmd.Dir = cwd`.

- [ ] **Step 1: Prompt test**

```go
func TestBuildPromptProvenance(t *testing.T) {
	tk := &store.Task{ID: 42, Title: "Fix bug", Description: "desc", Status: "todo"}
	p := BuildPrompt(tk)
	for _, want := range []string{"board web UI", "#42", "ask_user", "Fix bug", "desc"} {
		if !strings.Contains(p, want) {
			t.Fatalf("missing %q in %s", want, p)
		}
	}
}
```

- [ ] **Step 2: Implement prompt; test PASS**

- [ ] **Step 3: Runner missing-binary + FakeRunner tests**

```go
func TestCursorRunnerMissingBinary(t *testing.T) {
	old := lookPath
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	defer func() { lookPath = old }()
	_, err := (CursorRunner{}).Start(t.TempDir(), "hi")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFakeRunnerRecordsPrompt(t *testing.T) {
	f := &FakeRunner{ExitCode: 0}
	res, err := f.Start("/tmp/x", "hello")
	if err != nil || f.LastCwd != "/tmp/x" || f.LastPrompt != "hello" || res.PID != 4242 {
		t.Fatalf("%v %+v %+v", err, f, res)
	}
	code, _ := res.Wait()
	if code != 0 {
		t.Fatalf("code %d", code)
	}
}
```

(`FakeRunner` returns PID `4242`.)

- [ ] **Step 4: `go test ./internal/agent -count=1` — PASS**

- [ ] **Step 5: Commit**

```bash
git add internal/agent/
git commit -m "$(cat <<'EOF'
feat(agent): board provenance prompt and CursorRunner

Spawn cursor-agent -p with a prompt that forces ask_user for questions.
EOF
)"
```

---

### Task 6: Web API — paths, questions, runs list/answer

**Files:**
- Modify: `internal/web/web.go`
- Modify: `internal/web/web_test.go`

**Interfaces:**
- Handler construction:

  ```go
  type Config struct {
      Store  *store.Store
      Runner agent.Runner // nil → agent.CursorRunner{}
  }

  func Handler(st *store.Store) http.Handler {
      return HandlerConfig(Config{Store: st})
  }

  func HandlerConfig(cfg Config) http.Handler
  ```

- New routes:
  - `PUT /api/projects/{name}/path` — JSON `{path}`; `os.Stat` must be a directory; empty/`*` name → `_`
  - `GET /api/projects/paths`
  - `DELETE /api/projects/{name}/path`
  - `GET /api/questions?status=pending&task_id=`
  - `POST /api/questions/{id}/answer` — `{answer}`
  - `GET /api/runs?task_id=`

Match existing `/api/tasks/` path-parsing style in `web.go`.

- [ ] **Step 1: Tests**

```go
func TestProjectPathAPI(t *testing.T) {
	st := newStore(t)
	srv := httptest.NewServer(Handler(st))
	defer srv.Close()
	dir := t.TempDir()
	body, _ := json.Marshal(map[string]string{"path": dir})
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/projects/board/path", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("status=%d err=%v", resp.StatusCode, err)
	}
	r2, _ := http.Get(srv.URL + "/api/projects/paths")
	var list []store.ProjectPath
	json.NewDecoder(r2.Body).Decode(&list)
	if len(list) != 1 || list[0].Path != dir {
		t.Fatalf("%+v", list)
	}
}

func TestAnswerQuestionAPI(t *testing.T) {
	st := newStore(t)
	tk, _ := st.CreateTask(store.CreateTaskParams{Title: "t"})
	q, _ := st.CreateQuestion(tk.ID, "q?")
	srv := httptest.NewServer(Handler(st))
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/api/questions/"+strconv.FormatInt(q.ID, 10)+"/answer",
		"application/json", strings.NewReader(`{"answer":"a"}`))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("%v %d", err, resp.StatusCode)
	}
}
```

- [ ] **Step 2: Implement routes — tests PASS**

- [ ] **Step 3: Commit**

```bash
git add internal/web/web.go internal/web/web_test.go
git commit -m "$(cat <<'EOF'
feat(web): project path and question answer APIs

UI can remember cwds and answer ask_user prompts over HTTP.
EOF
)"
```

---

### Task 7: Web API — Run + Cancel with fake runner

**Files:**
- Modify: `internal/web/web.go`
- Modify: `internal/web/web_test.go`
- Modify: `cmd/board/main.go` — reconcile orphans on serve start

**Run flow (`POST /api/tasks/{id}/run`):**
1. Decode `{agent}`; default `cursor`; if not `cursor` → 400.
2. `GetTask`; project key = `*tk.Project` or `_`.
3. `GetProjectPath`; if `ErrNotFound` → **409** `{"need_path":true,"project":"<key>"}`.
4. `Runner.Start(path, agent.BuildPrompt(tk))`; on error → 400; **do not** create run.
5. `CreateRun(taskID, "cursor", pid)`; on `ErrRunActive` → Kill started process, return 409.
6. Keep `runID → Kill` in a mutex map on the server; goroutine waits, then `FinishRun` (`exited` if code==0 && err==nil else `failed`), delete map entry.
7. Return `{run_id, task_id, agent, status:"running"}`.

**Cancel (`POST /api/tasks/{id}/run/cancel`):**
1. `ActiveRunForTask`; call Kill from map (fallback `os.FindProcess(pid).Kill()`).
2. `FinishRun(..., "killed", nil)`.

**main.go serve start:**

```go
st.ReconcileOrphanRuns(func(pid int) bool {
    p, err := os.FindProcess(pid)
    if err != nil {
        return false
    }
    return p.Signal(syscall.Signal(0)) == nil
})
```

- [ ] **Step 1: Tests with FakeRunner**

```go
func TestRunNeedPath(t *testing.T) {
	st := newStore(t)
	p := "board"
	tk, _ := st.CreateTask(store.CreateTaskParams{Title: "t", Project: &p})
	fake := &agent.FakeRunner{}
	srv := httptest.NewServer(HandlerConfig(Config{Store: st, Runner: fake}))
	defer srv.Close()
	resp, _ := http.Post(srv.URL+"/api/tasks/"+strconv.FormatInt(tk.ID, 10)+"/run",
		"application/json", strings.NewReader(`{"agent":"cursor"}`))
	if resp.StatusCode != 409 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["need_path"] != true {
		t.Fatalf("%v", body)
	}
}

func TestRunStartsAgent(t *testing.T) {
	st := newStore(t)
	p := "board"
	tk, _ := st.CreateTask(store.CreateTaskParams{Title: "t", Project: &p})
	dir := t.TempDir()
	st.SetProjectPath("board", dir)
	fake := &agent.FakeRunner{ExitCode: 0}
	srv := httptest.NewServer(HandlerConfig(Config{Store: st, Runner: fake}))
	defer srv.Close()
	resp, _ := http.Post(srv.URL+"/api/tasks/"+strconv.FormatInt(tk.ID, 10)+"/run",
		"application/json", strings.NewReader(`{"agent":"cursor"}`))
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if fake.LastCwd != dir || !strings.Contains(fake.LastPrompt, "board web UI") {
		t.Fatalf("cwd=%q prompt=%q", fake.LastCwd, fake.LastPrompt)
	}
	tk2, _ := st.GetTask(tk.ID)
	if tk2.Status != "in_progress" {
		t.Fatalf("status %s", tk2.Status)
	}
}
```

- [ ] **Step 2: Implement Run/Cancel + main reconcile**

- [ ] **Step 3: `go test ./internal/web -count=1` — PASS**

- [ ] **Step 4: Commit**

```bash
git add internal/web/web.go internal/web/web_test.go cmd/board/main.go
git commit -m "$(cat <<'EOF'
feat(web): Run and Cancel task agent from the API

Spawn cursor-agent when a project path is known; reconcile orphans on serve.
EOF
)"
```

---

### Task 8: Frontend — Run, path prompt, cancel, ask-back, projects

**Files:**
- Modify: `internal/web/ui/src/App.svelte`
- Regenerate: `internal/web/ui/dist/**` via `bun run build`

**Behavior:**
1. Detail drawer: **Run** → `POST /api/tasks/{id}/run`. On 409 `need_path`, show inline path field; Save & Run = `PUT .../path` then `POST .../run`.
2. On load + after SSE kinds `question`/`answered`/`run`/`run_done`: refresh `GET /api/questions?status=pending` and active runs (`GET /api/runs?status=running` or filter client-side).
3. Active run on detail task → “Cursor running” + **Cancel**.
4. Modal for pending question: textarea + Submit → `POST /api/questions/{id}/answer`. Card badge when that task has a pending question.
5. Header **Projects** panel: list/edit/clear paths.
6. Extend `eventKindLabel` / verbs for `run`, `run_done`, `question`, `answered`.

Keep existing CSS variables and detail-drawer patterns.

- [ ] **Step 1: Implement App.svelte changes**

- [ ] **Step 2: Build embed assets**

```bash
cd internal/web/ui && bun install && bun run build
```

- [ ] **Step 3: `go test ./...` still PASS; `go build -o board ./cmd/board`**

- [ ] **Step 4: Commit**

```bash
git add internal/web/ui/src/App.svelte internal/web/ui/dist
git commit -m "$(cat <<'EOF'
feat(ui): Run agent, path map, and ask-back modal

Wire the web UI to spawn Cursor and answer ask_user prompts live.
EOF
)"
```

---

### Task 9: Docs

**Files:**
- Modify: `README.md` — add `ask_user` to MCP list; short “Run from web UI” bullet.
- Modify: `CLAUDE.md` — MCP surface + Run/ask_user/project_paths note.
- Modify: `plugin/skills/board/SKILL.md` and `internal/setup/cursor.go` embedded skill text — mention `ask_user` for human questions mid-run.

- [ ] **Step 1: Update docs to match shipped behavior**

- [ ] **Step 2: Verify**

```bash
go test ./...
test -z "$(gofmt -l .)"
```

- [ ] **Step 3: Commit**

```bash
git add README.md CLAUDE.md plugin/skills/board/SKILL.md internal/setup/cursor.go
git commit -m "$(cat <<'EOF'
docs: document Run from UI and ask_user

Keep README/CLAUDE/skill lists aligned with the new MCP tool.
EOF
)"
```

---

## Spec coverage checklist

| Spec requirement | Task |
|------------------|------|
| `project_paths` UI-managed map | 1, 6, 8 |
| `questions` + blocking ask | 2, 4, 6, 8 |
| `runs` + one active + reconcile | 3, 7 |
| Spawn cursor-agent + provenance prompt | 5, 7 |
| HTTP Run / Cancel / paths / answer / pending GET | 6, 7 |
| FE Run, path prompt, modal, projects | 8 |
| Token-small `ask_user` | 4 |
| No Claude/Codex in v1 | 7 (reject non-cursor) |
| Docs | 9 |

## Self-review notes

- `FakeRunner` defined in Task 5, reused in Task 7.
- `HandlerConfig` in Task 6; Run wired in Task 7.
- Global project key `"_"` consistent.
- `WaitForAnswer` must not hold a SQL transaction across poll sleeps (`MaxOpenConns(1)`).
- No TBD/TODO placeholders remain.

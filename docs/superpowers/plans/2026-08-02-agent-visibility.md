# Agent visibility — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show which agent is on a working card, a live mini-thread of their last 3 notes, and a drawer "Files touched" list filled by edit hooks for UI-launched Runs.

**Architecture:** Additive `run_files` table + `board run file` CLI; reorder UI spawn to `CreateRun` then `Start` with `BOARD_TASK_ID`/`BOARD_RUN_ID` env; web `/api/board` attaches `recent_agent_notes` for running tasks; Cursor `afterFileEdit` + Claude PostToolUse call the CLI fail-open; Svelte cards/drawer consume the new fields over existing SSE.

**Tech Stack:** Go 1.26, `modernc.org/sqlite`, Svelte 5 + Vite (Bun → embedded `ui/dist`), shell hooks (no jq).

**Spec:** `docs/superpowers/specs/2026-08-02-agent-visibility-design.md`

## Global Constraints

- Additive schema only — new table via `CREATE TABLE IF NOT EXISTS` in `store.go` `schema` const (same pattern as `runs`).
- Keep `db.SetMaxOpenConns(1)`.
- Hooks and `board run file` **fail open** (exit 0) when env/run missing.
- MCP `get_board` / `list_tasks` stay slim — `recent_agent_notes` is **web `/api/board` only**.
- One active run per task (`ErrRunActive`) unchanged.
- After any `internal/web/ui/src/` edit: `cd internal/web/ui && bun run build` and commit regenerated `dist/`.
- Tests: `go test ./...`; `gofmt`; no FE unit runner.
- Verify whether `cursor-agent` fires `afterFileEdit`; document result (Claude hooks still ship).

## File map

| Path | Responsibility |
|------|----------------|
| `internal/store/store.go` | `run_files` DDL; `Task.RecentAgentNotes` |
| `internal/store/run_file.go` | `AddRunFile`, `ListRunFiles` |
| `internal/store/run_file_test.go` | Dedupe, order, survive FinishRun |
| `internal/store/run.go` | `SetRunPID` |
| `internal/store/note.go` | `RecentNotes(taskID, limit, author)` |
| `internal/store/note_test.go` | RecentNotes filter/limit |
| `internal/store/board_enrich.go` | `AttachRecentAgentNotes` |
| `cmd/board/main.go` | `board run file` dispatch |
| `internal/web/web.go` | Spawn order + env; run files HTTP; board enrich |
| `internal/agent/runner.go` | `StartOpts.Env` |
| `internal/agent/runner_test.go` | Env passed through |
| `internal/setup/cursor.go` | Install `afterFileEdit` hook |
| `internal/setup/setup_test.go` | Hook install idempotent |
| `plugin/hooks/post_tool_use.sh` | Optional `board run file` on write tools |
| `internal/web/ui/src/App.svelte` | Badge, mini-thread, files section, SSE |
| `internal/web/ui/dist/**` | Regenerated embed |

---

### Task 1: `run_files` store API

**Files:**
- Modify: `internal/store/store.go` (append `run_files` CREATE to `schema`)
- Create: `internal/store/run_file.go`
- Create: `internal/store/run_file_test.go`
- Modify: `internal/store/run.go` (add `SetRunPID`)

**Interfaces:**
- Consumes: `Store`, `now()`, `emit`, `truncate`, `GetRun`
- Produces:
  ```go
  type RunFile struct {
      RunID       int64  `json:"run_id"`
      Path        string `json:"path"`
      FirstSeenAt string `json:"first_seen_at"`
  }

  // AddRunFile records path for runID. Empty/whitespace path → error.
  // Duplicate (run_id, path) is ignored (no error, no second event).
  // On first insert, emits event kind "run_file" with truncated path (~80).
  func (s *Store) AddRunFile(runID int64, path string) error

  // ListRunFiles returns paths newest-first (first_seen_at DESC, path ASC tiebreak).
  func (s *Store) ListRunFiles(runID int64) ([]RunFile, error)

  // SetRunPID updates runs.pid (after process start).
  func (s *Store) SetRunPID(runID int64, pid int) error
  ```

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestAddRunFileDedupeAndOrder -count=1`  
Expected: FAIL (undefined AddRunFile)

- [ ] **Step 3: Implement**

Append to `schema` const (before closing backtick):

```sql
CREATE TABLE IF NOT EXISTS run_files (
  run_id INTEGER NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  path TEXT NOT NULL,
  first_seen_at TEXT NOT NULL,
  PRIMARY KEY (run_id, path)
);
CREATE INDEX IF NOT EXISTS idx_run_files_run ON run_files(run_id);
```

Implement `AddRunFile` / `ListRunFiles` in `run_file.go` using `INSERT OR IGNORE`, emit `run_file` only when `RowsAffected() > 0`, and `SetRunPID` as `UPDATE runs SET pid = ? WHERE id = ?` (ErrNotFound if 0 rows).

- [ ] **Step 4: Run tests**

Run: `go test ./internal/store/ -run 'TestAddRunFile|TestCreateRun' -count=1`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/store/store.go internal/store/run_file.go internal/store/run_file_test.go internal/store/run.go
git commit -m "$(cat <<'EOF'
feat(store): run_files table and AddRunFile/ListRunFiles

EOF
)"
```

---

### Task 2: `RecentNotes` + web board enrichment

**Files:**
- Modify: `internal/store/store.go` — add `RecentAgentNotes []Note \`json:"recent_agent_notes,omitempty"\`` on `Task`
- Modify: `internal/store/note.go` — `RecentNotes`
- Modify: `internal/store/note_test.go`
- Create: `internal/store/board_enrich.go` — `AttachRecentAgentNotes`
- Modify: `internal/web/web.go` — after `GetBoard`, call enrich (not from MCP)

**Interfaces:**
```go
// RecentNotes: up to limit notes for taskID with author filter;
// SQL ORDER BY id DESC LIMIT n, then reverse to oldest→newest for UI.
func (s *Store) RecentNotes(taskID int64, limit int, author string) ([]Note, error)

// AttachRecentAgentNotes sets RecentAgentNotes (max n) on tasks with a running run.
func (s *Store) AttachRecentAgentNotes(b *Board, n int) error
```

- [ ] **Step 1: Failing tests**

```go
func TestRecentNotesAgentLimit(t *testing.T) {
	st := newTestStore(t)
	tk, _ := st.CreateTask(CreateTaskParams{Title: "t"})
	_, _ = st.AddNote(tk.ID, "h1", "human")
	_, _ = st.AddNote(tk.ID, "a1", "agent")
	_, _ = st.AddNote(tk.ID, "a2", "agent")
	_, _ = st.AddNote(tk.ID, "a3", "agent")
	_, _ = st.AddNote(tk.ID, "a4", "agent")
	notes, err := st.RecentNotes(tk.ID, 3, "agent")
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 3 || notes[0].Body != "a2" || notes[2].Body != "a4" {
		t.Fatalf("%+v", notes)
	}
}

func TestAttachRecentAgentNotesOnlyRunning(t *testing.T) {
	st := newTestStore(t)
	tk, _ := st.CreateTask(CreateTaskParams{Title: "t", Status: "in_progress"})
	_, _ = st.AddNote(tk.ID, "note", "agent")
	b, _ := st.GetBoard(nil)
	_ = st.AttachRecentAgentNotes(b, 3)
	for _, tsk := range b.InProgress {
		if tsk.ID == tk.ID && len(tsk.RecentAgentNotes) != 0 {
			t.Fatal("expected no notes without running run")
		}
	}
	_, _ = st.CreateRun(tk.ID, "cursor", 1)
	b, _ = st.GetBoard(nil)
	_ = st.AttachRecentAgentNotes(b, 3)
	for _, tsk := range b.InProgress {
		if tsk.ID == tk.ID {
			if len(tsk.RecentAgentNotes) != 1 || tsk.RecentAgentNotes[0].Body != "note" {
				t.Fatalf("%+v", tsk.RecentAgentNotes)
			}
			return
		}
	}
	t.Fatal("task missing")
}
```

- [ ] **Step 2: Run — expect FAIL**

`go test ./internal/store/ -run 'TestRecentNotesAgentLimit|TestAttachRecentAgentNotesOnlyRunning' -count=1`

- [ ] **Step 3: Implement** `RecentNotes`, `AttachRecentAgentNotes` (ListRuns nil,"running" → set of task IDs → RecentNotes per match). In web `/api/board`: `_ = st.AttachRecentAgentNotes(b, 3)`.

- [ ] **Step 4:** `go test ./internal/store/ ./internal/web/ -count=1`

- [ ] **Step 5: Commit**

```bash
git commit -m "$(cat <<'EOF'
feat(web): attach recent agent notes on board for active runs

EOF
)"
```

---

### Task 3: CLI `board run file`

**Files:**
- Modify: `cmd/board/main.go`

**Behavior:** `board run file <path>` reads `BOARD_RUN_ID`. Missing/invalid env or AddRunFile error → exit **0** (fail open). Bad argv → usage error.

- [ ] **Step 1: Add dispatch**

```go
case "run":
	return cmdRun(rest, stdout)
```

```go
func cmdRun(args []string, stdout io.Writer) error {
	if len(args) < 1 || args[0] != "file" {
		return fmt.Errorf("usage: board run file <path>")
	}
	return cmdRunFile(args[1:], stdout)
}

func cmdRunFile(args []string, stdout io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: board run file <path>")
	}
	path := strings.Join(args, " ")
	ridStr := os.Getenv("BOARD_RUN_ID")
	if ridStr == "" {
		return nil
	}
	rid, err := strconv.ParseInt(ridStr, 10, 64)
	if err != nil || rid <= 0 {
		return nil
	}
	st, err := openStore()
	if err != nil {
		return nil
	}
	defer st.Close()
	_ = st.AddRunFile(rid, path)
	return nil
}
```

Update usage string in `run()` default help to mention `run`.

- [ ] **Step 2:** `go build -o /tmp/board ./cmd/board && BOARD_RUN_ID= /tmp/board run file foo.go; echo $?` → `0`

- [ ] **Step 3: Commit**

```bash
git commit -m "$(cat <<'EOF'
feat(cli): board run file records path on BOARD_RUN_ID

EOF
)"
```

---

### Task 4: HTTP run files + spawn env (CreateRun before Start)

**Files:**
- Modify: `internal/agent/runner.go` — `StartOpts.Env []string`
- Modify: `internal/agent/runner_test.go` — FakeRunner records `LastEnv`
- Modify: `internal/web/web.go` — reorder spawn; env; `GET|POST /api/runs/{id}/files`
- Modify: `internal/web/web_test.go` as needed

**Critical:** Today Start runs before CreateRun, so `BOARD_RUN_ID` cannot exist at spawn. New order:

1. `CreateRun(taskID, agent, 0)`
2. `env := append(os.Environ(), fmt.Sprintf("BOARD_TASK_ID=%d", taskID), fmt.Sprintf("BOARD_RUN_ID=%d", run.ID))`
3. `runner.Start(StartOpts{Cwd, Prompt, Env: env, OnProgress})`
4. On start error → `FinishRun(..., "failed", ...)` and return
5. `SetRunPID(run.ID, started.PID)` then existing Wait goroutine

`StartOpts.Env`: if non-nil, `cmd.Env = opts.Env`. ClaudeRunner still strips `ANTHROPIC_API_KEY` from whatever env it uses.

Routes:
- `POST /api/runs/{id}/files` `{"path":"..."}` → AddRunFile → `{"ok":true}`
- `GET /api/runs/{id}/files` → `[]RunFile`

- [ ] **Step 1:** Extend `StartOpts` + FakeRunner `LastEnv`; test contains `BOARD_RUN_ID=9`

- [ ] **Step 2:** Rewrite `handleTaskRun` spawn block as above

- [ ] **Step 3:** Register files routes (match existing `/api/tasks/` parsing style)

- [ ] **Step 4:** `go test ./internal/agent/ ./internal/web/ ./internal/store/ -count=1`

- [ ] **Step 5: Commit**

```bash
git commit -m "$(cat <<'EOF'
feat: inject BOARD_* env on UI run spawn; run files HTTP API

EOF
)"
```

---

### Task 5: Cursor + Claude hooks via setup/plugin

**Files:**
- Modify: `internal/setup/cursor.go` (+ helper to upsert `~/.cursor/hooks.json`)
- Modify: `internal/setup/setup_test.go`
- Write script to `~/.cursor/hooks/board-after-file-edit.sh` from setup (mode 0755)
- Modify: `plugin/hooks/post_tool_use.sh` — read stdin once; on Write/Edit/MultiEdit also `board run file`

**Cursor script (fail open):**

```sh
#!/bin/sh
[ -z "$BOARD_RUN_ID" ] && exit 0
input=$(cat)
path=$(printf '%s' "$input" | sed -n 's/.*"file_path"[ ]*:[ ]*"\([^"]*\)".*/\1/p' | head -1)
[ -z "$path" ] && path=$(printf '%s' "$input" | sed -n 's/.*"path"[ ]*:[ ]*"\([^"]*\)".*/\1/p' | head -1)
[ -n "$path" ] && board run file "$path" >/dev/null 2>&1
exit 0
```

hooks.json entry: `"afterFileEdit": [{ "command": "./hooks/board-after-file-edit.sh" }]` under `~/.cursor`, preserve other hooks, idempotent.

Wire `InstallCursorHooks` into `InstallCursorIntegration`.

**Claude plugin:** refactor to `input=$(cat)` before parsing `tool_name`; for write-like tools extract path and call `board run file`.

- [ ] **Step 1:** Setup test — install twice, one afterFileEdit board hook, script executable

- [ ] **Step 2:** Implement

- [ ] **Step 3:** `go test ./internal/setup/ -count=1`

- [ ] **Step 4: Commit**

```bash
git commit -m "$(cat <<'EOF'
feat(setup): Cursor afterFileEdit + Claude path → board run file

EOF
)"
```

- [ ] **Step 5:** Manual verify Cursor UI Run file list; note finding if CLI skips hooks

---

### Task 6: UI — badge, mini-thread, files drawer, SSE

**Files:**
- Modify: `internal/web/ui/src/App.svelte`
- Build: `internal/web/ui/dist/**`

**Behavior:**

1. `agentStatusText(id)` = `` `${agentLabel} · ${waitStatusLabel(id)}` `` when run/starting/pending ask; use on card + drawer header.

2. Card: if `t.recent_agent_notes?.length`, show last 3 as `.agent-thread` list (2-line clamp); else keep `run.message` / "{Agent} is on it…".

3. Drawer: `GET /api/runs/{id}/files` when `latestRun(detail.id)` exists; section **Files touched** (max 40); empty: "No files recorded yet".

4. SSE: label `run_file`; on that kind refresh detail + files if open.

5. CSS for thread + files list.

- [ ] **Step 1:** Edit App.svelte

- [ ] **Step 2:** `cd internal/web/ui && bun install && bun run build`

- [ ] **Step 3:** Smoke with `go run ./cmd/board serve`

- [ ] **Step 4: Commit** src + dist

```bash
git commit -m "$(cat <<'EOF'
feat(ui): agent badge, note mini-thread, files touched drawer

EOF
)"
```

---

### Task 7: Full verification

- [ ] **Step 1:** `test -z "$(gofmt -l .)"`

- [ ] **Step 2:** `go test ./...`

- [ ] **Step 3:** `go vet ./...`

- [ ] **Step 4:** Task #636 note + `move_task` done if complete; include Cursor hook verification result

---

## Spec coverage checklist

| Spec requirement | Task |
|------------------|------|
| Agent name on working cards | 6 |
| Mini-thread last 3 agent notes | 2 + 6 |
| Drawer files touched | 1 + 4 + 6 |
| `run_files` table | 1 |
| `BOARD_*` env on spawn | 4 |
| `board run file` fail open | 3 |
| HTTP files API | 4 |
| Cursor afterFileEdit setup | 5 |
| Claude PostToolUse paths | 5 |
| SSE `run_file` | 1 + 6 |
| No roster strip | 6 (omitted on purpose) |
| MCP board stays slim | 2 |
| Verify cursor-agent hooks | 5 / 7 |

## Self-review notes

- Spawn **must** CreateRun before Start (Task 4) — differs from current Start-then-CreateRun.
- Claude hook must buffer stdin once (Task 5).
- No TBD placeholders in steps.

## Cursor CLI hook verification (2026-08-02)

- `board setup --yes` installs `~/.cursor/hooks.json` → `afterFileEdit` → `./hooks/board-after-file-edit.sh`.
- Hook stdin smoke with `BOARD_RUN_ID` set: records path via `board run file` (pass).
- Live UI Run via `cursor-agent`: editing a file in this session did **not** auto-append to `run_files` unless the hook was invoked manually — treat Cursor CLI file list as **best-effort / IDE-hook path** (matches spec risk). Claude PostToolUse path → `run_files` still ships; Cursor UI Runs still get agent badge + note mini-thread.

## Claude PostToolUse via setup (2026-08-02 follow-up)

- Gap: Task 5 only updated the Claude **plugin** hook; `board setup` installed SessionStart + CLAUDE.md but not PostToolUse, so machines without the plugin enabled never got file-list recording.
- Fix: `InstallClaudeRules` now also writes `~/.claude/hooks/board-post-tool-use.sh` and upserts `hooks.PostToolUse` (matcher `*`) in `~/.claude/settings.json`. Idempotent; preserves other PostToolUse groups.
- Verified: `board setup --yes` after rebuild; settings contain the absolute script path; hook stdin smoke exits 0.

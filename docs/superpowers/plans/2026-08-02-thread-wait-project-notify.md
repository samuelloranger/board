# Thread authors, wait states, project filter/select, ask notify — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Attribute Thread notes to human/agent, show hybrid wait labels (ask_user + waiting_ci), filter the board by `project_paths`, use a project `<select>` on create/edit, and fire Browser Notifications when an agent asks.

**Architecture:** Additive SQLite columns (`notes.author`, `runs.wait`). Web stamps human notes; MCP stamps agent notes and gains `set_run_wait`. UI derives wait labels (ask wins over `wait=ci`), chips filter via existing `?project=`, and Notification API hooks live SSE `question` events.

**Tech Stack:** Go 1.26, `modernc.org/sqlite`, MCP go-sdk, Svelte 5 + Vite (Bun → embedded `ui/dist`).

**Spec:** `docs/superpowers/specs/2026-08-02-thread-wait-project-notify-design.md`

## Global Constraints

- Additive migrations only — never edit the base `schema` string in `store.go`; use `ALTER TABLE` / swallow duplicate-column errors like existing `message` migration.
- Keep `db.SetMaxOpenConns(1)`.
- MCP write responses stay token-small (`ack` / `{id,task_id,wait}`).
- `project_paths` is the only project registry (no new projects table).
- After any `internal/web/ui/src/` edit: `cd internal/web/ui && bun run build` and commit regenerated `dist/`.
- Tests: `go test ./...`; format: `gofmt`; no FE unit runner.
- Do not notify on SSE backlog before `synced`.

## File map

| Path | Responsibility |
|------|----------------|
| `internal/store/store.go` | `Note.Author`; ALTER `notes.author`, `runs.wait` |
| `internal/store/note.go` | `AddNote(taskID, body, author)` |
| `internal/store/note_test.go` | Author stamping + legacy empty |
| `internal/store/task.go` | `taskNotes` SELECT includes `author` |
| `internal/store/run.go` | `Run.Wait`; `SetRunWait`; clear wait in `FinishRun`; all SELECTs |
| `internal/store/run_test.go` | Wait set/clear/finish |
| `internal/mcpserver/server.go` | `add_note` → agent; register `set_run_wait` |
| `internal/mcpserver/server_test.go` | set_run_wait happy + no active run |
| `internal/agent/prompt.go` | Mention `set_run_wait` for CI waits |
| `internal/agent/prompt_test.go` | Assert prompt mentions wait |
| `internal/web/web.go` | Note → human (pass author); runs JSON already via store |
| `internal/web/ui/src/App.svelte` | Thread labels, wait UI, filter chips, select, notify |

---

### Task 1: `notes.author` store + AddNote

**Files:**
- Modify: `internal/store/store.go` (`Note` struct + `Open` ALTER)
- Modify: `internal/store/note.go`
- Modify: `internal/store/task.go` (`taskNotes`)
- Modify: `internal/store/note_test.go`
- Modify: callers of `AddNote` (grep) to pass author — temporarily use `""` or `"agent"` until Task 3/4

**Interfaces:**
- Consumes: existing `Store`, `now()`, `GetTask`
- Produces:
  ```go
  type Note struct {
      ID        int64  `json:"id"`
      TaskID    int64  `json:"task_id"`
      Body      string `json:"body"`
      Author    string `json:"author,omitempty"` // "" | human | agent | system
      CreatedAt string `json:"created_at"`
  }

  // AddNote appends a note. author must be "", "human", "agent", or "system".
  func (s *Store) AddNote(taskID int64, body, author string) (*Note, error)
  ```
  - Reject unknown author strings with `fmt.Errorf("invalid note author %q", author)`.
  - Empty author is allowed (legacy / unlabeled).

- [ ] **Step 1: Write the failing test**

```go
func TestAddNoteAuthor(t *testing.T) {
	st := newTestStore(t)
	tk, _ := st.CreateTask(CreateTaskParams{Title: "t"})
	n, err := st.AddNote(tk.ID, "hello", "human")
	if err != nil {
		t.Fatal(err)
	}
	if n.Author != "human" {
		t.Fatalf("author=%q", n.Author)
	}
	got, _ := st.GetTask(tk.ID)
	if len(got.Notes) != 1 || got.Notes[0].Author != "human" {
		t.Fatalf("GetTask notes: %+v", got.Notes)
	}
	if _, err := st.AddNote(tk.ID, "x", "nope"); err == nil {
		t.Fatal("want invalid author error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestAddNoteAuthor -count=1`  
Expected: FAIL (AddNote signature / Author missing)

- [ ] **Step 3: Implement**

In `Open` after existing runs ALTER:

```go
_, _ = db.Exec(`ALTER TABLE notes ADD COLUMN author TEXT NOT NULL DEFAULT ''`)
_, _ = db.Exec(`ALTER TABLE runs ADD COLUMN wait TEXT NOT NULL DEFAULT ''`) // used in Task 2; harmless here or defer to Task 2
```

Prefer adding **only** `notes.author` in this task; add `runs.wait` in Task 2 to keep review gates clean.

Update `Note` struct; `AddNote` INSERT includes `author`; return Author; `taskNotes` SELECT/Scan author.

Update existing `TestAddNote` and every `AddNote(` call site to the 3-arg form (use `"agent"` for MCP-path tests, `""` or `"human"` as appropriate). Grep: `AddNote(`.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/store/ -count=1`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/store/store.go internal/store/note.go internal/store/note_test.go internal/store/task.go
# plus any call-site fixes required to compile
git commit -m "$(cat <<'EOF'
feat(store): add notes.author for human/agent thread labels

EOF
)"
```

---

### Task 2: `runs.wait` + SetRunWait

**Files:**
- Modify: `internal/store/store.go` (`Open` ALTER `runs.wait` if not done)
- Modify: `internal/store/run.go`
- Modify: `internal/store/run_test.go`

**Interfaces:**
- Consumes: `ActiveRunForTask`, `GetRun`, `FinishRun`
- Produces:
  ```go
  type Run struct {
      // ...existing fields...
      Wait string `json:"wait,omitempty"` // "" | "ci"
  }

  // SetRunWait sets wait on the active running run for taskID.
  // wait must be "" or "ci". Returns the updated run.
  func (s *Store) SetRunWait(taskID int64, wait string) (*Run, error)
  ```
  - No active run → `ErrNotFound`.
  - Invalid wait → `fmt.Errorf("invalid run wait %q", wait)`.
  - Emit `run_progress` detail `"wait:ci"` or `"wait:clear"`.
  - `FinishRun` also sets `wait=''` in the same UPDATE.

- [ ] **Step 1: Write the failing test**

```go
func TestSetRunWait(t *testing.T) {
	st := newTestStore(t)
	tk, _ := st.CreateTask(CreateTaskParams{Title: "t"})
	run, err := st.CreateRun(tk.ID, "cursor", 1)
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.SetRunWait(tk.ID, "ci")
	if err != nil || got.Wait != "ci" {
		t.Fatalf("set ci: %v %+v", err, got)
	}
	got, err = st.SetRunWait(tk.ID, "")
	if err != nil || got.Wait != "" {
		t.Fatalf("clear: %v %+v", err, got)
	}
	if _, err := st.SetRunWait(tk.ID, "nope"); err == nil {
		t.Fatal("want invalid wait")
	}
	_, _ = st.FinishRun(run.ID, "exited", nil, "done")
	finished, _ := st.GetRun(run.ID)
	if finished.Wait != "" {
		t.Fatalf("finish should clear wait, got %q", finished.Wait)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestSetRunWait -count=1`  
Expected: FAIL

- [ ] **Step 3: Implement**

- ALTER `runs.wait`.
- Add `Wait` to `Run`; extend every `SELECT`/`Scan` in `run.go` (GetRun, ListRuns, ActiveRunForTask, CreateRun return path).
- Implement `SetRunWait`.
- `FinishRun` UPDATE: `wait = ''` alongside status/ended_at/exit_code/message.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/store/ -count=1`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/store/store.go internal/store/run.go internal/store/run_test.go
git commit -m "$(cat <<'EOF'
feat(store): runs.wait for agent CI waiting state

EOF
)"
```

---

### Task 3: MCP `add_note` author + `set_run_wait` + prompt

**Files:**
- Modify: `internal/mcpserver/server.go`
- Modify: `internal/mcpserver/server_test.go`
- Modify: `internal/agent/prompt.go`
- Modify: `internal/agent/prompt_test.go`
- Modify: `internal/web/web.go` (note handler → `AddNote(..., "human")`) — include here so binary compiles end-to-end

**Interfaces:**
- Consumes: `AddNote`, `SetRunWait`
- Produces: MCP tool `set_run_wait` with args `task_id int64`, `wait string` (`"ci"` or `""`)

- [ ] **Step 1: Write failing MCP test**

```go
func TestSetRunWaitTool(t *testing.T) {
	// Follow existing server_test pattern: open store, BuildServer, CallTool.
	// Create task + CreateRun, then CallTool set_run_wait wait=ci, assert run.Wait.
	// CallTool with no run → error.
}
```

Mirror structure of `TestHandoffAndResumeTools` / ask_user tests in `server_test.go`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/mcpserver/ -run TestSetRunWaitTool -count=1`  
Expected: FAIL (tool missing)

- [ ] **Step 3: Implement**

```go
// add_note handler:
n, err := st.AddNote(a.ID, a.Body, "agent")

// web note case:
n, err := st.AddNote(id, body.Body, "human")

mcp.AddTool(s, &mcp.Tool{
    Name: "set_run_wait",
    Description: "Mark the active agent run as waiting on CI (wait='ci') or clear waiting (wait=''). Use when blocked on GitHub Actions or similar; clear when you resume work.",
}, func(...) {
    run, err := st.SetRunWait(a.TaskID, a.Wait)
    // return {id: run.ID, task_id: run.TaskID, wait: run.Wait}
})
```

Prompt (`prompt.go`) add under LIVE PROGRESS:

```
- If blocked on CI or an external job, call set_run_wait with wait='ci', then clear it (wait='') when you resume.
```

Assert substring in `prompt_test.go`.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/mcpserver/ ./internal/agent/ ./internal/web/ ./internal/store/ -count=1`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/mcpserver/ internal/agent/ internal/web/web.go
git commit -m "$(cat <<'EOF'
feat: MCP set_run_wait; stamp note authors human vs agent

EOF
)"
```

---

### Task 4: UI — Thread author labels + wait labels

**Files:**
- Modify: `internal/web/ui/src/App.svelte`

**Interfaces:**
- Consumes: `note.author`, `latestRun(id).wait`, `pendingAskFor(id)`, existing `runStatusLabel` / `isWorking`
- Produces: UI-only helpers:
  ```js
  function noteAuthorLabel(author) {
    if (author === "human") return "You";
    if (author === "agent") return "Agent";
    return "Note";
  }
  function waitStatusLabel(id) {
    if (pendingAskFor(id)) return "Waiting on you";
    const r = latestRun(id);
    if (r?.status === "running" && r.wait === "ci") return "Waiting on CI";
    return runStatusLabel(id); // existing
  }
  ```

- [ ] **Step 1: Thread note chrome**

On each `.d-note`, add:

```svelte
<span class="note-author a-{n.author || 'unknown'}">{noteAuthorLabel(n.author)}</span>
```

CSS: small uppercase chip; `a-human` muted/accent, `a-agent` prog blue, `a-unknown` muted.

- [ ] **Step 2: Card + drawer status**

Replace uses of `runStatusLabel(t.id)` / drawer working pill with `waitStatusLabel(id)` where the live status is shown. Keep Starting… when `isStarting`. Precedence already in `waitStatusLabel`.

Style **Waiting on you** with amber; **Waiting on CI** with prog/muted distinct from Working….

- [ ] **Step 3: Build**

Run: `cd internal/web/ui && bun run build`  
Expected: success; `dist/` updated

- [ ] **Step 4: Commit**

```bash
git add internal/web/ui/src/App.svelte internal/web/ui/dist
git commit -m "$(cat <<'EOF'
feat(ui): Thread author labels and hybrid wait status

EOF
)"
```

---

### Task 5: UI — project filter chips + create/edit `<select>`

**Files:**
- Modify: `internal/web/ui/src/App.svelte`

**Interfaces:**
- Consumes: `GET /api/projects/paths`, `GET /api/board?project=`, `GET /api/resume?project=`
- Produces: state `projectFilter` (`"*"` or project name), persisted `localStorage` key `board-project-filter`

- [ ] **Step 1: Filter state + load**

```js
let projectFilter = $state("*"); // "*" = All

function loadProjectFilter() {
  try {
    const v = localStorage.getItem("board-project-filter");
    if (v) projectFilter = v;
  } catch {}
}
function setProjectFilter(v) {
  projectFilter = v;
  try { localStorage.setItem("board-project-filter", v); } catch {}
  load();
}

async function load() {
  const q = projectFilter === "*" ? "*" : projectFilter;
  board = await (await fetch(`/api/board?project=${encodeURIComponent(q)}`)).json();
  const res = await (await fetch(`/api/resume?project=${encodeURIComponent(q)}`)).json();
  handoffs = res.handoffs ?? [];
  await loadAgentState();
}
```

Call `loadProjectFilter()` once at effect start before first `load()`.

Topbar (below brand or in actions area): chips **All** + each `projectPaths` entry (`"_"` → label `global`).

Activity soft-filter: when `projectFilter !== "*"`, hide events whose `task_id` resolves via `findTask` to a different project (events with no task always show).

- [ ] **Step 2: Replace create/edit project inputs with `<select>`**

```svelte
<select bind:value={addProject}>
  <option value="">global</option>
  {#each projectPaths as pp}
    <option value={pp.project === "_" ? "" : pp.project}>
      {pp.project === "_" ? "global" : pp.project}
    </option>
  {/each}
</select>
```

For edit: same options; if `detail.project` is set and not in `projectPaths`, add:

```svelte
<option value={detail.project}>{detail.project} (unmapped)</option>
```

Remove `list="project-options"` free-text inputs and the `<datalist>`. Keep hint when `projectPaths.length === 0`.

`saveEdit` / `createTask` already send `project` string — keep using select value (`""` = global / clear).

- [ ] **Step 3: Build**

Run: `cd internal/web/ui && bun run build`  
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/web/ui/src/App.svelte internal/web/ui/dist
git commit -m "$(cat <<'EOF'
feat(ui): project filter chips and path-backed project select

EOF
)"
```

---

### Task 6: UI — Browser Notification on ask

**Files:**
- Modify: `internal/web/ui/src/App.svelte`

**Interfaces:**
- Consumes: SSE `kind=question` after `synced`; `Notification` API
- Produces: banner + `notifyOnAsk` helpers; `localStorage` key `board-notify-ask-dismissed`

- [ ] **Step 1: Helpers + banner**

```js
let showNotifyBanner = $state(false);

function maybeShowNotifyBanner() {
  if (typeof Notification === "undefined") return;
  if (Notification.permission !== "default") return;
  try {
    if (localStorage.getItem("board-notify-ask-dismissed") === "1") return;
  } catch {}
  showNotifyBanner = true;
}

async function enableAskNotify() {
  showNotifyBanner = false;
  if (typeof Notification === "undefined") return;
  await Notification.requestPermission();
}

function dismissNotifyBanner() {
  showNotifyBanner = false;
  try { localStorage.setItem("board-notify-ask-dismissed", "1"); } catch {}
}

function notifyAsk(taskId, question) {
  if (typeof Notification === "undefined") return;
  if (Notification.permission !== "granted") return;
  if (!document.hidden && detail?.id === taskId) return;
  const n = new Notification("Agent asks", {
    body: String(question || "").slice(0, 80),
    tag: "board-ask-" + taskId,
  });
  n.onclick = () => {
    window.focus();
    openDetail(findTask(taskId) ?? { id: taskId });
    n.close();
  };
}
```

Render a slim banner under the topbar when `showNotifyBanner`.

Call `maybeShowNotifyBanner()` after first load / when permission still default.

- [ ] **Step 2: Wire SSE**

In `es.onmessage`, after `synced` is false:

```js
if (!syncing && ev.kind === "question" && ev.task_id) {
  notifyAsk(ev.task_id, ev.detail);
  if (!detail) openDetail(findTask(ev.task_id) ?? { id: ev.task_id });
}
```

(Keep existing auto-open-when-no-detail behavior.)

Do **not** call `notifyAsk` while `syncing === true`.

- [ ] **Step 3: Build + smoke**

Run: `cd internal/web/ui && bun run build && cd ../../.. && go build -o board ./cmd/board && go test ./...`  
Expected: all PASS

Manual smoke (document in commit body if done): grant permission, hide tab, insert pending question or trigger ask_user → notification appears.

- [ ] **Step 4: Commit**

```bash
git add internal/web/ui/src/App.svelte internal/web/ui/dist
git commit -m "$(cat <<'EOF'
feat(ui): browser notifications when an agent asks

EOF
)"
```

---

### Task 7: Docs touch-up

**Files:**
- Modify: `CLAUDE.md` and/or `README.md` — one short bullet each for `set_run_wait`, note authors, project select/filter, ask notifications
- Update design spec status line to **Design approved — implemented** only after code lands (optional last commit)

- [ ] **Step 1: Edit docs** to match shipped behavior (no speculative APIs).
- [ ] **Step 2: Commit**

```bash
git add CLAUDE.md README.md docs/superpowers/specs/2026-08-02-thread-wait-project-notify-design.md
git commit -m "$(cat <<'EOF'
docs: thread authors, wait states, project filter, ask notify

EOF
)"
```

---

## Spec coverage checklist

| Spec requirement | Task |
|---|---|
| `notes.author` human/agent/system | 1, 3 |
| Legacy empty author unlabeled | 1, 4 |
| `runs.wait` ci / clear / finish clears | 2 |
| MCP `set_run_wait` | 3 |
| Prompt mentions wait | 3 |
| Web note = human | 3 |
| Thread author labels | 4 |
| Wait precedence ask > ci > working | 4 |
| Filter chips + localStorage | 5 |
| Select from `project_paths` + unmapped option | 5 |
| Browser notify + permission banner | 6 |
| No notify on SSE backlog | 6 |
| Docs | 7 |

## Out of scope (do not implement)

Run durability, ntfy, free-text projects, separate projects table, stdout transcript, multi-agent runners.

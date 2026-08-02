# Thread authors, wait states, project filter/select, ask notify

**Status:** Design approved — implemented  
**Date:** 2026-08-02  
**Project:** board  
**Related:** `docs/superpowers/specs/2026-08-02-fe-trigger-agent-ask-back-design.md`  
**Plan:** `docs/superpowers/plans/2026-08-02-thread-wait-project-notify.md`

## Summary

Polish the agent Run loop and project UX in one vertical slice:

1. **Thread authors** — every note has an explicit `author` (`human` | `agent` | `system`).
2. **Waiting states** — hybrid: pending `ask_user` → “Waiting on you”; agent can mark `waiting_ci`; otherwise existing Working/Done/Failed.
3. **Project filter** — topbar chips from `project_paths` (+ All), persisted in `localStorage`.
4. **Notify on ask** — Browser Notification API when a question arrives and the tab isn’t focused on that task.
5. **Project select** — create/edit task project fields are a `<select>` over `project_paths` (+ global). No free text.

## Goals

- Thread reads as a conversation (who wrote what).
- Quiet runs are distinguishable: waiting on human vs waiting on CI vs working.
- Board can be scoped to one mapped project without leaving `project=*`-only UX.
- Humans notice `ask_user` without staring at the tab.
- Task project assignment only uses pre-mapped projects (`project_paths`).

## Non-goals

- Run durability across `board serve` restarts.
- ntfy / server push / multi-agent runners / stdout transcript panel.
- Human→agent “nudge” beyond posting a human-authored note.
- Free-text project names outside `project_paths`.
- Separate `projects` registry table.
- Auto-moving tasks to `done` on agent exit (unchanged).

## Decisions (locked)

| Topic | Choice |
|---|---|
| Note authorship | Explicit `notes.author`: `human` \| `agent` \| `system` |
| Waiting states | Hybrid: infer ask_user; agent sets/clears `waiting_ci` |
| Notify channel | Browser Notification API only |
| Project source of truth | `project_paths` only |
| Ship shape | One vertical slice |

## Data model

Additive only (existing ALTER / CREATE IF NOT EXISTS patterns).

### `notes.author`

```sql
ALTER TABLE notes ADD COLUMN author TEXT NOT NULL DEFAULT '';
-- Application CHECK: '' | 'human' | 'agent' | 'system'
-- '' = legacy / unlabeled in UI (do not backfill as agent)
```

- Web `POST /api/tasks/{id}/note` → always `author=human`.
- MCP `add_note` → always `author=agent`.
- `system` reserved (unused in v1 inserts; allowed for future).
- `GetTask` / list responses include `author` when set.

### Run wait flag

Prefer a dedicated column on `runs` (clear semantics, no overloading `status`):

```sql
ALTER TABLE runs ADD COLUMN wait TEXT NOT NULL DEFAULT '';
-- '' | 'ci'  (extensible later: 'external', …)
```

- Run `status` stays `running|exited|failed|killed`.
- While `status=running` and `wait='ci'` → UI label **Waiting on CI**.
- Pending question for the task **wins** over `wait` → **Waiting on you**.
- `FinishRun` clears `wait` to `''`.
- Answering an ask does not auto-clear `waiting_ci` (agent must clear or finish).

### Unchanged

- `project_paths` schema as today.
- `questions` table as today.

## MCP / API

### MCP

- `add_note` — sets `author=agent` (no client override in v1). Description unchanged aside from noting agent attribution.
- **`set_run_wait`** (new, token-small ack):
  - Args: `task_id` (required), `wait` (`ci` | `""` to clear).
  - Finds latest **running** run for that task; errors if none.
  - Sets `runs.wait`, emits `run_progress` (or `run_wait`) event with short detail.
  - Returns `{id: run_id, task_id, wait}`.

Prompt tweak (provenance / LIVE PROGRESS): if blocked on CI or an external job, call `set_run_wait` with `ci`, then clear when resuming work.

### Web HTTP

- `POST /api/tasks/{id}/note` — body `{body}`; store sets `author=human`.
- `GET` task / board payloads — notes include `author` when non-empty.
- `GET /api/runs` — include `wait`.
- No new HTTP for set_run_wait in v1 (agent-only via MCP). Optional later for UI.

## UI

### Thread

- Section title remains **Thread**.
- Each note shows a small author label: **You** (`human`), **Agent** (`agent`), muted **Note** when `author` empty.
- Ask card stays pinned above the composer; composer stays above the list.
- Human composer → existing note endpoint (human author).

### Wait labels (card + drawer header)

Precedence while a run is relevant:

1. Pending ask for task → **Waiting on you** (amber).
2. Else `run.status=running` and `run.wait=ci` → **Waiting on CI**.
3. Else existing Starting / Working / Done / Failed / Cancelled.

### Project filter

- Topbar chips: **All** + one chip per `project_paths` entry (show `"_"` as **global**).
- Selection in `localStorage` key `board-project-filter`.
- Default **All** → `/api/board?project=*` and `/api/resume?project=*` (current behavior).
- Named chip → `?project=<name>`.
- Handoff lane uses the same resume query.
- Activity feed: soft client filter by `task.project` when a named chip is selected; **All** shows everything.

### Project select (create + edit)

- `<select>`: option **global** (empty project) + each `project_paths` project name.
- No free-text input.
- If `project_paths` is empty: only **global**, plus hint to open the paths panel.
- Existing task whose `project` is not in `project_paths`: select still shows that value as a disabled/extra option until remapped or cleared, so edit doesn’t silently wipe it.

### Notify on ask

- If `Notification.permission === "default"`, show a dismissible banner once per browser profile (track dismiss in `localStorage`): “Notify me when an agent asks” → `requestPermission()`.
- On live SSE `kind=question` (after synced, not backlog replay):
  - If permission `granted` and (`document.hidden` OR open drawer is not that `task_id`) →
    `new Notification("Agent asks", { body: truncate(question, ~80), tag: "board-ask-" + taskId })`.
  - Notification click → `focus()` + `openDetail` / `?task=`.
- Permission `denied`: no banner spam; Thread + asks badge remain the UX.
- Do not notify for historical backlog during SSE sync.

## Architecture notes

- Fits existing three-surface binary; no new daemon.
- Authors and wait are store concerns; UI remains a consumer of SSE + REST.
- `project_paths` already exists for Run cwd; reusing it avoids a second registry.

```
add_note (MCP)  → notes.author=agent → SSE note → Thread “Agent”
POST /note (UI) → notes.author=human → SSE note → Thread “You”
ask_user        → questions pending  → UI “Waiting on you” + optional Notification
set_run_wait ci → runs.wait=ci       → UI “Waiting on CI”
```

## Testing

- Store: AddNote with author; default/empty author on old rows; SetRunWait / clear on FinishRun.
- MCP: add_note returns ack; set_run_wait requires running run; clear works.
- Web: note endpoint stamps human; runs JSON includes wait.
- UI: manual smoke — filter chips, select options, ask notification with tab hidden (document in plan).

## Ship order

1. Store migrations + note author + run wait helpers/tests.
2. MCP `add_note` author + `set_run_wait` + prompt line; web note author + runs `wait` in JSON.
3. Thread author labels + wait label precedence on card/drawer.
4. Project `<select>` + filter chips + localStorage.
5. Notification banner + SSE hook (skip backlog).
6. `bun run build`, commit `dist/`, restart serve as usual.

## Open questions

None — all forks resolved in brainstorming.

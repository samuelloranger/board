# FE trigger agent + ask-back

**Status:** Design approved  
**Date:** 2026-08-02  
**Project:** board  
**Related:** Original product design: `~/sites/docs/superpowers/specs/2026-07-04-board-kanban-mcp-design.md`

## Summary

Let a human start an agent on a task from the board web UI. The agent knows the
run came from board, works in the task’s project directory, and can ask the
human questions mid-run via a blocking MCP tool that surfaces in the UI.

v1 ships **Cursor Agent CLI only** (`cursor-agent`). Claude Code and Codex plug
into the same seam later. Bridge between `board serve` (UI + spawn) and
`board mcp` (`ask_user`) is the existing shared SQLite DB + SSE events — no new
daemon, no MCP-over-HTTP.

## Goals

- From the web UI, **Run** a task with Cursor (agent picker reserved for later).
- Spawned agent receives a fixed **board provenance** preamble plus task context.
- Agent asks the human via MCP **`ask_user`**; question appears in the UI; agent
  blocks until answered (or cancelled/timeout).
- Remember **project name → absolute path** in SQLite, managed only through the
  UI (prompt on first Run for that project; editable later). No hand-edited
  JSON config.
- Fit existing architecture: one binary, mcp + serve share `~/.board/board.db`,
  additive migrations, token-small MCP defaults, loopback-only serve.

## Non-goals (v1)

- Spawning Claude Code or Codex (same API shape later; argv differs).
- Live agent transcript / streaming stdout in the UI.
- Soft dispatch only (mark + wait for next session) — v1 actually spawns.
- In-process MCP-over-HTTP on serve.
- Background / cloud agent APIs.
- Multiple concurrent runs on the same task.
- Auto-moving a task to `done` when the agent exits.
- Auth or non-loopback exposure of Run (same threat model as today).

## Architecture

Two processes, one DB:

```
FE "Run" → POST /api/tasks/{id}/run
         → serve resolves project path (or 409 need_path → UI saves path)
         → serve creates runs row, moves task → in_progress, emits run event
         → serve spawns cursor-agent in that cwd with board preamble + task

Agent needs input → MCP ask_user(task_id, question)
         → insert questions row (pending)
         → event kind=question → SSE → FE modal
         → POST /api/questions/{id}/answer
         → ask_user poll loop sees answer, returns {answer}

Agent exits → serve marks run exited/failed, cancels unanswered questions
```

- **`board serve`**: UI, Run/Cancel APIs, project-path CRUD, process spawn/reap
  (goroutine `Wait`s the child and updates the `runs` row on exit).
- **`board mcp`**: existing tools + `ask_user`. Cursor already gets board via
  stdio MCP from setup; the spawned `cursor-agent` uses that same config.
- **Bridge**: SQLite rows + `/api/events` SSE. `ask_user` polls the DB (hundreds
  of ms latency is fine).
- **Null-project tasks**: Run still needs a cwd. UI requires the user to set an
  absolute path for that run (stored under project key `"_"` / global, or a
  one-shot path on the Run request). Prefer prompting like a named project.

### Provenance prompt

Every spawn prompt starts with a fixed header stating:

1. This run was launched from the **board web UI** (not an interactive IDE chat).
2. Task id, title, status, description, recent notes.
3. Use board MCP to update the task; for human questions call **`ask_user`**
   with this task id — do not rely on terminal stdin prompts.

## Data model

Additive migrations only (`ALTER` / `CREATE TABLE IF NOT EXISTS` in `Open`).

### `project_paths`

| Column | Type | Notes |
|--------|------|--------|
| `project` | TEXT PK | Existing project name string (git folder name) |
| `path` | TEXT NOT NULL | Absolute directory |
| `updated_at` | TEXT | RFC3339 |

Managed only via UI/API.

### `questions`

| Column | Type | Notes |
|--------|------|--------|
| `id` | INTEGER PK | |
| `task_id` | INTEGER NOT NULL | FK → tasks ON DELETE CASCADE |
| `question` | TEXT NOT NULL | |
| `answer` | TEXT NULL | null while pending |
| `status` | TEXT | `pending` \| `answered` \| `cancelled` |
| `created_at` | TEXT | |
| `answered_at` | TEXT NULL | |

### `runs`

| Column | Type | Notes |
|--------|------|--------|
| `id` | INTEGER PK | |
| `task_id` | INTEGER NOT NULL | |
| `agent` | TEXT NOT NULL | v1 always `cursor`; later `claude` / `codex` |
| `pid` | INTEGER NULL | |
| `status` | TEXT | `running` \| `exited` \| `failed` \| `killed` |
| `started_at` | TEXT | |
| `ended_at` | TEXT NULL | |
| `exit_code` | INTEGER NULL | |

**Invariants**

- At most one `running` run per task; second Run rejected until the first ends.
- On run end (exit / fail / kill): pending questions for that task → `cancelled`.
- Task is moved to `in_progress` on Run if not already; exit does **not** auto-done.

### Events

Reuse `events` with kinds: `run`, `run_done`, `question`, `answered`. Detail is
short; full bodies stay in `questions` / `runs`.

## HTTP API (`board serve`)

| Method | Path | Behavior |
|--------|------|----------|
| `POST` | `/api/tasks/{id}/run` | Body `{ "agent": "cursor" }`. Resolve path; if missing → **409** `{ "need_path": true, "project": "…" }`. Reject if run already `running`. Create run, move in_progress, spawn. Return `{ run_id, task_id, agent, status: "running" }`. |
| `POST` | `/api/tasks/{id}/run/cancel` | Kill process; run → `killed`; cancel pending questions. |
| `GET` | `/api/runs?task_id=` | Optional listing for UI (hydrate running chip on reload). |
| `PUT` | `/api/projects/{name}/path` | Body `{ "path": "/abs/..." }`. Validate directory exists; upsert `project_paths`. |
| `GET` | `/api/projects/paths` | List remembered mappings. |
| `GET` | `/api/questions?status=pending` | Hydrate open ask-back modals on UI load (SSE alone is not enough after refresh). Optional `task_id` filter. |
| `POST` | `/api/questions/{id}/answer` | Body `{ "answer": "…" }`. Only if `pending`. |

Injectable process runner in tests (no real `cursor-agent` in CI).

## MCP

### `ask_user`

- Args: `task_id` (required in v1), `question` (required).
- Inserts pending `questions` row, emits `question` event, polls until
  `answered` / `cancelled` or timeout (~30 minutes).
- Returns `{ answer }` on success; error on cancel/timeout.
- Response stays token-small (no echoing the question back at length).

Existing tools unchanged. Default list/board payloads stay slim.

## UI

**Task detail drawer**

- **Run** button (Cursor in v1; agent picker later).
- Missing path → inline “Where is project *X* on disk?” → Save & Run
  (`PUT` path then `POST` run).
- While `running`: status chip + **Cancel** (Run hidden).
- Pending question (SSE + `GET /api/questions?status=pending` on load):
  modal/panel — question, answer textarea, Submit. Card badge until answered.
- Exit: activity feed only (`run_done` / failure). No live transcript in v1.

**Card menu**: optional Run shortcut (same path as detail).

**Projects**: small list of name→path (edit/clear) using the same path API.

## Errors & lifecycle

| Case | Behavior |
|------|----------|
| `cursor-agent` not on PATH | 400 with clear message; no half-created run |
| Path missing | 409 `need_path` |
| Path invalid after save | 400 |
| Non-zero / crash | run `failed`; pending questions `cancelled`; task stays `in_progress` |
| Cancel / ask timeout | question `cancelled`; `ask_user` errors |
| Serve restart mid-run | On startup, reconcile: dead pid → `failed`; answers still work if MCP process still polling |

Spawning an agent is local-user power; keep serve on loopback; document in README.

## Testing

- **Store**: project_paths upsert; questions answer/cancel; runs create/finish;
  one-running-per-task guard; startup reconcile.
- **MCP**: `ask_user` happy path (insert → answer via store → return);
  cancel/timeout.
- **Web**: Run 409 need_path; Run with fake runner; answer endpoint; cancel
  kills stub process.
- **FE**: no unit runner in repo — manual smoke (Run → ask_user → answer).

## Follow-ups (post-v1)

- Agent picker: Claude Code CLI, Codex CLI (same Run API, different argv).
- Infer `task_id` for `ask_user` from “current run” so agents omit it.
- Optional live stdout panel.
- Soft-dispatch mode for agents that cannot be spawned.

## Success criteria

1. From the web UI, Run on a task with a remembered (or freshly entered) path
   starts `cursor-agent` in that directory with board provenance in the prompt.
2. While running, `ask_user` from the agent opens a UI prompt; submitting an
   answer unblocks the tool with that text.
3. Cancel stops the process and cancels pending questions.
4. Claude/Codex are not required for v1 to ship.

# Agent visibility — who is working, progress, files touched

**Status:** Shipped on main (2026-08-02). Cursor CLI `afterFileEdit` best-effort — see plan verification note. Claude PostToolUse also installed by `board setup` into `~/.claude/settings.json` (not plugin-only).  
**Date:** 2026-08-02  
**Project:** board  
**Task:** #636  
**Related:**  
- `docs/superpowers/specs/2026-08-02-fe-trigger-agent-ask-back-design.md`  
- `docs/superpowers/specs/2026-08-02-thread-wait-project-notify-design.md`

## Summary

When an agent is working on a task, the board should make it obvious **which
agent** is on it, show **more of their live work** on the card, and in the
drawer list **which files they edited**.

v1 builds on the existing `runs` + notes + SSE path: stronger agent badges and
a mini-thread of recent agent notes on cards; a new `run_files` table filled by
Cursor `afterFileEdit` and Claude PostToolUse hooks when the agent was spawned
from the UI (env carries `BOARD_TASK_ID` / `BOARD_RUN_ID`). No separate “active
agents” roster — scan the columns.

## Goals

1. On every working card: show which agent (Cursor / Claude / Codex) is on it.
2. On working cards: show a mini-thread of the last **3** agent-authored notes
   (live via existing SSE `note` refresh).
3. In the task drawer: show a deduped **Files touched** list for the
   active/latest UI Run.
4. Wire Cursor **`afterFileEdit`** and Claude **PostToolUse** (when a file path
   is present) to append paths to that run via spawn-time env.

## Non-goals (v1)

- Separate topbar/sidebar “active agents” roster (scan-the-columns instead).
- File lists for MCP-only or IDE sessions that never got a UI Run (no env).
- Codex file hooks; Cursor `beforeFileEdit` (does not exist for agent edits —
  use `afterFileEdit`).
- Full stdout transcript panel / replacing notes with a `run_events` log.
- Naming the specific agent in note author labels (keep You / Agent).
- Multi-run concurrent file attribution beyond one active run per task
  (already enforced by `ErrRunActive`).

## Decisions (locked)

| Topic | Choice |
|---|---|
| Board-wide who’s-working | Stronger badges on cards; no roster strip |
| Richer card progress | Mini-thread of last 3 `author=agent` notes |
| Progress fallback | Keep `run.message` / “{Agent} is on it…” when no agent notes yet |
| Files source | Cursor `afterFileEdit` + Claude PostToolUse path extract |
| Hook → task association | UI Run only: `BOARD_TASK_ID` + `BOARD_RUN_ID` in child env |
| File storage | `run_files` table (UNIQUE run_id+path), not JSON blob / event scrape |
| Approach | UI polish + light run fields (not event-feed-only, not full run_events) |

## Architecture

```
UI "Run" → CreateRun → spawn agent with BOARD_TASK_ID + BOARD_RUN_ID
                     → card shows agent badge + notes mini-thread

Agent edits file → Cursor afterFileEdit / Claude PostToolUse
                 → board run file <path>   (reads BOARD_RUN_ID; exit 0 always)
                 → AddRunFile → emit run_file → SSE → drawer list updates

MCP add_note     → notes (author=agent) → SSE note → card mini-thread
```

Two processes, one DB (unchanged): `board serve` owns spawn + HTTP/SSE;
hooks shell out to `board` on PATH. Fail open — missing env or binary never
blocks the agent.

## Data model

Additive only (existing ALTER / CREATE IF NOT EXISTS patterns).

### `run_files`

```sql
CREATE TABLE IF NOT EXISTS run_files (
  run_id INTEGER NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  path TEXT NOT NULL,
  first_seen_at TEXT NOT NULL,
  PRIMARY KEY (run_id, path)
);
CREATE INDEX IF NOT EXISTS idx_run_files_run ON run_files(run_id);
```

- `AddRunFile(runID, path)` — normalize (trim); reject empty; INSERT OR IGNORE;
  on real insert emit `run_file` with truncated path (~80 chars).
- `ListRunFiles(runID)` — order by `first_seen_at DESC` (newest first).
- Soft display cap in UI ~40 paths; store may keep all for a run.
- `FinishRun` does **not** delete files — drawer can still show files for the
  latest (exited) run.

### Unchanged

- `runs` columns (`agent`, `message`, `wait`, status…).
- `notes.author` human/agent labeling.
- One active run per task.
- MCP `get_board` / `list_tasks` stay slim (no notes) — card mini-thread is a
  **web UI** concern only.

### Card mini-thread data (explicit)

Today `/api/board` returns tasks without notes; the drawer loads notes via
`GET /api/tasks/{id}`. For the card mini-thread, v1 does:

- Add store helper `RecentNotes(taskID, limit, authorFilter)` (author=`agent`,
  newest ids, return ascending for display).
- Web `/api/board` attaches `recent_agent_notes` (max 3: `id`, `body`, `author`,
  `created_at`) on tasks that currently have a **running** run (join via
  `ActiveRunForTask` / batch query — avoid N+1).
- On SSE `note` for a task, existing `load()` refresh is enough to update cards.
- Do **not** attach full note history on the board payload.

## Spawn / env

When `board serve` creates a UI Run and starts the agent process, pass at least:

- `BOARD_TASK_ID=<task id>`
- `BOARD_RUN_ID=<run id>`

Preserve existing env handling (e.g. Claude strips `ANTHROPIC_API_KEY`).
Unit-test that the child env includes these keys.

## Ingress

### CLI

`board run file <path>`

- Requires `BOARD_RUN_ID` in the environment (or optional `--run-id` for tests).
- Calls `AddRunFile`; prints nothing useful on success (hooks are silent).
- Missing env / unknown run / empty path → exit 0 (fail open) unless a future
  `--strict` flag is added for tests.

### HTTP (optional, same semantics)

`POST /api/runs/{id}/files` body `{ "path": "..." }` — loopback only; useful for
tests and non-shell hooks. Returns ack `{ok:true}` or 404.

### Events

- New kind `run_file` on the activity feed / SSE (alongside `run_progress`).
- UI: on `run_file` / `note` / `run_progress`, refresh agent state; if detail
  open for that task, refresh detail (and thus files list).

## Hooks & setup

### Cursor

`board setup` installs an `afterFileEdit` hook (user-level via setup, matching
how other Cursor setup pieces are installed) that:

1. Parses edited path from stdin JSON.
2. If `BOARD_RUN_ID` is set, runs `board run file "$path"`.
3. Always exits 0; never fail-closed.

IDE / MCP sessions without a UI Run simply no-op (no env).

### Claude Code

Extend `plugin/hooks/post_tool_use.sh` (and any mirrored setup install):

- Keep existing tool-name → `board event tool` behavior.
- When stdin JSON looks like a write/edit and a file path is extractable,
  also call `board run file`.
- Still no `jq` — sed/grep; always `exit 0`.

### Codex

No file hook in v1. Cards still show the Codex badge + notes mini-thread.

## UI

### Card

- Status line: agent label + wait/work label, e.g. `Cursor · Working…`,
  `Claude · Waiting on CI`, `Cursor · Waiting on you`.
- Mini-thread: last **3** notes with `author=agent`, newest at bottom; ~2-line
  clamp each.
- If no agent notes yet: keep today’s `run.message` or muted
  “{Agent} is on it…”.
- No roster strip.

### Drawer

- Sticky header status uses the same agent · state badge.
- New **Files touched** section when a latest run exists: monospace paths,
  newest first; empty copy: “No files recorded yet”.
- Full notes thread unchanged (You / Agent authors).

## Error handling

- Hooks and `board run file` fail open (exit 0 / no throw into the agent).
- Duplicate paths are ignored (PRIMARY KEY).
- Orphan / missing run id → no-op.
- Soft UI cap if a run touches many files; do not crash the drawer.

## Testing

- Store: AddRunFile dedupe; ListRunFiles newest-first; files survive FinishRun.
- CLI: with/without `BOARD_RUN_ID`; empty path.
- Spawn: child env includes `BOARD_TASK_ID` and `BOARD_RUN_ID`.
- Hook scripts: fixture stdin smoke (shell).
- Manual UI: Run → agent notes appear on card; edit file → drawer list updates
  via SSE. (No frontend test runner in this repo.)

## Risks & verification

- **Does `cursor-agent` (CLI) invoke Cursor `afterFileEdit` hooks?** Setup installs
  the hook for the IDE/agent hook system; headless `cursor-agent -p` may not
  fire it. **Verify during implementation.** If it does not:
  - Claude PostToolUse path → `run_files` still ships.
  - Cursor UI Runs still get agent badge + notes mini-thread (agents already
    note paths in `add_note` per provenance prompt).
  - Document Cursor file list as best-effort / IDE-hook path; follow-up if CLI
    needs another signal (out of v1 unless a one-line fix exists).
- Claude tool JSON shapes vary by tool name — path extraction is best-effort
  sed; missed paths are acceptable (fail open).

## Ship shape

One vertical slice:

1. `run_files` + store APIs + tests  
2. `board run file` (+ optional HTTP `GET/POST` files)  
3. Spawn env injection  
4. Cursor setup hook + Claude plugin hook update  
5. Web `/api/board` `recent_agent_notes` for running tasks  
6. UI card badge + mini-thread + drawer files + SSE `run_file`  
7. `bun run build` and commit regenerated `internal/web/ui/dist/`
8. Verify Cursor CLI hook behavior; record result in the plan/PR

## Out of scope follow-ups (explicit)

- Active-agents roster strip.
- Marker-file association for IDE hooks without child env.
- Codex hooks; richer Claude path parsing if tool JSON shapes differ.
- Agent-specific note author labels (`Cursor` vs generic `Agent`).
- `run_events` unified activity stream.

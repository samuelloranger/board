# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`board` is a cross-client kanban board for AI agents: one central board on your machine, shared by
every MCP-capable client (Claude Code, Codex CLI, Cursor, Antigravity). It is a coordination bus, not
a private todo list — `handoff` parks a task for another agent with context, and `resume` restores a
session's working context in one call, so a Codex session can pick up what a Claude Code session
started. Public repo, MIT, `github.com/samuelloranger/board`.

**Three surfaces, one binary.** `cmd/board` dispatches argv into three transports over the same
`*store.Store`: `board mcp` (stdio MCP server, `internal/mcpserver`), the terminal subcommands
(`add`/`list`/`board`/`move`/`archive`/`note`/`event`, implemented inline in `main.go`), and
`board serve` (an HTTP JSON API + embedded Svelte UI, `internal/web`). No daemon, no auth, no network
dependency — the store is the only shared state, and all three can run at once against the same DB.

## Layout

- `cmd/board/` — `main.go`: argv dispatch + every CLI subcommand + `runMCP`/`runServe`/`runSetup`.
- `internal/store/` — SQLite data layer. `store.go` (schema/Open), `task.go`, `note.go`, `board.go`,
  `handoff.go`, `event.go` (activity feed), `project.go` (git-root project detection),
  `project_path.go` / `question.go` / `run.go` (FE agent Run + ask_user bridge).
- `internal/agent/` — provenance prompt + `CursorRunner` for spawning `cursor-agent`.
- `internal/mcpserver/` — `BuildServer(st, defaultProject)`, the MCP tool surface.
- `internal/web/` — `web.go` HTTP handlers + `//go:embed all:ui/dist`; `ui/` is the Svelte 5 + Vite app.
- `internal/setup/` — `board setup`: writes MCP server config into each client's config file,
  installs Claude Code auto-update rules, and installs Cursor skill/rules + Agent CLI allowlist.
- `plugin/` — a Claude Code plugin (`.claude-plugin/plugin.json`, `skills/board/SKILL.md`,
  `hooks/{post_tool_use,stop}.sh`) that bundles the MCP config, usage skill, and activity hooks.
- `install.sh` / `install_test.sh` — installer and its dry-run smoke test. `docs/` is just the README screenshot.

## Commands

Go 1.26.4, module `github.com/samuelloranger/board`. There is no Makefile — these are the real commands
(they mirror `.github/workflows/ci.yml`).

```bash
go build ./cmd/board            # binary -> ./board (gitignored)
go test ./...                   # all tests; every internal package has _test.go files
go test ./internal/store        # one package
go vet ./...
test -z "$(gofmt -l .)"         # CI fails on any unformatted file

./board mcp                     # MCP server on stdio (what AI clients spawn)
./board serve                   # web UI + JSON API on 127.0.0.1:7420
./board serve --addr 0.0.0.0:9000
./board board                   # CLI: the three columns for the current project
./board setup [--yes]           # register the MCP server across detected clients
```

Web UI (Bun, from `internal/web/ui/`):

```bash
bun install
bun run build                   # vite build -> internal/web/ui/dist (the embedded assets)
```

There is no `vite dev` script and no frontend test runner — `bun run build` then `go run ./cmd/board serve`.

## MCP tool surface

`create_task`, `list_tasks`, `get_task`, `update_task`, `move_task`, `archive_task`, `unarchive_task`,
`delete_task`, `add_note`, `get_board`, `handoff`, `resume`, `ask_user`, `set_run_wait`.

**Token-small responses are a core project value, not an optimization.** The caller is an agent whose
context is the scarce resource, so `internal/mcpserver/server.go` deliberately withholds data:

- `ack()` — write tools return only `{id, title, status}`. Echoing back descriptions and note bodies
  was the single largest source of wasted tokens in agent transcripts.
- `slim()` — `get_board` / `list_tasks` return `id, title, status` plus `priority`/`due_date`/
  `handoff_to`/`project` only when set. Never descriptions, notes, timestamps, or tags.
- Caps: `doneLimit = 10` (newest done tasks in `get_board`), `listLimit = 50` (`list_tasks`). When they
  truncate they say so with the real total (`done_note`, `truncated`) instead of lying by omission.
- Escape hatches: `verbose: true` for full tasks, `limit` to raise the list cap, `get_task` for one
  task in full.
- `ask_user` returns only `{answer}` after the human replies in the web UI (SQLite bridge with
  `board serve`; browser can notify when the tab isn’t on that task).
- `set_run_wait` marks the active run `wait=ci` (or clears it) so the UI shows **Waiting on CI**.
- Notes carry `author` (`human` from the web UI, `agent` from MCP). Project create/edit is a
  `<select>` over `project_paths`; topbar chips filter the board.

When adding a tool, keep the default response minimal and put the full payload behind `verbose`.

## Data model

SQLite via CGO-free `modernc.org/sqlite`, DB at `~/.board/board.db` (override with `BOARD_DB`).
Schema is one `CREATE TABLE IF NOT EXISTS` const in `store.go`:

- `tasks` — id, title, description, `status CHECK(todo|in_progress|done)`, project, `priority CHECK(low|medium|high)`,
  due_date, archived, created_at, updated_at, plus `handoff_to` / `handoff_reason` added by ALTER.
- `tags` (task_id, tag) and `notes` (append-only per task), both `ON DELETE CASCADE`.
- `events` — the activity feed: kind, detail, optional task_id. Streamed as SSE from `/api/events`.
- Indexes on `tasks(project)` and `tasks(status)`.

Opened WAL + `foreign_keys(ON)` + `busy_timeout(5000)`, and **`db.SetMaxOpenConns(1)`** — modernc's
driver allows one writer, so the pool is capped at a single connection to serialize writers instead of
racing them into `SQLITE_BUSY`. Don't raise it.

## Conventions & gotchas

- **`internal/web/ui/dist/` is committed on purpose.** `//go:embed all:ui/dist` means `go build` fails
  without it, so a fresh checkout must build with no Bun installed. Consequence: **after editing
  `ui/src/`, run `bun run build` and commit the regenerated `dist/`** — otherwise the binary silently
  ships the old UI. Hashed asset filenames change on every build, so the diff is add+delete, not a modify.
- **Migrations are additive only.** New columns go in the `for _, col := range []string{...}` ALTER loop
  in `Open` (duplicate-column errors are intentionally swallowed). Never edit the base schema string —
  existing DBs already ran it.
- **Project scope is implicit.** `store.DetectProject` walks up from the process CWD to the nearest
  `.git` and uses that folder's name. So the MCP server's default project is whatever directory the
  client launched it from, and CLI commands are scoped to the repo you're standing in. `nil` = global.
  In MCP tools, an explicit `project: "*"` means *all projects*; omitting it means *current*.
- **A no-op `move_task` must not clear a handoff.** `MoveTask` only nulls `handoff_to`/`handoff_reason`
  when the status actually changes — moving to the status a task already has preserves the context.
- `store.emit` (activity events) is best-effort and swallows errors: logging must never fail a real
  mutation. `LogEvent` (from `board event`) does return errors.
- The web API has **no auth and binds loopback by default**. `--addr 0.0.0.0` exposes an unauthenticated
  read/write board to the LAN. Keep it loopback.
- The plugin hooks shell out to a `board` binary on `PATH` and parse hook JSON with `sed`, not `jq`, to
  avoid the dependency. They exit 0 unconditionally so a missing binary never breaks a Claude session.
- `dist_release/`, `board`, and `*.db*` are gitignored.

## Releasing

Tag-driven: pushing `vX.Y.Z` runs `.github/workflows/release.yml`, which builds four static binaries and
attaches them to the GitHub Release. See the `releasing-board` skill
(`.claude/skills/releasing-board/SKILL.md`) for the full procedure and rollback.

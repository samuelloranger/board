---
name: board
description: Use to track durable work needing a record or later handoff. Create a task when requested or for a concrete follow-up that will outlive this session.
---

# Using the board

You have a kanban board via the `board` MCP server. Use it to persist work across sessions.

## When to use
- User asks to track work, or a concrete independent follow-up will outlive this session → `create_task` (title + short description).
- Beginning tracked work → `move_task` to `in_progress` once.
- Finishing tracked work → `add_note` (outcome + verification), then `move_task` to `done`.
- Meaningful finding or decision on a tracked task → `add_note`.
- Board state needed to choose work → `get_board` (current project) or `list_tasks`.
- **Resuming tracked work** → call `resume` to restore in-progress work and handoffs.
- **Blocked / needs another agent or a human** → `handoff(id, to, reason)` with
  `to` = the receiver (`human`, `codex`, …) and a clear reason. Don't just
  leave the task idle.
- **Need a quick human answer mid-run (especially if launched from the board UI)** →
  `ask_user(task_id, question)` and wait for the reply in the web UI.

## Rules
- Do not create tasks for one-off questions, small edits, routine investigation steps, or incidental findings. Put those in the response or an existing task.
- Keep exactly one task `in_progress` at a time when possible.
- Archive (not delete) completed work you want to keep a record of: `archive_task`.
- File paths in notes are repo-relative only — never absolute or home paths.

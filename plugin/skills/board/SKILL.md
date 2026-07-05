---
name: board
description: Use to track multi-step work on a persistent kanban board. Create a task when starting non-trivial work, move it across todo/in_progress/done as you progress, and record findings as notes.
---

# Using the board

You have a kanban board via the `board` MCP server. Use it to persist work across sessions.

## When to use
- Starting multi-step work → `create_task` (title + short description). It auto-scopes to the current project.
- Beginning a task → `move_task` to `in_progress`.
- Finishing → `move_task` to `done`.
- Learning something worth remembering → `add_note` on the task.
- Reviewing state → `get_board` (current project) or `list_tasks`.

## Rules
- One task per meaningful unit of work. Don't create tasks for trivial one-liners.
- Keep exactly one task `in_progress` at a time when possible.
- Archive (not delete) completed work you want to keep a record of: `archive_task`.

#!/bin/sh
# Claude Code PostToolUse hook: log the tool name to the board activity feed,
# and best-effort record write paths on the active UI Run (BOARD_RUN_ID).
# Hook input arrives as JSON on stdin; parse with sed (no jq).
input=$(cat)
tool=$(printf '%s' "$input" | sed -n 's/.*"tool_name"[ ]*:[ ]*"\([^"]*\)".*/\1/p' | head -1)
[ -n "$tool" ] && board event tool "$tool" >/dev/null 2>&1
case "$tool" in
  Write|Edit|MultiEdit|NotebookEdit)
    path=$(printf '%s' "$input" | sed -n 's/.*"file_path"[ ]*:[ ]*"\([^"]*\)".*/\1/p' | head -1)
    [ -z "$path" ] && path=$(printf '%s' "$input" | sed -n 's/.*"path"[ ]*:[ ]*"\([^"]*\)".*/\1/p' | head -1)
    [ -n "$path" ] && board run file "$path" >/dev/null 2>&1
    ;;
esac
exit 0

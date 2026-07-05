#!/bin/sh
# Claude Code PostToolUse hook: log the tool name to the board activity feed.
# Hook input arrives as JSON on stdin; extract tool_name with a tiny grep/sed
# (avoids a jq dependency).
tool=$(sed -n 's/.*"tool_name"[ ]*:[ ]*"\([^"]*\)".*/\1/p')
[ -n "$tool" ] && board event tool "$tool" >/dev/null 2>&1
exit 0

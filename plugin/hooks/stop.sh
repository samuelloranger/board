#!/bin/sh
# Claude Code Stop hook: mark the session boundary on the board feed.
board event session "ended" >/dev/null 2>&1
exit 0

#!/usr/bin/env bash
set -euo pipefail
# Runs install.sh in dry-run mode and asserts it resolves a target + would call setup.
out=$(BOARD_SKIP_DOWNLOAD=1 BOARD_DRY_RUN=1 sh ./install.sh --yes 2>&1)
echo "$out" | grep -q "target:" || { echo "FAIL: no target resolved"; echo "$out"; exit 1; }
echo "$out" | grep -q "would run: .*board setup --yes" || { echo "FAIL: setup not invoked"; echo "$out"; exit 1; }
echo "PASS"

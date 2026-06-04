#!/usr/bin/env bash
# Run deadcode on bleephub to detect unreachable functions.
# Requires golang.org/x/tools/cmd/deadcode in $PATH or ~/go/bin.
set -euo pipefail

DEADCODE=$(command -v deadcode 2>/dev/null || echo "$HOME/go/bin/deadcode")
if [[ ! -x "$DEADCODE" ]]; then
  echo "bleephub-deadcode: deadcode not found; install with:" >&2
  echo "  go install golang.org/x/tools/cmd/deadcode@latest" >&2
  exit 1
fi

out=$(cd bleephub && GOWORK=off "$DEADCODE" -tags noui ./cmd/ 2>&1)
if [[ -n "$out" ]]; then
  echo "FAIL: bleephub deadcode found unreachable functions:" >&2
  echo "$out" >&2
  exit 1
fi
echo "bleephub-deadcode: OK"

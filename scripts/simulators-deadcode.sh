#!/usr/bin/env bash
# Run deadcode on the cloud simulators to detect unreachable functions.
# Each simulator (aws/gcp/azure) is its own Go module with a `package main`
# entry point at the module root, so deadcode is run per-module with the
# `noui` build tag (so the embedded dist/ build artifact is not required) and
# GOWORK=off (the simulators are excluded from the workspace).
# Requires golang.org/x/tools/cmd/deadcode in $PATH or ~/go/bin.
set -euo pipefail

DEADCODE=$(command -v deadcode 2>/dev/null || echo "$HOME/go/bin/deadcode")
if [[ ! -x "$DEADCODE" ]]; then
  echo "simulators-deadcode: deadcode not found; install with:" >&2
  echo "  go install golang.org/x/tools/cmd/deadcode@latest" >&2
  exit 1
fi

ROOT=$(git rev-parse --show-toplevel)
fail=0
for cloud in aws gcp azure; do
  out=$(cd "$ROOT/simulators/$cloud" && GOWORK=off "$DEADCODE" -tags noui . 2>&1)
  if [[ -n "$out" ]]; then
    echo "FAIL: simulators/$cloud deadcode found unreachable functions:" >&2
    echo "$out" >&2
    fail=1
  else
    echo "simulators-deadcode: $cloud OK"
  fi
done
exit "$fail"

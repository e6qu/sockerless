#!/usr/bin/env bash
# Run dupl (Go copy-paste detector) on bleephub source.
# Threshold: 200 tokens (irreducible structural HTTP-handler clones below this).
#
# File names are fed to dupl via its `-files` stdin interface (one per line).
# This is the documented scripting interface; passing a long list of paths as
# positional arguments makes dupl mis-parse them as a single path
# ("file name too long") and silently scan nothing.
#
# Requires github.com/mibk/dupl in $PATH or ~/go/bin.
set -euo pipefail

DUPL=$(command -v dupl 2>/dev/null || echo "$HOME/go/bin/dupl")
if [[ ! -x "$DUPL" ]]; then
  echo "bleephub-dupl: dupl not found; install with:" >&2
  echo "  go install github.com/mibk/dupl@latest" >&2
  exit 1
fi

ROOT=$(git rev-parse --show-toplevel)
if [[ -z "$(find "$ROOT/bleephub" -maxdepth 1 -name "*.go" ! -name "*_test.go" -type f)" ]]; then
  echo "bleephub-dupl: no Go files found" >&2
  exit 1
fi

out=$(find "$ROOT/bleephub" -maxdepth 1 -name "*.go" ! -name "*_test.go" -type f | sort | "$DUPL" -t 200 -files 2>&1)
count=$(echo "$out" | grep -c "^found" || true)
if [[ "$count" -gt 0 ]]; then
  echo "FAIL: bleephub dupl found $count clone group(s) above threshold (200 tokens):" >&2
  echo "$out" >&2
  exit 1
fi
echo "bleephub-dupl: OK (threshold: 200 tokens)"

#!/usr/bin/env bash
# Lint Go modules that contain changed files.

set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

# Collect unique module directories
modules=()
for f in "$@"; do
  dir=$(dirname "$f")
  while [ "$dir" != "." ] && [ ! -f "$dir/go.mod" ]; do
    dir=$(dirname "$dir")
  done
  if [ -f "$dir/go.mod" ]; then
    modules+=("$dir")
  fi
done

# Deduplicate
sorted=$(printf '%s\n' "${modules[@]}" | sort -u)

failed=0
for mod in $sorted; do
  # Skip modules needing UI build artifacts
  if grep -qr 'all:dist' "$mod"/*.go 2>/dev/null && [ ! -d "$mod/dist" ]; then
    echo "lint: $mod (skipped — dist/ not built)"
    continue
  fi
  echo "lint: $mod"
  if ! (cd "$mod" && GOWORK=off golangci-lint run --timeout 2m ./...); then
    failed=1
  fi
done

exit $failed

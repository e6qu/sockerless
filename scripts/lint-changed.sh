#!/usr/bin/env bash
# Lint only Go modules that contain changed files.
# Called by pre-commit with changed file paths as arguments.

set -euo pipefail

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

for mod in $sorted; do
  echo "lint: $mod"
  (cd "$mod" && golangci-lint run --timeout 2m ./...) || exit 1
done

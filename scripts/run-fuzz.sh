#!/usr/bin/env bash
# run-fuzz.sh — exploratory fuzzing across modules.
#
# Runs every Go fuzz target (func FuzzXxx) found in the given module dirs for a
# fixed time each, using the committed seed corpus plus exploration. Exits
# non-zero if any target finds a crasher (Go writes the failing input under
# <pkg>/testdata/fuzz/<FuzzName>/ and the `go test` invocation fails).
#
# Usage:   scripts/run-fuzz.sh [module-dir ...]
# Env:     FUZZTIME_SECONDS (default 60) — seconds per fuzz target.
#
# bash + zsh portable; shellcheck-clean. The committed seed corpus already runs
# under plain `go test` as a regression net; this script is the discovery pass.
set -u

SECS="${FUZZTIME_SECONDS:-60}"
status=0
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

for dir in "$@"; do
  if [ ! -f "$dir/go.mod" ]; then
    echo "skip $dir (no go.mod)"
    continue
  fi
  # Every *_test.go file that declares at least one fuzz target.
  grep -rl '^func Fuzz' "$dir" --include='*_test.go' >"$tmp" 2>/dev/null || true
  while IFS= read -r f; do
    [ -n "$f" ] || continue
    pkgdir="$(dirname "$f")"
    rel="."
    if [ "$pkgdir" != "$dir" ]; then
      rel="./${pkgdir#"$dir"/}"
    fi
    while IFS= read -r fn; do
      [ -n "$fn" ] || continue
      echo "=== [$dir] $rel $fn (${SECS}s) ==="
      if ! (cd "$dir" && GOWORK=off CGO_ENABLED=0 go test -run='^$' -fuzz="^${fn}\$" -fuzztime="${SECS}s" "$rel"); then
        echo "!!! CRASHER: $dir $rel $fn"
        status=1
      fi
    done <<EOF
$(grep -oE '^func Fuzz[A-Za-z0-9_]+' "$f" | sed 's/^func //')
EOF
  done <"$tmp"
done

if [ "$status" -ne 0 ]; then
  echo "fuzzing found at least one crasher — see the failing input under testdata/fuzz/"
fi
exit "$status"

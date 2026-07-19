#!/usr/bin/env bash
# run-fuzz.sh — exploratory fuzzing across modules.
#
# Runs every Go fuzz target (func FuzzXxx) found in the given module dirs for a
# fixed time each, using the committed seed corpus plus exploration. Exits
# non-zero if any target fails. Go writes newly minimized crash inputs under
# <pkg>/testdata/fuzz/<FuzzName>/; only those untracked inputs are collected.
#
# Usage:   scripts/run-fuzz.sh [module-dir ...]
# Env:     FUZZTIME_SECONDS (default 60) — seconds per fuzz target.
#          FUZZ_PARALLEL (default 4) — workers per fuzz target.
#
# bash + zsh portable; shellcheck-clean. The committed seed corpus already runs
# under plain `go test` as a regression net; this script is the discovery pass.
set -u

SECS="${FUZZTIME_SECONDS:-60}"
PARALLEL="${FUZZ_PARALLEL:-4}"
status=0
tmp="$(mktemp)"
artifact_dir=".fuzz-artifacts"
rm -rf "$artifact_dir"
trap 'rm -f "$tmp"' EXIT

collect_new_crashers() {
  git ls-files --others --exclude-standard -- '*/testdata/fuzz/*/*' | while IFS= read -r crasher; do
    [ -n "$crasher" ] || continue
    destination="$artifact_dir/$crasher"
    mkdir -p "$(dirname "$destination")"
    cp "$crasher" "$destination"
  done
}

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
    # A repository group can contain nested Go modules (every simulator has a
    # separate shared module). The parent module cannot address that package;
    # its own matrix entry runs it independently.
    module_dir="$pkgdir"
    while [ "$module_dir" != "$dir" ] && [ ! -f "$module_dir/go.mod" ]; do
      module_dir="$(dirname "$module_dir")"
    done
    [ "$module_dir" = "$dir" ] || continue
    rel="."
    if [ "$pkgdir" != "$dir" ]; then
      rel="./${pkgdir#"$dir"/}"
    fi
    while IFS= read -r fn; do
      [ -n "$fn" ] || continue
      echo "=== [$dir] $rel $fn (${SECS}s) ==="
      # Simulator fuzzing exercises protocol parsers, not the separately built
      # embedded SPA. The noui tag selects the repository's headless entrypoint.
      if ! (cd "$dir" && GOWORK=off CGO_ENABLED=0 go test -tags=noui -run='^$' -fuzz="^${fn}\$" -fuzztime="${SECS}s" -parallel="$PARALLEL" "$rel"); then
        echo "!!! FUZZ TARGET FAILED: $dir $rel $fn"
        collect_new_crashers
        status=1
      fi
    done <<EOF
$(grep -oE '^func Fuzz[A-Za-z0-9_]+' "$f" | sed 's/^func //')
EOF
  done <"$tmp"
done

if [ "$status" -ne 0 ]; then
  if [ -d "$artifact_dir" ]; then
    echo "fuzzing found at least one new crasher — minimized inputs are in $artifact_dir"
  else
    echo "at least one fuzz target failed without producing a new crasher"
  fi
fi
exit "$status"

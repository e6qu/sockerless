#!/usr/bin/env bash
# Updates all badge values in README.md based on current codebase stats.
# Used as a pre-push hook or run manually.
#
# Mirror remote pushes (any pre-commit `PRE_COMMIT_REMOTE_NAME` other
# than `origin`) are intentional fast-forwards of origin/main; they
# carry whatever badges origin/main already has, so this hook is a
# no-op for them.

set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

remote_name="${PRE_COMMIT_REMOTE_NAME:-origin}"
if [ "$remote_name" != "origin" ]; then
  exit 0
fi

readme="README.md"

# Portable sed -i (macOS vs Linux)
sedi() { if [[ "$OSTYPE" == darwin* ]]; then sed -i '' "$@"; else sed -i "$@"; fi; }

count_tracked_lines() {
  local root=$1
  local kind=$2
  git ls-files -z -- "$root" |
    while IFS= read -r -d '' file; do
      case "$file" in
        */vendor/*|*/node_modules/*|*/dist/*) continue ;;
      esac
      case "$kind" in
        go)
          case "$file" in *.go) ;; *) continue ;; esac
          case "$file" in *_test.go) continue ;; esac
          ;;
        gotest)
          case "$file" in *_test.go) ;; *) continue ;; esac
          ;;
        ts)
          case "$file" in *.ts|*.tsx) ;; *) continue ;; esac
          ;;
        *)
          echo "unknown count kind: $kind" >&2
          return 1
          ;;
      esac
      wc -l <"$file" | tr -d ' '
    done |
    awk '{ s += $1 } END { print s + 0 }'
}

fmt_k() {
  local n=${1:-0}
  if [ "$n" -ge 1000 ]; then
    local k=$((n / 1000))
    local r=$(( (n % 1000) / 100 ))
    if [ "$r" -gt 0 ]; then echo "${k}.${r}k"; else echo "${k}k"; fi
  else
    echo "$n"
  fi
}

# Top-level
go_src=$(count_tracked_lines . go)
go_test=$(count_tracked_lines . gotest)
ts_total=$(count_tracked_lines . ts)
go_modules=$(git ls-files -- 'go.mod' '*/go.mod' | wc -l | tr -d ' ')

sedi "s|Go-[0-9.]*k_lines|Go-$(fmt_k "$go_src")_lines|g" "$readme"
sedi "s|TypeScript-[0-9.]*k_lines|TypeScript-$(fmt_k "${ts_total:-0}")_lines|g" "$readme"
sedi "s|Tests-[0-9.]*k_lines|Tests-$(fmt_k "$go_test")_lines|g" "$readme"
sedi "s|Go_Modules-[0-9]*+-|Go_Modules-${go_modules}-|g" "$readme"

# Per-module Go badges
for pair in \
  "core:backends/core" \
  "bleephub:bleephub" \
  "sim%2Faws:simulators/aws" \
  "sim%2Fazure:simulators/azure" \
  "sim%2Fgcp:simulators/gcp" \
  "admin:admin" \
  "ecs:backends/ecs" \
  "cloudrun:backends/cloudrun" \
  "aca:backends/aca" \
  "docker:backends/docker" \
  "agent:agent" \
  "api:api" \
  "azf:backends/azure-functions" \
  "cli:cmd/sockerless" \
  "gcf:backends/cloudrun-functions" \
  "lambda:backends/lambda" \
; do
  badge="${pair%%:*}"
  dir="${pair#*:}"
  if [ -d "$dir" ]; then
    val=$(fmt_k "$(count_tracked_lines "$dir" go)")
    sedi "s|badge/${badge}-[0-9.k]*-|badge/${badge}-${val}-|g" "$readme"
  fi
done

# Per-module TypeScript badges
for pair in \
  "ui%2Fadmin:ui/packages/admin" \
  "ui%2Fcore:ui/packages/core" \
  "ui%2Fbleephub:ui/packages/bleephub" \
  "ui%2Fsim--aws:ui/packages/sim-aws" \
  "ui%2Fsim--gcp:ui/packages/sim-gcp" \
  "ui%2Fsim--azure:ui/packages/sim-azure" \
  "ui%2Ffrontend--docker:ui/packages/frontend-docker" \
; do
  badge="${pair%%:*}"
  dir="${pair#*:}"
  if [ -d "$dir" ]; then
    val=$(fmt_k "$(count_tracked_lines "$dir" ts)")
    sedi "s|badge/${badge}-[0-9.k]*-|badge/${badge}-${val}-|g" "$readme"
  fi
done

# Stage if changed, and FAIL so the update is committed rather than left
# stranded as an uncommitted staged edit (which then trips the CI badge check).
# When badges are already current this is a clean no-op. The `git add` makes the
# refreshed README ready to commit; the non-zero exit forces you to.
if ! git diff --quiet "$readme" 2>/dev/null; then
  git add "$readme"
  echo "ERROR: README badges were stale; refreshed and staged $readme." >&2
  echo "Commit it (amend it into your change) and run the push/commit again." >&2
  exit 1
fi
echo "badges: up to date"

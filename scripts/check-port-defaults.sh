#!/usr/bin/env bash
# Per BUG-710 — fail if any source/markdown/script/terraform file outside
# excluded fixtures still references the obsolete default ports :2375 or
# :9100. Sockerless's canonical default is :3375 (avoids Docker daemon
# collision). Run from repo root or any subdirectory; resolves relative
# to the script's own location.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# Allow listed fixtures + dist bundles + log captures + the bug record itself.
# Files where the obsolete ports are intentionally referenced (bug history,
# fixtures, this very check). Filtered post-grep since BSD grep's --exclude
# composition with --include is fragile.
ALLOWED_FILES_RE='/(BUGS|PLAN|WHAT_WE_DID|DO_NEXT|STATUS|MEMORY)\.md:|/configfile_test\.go:|/check-port-defaults\.sh:|/_tasks/|/specs/|/ARCHITECTURE\.md:|/raw-output-.*\.log:|/\.pre-commit-config\.yaml:'

INCLUDES=(
  --include=*.go
  --include=*.md
  --include=*.sh
  --include=*.yml
  --include=*.yaml
  --include=*.toml
  --include=*.tf
  --include=*.hcl
)

found=$(grep -REn --exclude-dir=node_modules --exclude-dir=dist --exclude-dir=.git --exclude-dir=tasks "${INCLUDES[@]}" -- ':2375|:9100' "$REPO_ROOT" 2>/dev/null \
  | grep -Ev "$ALLOWED_FILES_RE" \
  | grep -Ev '/dist/' \
  || true)

if [ -n "$found" ]; then
  echo "BUG-710 regression: obsolete port reference (use :3375 instead)" >&2
  echo "$found" >&2
  exit 1
fi

echo "OK: no :2375 or :9100 references in production sources"

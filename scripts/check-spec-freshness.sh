#!/usr/bin/env bash
# Report drift between the vendored cloud API specs (specs/cloud-api/) and
# their upstreams. Informational only — never gates: the pins are
# intentional snapshots, refreshed deliberately with the fetch scripts.
#
# AWS + Azure pins are upstream commit SHAs: drift means the upstream file
# changed in a commit after the pin. GCP pins are Discovery "revision"
# fields: drift means the live document's revision moved.
#
# Usage: scripts/check-spec-freshness.sh [aws|gcp|azure]   (default: all)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CLOUDS="${1:-aws gcp azure}"

check_repo_pinned() {
  # SOURCES.md rows: | `file` | `repo` | `path` | lic | `sha` | time |
  local sources="$1"
  [ -f "$sources" ] || return 0
  grep '^| `' "$sources" | while IFS='|' read -r _ file repo path _lic pin _; do
    file="$(echo "$file" | tr -d ' \`')"
    repo="$(echo "$repo" | tr -d ' \`')"
    path="$(echo "$path" | tr -d ' \`')"
    pin="$(echo "$pin" | tr -d ' \`')"
    case "$pin" in revision*) continue ;; esac
    latest="$(gh api "repos/$repo/commits?path=$path&per_page=1" --jq '.[0].sha' 2>/dev/null || echo "")"
    if [ -z "$latest" ]; then
      echo "?     $file: cannot query upstream ($repo $path)"
    elif [ "$latest" = "$pin" ]; then
      echo "ok    $file"
    else
      echo "DRIFT $file: pinned ${pin:0:12}, upstream tip ${latest:0:12} ($repo $path)"
    fi
  done
}

check_gcp() {
  local sources="$ROOT/specs/cloud-api/gcp/SOURCES.md"
  [ -f "$sources" ] || return 0
  grep '^| `' "$sources" | while IFS='|' read -r _ file host path _lic pin _; do
    file="$(echo "$file" | tr -d ' \`')"
    host="$(echo "$host" | tr -d ' \`')"
    path="$(echo "$path" | tr -d ' \`')"
    pin="$(echo "$pin" | tr -d ' \`')"
    url="https://$host/${path//\\/}"
    live="$(curl -fsSL "$url" 2>/dev/null | jq -r .revision 2>/dev/null || echo "")"
    if [ -z "$live" ]; then
      echo "?     $file: cannot fetch $url"
    elif [ "revision$live" = "$pin" ]; then
      echo "ok    $file"
    else
      echo "DRIFT $file: pinned $pin, live revision $live"
    fi
  done
}

for cloud in $CLOUDS; do
  echo "== $cloud"
  case "$cloud" in
    aws | azure) check_repo_pinned "$ROOT/specs/cloud-api/$cloud/SOURCES.md" ;;
    gcp) check_gcp ;;
    *)
      echo "unknown cloud: $cloud" >&2
      exit 2
      ;;
  esac
done

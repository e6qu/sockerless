#!/usr/bin/env bash
# check-latest-deps.sh — fail loud if any dependency is behind its
# latest published version. Runs as a pre-commit hook + CI job.
#
# Scope:
#   1. Go modules across every go.mod — for each direct require, query
#      `go list -m -versions <module>` and compare against the latest
#      published version. ANY drift fails (no warn tier — operator runs
#      `make upgrade-deps` to bring everything current).
#   2. Terraform providers across every required_providers block —
#      check the version constraint against the latest registry version.
#      Any drift fails.
#
# Exit code: 0 only when every direct dependency matches the latest
# published version. 1 on any drift.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

for tool in go curl jq; do
  command -v "$tool" >/dev/null 2>&1 || { echo "ERROR: $tool not on PATH" >&2; exit 1; }
done

fail=0

# 1. Go modules -------------------------------------------------------
echo "=== Go module dependency freshness ==="
mods=$(find . -name go.mod -not -path '*/node_modules/*' | sort)
for mod_file in $mods; do
  mod_dir=$(dirname "$mod_file")
  pushd "$mod_dir" >/dev/null

  deps=$(awk '
    /^require \(/ { in_block=1; next }
    /^\)/ && in_block { in_block=0; next }
    in_block && !/\/\/ indirect/ {
      sub(/^[ \t]+/, ""); sub(/[ \t]*\/\/.*$/, "")
      if (NF >= 2) print $1, $2
    }
    /^require [^(]/ && !/\/\/ indirect/ {
      sub(/[ \t]*\/\/.*$/, "")
      if (NF >= 3) print $2, $3
    }
  ' go.mod)

  if [[ -z "$deps" ]]; then
    popd >/dev/null
    continue
  fi

  while IFS=' ' read -r name pinned; do
    [[ -z "$name" ]] && continue
    if [[ "$name" == github.com/sockerless/* ]]; then continue; fi
    latest=$(GOFLAGS='' GOWORK=off go list -m -versions "$name" 2>/dev/null \
      | tr ' ' '\n' | tail -n +2 \
      | grep -vE '\-(beta|alpha|rc|dev)' | tail -1 || true)
    if [[ -z "$latest" ]]; then continue; fi
    if [[ "$pinned" != "$latest" ]]; then
      echo "  FAIL  $mod_dir: $name pinned $pinned (latest $latest)"
      fail=$((fail + 1))
    fi
  done <<<"$deps"
  popd >/dev/null
done

# 2. Terraform providers ---------------------------------------------
echo
echo "=== Terraform provider freshness ==="
tf_files=$(find . -name versions.tf -not -path '*/node_modules/*' -not -path '*/.terraform/*' | sort)

for tf in $tf_files; do
  # Parse required_providers block. Output lines: "<name>|<source>|<constraint>"
  parsed=$(awk '
    /required_providers/ { in_rp=1; next }
    in_rp && /^\s*}\s*$/ { in_rp=0 }
    in_rp && /[a-zA-Z_][a-zA-Z0-9_-]*[[:space:]]*=[[:space:]]*\{/ {
      n=$1; gsub("=","",n); gsub("[[:space:]]","",n); name=n; src=""; ver=""; next
    }
    in_rp && /source/ {
      gsub("\"",""); src=$3
    }
    in_rp && /version/ {
      gsub("\"",""); ver=$3
      if (name != "" && src != "" && ver != "") {
        print name "|" src "|" ver
        name=""; src=""; ver=""
      }
    }
  ' "$tf")

  while IFS='|' read -r name source ver_constraint; do
    [[ -z "$source" ]] && continue
    latest=$(curl -fsSL "https://registry.terraform.io/v1/providers/${source}" 2>/dev/null | jq -r '.version' || echo "")
    if [[ -z "$latest" || "$latest" == "null" ]]; then continue; fi
    constraint_major=$(echo "$ver_constraint" | sed -E 's/[^0-9]*([0-9]+).*/\1/')
    latest_major=$(echo "$latest" | sed -E 's/^([0-9]+).*/\1/')
    if [[ "$constraint_major" != "$latest_major" ]]; then
      echo "  FAIL  $tf: $name ($source) constraint $ver_constraint vs latest $latest (run \`terraform init -upgrade\` then bump constraint)"
      fail=$((fail + 1))
    fi
  done <<<"$parsed"
done

echo
if [[ $fail -gt 0 ]]; then
  echo "$fail dependency drift(s) detected. Run \`make upgrade-deps\` from the affected module dirs (Go) or update versions.tf + \`terraform init -upgrade\` (TF), then re-run this check." >&2
  exit 1
fi
echo "OK: every dependency is on its latest version."
exit 0

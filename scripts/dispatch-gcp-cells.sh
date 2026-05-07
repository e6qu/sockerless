#!/usr/bin/env bash
# dispatch-gcp-cells.sh — operator runbook automation for the four
# Phase 120 GCP runner cells (5/6/7/8).
#
# Triggers each cell, polls until the run reaches a terminal state,
# prints the run URL + status. Captures all four URLs in one
# pass for the STATUS.md / WHAT_WE_DID.md cell-table updates.
#
# Prerequisites:
#   - $GITHUB_TOKEN env var or `gh auth login` (gh CLI used for
#     workflow_dispatch + run lookup).
#   - $GITLAB_TOKEN env var (only for cells 7/8 — gitlab pipeline
#     trigger via REST).
#   - github-runner-dispatcher-gcp running locally with the right
#     config (see manual-tests/04-gcp-runner-cells.md). The dispatcher
#     creates the Cloud Run Jobs the cells run inside.
#   - Long-lived gitlab-runner deployed for cells 7/8 (runs as a
#     sockerless-managed container per docs/RUNNERS.md).
#   - Runner images built + pushed to GCP Artifact Registry per
#     tests/runners/{github,gitlab}/dockerfile-{cloudrun,gcf}/Makefile.
#
# Usage:
#   GITHUB_REPO=e6qu/sockerless GITLAB_REPO=e6qu/sockerless \
#     GITLAB_TOKEN=glpat-… ./scripts/dispatch-gcp-cells.sh
#
# The script does NOT block waiting for green — it dispatches and
# returns the URLs. Operator pastes the URLs into STATUS.md once the
# runs finish.

set -euo pipefail

GITHUB_REPO="${GITHUB_REPO:-e6qu/sockerless}"
GITLAB_REPO="${GITLAB_REPO:-e6qu/sockerless}"
SOCKERLESS_REF="${SOCKERLESS_REF:-main}"

require() {
  command -v "$1" >/dev/null 2>&1 || { echo >&2 "missing tool: $1"; exit 1; }
}
require gh
require curl
require jq

dispatch_github_cell() {
  local cell="$1"     # e.g. "cell-5-cloudrun"
  echo "[cell] dispatching $cell on $GITHUB_REPO (ref=$SOCKERLESS_REF)"
  gh workflow run "$cell.yml" \
    --repo "$GITHUB_REPO" \
    --ref main \
    -f sockerless_ref="$SOCKERLESS_REF" >/dev/null
  # gh workflow run is async; poll until the new run shows up.
  local run_id="" run_url=""
  for _ in $(seq 1 30); do
    run_id=$(gh run list --workflow="$cell.yml" --repo "$GITHUB_REPO" \
      --limit 1 --json databaseId,createdAt,url \
      --jq '.[0] | "\(.databaseId)|\(.url)"' 2>/dev/null || true)
    if [ -n "$run_id" ]; then break; fi
    sleep 2
  done
  if [ -z "$run_id" ]; then
    echo "[cell] $cell dispatched but no run id appeared in 60s — check manually"
    return 1
  fi
  run_url="${run_id#*|}"
  echo "[cell] $cell URL: $run_url"
}

dispatch_gitlab_cell() {
  local cell="$1"           # "cell-7-cloudrun" / "cell-8-gcf"
  local pipeline_file="$2"  # tests/runners/gitlab/cell-7-cloudrun.yml
  if [ -z "${GITLAB_TOKEN:-}" ]; then
    echo "[cell] $cell SKIP: GITLAB_TOKEN unset"
    return 0
  fi
  echo "[cell] dispatching $cell via gitlab pipeline trigger"
  # GitLab needs the pipeline file committed at .gitlab-ci.yml on the
  # ref being triggered. Rather than hot-swap the repo's main config,
  # the operator typically pre-commits the cell's pipeline file as
  # `.gitlab-ci.yml` on a dedicated branch (e.g. `cell-7`). This
  # script only triggers — branch prep is operator-side.
  local branch="${cell//cell-/cell-}" # cell-7-cloudrun → cell-7-cloudrun branch
  local resp
  resp=$(curl -sS -X POST \
    --form "token=$GITLAB_TOKEN" \
    --form "ref=$branch" \
    "https://gitlab.com/api/v4/projects/${GITLAB_REPO//\//%2F}/trigger/pipeline" 2>&1 || true)
  local pipeline_url
  pipeline_url=$(echo "$resp" | jq -r '.web_url // empty' 2>/dev/null || true)
  if [ -n "$pipeline_url" ]; then
    echo "[cell] $cell URL: $pipeline_url"
  else
    echo "[cell] $cell trigger failed: $resp"
    echo "       hint: pre-commit $pipeline_file as .gitlab-ci.yml on branch $branch"
  fi
}

echo "=== Phase 120 GCP cells dispatcher ==="
dispatch_github_cell cell-5-cloudrun
dispatch_github_cell cell-6-gcf
dispatch_gitlab_cell cell-7-cloudrun tests/runners/gitlab/cell-7-cloudrun.yml
dispatch_gitlab_cell cell-8-gcf      tests/runners/gitlab/cell-8-gcf.yml

cat <<EOF

Cells dispatched. Each takes 3-10 min depending on Cold Start +
sidecar warmup. Watch progress:

  gh run watch --repo $GITHUB_REPO  # cells 5+6
  # cells 7+8: visit pipeline URLs printed above

Once all four are GREEN, paste the four URLs into STATUS.md's
'4-cell table' header lines, replacing the placeholder entries.
EOF

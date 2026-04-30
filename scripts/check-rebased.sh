#!/usr/bin/env bash
# Checks that the current branch is rebased on top of origin/main.
# Fails if origin/main has commits not in the current branch.
#
# Mirror remotes (any remote whose name is not exactly `origin`) are
# expected to track origin/main verbatim — fast-forward pushes of main
# to them are intentional and exempt from the rebase / "no main push"
# checks. pre-commit exposes the target remote via $PRE_COMMIT_REMOTE_NAME.

set -euo pipefail

remote_name="${PRE_COMMIT_REMOTE_NAME:-origin}"
if [ "$remote_name" != "origin" ]; then
  # Mirror push: nothing to validate against origin/main since the
  # mirror is *supposed* to receive main as-is.
  exit 0
fi

branch=$(git rev-parse --abbrev-ref HEAD)

# Never push directly to main on origin
if [ "$branch" = "main" ]; then
  echo "ERROR: Do not push directly to main on origin. Create a branch first."
  exit 1
fi

# Fetch latest origin/main without merging
git fetch origin main --quiet 2>/dev/null || true

# Check local main is in sync with origin/main
local_main=$(git rev-parse main 2>/dev/null || echo "")
origin_main=$(git rev-parse origin/main 2>/dev/null || echo "")

if [ -n "$local_main" ] && [ -n "$origin_main" ] && [ "$local_main" != "$origin_main" ]; then
  echo "ERROR: Local main ($local_main) differs from origin/main ($origin_main)."
  echo "Sync first: git checkout main && git pull origin main"
  exit 1
fi

# Check branch is rebased on origin/main
behind=$(git rev-list --count "HEAD..origin/main" 2>/dev/null || echo "0")

if [ "$behind" -gt 0 ]; then
  echo "ERROR: Branch '$branch' is $behind commit(s) behind origin/main."
  echo "Rebase before pushing: git fetch origin main && git rebase origin/main"
  exit 1
fi

# Check linear history (no merge commits since origin/main)
merges=$(git rev-list --merges "origin/main..HEAD" 2>/dev/null | wc -l | tr -d ' ')

if [ "$merges" -gt 0 ]; then
  echo "ERROR: Branch '$branch' has $merges merge commit(s). History must be linear."
  echo "Rebase instead of merging: git rebase origin/main"
  exit 1
fi

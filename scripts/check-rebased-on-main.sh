#!/usr/bin/env bash
# Enforce that the current branch is rebased on top of origin/main — i.e. it
# contains origin/main's tip, so it merges cleanly and its CI reflects the
# latest base. A branch that has fallen behind origin/main must be rebased
# before committing/pushing/merging.
#
# This is the CI counterpart of the pre-push `check-rebased.sh` hook: rebase
# state is gated at push time locally (check-rebased.sh, which also enforces
# local-main sync + linear history), and re-checked in CI by the
# rebased-on-main job so a push that skipped the pre-push hooks can't merge a
# behind branch. It compares against FETCH_HEAD (the freshly fetched
# origin/main tip) rather than the remote-tracking ref, which is reliable under
# CI's narrow-refspec checkouts. In CI a fetch failure or a behind branch
# fails; run locally it skips on a failed fetch (offline).
set -euo pipefail

network_required=""
[ -n "${CI:-}" ] && network_required="yes"

if ! git rev-parse --git-dir >/dev/null 2>&1; then
  exit 0
fi

# Fetch the latest origin/main into FETCH_HEAD.
if ! git fetch -q origin main 2>/dev/null; then
  if [ -n "$network_required" ]; then
    echo "ERROR: could not fetch origin/main to verify the branch is rebased." >&2
    exit 1
  fi
  echo "check-rebased-on-main: could not fetch origin/main (offline?); skipping (CI enforces this)."
  exit 0
fi

# HEAD is on top of origin/main iff origin/main's tip is an ancestor of HEAD.
# (A commit is its own ancestor, so this also passes when HEAD == origin/main,
# e.g. on main itself.)
if git merge-base --is-ancestor FETCH_HEAD HEAD; then
  echo "check-rebased-on-main: branch is on top of origin/main — OK."
  exit 0
fi

behind=$(git rev-list --count "HEAD..FETCH_HEAD" 2>/dev/null || echo "?")
echo "ERROR: this branch is NOT rebased on top of origin/main" >&2
echo "(${behind} commit(s) are on origin/main but not in this branch)." >&2
echo "Rebase before committing/pushing/merging:" >&2
echo "  git fetch origin main && git rebase origin/main" >&2
exit 1

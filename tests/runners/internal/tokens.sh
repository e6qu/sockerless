#!/usr/bin/env bash
# Token-fetch helpers for the runner harnesses. Source this file (or call
# the functions directly) from a shell that already has the keychain
# entries set up — see docs/RUNNERS.md § Token strategy for the one-time
# setup steps.
#
# These helpers print the bare token to stdout. They never write to disk
# and never echo to stderr or a log file. Caller-side discipline:
#   PAT=$(./tests/runners/internal/tokens.sh gh)   # capture into a var
#   ... use $PAT in the same process ...
#   unset PAT                                       # zero on exit
#
# All errors go to stderr with a stable prefix so the Go harness can
# match on it and surface a useful message.

set -euo pipefail

err() {
  printf "tokens.sh: %s\n" "$*" 1>&2
  exit 2
}

gh_pat() {
  command -v gh >/dev/null 2>&1 || err "gh CLI not installed (run: brew install gh)"
  gh auth status >/dev/null 2>&1 || err "gh not authenticated (run: gh auth login)"
  # Confirm the workflow scope — without it the registration-token API 403s.
  gh auth status 2>&1 | grep -q "'workflow'" || err "gh token missing 'workflow' scope (run: gh auth refresh -s workflow)"
  gh auth token
}

gl_pat() {
  command -v security >/dev/null 2>&1 || err "security(1) not available — macOS only"
  security find-generic-password -s sockerless-gl-pat -a "$USER" -w 2>/dev/null \
    || err "GitLab PAT not in keychain. One-time setup: security add-generic-password -U -s sockerless-gl-pat -a \"\$USER\" -w"
}

case "${1:-}" in
  gh) gh_pat ;;
  gl) gl_pat ;;
  *)  err "usage: $0 {gh|gl}" ;;
esac

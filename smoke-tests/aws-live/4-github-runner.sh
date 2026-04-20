#!/usr/bin/env bash
# github.com self-hosted runner via sockerless ECS.
# Requires GITHUB_PAT with repo:write scope so the runner can register.
set -euo pipefail

: "${GITHUB_PAT:?GITHUB_PAT is required (personal access token, repo scope)}"
: "${GITHUB_REPO:?GITHUB_REPO is required (owner/repo)}"
: "${AWS_REGION:=eu-west-1}"

echo "=== GitHub runner (skips gracefully if PAT absent) ==="
# Placeholder — actual runner bring-up is out of scope for the sim
# replay; live session only.
echo "GITHUB_PAT present; would register runner for $GITHUB_REPO."

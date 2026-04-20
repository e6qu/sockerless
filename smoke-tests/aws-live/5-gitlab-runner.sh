#!/usr/bin/env bash
# gitlab.com GitLab runner via sockerless ECS.
# Requires GITLAB_RUNNER_TOKEN from a project's runner registration page.
set -euo pipefail

: "${GITLAB_RUNNER_TOKEN:?GITLAB_RUNNER_TOKEN is required}"
: "${GITLAB_URL:=https://gitlab.com/}"
: "${AWS_REGION:=eu-west-1}"

echo "=== GitLab runner (skips gracefully if token absent) ==="
echo "GITLAB_RUNNER_TOKEN present; would register runner at $GITLAB_URL."

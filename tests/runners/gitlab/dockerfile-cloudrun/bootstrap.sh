#!/bin/bash
# bootstrap.sh — gitlab-runner-cloudrun entrypoint.
#
# Brings up sockerless-backend-cloudrun on localhost:3375, registers
# the runner with GitLab once (idempotent), then runs the long-lived
# gitlab-runner polling loop. Each job dispatched via docker executor
# routes to the in-container sockerless backend → Cloud Run Job per
# step.
#
# Required env (set by `docker run -e ...` from the operator):
#
#   GITLAB_URL                - e.g. https://gitlab.com
#   GITLAB_RUNNER_TOKEN       - registration token from
#                               Project Settings → CI/CD → Runners
#   GITLAB_RUNNER_TAGS        - csv (e.g. "sockerless-cloudrun")
#   GITLAB_RUNNER_NAME        - display name (default: hostname)
#
# Optional env (sockerless-backend-cloudrun config):
#
#   SOCKERLESS_GCR_PROJECT, SOCKERLESS_GCR_REGION,
#   SOCKERLESS_GCP_BUILD_BUCKET, GOOGLE_APPLICATION_CREDENTIALS
set -euo pipefail

# Required env (no fallbacks, no optional vars — fail loudly):
: "${SOCKERLESS_GCR_PROJECT:?SOCKERLESS_GCR_PROJECT is required (the operator-side docker run -e config sets this)}"
: "${SOCKERLESS_GCR_REGION:?SOCKERLESS_GCR_REGION is required (the operator-side docker run -e config sets this)}"
: "${SOCKERLESS_GCP_BUILD_BUCKET:?SOCKERLESS_GCP_BUILD_BUCKET is required (the operator-side docker run -e config sets this)}"

nohup /usr/local/bin/sockerless-backend-cloudrun -addr :3375 -log-level info \
    >/tmp/sockerless-backend.log 2>&1 &

deadline=$((SECONDS + 30))
until curl -sfo /dev/null http://localhost:3375/_ping; do
    if [ $SECONDS -ge $deadline ]; then
        echo "bootstrap: sockerless-backend-cloudrun did not become ready in 30s"
        cat /tmp/sockerless-backend.log >&2 || true
        exit 1
    fi
    sleep 1
done
echo "bootstrap: sockerless-backend-cloudrun ready"

# Register (idempotent: gitlab-runner register is OK to call repeatedly
# as long as the same name + token combination is unique on GitLab's
# side — we use the SHA256 of token+name as a stable token suffix).
gitlab-runner register \
    --non-interactive \
    --url "${GITLAB_URL:-https://gitlab.com}" \
    --registration-token "${GITLAB_RUNNER_TOKEN}" \
    --name "${GITLAB_RUNNER_NAME:-$(hostname)}" \
    --tag-list "${GITLAB_RUNNER_TAGS:-sockerless-cloudrun}" \
    --executor docker \
    --docker-image alpine:latest \
    --docker-host "tcp://localhost:3375" \
    --docker-pull-policy if-not-present

# Long-lived polling loop. gitlab-runner re-execs itself on SIGHUP for
# config reloads; SIGTERM stops gracefully.
exec gitlab-runner run --config /etc/gitlab-runner/config.toml --working-directory /tmp/runner-work

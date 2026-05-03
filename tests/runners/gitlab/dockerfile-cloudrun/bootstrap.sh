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

# Backend log goes to stderr so Cloud Logging captures it (without
# this redirect to /tmp/sockerless-backend.log it never surfaced and
# BUG-917 was undiagnosable). Use `tee` to keep both file + stderr.
nohup /usr/local/bin/sockerless-backend-cloudrun -addr :3375 -log-level debug \
    > >(tee /tmp/sockerless-backend.log >&2) 2>&1 &

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

# BUG-913: gitlab-runner crashes with `chdir: no such file or directory`
# if --working-directory doesn't exist. Cloud Run gives us an empty
# rootfs (no host bind mounts); create the work dir up-front.
mkdir -p /tmp/runner-work

# Cloud Run $PORT healthcheck. Cloud Run requires the container to
# bind $PORT (default 8080). socat proxies $PORT → sockerless backend
# on :3375 so /_ping etc. answer the healthchecks. Without this the
# revision never reaches Ready.
if [ -n "${PORT:-}" ]; then
    nohup socat "TCP-LISTEN:${PORT},reuseaddr,fork" "TCP:127.0.0.1:3375" \
        >/tmp/socat.log 2>&1 &
    echo "bootstrap: socat \$PORT=${PORT} → sockerless-backend-cloudrun:3375"
fi

# Register using the GitLab 16+ runner auth token (`glrt-...`), not
# the deprecated `--registration-token`. Idempotent on the same token.
gitlab-runner register \
    --non-interactive \
    --url "${GITLAB_URL:-https://gitlab.com}" \
    --token "${GITLAB_RUNNER_TOKEN}" \
    --name "${GITLAB_RUNNER_NAME:-$(hostname)}" \
    --executor docker \
    --docker-image alpine:latest \
    --docker-host "tcp://localhost:3375" \
    --docker-pull-policy if-not-present

# BUG-915: --docker-disable-cache CLI flag doesn't always propagate
# to config.toml. Post-edit to ensure disable_cache=true (the default
# gitlab-runner cache volume name exceeds GCS's 63-char bucket limit).
sed -i 's/disable_cache = false/disable_cache = true/' /etc/gitlab-runner/config.toml

# BUG-918 wedge: pin helper_image to the tag-form so gitlab-runner's
# permission containers don't reference the bare sha256:<digest> form
# that sockerless's parseDockerRef mangles into a broken AR URL.
# Insert helper_image line after [runners.docker] section header.
sed -i '/\[runners.docker\]/a\
    helper_image = "registry.gitlab.com/gitlab-org/gitlab-runner/gitlab-runner-helper:x86_64-v17.5.0"' \
    /etc/gitlab-runner/config.toml

echo "bootstrap: gitlab-runner config.toml:"
cat /etc/gitlab-runner/config.toml

# Long-lived polling loop. gitlab-runner re-execs itself on SIGHUP for
# config reloads; SIGTERM stops gracefully.
exec gitlab-runner run --config /etc/gitlab-runner/config.toml --working-directory /tmp/runner-work

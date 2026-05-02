#!/bin/bash
# bootstrap.sh — gitlab-runner-gcf entrypoint. Mirror of
# gitlab/dockerfile-cloudrun/bootstrap.sh; only the backend binary +
# port differ.
set -euo pipefail

# Required env (no fallbacks, no optional vars — fail loudly):
: "${SOCKERLESS_GCF_PROJECT:?SOCKERLESS_GCF_PROJECT is required (the operator-side docker run -e config sets this)}"
: "${SOCKERLESS_GCF_REGION:?SOCKERLESS_GCF_REGION is required (the operator-side docker run -e config sets this)}"
: "${SOCKERLESS_GCP_BUILD_BUCKET:?SOCKERLESS_GCP_BUILD_BUCKET is required (the operator-side docker run -e config sets this)}"

nohup /usr/local/bin/sockerless-backend-gcf -addr :3376 -log-level info \
    >/tmp/sockerless-backend.log 2>&1 &

deadline=$((SECONDS + 30))
until curl -sfo /dev/null http://localhost:3376/_ping; do
    if [ $SECONDS -ge $deadline ]; then
        echo "bootstrap: sockerless-backend-gcf did not become ready in 30s"
        cat /tmp/sockerless-backend.log >&2 || true
        exit 1
    fi
    sleep 1
done
echo "bootstrap: sockerless-backend-gcf ready"

if [ -n "${PORT:-}" ]; then
    nohup socat "TCP-LISTEN:${PORT},reuseaddr,fork" "TCP:127.0.0.1:3376" \
        >/tmp/socat.log 2>&1 &
    echo "bootstrap: socat \$PORT=${PORT} → sockerless-backend-gcf:3376"
fi

gitlab-runner register \
    --non-interactive \
    --url "${GITLAB_URL:-https://gitlab.com}" \
    --token "${GITLAB_RUNNER_TOKEN}" \
    --name "${GITLAB_RUNNER_NAME:-$(hostname)}" \
    --executor docker \
    --docker-image alpine:latest \
    --docker-host "tcp://localhost:3376" \
    --docker-pull-policy if-not-present

exec gitlab-runner run --config /etc/gitlab-runner/config.toml --working-directory /tmp/runner-work

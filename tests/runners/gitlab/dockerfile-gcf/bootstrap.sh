#!/bin/bash
# bootstrap.sh — gitlab-runner-gcf entrypoint. Mirror of
# gitlab/dockerfile-cloudrun/bootstrap.sh; only the backend binary +
# port differ.
set -euo pipefail

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

gitlab-runner register \
    --non-interactive \
    --url "${GITLAB_URL:-https://gitlab.com}" \
    --registration-token "${GITLAB_RUNNER_TOKEN}" \
    --name "${GITLAB_RUNNER_NAME:-$(hostname)}" \
    --tag-list "${GITLAB_RUNNER_TAGS:-sockerless-gcf}" \
    --executor docker \
    --docker-image alpine:latest \
    --docker-host "tcp://localhost:3376" \
    --docker-pull-policy if-not-present

exec gitlab-runner run --config /etc/gitlab-runner/config.toml --working-directory /tmp/runner-work

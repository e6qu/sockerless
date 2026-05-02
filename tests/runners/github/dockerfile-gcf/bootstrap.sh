#!/bin/bash
# bootstrap.sh — entrypoint for the sockerless-runner-gcf image.
#
# Brings up the in-image sockerless-backend-gcf on localhost:3376,
# then registers + runs an ephemeral GitHub Actions runner. Mirror of
# dockerfile-cloudrun/bootstrap.sh, only the backend binary differs.
set -euo pipefail

# Required env (no fallbacks, no optional vars — fail loudly):
: "${SOCKERLESS_GCF_PROJECT:?SOCKERLESS_GCF_PROJECT is required (set by github-runner-dispatcher-gcp from the gcp_project label config)}"
: "${SOCKERLESS_GCF_REGION:?SOCKERLESS_GCF_REGION is required (set by github-runner-dispatcher-gcp from the gcp_region label config)}"
: "${SOCKERLESS_GCP_BUILD_BUCKET:?SOCKERLESS_GCP_BUILD_BUCKET is required (set by github-runner-dispatcher-gcp from the build_bucket label config)}"

nohup /usr/local/bin/sockerless-backend-gcf -addr :3376 -log-level info \
    >/tmp/sockerless-backend.log 2>&1 &
SOCKERLESS_PID=$!

deadline=$((SECONDS + 30))
until curl -sfo /dev/null http://localhost:3376/_ping; do
    if [ $SECONDS -ge $deadline ]; then
        echo "bootstrap: sockerless-backend-gcf did not become ready in 30s"
        cat /tmp/sockerless-backend.log >&2 || true
        exit 1
    fi
    sleep 1
done
echo "bootstrap: sockerless-backend-gcf ready (pid=$SOCKERLESS_PID)"

cd /opt/runner
sudo -u runner ./config.sh \
    --unattended --replace --ephemeral \
    --url "https://github.com/${RUNNER_REPO}" \
    --token "${RUNNER_REG_TOKEN}" \
    --name "${RUNNER_NAME}" \
    --labels "${RUNNER_LABELS}" \
    --work /tmp/runner-work

exec sudo -u runner -E timeout "${RUNNER_IDLE_SECONDS:-60}" ./run.sh --once

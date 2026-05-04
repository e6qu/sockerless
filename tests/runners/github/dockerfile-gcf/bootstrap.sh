#!/bin/bash
# bootstrap.sh — entrypoint for the sockerless-runner-gcf image.
#
# Brings up the in-image sockerless-backend-gcf on localhost:3376,
# then registers + runs an ephemeral GitHub Actions runner. Mirror of
# dockerfile-cloudrun/bootstrap.sh, only the backend binary differs.
set -euo pipefail

# Auto-discover sockerless config from GCP instance metadata. See
# dockerfile-cloudrun/bootstrap.sh for full rationale (dispatcher
# scope cleanup; runner image owns its config via cloud primitives).
META=http://metadata.google.internal/computeMetadata/v1
HDR='Metadata-Flavor: Google'
export SOCKERLESS_GCF_PROJECT=$(curl -sf -H "$HDR" $META/project/project-id)
export SOCKERLESS_GCF_REGION=$(curl -sf -H "$HDR" $META/instance/region | awk -F/ '{print $NF}')
export SOCKERLESS_GCP_BUILD_BUCKET="${SOCKERLESS_GCF_PROJECT}-build"
export SOCKERLESS_GCP_SHARED_VOLUMES="runner-workspace=/tmp/runner-work=${SOCKERLESS_GCF_PROJECT}-runner-workspace,runner-externals=/opt/runner/externals=${SOCKERLESS_GCF_PROJECT}-runner-workspace"
echo "bootstrap: auto-discovered project=$SOCKERLESS_GCF_PROJECT region=$SOCKERLESS_GCF_REGION"

nohup /usr/local/bin/sockerless-backend-gcf -addr :3376 -log-level debug \
    > >(tee /tmp/sockerless-backend.log >&2) 2>&1 &
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

exec sudo -u runner -E timeout "${RUNNER_IDLE_SECONDS:-3600}" ./run.sh --once

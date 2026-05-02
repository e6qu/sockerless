#!/bin/bash
# bootstrap.sh — entrypoint for the sockerless-runner-cloudrun image.
#
# Brings up the in-image sockerless-backend-cloudrun on localhost:3375,
# then registers + runs an ephemeral GitHub Actions runner. Exits when
# the runner completes one job (--ephemeral mode).
#
# Required env (set by github-runner-dispatcher-gcp's Cloud Run Job container env):
#
#   RUNNER_REG_TOKEN  - ephemeral registration token from
#                       POST /repos/<r>/actions/runners/registration-token
#   RUNNER_REPO       - owner/repo
#   RUNNER_NAME       - unique runner name (logged in Actions UI)
#   RUNNER_LABELS     - csv of labels (e.g. "sockerless-cloudrun")
#   RUNNER_IDLE_SECONDS - seconds to wait for a job before exiting
#
# Optional env (sockerless-backend-cloudrun config):
#
#   SOCKERLESS_GCR_PROJECT - GCP project (sockerless-live-46x3zg4imo)
#   SOCKERLESS_GCR_REGION  - GCP region (us-central1)
#   SOCKERLESS_GCP_BUILD_BUCKET - GCS bucket for Cloud Build context
#   GOOGLE_APPLICATION_CREDENTIALS - SA key path (mounted via
#                                    runner-task secret)
set -euo pipefail

# Required env (no fallbacks, no optional vars — fail loudly):
: "${SOCKERLESS_GCR_PROJECT:?SOCKERLESS_GCR_PROJECT is required (set by github-runner-dispatcher-gcp from the gcp_project label config)}"
: "${SOCKERLESS_GCR_REGION:?SOCKERLESS_GCR_REGION is required (set by github-runner-dispatcher-gcp from the gcp_region label config)}"
: "${SOCKERLESS_GCP_BUILD_BUCKET:?SOCKERLESS_GCP_BUILD_BUCKET is required (set by github-runner-dispatcher-gcp from the build_bucket label config)}"

# Sockerless backend in background. -log-level info keeps CloudWatch /
# CloudLogging output manageable.
nohup /usr/local/bin/sockerless-backend-cloudrun -addr :3375 -log-level info \
    >/tmp/sockerless-backend.log 2>&1 &
SOCKERLESS_PID=$!

# Wait for /_ping with a 30s budget. Cold-start latency on Cloud Run
# usually puts this well under 5s; the budget covers slow image pulls.
deadline=$((SECONDS + 30))
until curl -sfo /dev/null http://localhost:3375/_ping; do
    if [ $SECONDS -ge $deadline ]; then
        echo "bootstrap: sockerless-backend-cloudrun did not become ready in 30s"
        cat /tmp/sockerless-backend.log >&2 || true
        exit 1
    fi
    sleep 1
done
echo "bootstrap: sockerless-backend-cloudrun ready (pid=$SOCKERLESS_PID)"

cd /opt/runner
sudo -u runner ./config.sh \
    --unattended --replace --ephemeral \
    --url "https://github.com/${RUNNER_REPO}" \
    --token "${RUNNER_REG_TOKEN}" \
    --name "${RUNNER_NAME}" \
    --labels "${RUNNER_LABELS}" \
    --work /tmp/runner-work

# --once: runner exits after one job, matching the github-runner-
# dispatcher's per-job model. Idle timeout via the runner's natural
# polling loop — no job picked up within RUNNER_IDLE_SECONDS = exit.
exec sudo -u runner -E timeout "${RUNNER_IDLE_SECONDS:-3600}" ./run.sh --once

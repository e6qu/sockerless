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

# Per CLOUD_RESOURCE_MAPPING.md "Adjustment to dispatcher scope", the
# dispatcher MUST NOT inject sockerless-shaped env. The runner image
# auto-discovers its sockerless backend config from the GCP instance
# metadata server (every Cloud Run instance has access to it via
# metadata.google.internal). Project + region come from metadata
# directly; build_bucket + runner_workspace_bucket come from
# convention (project_id-build, project_id-runner-workspace).
#
# This is real cloud primitive use (metadata server is the GCP-blessed
# config-discovery path), not a synthetic fallback.
META=http://metadata.google.internal/computeMetadata/v1
HDR='Metadata-Flavor: Google'
export SOCKERLESS_GCR_PROJECT=$(curl -sf -H "$HDR" $META/project/project-id)
export SOCKERLESS_GCR_REGION=$(curl -sf -H "$HDR" $META/instance/region | awk -F/ '{print $NF}')
export SOCKERLESS_GCP_BUILD_BUCKET="${SOCKERLESS_GCR_PROJECT}-build"
# 4-tuple `name=path=bucket=backing`. Cells 5+6 (GH actions/
# runner) use `gcs-sync` because the runner-task and JOB pod-Service are
# separate Cloud Run resources writing/reading the same workspace
# concurrently — GCSFuse stale-handle on per-step rewrites
# rules out `gcs-fuse`. `gcs-sync` syncs at exec boundaries via SDK.
export SOCKERLESS_GCP_SHARED_VOLUMES="runner-workspace=/tmp/runner-work=${SOCKERLESS_GCR_PROJECT}-runner-workspace=gcs-sync,runner-externals=/opt/runner/externals=${SOCKERLESS_GCR_PROJECT}-runner-workspace=gcs-sync"
# Runner-pattern containers (long-lived) need Cloud Run
# Service path. UseService=1 + VPC connector required for cross-revision
# DNS via internal ingress.
export SOCKERLESS_GCR_USE_SERVICE=1
export SOCKERLESS_GCR_VPC_CONNECTOR="projects/${SOCKERLESS_GCR_PROJECT}/locations/${SOCKERLESS_GCR_REGION}/connectors/sockerless-connector"
echo "bootstrap: auto-discovered project=$SOCKERLESS_GCR_PROJECT region=$SOCKERLESS_GCR_REGION use_service=1"

# Sockerless backend in background. -log-level info keeps CloudWatch /
# CloudLogging output manageable.
nohup /usr/local/bin/sockerless-backend-cloudrun -addr :3375 -log-level debug \
    > >(tee /tmp/sockerless-backend.log >&2) 2>&1 &
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

# --- idle gate (shared across runner images; edit all copies together) ---
# The dispatcher spawns one ephemeral runner per queued job, and GitHub
# may hand the job to a different runner (duplicate-spawn race / seen-set
# loss). Bound only the PRE-PICKUP window: if no job starts within
# RUNNER_IDLE_SECONDS (default 120) the runner exits cleanly; once a job
# is picked up (a Runner.Worker child appears) it runs unbounded by this
# gate, to the job's own timeout. A whole-process `timeout` would kill
# in-flight jobs; an absent gate leaves never-assigned runners waiting
# forever.
job_started() {
  local d
  for d in /proc/[0-9]*; do
    if tr '\0' ' ' < "$d/cmdline" 2>/dev/null | grep -q 'Runner\.Worker'; then
      return 0
    fi
  done
  return 1
}

run_with_idle_gate() {
  local idle="${RUNNER_IDLE_SECONDS:-120}"
  local marker
  marker=$(mktemp)
  rm -f "$marker"
  "$@" &
  local run_pid=$!
  (
    deadline=$((SECONDS + idle))
    while [ "$SECONDS" -lt "$deadline" ]; do
      kill -0 "$run_pid" 2>/dev/null || exit 0
      job_started && exit 0
      sleep 2
    done
    job_started && exit 0
    echo "idle-gate: no job picked up within ${idle}s; stopping runner" >&2
    touch "$marker"
    kill "$run_pid" 2>/dev/null || true
  ) &
  local gate_pid=$!
  local rc=0
  wait "$run_pid" || rc=$?
  kill "$gate_pid" 2>/dev/null || true
  wait "$gate_pid" 2>/dev/null || true
  if [ -e "$marker" ]; then
    rm -f "$marker"
    return 0 # idle exit is the expected no-job outcome, not a failure
  fi
  return "$rc"
}
# --- end idle gate ---

cd /opt/runner
sudo -u runner ./config.sh \
    --unattended --replace --ephemeral \
    --url "https://github.com/${RUNNER_REPO}" \
    --token "${RUNNER_REG_TOKEN}" \
    --name "${RUNNER_NAME}" \
    --labels "${RUNNER_LABELS}" \
    --work /tmp/runner-work

# --once: runner exits after one job, matching the github-runner-
# dispatcher's per-job model. The Cloud Run task timeout (the
# dispatcher's runner_job_timeout) bounds the job itself.
run_with_idle_gate sudo -u runner -E ./run.sh --once

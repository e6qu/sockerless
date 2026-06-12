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
# 4-tuple `name=path=bucket=backing`. Cells 5+6 use
# `gcs-sync` for concurrent runner-task ↔ JOB pod-Service propagation.
export SOCKERLESS_GCP_SHARED_VOLUMES="runner-workspace=/tmp/runner-work=${SOCKERLESS_GCF_PROJECT}-runner-workspace=gcs-sync,runner-externals=/opt/runner/externals=${SOCKERLESS_GCF_PROJECT}-runner-workspace=gcs-sync"
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

run_with_idle_gate sudo -u runner -E ./run.sh --once

#!/usr/bin/env bash
# Entrypoint for the dispatcher-spawned ephemeral runner (TEST 14).
# Speaks the canonical dispatcher contract: RUNNER_REG_TOKEN +
# RUNNER_REPO (owner/repo) + RUNNER_NAME + RUNNER_LABELS, plus
# RUNNER_SERVER_URL baked into the image (bleephub reachable from the
# host engine via host.docker.internal on the published port 80).
set -euo pipefail

: "${RUNNER_SERVER_URL:?RUNNER_SERVER_URL not set}"
: "${RUNNER_REPO:?RUNNER_REPO not set (owner/repo)}"
: "${RUNNER_REG_TOKEN:?RUNNER_REG_TOKEN not set}"
: "${RUNNER_NAME:?RUNNER_NAME not set}"
: "${RUNNER_LABELS:?RUNNER_LABELS not set}"

export RUNNER_ALLOW_RUNASROOT=1
export GITHUB_ACTIONS_RUNNER_TLS_NO_VERIFY=1

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

cd /runner
./config.sh \
  --unattended --replace --ephemeral \
  --url "${RUNNER_SERVER_URL}/${RUNNER_REPO}" \
  --token "$RUNNER_REG_TOKEN" \
  --name "$RUNNER_NAME" \
  --labels "$RUNNER_LABELS" \
  --no-default-labels

run_with_idle_gate ./run.sh --once

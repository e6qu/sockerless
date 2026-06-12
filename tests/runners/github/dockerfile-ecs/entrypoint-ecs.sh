#!/usr/bin/env bash
# Single-container runner entrypoint for sockerless ECS dispatch.
#
# Starts sockerless-backend-ecs on localhost:3375 in the background,
# waits for it to be ready, then registers + runs `actions/runner`
# with DOCKER_HOST=tcp://localhost:3375. The runner's docker calls
# flow through sockerless to ECS Fargate.

set -euo pipefail

# Registration contract — the same RUNNER_REG_TOKEN / RUNNER_REPO
# (owner/repo) env names the github-runner-dispatcher injects.
: "${RUNNER_REPO:?RUNNER_REPO not set (owner/repo)}"
: "${RUNNER_REG_TOKEN:?RUNNER_REG_TOKEN not set}"
: "${RUNNER_NAME:?RUNNER_NAME not set}"
: "${RUNNER_LABELS:?RUNNER_LABELS not set}"

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

# Populate the EFS-mounted /home/runner/externals from the image-staged
# copy. Skips if already populated (looks for node20/bin/node — a
# stable marker present in any healthy externals tree). On a fresh
# access point, streams via tar pipe (much faster than `cp -r` on NFS
# for the thousands of small node_modules files in externals).
if [ ! -x /home/runner/externals/node20/bin/node ] && [ -d /home/runner/externals.staged ]; then
  echo "populating externals (image → EFS, tar pipe)…"
  ts=$(date +%s)
  ( cd /home/runner/externals.staged && tar cf - . ) | ( cd /home/runner/externals && tar xf - )
  echo "externals populated in $(( $(date +%s) - ts ))s"
else
  echo "externals already populated on EFS (node20/bin/node present)"
fi

# Start sockerless ECS backend in the background. It reads its
# config from the env vars set on the ECS task definition.
sudo -E /usr/local/bin/sockerless-backend-ecs -addr :3375 -log-level debug 2>&1 \
  | sed -u 's/^/[sockerless] /' &
SOCKERLESS_PID=$!

# Wait for sockerless to be reachable.
for i in $(seq 1 60); do
  if curl -fsS http://localhost:3375/_ping > /dev/null 2>&1; then
    echo "sockerless-backend-ecs listening on :3375 (pid=$SOCKERLESS_PID)"
    break
  fi
  sleep 0.5
done
if ! curl -fsS http://localhost:3375/_ping > /dev/null 2>&1; then
  echo "FATAL: sockerless-backend-ecs never became ready"
  exit 1
fi

# Cleanup sockerless when the runner exits.
cleanup() {
  echo "shutting down sockerless-backend-ecs (pid=$SOCKERLESS_PID)"
  sudo kill "$SOCKERLESS_PID" 2>/dev/null || true
}
trap cleanup EXIT

# Configure the runner with the registration token.
./config.sh \
  --url "https://github.com/${RUNNER_REPO}" \
  --token "$RUNNER_REG_TOKEN" \
  --name "$RUNNER_NAME" \
  --labels "$RUNNER_LABELS" \
  --unattended --ephemeral --replace

# Run with DOCKER_HOST pointing at sockerless on localhost.
export DOCKER_HOST=tcp://localhost:3375
run_with_idle_gate ./run.sh --once

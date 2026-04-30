#!/usr/bin/env bash
# Single-container runner entrypoint for sockerless ECS dispatch.
#
# Starts sockerless-backend-ecs on localhost:3375 in the background,
# waits for it to be ready, then registers + runs `actions/runner`
# with DOCKER_HOST=tcp://localhost:3375. The runner's docker calls
# flow through sockerless to ECS Fargate.

set -euo pipefail

: "${RUNNER_REPO_URL:?RUNNER_REPO_URL not set}"
: "${RUNNER_TOKEN:?RUNNER_TOKEN not set}"
: "${RUNNER_NAME:?RUNNER_NAME not set}"
: "${RUNNER_LABELS:?RUNNER_LABELS not set}"

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
  --url "$RUNNER_REPO_URL" \
  --token "$RUNNER_TOKEN" \
  --name "$RUNNER_NAME" \
  --labels "$RUNNER_LABELS" \
  --unattended --ephemeral --replace

# Run with DOCKER_HOST pointing at sockerless on localhost.
export DOCKER_HOST=tcp://localhost:3375
exec ./run.sh

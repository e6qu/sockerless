#!/usr/bin/env bash
# Starts the real Docker passthrough backend and Admin server for Playwright.
# Expects ADMIN_BIN and BACKEND_BIN to point at compiled binaries.
set -euo pipefail

BACKEND_PORT="${BACKEND_PORT:-29100}"
ADMIN_PORT="${ADMIN_PORT:-29090}"
: "${BACKEND_BIN:?BACKEND_BIN must point to the compiled Docker backend}"
: "${ADMIN_BIN:?ADMIN_BIN must point to the compiled Admin server}"

SOCKERLESS_HOME="$(mktemp -d)"
export SOCKERLESS_HOME

docker_host="${DOCKER_HOST:-}"
if [[ -z "$docker_host" && -S /var/run/docker.sock ]]; then
  docker_host="unix:///var/run/docker.sock"
fi
if [[ -z "$docker_host" ]] && command -v podman >/dev/null 2>&1; then
  podman_socket="$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}' 2>/dev/null || true)"
  if [[ -n "$podman_socket" && -S "$podman_socket" ]]; then
    docker_host="unix://${podman_socket}"
  fi
fi
if [[ -z "$docker_host" ]]; then
  echo "a reachable Docker or Podman API socket is required" >&2
  exit 1
fi

"$BACKEND_BIN" --addr ":${BACKEND_PORT}" --docker-host "$docker_host" --log-level warn &
BACKEND_PID=$!

ADMIN_PID=""
cleanup() {
  trap - EXIT INT TERM
  if [[ -n "$ADMIN_PID" ]]; then
    kill "$ADMIN_PID" 2>/dev/null || true
    wait "$ADMIN_PID" 2>/dev/null || true
  fi
  kill "$BACKEND_PID" 2>/dev/null || true
  wait "$BACKEND_PID" 2>/dev/null || true
  rm -rf "$SOCKERLESS_HOME"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# Wait for the real backend to be ready.
attempt=0
while ((attempt < 30)); do
  if ! kill -0 "$BACKEND_PID" 2>/dev/null; then
    echo "Docker backend exited before becoming ready" >&2
    exit 1
  fi
  if curl -s "http://localhost:${BACKEND_PORT}/internal/v1/healthz" > /dev/null 2>&1; then
    break
  fi
  sleep 0.1
  attempt=$((attempt + 1))
done
curl --fail --silent "http://localhost:${BACKEND_PORT}/internal/v1/healthz" >/dev/null

# Start Admin pointing at the real backend.
"$ADMIN_BIN" \
  -addr ":${ADMIN_PORT}" \
  -backend "docker=http://localhost:${BACKEND_PORT}" &
ADMIN_PID=$!

# Wait for admin to be ready
attempt=0
while ((attempt < 30)); do
  if ! kill -0 "$ADMIN_PID" 2>/dev/null; then
    echo "admin server exited before becoming ready" >&2
    exit 1
  fi
  if curl -s "http://localhost:${ADMIN_PORT}/api/v1/components" > /dev/null 2>&1; then
    break
  fi
  sleep 0.1
  attempt=$((attempt + 1))
done
curl --fail --silent "http://localhost:${ADMIN_PORT}/api/v1/components" >/dev/null

echo "Docker backend PID=$BACKEND_PID on :$BACKEND_PORT"
echo "Admin server PID=$ADMIN_PID on :$ADMIN_PORT"

wait "$ADMIN_PID"

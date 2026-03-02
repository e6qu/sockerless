#!/usr/bin/env bash
# Generic start script for Playwright E2E tests.
# Works for backends, simulators, and the frontend.
#
# Required env vars:
#   SERVER_BIN   — path to the compiled Go binary
#   SERVER_PORT  — port to listen on
#   HEALTH_URL   — full URL for the health endpoint (e.g. http://localhost:19210/internal/v1/healthz)
#
# Optional env vars:
#   SERVER_ARGS    — extra args to pass to the binary
#   SIM_MODE=1     — use SIM_LISTEN_ADDR instead of -addr flag
#   FRONTEND_MODE=1 — use -mgmt-addr, -addr :0, -backend http://localhost:1
set -e

if [ -z "$SERVER_BIN" ] || [ -z "$SERVER_PORT" ] || [ -z "$HEALTH_URL" ]; then
  echo "ERROR: SERVER_BIN, SERVER_PORT, and HEALTH_URL must be set" >&2
  exit 1
fi

cleanup() {
  kill $SERVER_PID 2>/dev/null || true
}
trap cleanup EXIT

if [ "${SIM_MODE:-}" = "1" ]; then
  SIM_LISTEN_ADDR=":${SERVER_PORT}" "$SERVER_BIN" $SERVER_ARGS &
elif [ "${FRONTEND_MODE:-}" = "1" ]; then
  "$SERVER_BIN" -mgmt-addr ":${SERVER_PORT}" -addr ":0" -backend "http://localhost:1" $SERVER_ARGS &
else
  "$SERVER_BIN" -addr ":${SERVER_PORT}" $SERVER_ARGS &
fi
SERVER_PID=$!

# Wait for health endpoint (up to 10s)
for i in $(seq 1 100); do
  if curl -sf "$HEALTH_URL" > /dev/null 2>&1; then
    break
  fi
  if ! kill -0 $SERVER_PID 2>/dev/null; then
    echo "ERROR: server process exited unexpectedly" >&2
    exit 1
  fi
  sleep 0.1
done

if ! curl -sf "$HEALTH_URL" > /dev/null 2>&1; then
  echo "ERROR: server did not become healthy within 10s" >&2
  exit 1
fi

echo "Server PID=$SERVER_PID on :$SERVER_PORT"
wait $SERVER_PID

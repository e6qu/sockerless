#!/usr/bin/env bash
# Starts bleephub server for Playwright E2E tests.
# Expects SERVER_BIN to be set to the path of the compiled bleephub binary.
set -e

PORT="${PORT:-15555}"

"$SERVER_BIN" -addr ":${PORT}" -log-level debug &
SERVER_PID=$!

# Wait for server to be ready
for i in $(seq 1 30); do
  if curl -s "http://localhost:${PORT}/health" > /dev/null 2>&1; then
    break
  fi
  sleep 0.1
done

echo "bleephub PID=$SERVER_PID on :$PORT"

cleanup() {
  kill $SERVER_PID 2>/dev/null || true
}
trap cleanup EXIT

wait $SERVER_PID

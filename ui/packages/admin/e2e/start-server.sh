#!/usr/bin/env bash
# Starts mock backend + admin server for Playwright E2E tests.
# Expects ADMIN_BIN to be set to the path of the compiled sockerless-admin binary.
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
MOCK_PORT="${MOCK_BACKEND_PORT:-19100}"
ADMIN_PORT="${ADMIN_PORT:-19090}"

export SOCKERLESS_HOME="$(mktemp -d)"

# Start mock backend
node "$SCRIPT_DIR/mock-backend.mjs" &
MOCK_PID=$!

# Wait for mock backend to be ready
for i in $(seq 1 30); do
  if curl -s "http://localhost:${MOCK_PORT}/internal/v1/healthz" > /dev/null 2>&1; then
    break
  fi
  sleep 0.1
done

# Start admin server pointing at mock backend
"$ADMIN_BIN" \
  -addr ":${ADMIN_PORT}" \
  -backend "memory=http://localhost:${MOCK_PORT}" &
ADMIN_PID=$!

# Wait for admin to be ready
for i in $(seq 1 30); do
  if curl -s "http://localhost:${ADMIN_PORT}/api/v1/components" > /dev/null 2>&1; then
    break
  fi
  sleep 0.1
done

echo "Mock backend PID=$MOCK_PID on :$MOCK_PORT"
echo "Admin server PID=$ADMIN_PID on :$ADMIN_PORT"

# Keep running until killed
cleanup() {
  kill $ADMIN_PID 2>/dev/null || true
  kill $MOCK_PID 2>/dev/null || true
  rm -rf "$SOCKERLESS_HOME"
}
trap cleanup EXIT

wait $ADMIN_PID

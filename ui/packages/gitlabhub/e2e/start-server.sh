#!/usr/bin/env bash
set -e
PORT="${PORT:-15556}"
"$SERVER_BIN" -addr ":${PORT}" -log-level debug &
SERVER_PID=$!

# Wait for server to be ready (poll /health for up to 3 seconds)
for i in $(seq 1 30); do
  if curl -s "http://localhost:${PORT}/health" > /dev/null 2>&1; then
    break
  fi
  sleep 0.1
done

echo "gitlabhub PID=$SERVER_PID on :$PORT"
cleanup() { kill $SERVER_PID 2>/dev/null || true; }
trap cleanup EXIT
wait $SERVER_PID

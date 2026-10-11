#!/usr/bin/env bash
# Lambda Runtime API + agent-as-handler — overlay build, docker exec.
# Requires SOCKERLESS_CALLBACK_URL pointing at a publicly-reachable
# endpoint that mounts /v1/lambda/reverse (see ngrok / cloudflared).
set -euo pipefail

: "${AWS_REGION:=eu-west-1}"
: "${ECS_OUT:=/tmp/ecs-out.json}"
: "${AWS_INFRA_OUTPUT:=/tmp/aws-infra-out.json}"
export AWS_REGION

# shellcheck source=smoke-tests/aws-live/lib.sh
source "$(dirname "$0")/lib.sh"
require_file "$ECS_OUT"
require_file "$AWS_INFRA_OUTPUT"
lambda_backend_env "$ECS_OUT" "$AWS_INFRA_OUTPUT"

BACKEND_BIN="${BACKEND_BIN:-./sockerless-backend-lambda}"
cleanup() {
  if [ -n "${BACKEND_PID:-}" ]; then
    kill "$BACKEND_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

echo "=== Lambda backend + agent-as-handler ==="
start_backend "$BACKEND_BIN" 127.0.0.1:2376 /tmp/lambda-backend.log

export DOCKER_HOST="tcp://127.0.0.1:2376"

echo "--- 3.1 docker run -d alpine sleep 60 (overlay build + reverse-agent) ---"
# docker start returns once the bootstrap inside the function has dialled
# back, so the container is ready for exec when run -d returns.
CID=$(docker run -d --name skls-r3 alpine:latest sleep 60)

echo "--- 3.2 docker exec ---"
docker exec "$CID" echo "exec-via-reverse-agent" | grep exec-via-reverse-agent

echo "--- 3.3 docker logs -f tail ---"
# The follow outlives the 5-second window by design; timeout's 124 is the
# expected end of the stream, any other failure is a failure.
status=0
timeout 5 docker logs -f "$CID" || status=$?
if [ "$status" -ne 0 ] && [ "$status" -ne 124 ]; then
  echo "docker logs -f failed with $status" >&2
  exit 1
fi

echo "--- 3.4 docker kill + NoSuchContainer on subsequent exec ---"
docker kill "$CID"
if docker exec "$CID" echo should-fail 2>&1 | grep -q 'No such container'; then
  echo "post-kill exec correctly returned NoSuchContainer"
else
  echo "FAIL: post-kill exec did not return NoSuchContainer" >&2
  exit 1
fi
docker rm "$CID"

echo "=== complete ==="

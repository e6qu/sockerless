#!/usr/bin/env bash
# Lambda baseline against live AWS — docker run, logs, invoke.
# docker run against the Lambda backend: create, invoke, logs, rm.
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

echo "=== starting Lambda backend on :2376 ==="
start_backend "$BACKEND_BIN" 127.0.0.1:2376 /tmp/lambda-backend.log

export DOCKER_HOST="tcp://127.0.0.1:2376"

echo "--- 2.1 docker run --rm echo ---"
docker run --rm alpine:latest echo "hello-from-lambda"

echo "--- 2.2 docker run -d + logs ---"
CID=$(docker run -d --name skls-r2 alpine:latest echo "lambda-baseline")
docker wait "$CID"
docker logs "$CID" | grep lambda-baseline
docker rm -f "$CID"

echo "=== complete ==="

#!/usr/bin/env bash
# Lambda baseline against live AWS — docker run, logs, invoke.
# docker run against the Lambda backend: create, invoke, logs, rm.
set -euo pipefail

: "${AWS_REGION:=eu-west-1}"
: "${AWS_INFRA_OUTPUT:=/tmp/aws-infra-out.json}"

if [ ! -f "$AWS_INFRA_OUTPUT" ]; then
  echo "missing $AWS_INFRA_OUTPUT — run 0-infra-up.sh first" >&2
  exit 1
fi

jq_val() { jq -r ".$1.value" "$AWS_INFRA_OUTPUT"; }
SOCKERLESS_LAMBDA_ROLE_ARN="$(jq_val execution_role_arn)"
export SOCKERLESS_LAMBDA_ROLE_ARN
SOCKERLESS_LAMBDA_LOG_GROUP="$(jq_val log_group_name)"
export SOCKERLESS_LAMBDA_LOG_GROUP
export AWS_REGION

BACKEND_BIN="${BACKEND_BIN:-./sockerless-backend-lambda}"
cleanup() {
  if [ -n "${BACKEND_PID:-}" ]; then
    kill "$BACKEND_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

echo "=== starting Lambda backend on :2376 ==="
"$BACKEND_BIN" --addr 127.0.0.1:2376 --log-level debug 2>/tmp/lambda-backend.log &
BACKEND_PID=$!
sleep 2

export DOCKER_HOST="tcp://127.0.0.1:2376"

echo "--- 2.1 docker run --rm echo ---"
docker run --rm alpine:latest echo "hello-from-lambda"

echo "--- 2.2 docker run -d + logs ---"
CID=$(docker run -d --name skls-r2 alpine:latest echo "lambda-baseline")
sleep 5
docker logs "$CID" | grep lambda-baseline
docker rm -f "$CID"

echo "=== complete ==="

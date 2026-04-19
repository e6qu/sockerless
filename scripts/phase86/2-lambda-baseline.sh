#!/usr/bin/env bash
# Phase 86 Runbook 2 — Lambda baseline against live AWS.
# docker run against the Lambda backend: create, invoke, logs, rm.
set -euo pipefail

: "${AWS_REGION:=eu-west-1}"
: "${ECS_OUT:=/tmp/ecs-out.json}"

if [ ! -f "$ECS_OUT" ]; then
  echo "missing $ECS_OUT — run 0-infra-up.sh first" >&2
  exit 1
fi

jq_val() { jq -r ".$1.value" "$ECS_OUT"; }
SOCKERLESS_LAMBDA_ROLE_ARN="$(jq_val lambda_role_arn)"
export SOCKERLESS_LAMBDA_ROLE_ARN
export SOCKERLESS_LAMBDA_LOG_GROUP="/sockerless/live/lambda"
export AWS_REGION

BACKEND_BIN="${BACKEND_BIN:-./sockerless-backend-lambda}"
cleanup() { kill "${BACKEND_PID:-0}" 2>/dev/null || true; }
trap cleanup EXIT

echo "=== Phase 86 Runbook 2: starting Lambda backend on :2376 ==="
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

echo "=== Runbook 2 complete ==="

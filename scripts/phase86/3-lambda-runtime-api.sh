#!/usr/bin/env bash
# Phase 86 Runbook 3 — Lambda Runtime-API + agent-as-handler.
# Requires SOCKERLESS_CALLBACK_URL pointing at a publicly-reachable
# endpoint that mounts /v1/lambda/reverse (see ngrok / cloudflared).
set -euo pipefail

: "${SOCKERLESS_CALLBACK_URL:?SOCKERLESS_CALLBACK_URL is required (wss://...)}"
: "${AWS_REGION:=eu-west-1}"
: "${ECS_OUT:=/tmp/ecs-out.json}"

jq_val() { jq -r ".$1.value" "$ECS_OUT"; }
SOCKERLESS_LAMBDA_ROLE_ARN="$(jq_val lambda_role_arn)"
export SOCKERLESS_LAMBDA_ROLE_ARN
export SOCKERLESS_AGENT_BINARY="${SOCKERLESS_AGENT_BINARY:-/opt/sockerless/sockerless-agent}"
export SOCKERLESS_LAMBDA_BOOTSTRAP="${SOCKERLESS_LAMBDA_BOOTSTRAP:-/opt/sockerless/sockerless-lambda-bootstrap}"
export AWS_REGION

BACKEND_BIN="${BACKEND_BIN:-./sockerless-backend-lambda}"
cleanup() { kill "${BACKEND_PID:-0}" 2>/dev/null || true; }
trap cleanup EXIT

echo "=== Phase 86 Runbook 3: Lambda backend + agent-as-handler ==="
"$BACKEND_BIN" --addr 127.0.0.1:2376 --log-level debug 2>/tmp/lambda-backend.log &
BACKEND_PID=$!
sleep 2

export DOCKER_HOST="tcp://127.0.0.1:2376"

echo "--- 3.1 docker run -d alpine sleep 60 (overlay build + reverse-agent) ---"
CID=$(docker run -d --name skls-r3 alpine:latest sleep 60)
sleep 10

echo "--- 3.2 docker exec ---"
docker exec "$CID" echo "exec-via-reverse-agent" | grep exec-via-reverse-agent

echo "--- 3.3 docker logs -f tail ---"
timeout 5 docker logs -f "$CID" || true

echo "--- 3.4 docker kill + NoSuchContainer on subsequent exec ---"
docker kill "$CID"
if docker exec "$CID" echo should-fail 2>&1 | grep -q 'No such container'; then
  echo "post-kill exec correctly returned NoSuchContainer"
else
  echo "FAIL: post-kill exec did not return NoSuchContainer" >&2
  exit 1
fi
docker rm "$CID"

echo "=== Runbook 3 complete ==="

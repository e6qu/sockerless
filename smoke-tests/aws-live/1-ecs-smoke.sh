#!/usr/bin/env bash
# ECS basic smoke against live AWS — docker run, logs, cross-container DNS.
# Backend binds to 127.0.0.1:3375; DOCKER_HOST points clients at it.
set -euo pipefail

: "${AWS_REGION:=eu-west-1}"
: "${ECS_OUT:=/tmp/ecs-out.json}"
export AWS_REGION

# shellcheck source=smoke-tests/aws-live/lib.sh
source "$(dirname "$0")/lib.sh"
require_file "$ECS_OUT"
ecs_backend_env "$ECS_OUT"

BACKEND_BIN="${BACKEND_BIN:-./sockerless-backend-ecs}"
cleanup() { kill "${BACKEND_PID:-0}" 2>/dev/null || true; }
trap cleanup EXIT

echo "=== starting ECS backend on :3375 ==="
start_backend "$BACKEND_BIN" 127.0.0.1:3375 /tmp/backend.log

export DOCKER_HOST="tcp://127.0.0.1:3375"

echo "--- 1.1 docker run --rm alpine echo ---"
docker run --rm alpine:latest echo "hello-from-live-fargate"

echo "--- 1.2 docker run -d + logs ---"
CID=$(docker run -d --name skls-r1 alpine:latest sh -c 'for i in 1 2 3; do echo tick-$i; sleep 1; done; echo done')
docker wait "$CID"
docker logs "$CID" | grep tick-2
docker rm -f "$CID"

echo "--- 1.3 cross-container DNS ---"
docker network create skls-r1-net
docker run -d --name svc --network skls-r1-net alpine:latest sh -c 'nc -l -p 8080 -e echo hello-from-svc'
# The client retries until the service task is up and listening.
docker run --rm --network skls-r1-net alpine:latest \
  sh -c 'for i in $(seq 1 60); do nc svc 8080 && exit 0; sleep 1; done; exit 1' | grep hello-from-svc
docker rm -f svc
docker network rm skls-r1-net

echo "=== ECS smoke complete ==="

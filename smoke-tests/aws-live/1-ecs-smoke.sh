#!/usr/bin/env bash
# ECS basic smoke against live AWS — docker run, logs, cross-container DNS.
# Backend binds to 127.0.0.1:3375; DOCKER_HOST points clients at it.
set -euo pipefail

: "${AWS_REGION:=eu-west-1}"
: "${ECS_OUT:=/tmp/ecs-out.json}"

if [ ! -f "$ECS_OUT" ]; then
  echo "missing $ECS_OUT — run 0-infra-up.sh first" >&2
  exit 1
fi

jq_val() { jq -r ".$1.value" "$ECS_OUT"; }
SOCKERLESS_ECS_CLUSTER="$(jq_val ecs_cluster_name)"
SOCKERLESS_ECS_SUBNETS="$(jq_val ecs_private_subnets | tr -d '[]" ' | tr ',' ',')"
SOCKERLESS_ECS_EXECUTION_ROLE_ARN="$(jq_val ecs_execution_role_arn)"
SOCKERLESS_ECS_TASK_ROLE_ARN="$(jq_val ecs_task_role_arn)"
SOCKERLESS_ECR_REPO="$(jq_val ecr_repo_url)"
SOCKERLESS_CLOUDMAP_NAMESPACE="$(jq_val cloudmap_namespace_id)"
export SOCKERLESS_ECS_CLUSTER SOCKERLESS_ECS_SUBNETS SOCKERLESS_ECS_EXECUTION_ROLE_ARN
export SOCKERLESS_ECS_TASK_ROLE_ARN SOCKERLESS_ECR_REPO SOCKERLESS_CLOUDMAP_NAMESPACE
export AWS_REGION

BACKEND_BIN="${BACKEND_BIN:-./sockerless-backend-ecs}"
cleanup() { kill "${BACKEND_PID:-0}" 2>/dev/null || true; }
trap cleanup EXIT

echo "=== starting ECS backend on :3375 ==="
"$BACKEND_BIN" --addr 127.0.0.1:3375 --log-level debug 2>/tmp/backend.log &
BACKEND_PID=$!
sleep 2

export DOCKER_HOST="tcp://127.0.0.1:3375"

echo "--- 1.1 docker run --rm alpine echo ---"
docker run --rm alpine:latest echo "hello-from-live-fargate"

echo "--- 1.2 docker run -d + logs ---"
CID=$(docker run -d --name skls-r1 alpine:latest sh -c 'for i in 1 2 3; do echo tick-$i; sleep 1; done; echo done')
sleep 6
docker logs "$CID" | grep tick-2
docker rm -f "$CID"

echo "--- 1.3 cross-container DNS ---"
docker network create skls-r1-net
docker run -d --name svc --network skls-r1-net alpine:latest sh -c 'nc -l -p 8080 -e echo hello-from-svc'
docker run --rm --network skls-r1-net alpine:latest sh -c 'sleep 3; nc svc 8080' | grep hello-from-svc
docker rm -f svc
docker network rm skls-r1-net

echo "=== ECS smoke complete ==="

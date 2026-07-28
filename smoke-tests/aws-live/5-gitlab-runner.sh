#!/usr/bin/env bash
# Register and run a gitlab.com runner whose container executes on the live
# Amazon Elastic Container Service (ECS) backend.
set -euo pipefail

: "${GITLAB_RUNNER_TOKEN:?GITLAB_RUNNER_TOKEN is required}"
: "${GITLAB_URL:=https://gitlab.com/}"
: "${AWS_REGION:=eu-west-1}"
: "${ECS_OUT:=/tmp/ecs-out.json}"
: "${BACKEND_BIN:=./sockerless-backend-ecs}"

if [ ! -f "$ECS_OUT" ]; then
  echo "missing $ECS_OUT — run 0-infra-up.sh first" >&2
  exit 1
fi

jq_val() { jq -r ".$1.value" "$ECS_OUT"; }
SOCKERLESS_ECS_CLUSTER="$(jq_val ecs_cluster_name)"
SOCKERLESS_ECS_SUBNETS="$(jq_val ecs_private_subnets | tr -d '[]" ')"
SOCKERLESS_ECS_EXECUTION_ROLE_ARN="$(jq_val ecs_execution_role_arn)"
SOCKERLESS_ECS_TASK_ROLE_ARN="$(jq_val ecs_task_role_arn)"
SOCKERLESS_ECR_REPO="$(jq_val ecr_repo_url)"
SOCKERLESS_CLOUDMAP_NAMESPACE="$(jq_val cloudmap_namespace_id)"
export AWS_REGION SOCKERLESS_CLOUDMAP_NAMESPACE SOCKERLESS_ECR_REPO
export SOCKERLESS_ECS_CLUSTER SOCKERLESS_ECS_EXECUTION_ROLE_ARN
export SOCKERLESS_ECS_SUBNETS SOCKERLESS_ECS_TASK_ROLE_ARN

"$BACKEND_BIN" --addr 127.0.0.1:3376 --log-level debug 2>/tmp/gitlab-runner-backend.log &
BACKEND_PID=$!
trap 'kill "$BACKEND_PID" 2>/dev/null || :' EXIT

for _ in $(seq 1 30); do
  if curl --fail --silent http://127.0.0.1:3376/_ping >/dev/null; then
    break
  fi
  sleep 1
done
curl --fail --silent http://127.0.0.1:3376/_ping >/dev/null
export DOCKER_HOST=tcp://127.0.0.1:3376

runner_name="sockerless-ecs-${GITHUB_RUN_ID:-manual}-${GITHUB_RUN_ATTEMPT:-1}"
container_id="$(
  docker run --detach \
    --name "$runner_name" \
    --env GITLAB_URL="$GITLAB_URL" \
    --env GITLAB_RUNNER_TOKEN="$GITLAB_RUNNER_TOKEN" \
    --env RUNNER_NAME="$runner_name" \
    gitlab/gitlab-runner:alpine \
    sh -ec 'gitlab-runner register --non-interactive --url "$GITLAB_URL" --token "$GITLAB_RUNNER_TOKEN" --name "$RUNNER_NAME" --executor shell && exec gitlab-runner run --user=gitlab-runner --working-directory=/home/gitlab-runner'
)"

for _ in $(seq 1 60); do
  if docker ps --quiet --filter "id=$container_id" | grep -q .; then
    if ! logs="$(docker logs "$container_id" 2>&1)"; then
      echo "failed to read gitlab.com runner logs from $container_id" >&2
      exit 1
    fi
    if grep -q "Starting multi-runner" <<<"$logs"; then
      echo "gitlab.com runner $runner_name registered and polling through Amazon ECS"
      exit 0
    fi
  fi
  sleep 2
done

docker logs "$container_id" >&2
echo "gitlab.com runner $runner_name did not register and begin polling" >&2
exit 1

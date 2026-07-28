#!/usr/bin/env bash
# Register an ephemeral github.com Actions runner whose container executes on
# the live Amazon Elastic Container Service (ECS) backend.
set -euo pipefail

: "${GITHUB_PAT:?GITHUB_PAT is required (repository administration scope)}"
: "${GITHUB_REPO:?GITHUB_REPO is required (owner/repository)}"
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

"$BACKEND_BIN" --addr 127.0.0.1:3375 --log-level debug 2>/tmp/github-runner-backend.log &
BACKEND_PID=$!
trap 'kill "$BACKEND_PID" 2>/dev/null || :' EXIT

for _ in $(seq 1 30); do
  if curl --fail --silent http://127.0.0.1:3375/_ping >/dev/null; then
    break
  fi
  sleep 1
done
curl --fail --silent http://127.0.0.1:3375/_ping >/dev/null
export DOCKER_HOST=tcp://127.0.0.1:3375

registration_json="$(
  curl --fail --silent --show-error \
    --request POST \
    --header "Accept: application/vnd.github+json" \
    --header "Authorization: Bearer $GITHUB_PAT" \
    --header "X-GitHub-Api-Version: 2022-11-28" \
    "https://api.github.com/repos/$GITHUB_REPO/actions/runners/registration-token"
)"
registration_token="$(jq -er .token <<<"$registration_json")"
runner_name="sockerless-ecs-${GITHUB_RUN_ID:-manual}-${GITHUB_RUN_ATTEMPT:-1}"

container_id="$(
  docker run --detach \
    --name "$runner_name" \
    --env REPO_URL="https://github.com/$GITHUB_REPO" \
    --env RUNNER_TOKEN="$registration_token" \
    --env RUNNER_NAME="$runner_name" \
    --env RUNNER_SCOPE=repo \
    --env LABELS=sockerless-ecs \
    --env EPHEMERAL=true \
    myoung34/github-runner:latest
)"

for _ in $(seq 1 60); do
  runner_state="$(
    curl --fail --silent --show-error \
      --header "Accept: application/vnd.github+json" \
      --header "Authorization: Bearer $GITHUB_PAT" \
      --header "X-GitHub-Api-Version: 2022-11-28" \
      "https://api.github.com/repos/$GITHUB_REPO/actions/runners?per_page=100" |
      jq -r --arg name "$runner_name" '.runners[] | select(.name == $name) | .status'
  )"
  if [ "$runner_state" = "online" ]; then
    echo "github.com runner $runner_name registered online through Amazon ECS"
    exit 0
  fi
  sleep 2
done

docker logs "$container_id" >&2
echo "github.com runner $runner_name did not become online" >&2
exit 1

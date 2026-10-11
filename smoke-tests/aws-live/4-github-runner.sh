#!/usr/bin/env bash
# Register an ephemeral github.com Actions runner whose container executes on
# the live Amazon Elastic Container Service (ECS) backend.
set -euo pipefail

: "${GITHUB_PAT:?GITHUB_PAT is required (repository administration scope)}"
: "${GITHUB_REPO:?GITHUB_REPO is required (owner/repository)}"
: "${AWS_REGION:=eu-west-1}"
: "${ECS_OUT:=/tmp/ecs-out.json}"
: "${BACKEND_BIN:=./sockerless-backend-ecs}"

export AWS_REGION

# shellcheck source=smoke-tests/aws-live/lib.sh
source "$(dirname "$0")/lib.sh"
require_file "$ECS_OUT"
ecs_backend_env "$ECS_OUT"

trap 'kill "${BACKEND_PID:-0}" 2>/dev/null || :' EXIT
start_backend "$BACKEND_BIN" 127.0.0.1:3375 /tmp/github-runner-backend.log
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

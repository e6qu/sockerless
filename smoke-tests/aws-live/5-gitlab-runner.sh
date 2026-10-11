#!/usr/bin/env bash
# Register and run a gitlab.com runner whose container executes on the live
# Amazon Elastic Container Service (ECS) backend.
set -euo pipefail

: "${GITLAB_RUNNER_TOKEN:?GITLAB_RUNNER_TOKEN is required}"
: "${GITLAB_URL:=https://gitlab.com/}"
: "${AWS_REGION:=eu-west-1}"
: "${ECS_OUT:=/tmp/ecs-out.json}"
: "${BACKEND_BIN:=./sockerless-backend-ecs}"

export AWS_REGION

# shellcheck source=smoke-tests/aws-live/lib.sh
source "$(dirname "$0")/lib.sh"
require_file "$ECS_OUT"
ecs_backend_env "$ECS_OUT"

trap 'kill "${BACKEND_PID:-0}" 2>/dev/null || :' EXIT
start_backend "$BACKEND_BIN" 127.0.0.1:3376 /tmp/gitlab-runner-backend.log
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

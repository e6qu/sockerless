#!/usr/bin/env bash
# Shared by the live-AWS smoke scripts: the backends' environment from the
# Terraform outputs 0-infra-up.sh caches, and a backend started and waited on.
# The mappings are the ones tests/terraform-integration/run-test.sh applies to
# the same modules against the simulator.

# out_val FILE KEY prints a Terraform output's value; an output the module
# does not have fails the script instead of yielding the string "null".
out_val() {
  jq -er ".$2.value" "$1" || { echo "Terraform output $2 is missing from $1" >&2; return 1; }
}

# out_csv FILE KEY prints a list output joined with commas.
out_csv() {
  jq -er ".$2.value | join(\",\")" "$1" || { echo "Terraform output $2 is missing from $1" >&2; return 1; }
}

require_file() {
  if [ ! -f "$1" ]; then
    echo "missing $1 — run 0-infra-up.sh first" >&2
    exit 1
  fi
}

# ecs_backend_env FILE exports the Amazon ECS backend's coordinates from the
# ECS environment's outputs. The tasks' CPU architecture is the operator's
# choice, as it is for the module; it has no default.
ecs_backend_env() {
  local out="$1"
  : "${SOCKERLESS_ECS_CPU_ARCHITECTURE:?SOCKERLESS_ECS_CPU_ARCHITECTURE is required (X86_64 or ARM64)}"
  SOCKERLESS_ECS_CLUSTER="$(out_val "$out" ecs_cluster_name)"
  SOCKERLESS_ECS_SUBNETS="$(out_csv "$out" private_subnet_ids)"
  SOCKERLESS_ECS_SECURITY_GROUPS="$(out_val "$out" task_security_group_id)"
  SOCKERLESS_ECS_TASK_ROLE_ARN="$(out_val "$out" task_role_arn)"
  SOCKERLESS_ECS_EXECUTION_ROLE_ARN="$(out_val "$out" execution_role_arn)"
  SOCKERLESS_ECS_LOG_GROUP="$(out_val "$out" log_group_name)"
  SOCKERLESS_AGENT_EFS_ID="$(out_val "$out" efs_filesystem_id)"
  export SOCKERLESS_ECS_CPU_ARCHITECTURE SOCKERLESS_ECS_CLUSTER SOCKERLESS_ECS_SUBNETS
  export SOCKERLESS_ECS_SECURITY_GROUPS SOCKERLESS_ECS_TASK_ROLE_ARN
  export SOCKERLESS_ECS_EXECUTION_ROLE_ARN SOCKERLESS_ECS_LOG_GROUP SOCKERLESS_AGENT_EFS_ID
}

# lambda_backend_env ECS_FILE LAMBDA_FILE exports the AWS Lambda backend's
# coordinates: the network and file system the functions share with ECS, and
# the Lambda environment's role, log group, and the AWS CodeBuild project,
# bucket and Amazon ECR repository every function's bootstrap overlay is built
# with. The functions dial back to SOCKERLESS_CALLBACK_URL, an address the
# operator publishes for the backend (wss://…/v1/lambda/reverse).
lambda_backend_env() {
  local ecs_out="$1" lambda_out="$2"
  : "${SOCKERLESS_CALLBACK_URL:?SOCKERLESS_CALLBACK_URL is required (wss://…/v1/lambda/reverse, reachable from the functions)}"
  : "${SOCKERLESS_LAMBDA_ARCHITECTURE:?SOCKERLESS_LAMBDA_ARCHITECTURE is required (x86_64 or arm64, as the module was applied with)}"
  : "${SOCKERLESS_AGENT_BINARY:?SOCKERLESS_AGENT_BINARY is required (the agent the overlay carries)}"
  : "${SOCKERLESS_LAMBDA_BOOTSTRAP:?SOCKERLESS_LAMBDA_BOOTSTRAP is required (the bootstrap the overlay runs)}"
  SOCKERLESS_LAMBDA_SUBNETS="$(out_csv "$ecs_out" private_subnet_ids)"
  SOCKERLESS_LAMBDA_SECURITY_GROUPS="$(out_val "$ecs_out" task_security_group_id)"
  SOCKERLESS_LAMBDA_AGENT_EFS_ID="$(out_val "$ecs_out" efs_filesystem_id)"
  SOCKERLESS_LAMBDA_ROLE_ARN="$(out_val "$lambda_out" execution_role_arn)"
  SOCKERLESS_LAMBDA_LOG_GROUP="$(out_val "$lambda_out" log_group_name)"
  SOCKERLESS_LAMBDA_CODEBUILD_PROJECT="$(out_val "$lambda_out" codebuild_project_name)"
  SOCKERLESS_LAMBDA_BUILD_BUCKET="$(out_val "$lambda_out" build_context_bucket)"
  SOCKERLESS_LAMBDA_OVERLAY_ECR_REPO="$(out_val "$lambda_out" ecr_repository_url)"
  export SOCKERLESS_CALLBACK_URL SOCKERLESS_LAMBDA_ARCHITECTURE
  export SOCKERLESS_AGENT_BINARY SOCKERLESS_LAMBDA_BOOTSTRAP
  export SOCKERLESS_LAMBDA_SUBNETS SOCKERLESS_LAMBDA_SECURITY_GROUPS SOCKERLESS_LAMBDA_AGENT_EFS_ID
  export SOCKERLESS_LAMBDA_ROLE_ARN SOCKERLESS_LAMBDA_LOG_GROUP
  export SOCKERLESS_LAMBDA_CODEBUILD_PROJECT SOCKERLESS_LAMBDA_BUILD_BUCKET SOCKERLESS_LAMBDA_OVERLAY_ECR_REPO
}

# start_backend BIN ADDR LOG starts a backend and returns once it answers
# /_ping, failing if it exits or does not answer within a minute. The PID is
# left in BACKEND_PID for the caller's cleanup.
start_backend() {
  local bin="$1" addr="$2" log="$3"
  "$bin" --addr "$addr" --log-level debug 2>"$log" &
  BACKEND_PID=$!
  for _ in $(seq 1 60); do
    if curl --fail --silent "http://$addr/_ping" >/dev/null; then
      return 0
    fi
    if ! kill -0 "$BACKEND_PID" 2>/dev/null; then
      echo "backend $bin exited before answering /_ping:" >&2
      tail -40 "$log" >&2
      return 1
    fi
    sleep 1
  done
  echo "backend $bin did not answer /_ping at $addr within 60s:" >&2
  tail -40 "$log" >&2
  return 1
}

#!/usr/bin/env bash
# Bring up one live-AWS scratch environment and cache its Terraform outputs.
set -euo pipefail

: "${AWS_REGION:=eu-west-1}"
TG_DIR="${TG_DIR:-terraform/environments/ecs/live}"
INFRA_OUTPUT="${INFRA_OUTPUT:-/tmp/aws-infra-out.json}"

echo "=== terragrunt apply in $TG_DIR ==="
cd "$TG_DIR"
terragrunt init -reconfigure
terragrunt apply -auto-approve
terragrunt output -json > "$INFRA_OUTPUT"
echo "=== outputs cached at $INFRA_OUTPUT ==="

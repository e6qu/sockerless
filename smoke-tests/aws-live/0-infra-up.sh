#!/usr/bin/env bash
# Bring up the live-AWS scratch environment and cache terragrunt outputs
# to /tmp/ecs-out.json so downstream scripts can source env vars.
set -euo pipefail

: "${AWS_REGION:=eu-west-1}"
TG_DIR="${TG_DIR:-terraform/environments/ecs/live}"

echo "=== terragrunt apply in $TG_DIR ==="
cd "$TG_DIR"
terragrunt init -reconfigure
terragrunt apply -auto-approve
terragrunt output -json > /tmp/ecs-out.json
echo "=== outputs cached at /tmp/ecs-out.json ==="

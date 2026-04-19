#!/usr/bin/env bash
# Phase 86 Runbook 0 — bring up the live-AWS scratch environment.
# terragrunt apply; cache outputs to /tmp/ecs-out.json so downstream
# runbooks can source env vars.
set -euo pipefail

: "${AWS_REGION:=eu-west-1}"
TG_DIR="${TG_DIR:-deploy/live/ecs}"

echo "=== Phase 86 Runbook 0: terragrunt apply in $TG_DIR ==="
cd "$TG_DIR"
terragrunt init -reconfigure
terragrunt apply -auto-approve
terragrunt output -json > /tmp/ecs-out.json
echo "=== outputs cached at /tmp/ecs-out.json ==="

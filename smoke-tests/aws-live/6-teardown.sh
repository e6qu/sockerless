#!/usr/bin/env bash
# terragrunt destroy + zero-residue audit.
# Always runs under `if: always()` so a broken earlier script still
# releases scratch AWS resources.
set -euo pipefail

: "${AWS_REGION:=eu-west-1}"
: "${LAMBDA_REGION:=us-east-1}"
TG_DIR="${TG_DIR:-terraform/environments/ecs/live}"
LAMBDA_TG_DIR="${LAMBDA_TG_DIR:-terraform/environments/lambda/live}"

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

if [ -d "$REPO_ROOT/$LAMBDA_TG_DIR" ]; then
  echo "=== terragrunt destroy in $LAMBDA_TG_DIR ==="
  ( cd "$REPO_ROOT/$LAMBDA_TG_DIR" && terragrunt destroy -auto-approve || true )
fi

echo "=== terragrunt destroy in $TG_DIR ==="
cd "$REPO_ROOT/$TG_DIR"
terragrunt destroy -auto-approve || true

echo "--- residue check ---"
fail=0

clusters=$(aws ecs list-clusters --region "$AWS_REGION" --query 'clusterArns' --output text | tr '\t' '\n' | grep -i sockerless || true)
if [ -n "$clusters" ]; then
  echo "LEAK: residual ECS clusters:" >&2
  echo "$clusters" >&2
  fail=1
fi

functions=$(aws lambda list-functions --region "$LAMBDA_REGION" --query 'Functions[].FunctionName' --output text | tr '\t' '\n' | grep -E '^(skls-|sockerless-)' || true)
if [ -n "$functions" ]; then
  echo "LEAK: residual Lambda functions:" >&2
  echo "$functions" >&2
  fail=1
fi

namespaces=$(aws servicediscovery list-namespaces --region "$AWS_REGION" --query 'Namespaces[].Name' --output text | tr '\t' '\n' | grep -i sockerless || true)
if [ -n "$namespaces" ]; then
  echo "LEAK: residual Cloud Map namespaces:" >&2
  echo "$namespaces" >&2
  fail=1
fi

if [ "$fail" -ne 0 ]; then
  echo "=== teardown LEFT RESIDUE — investigate and clean manually ===" >&2
  exit 1
fi

echo "=== teardown complete (no residue) ==="

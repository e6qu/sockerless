#!/usr/bin/env bash
# Provisions the same cloud resources that an operator supplies to a backend.
# The only test-specific input is the simulator endpoint coordinate.
set -euo pipefail

if [[ -z "${SIMULATOR_URL:-}" || -z "${SIMULATOR_SETUP:-}" ]]; then
  echo "ERROR: SIMULATOR_URL and SIMULATOR_SETUP are required" >&2
  exit 1
fi

put_json() {
  local url="$1"
  local body="$2"
  curl --fail --silent --show-error \
    --request PUT \
    --header "Content-Type: application/json" \
    --data "$body" \
    "$url" >/dev/null
}

case "$SIMULATOR_SETUP" in
  aws-ecs)
    curl --fail --silent --show-error \
      --request POST \
      --header "Content-Type: application/x-amz-json-1.1" \
      --header "X-Amz-Target: AmazonEC2ContainerServiceV20141113.CreateCluster" \
      --data '{"clusterName":"sockerless-e2e"}' \
      "$SIMULATOR_URL/" >/dev/null
    ;;
  gcp)
    curl --fail --silent --show-error \
      --request POST \
      --header "Content-Type: application/json" \
      --data '{"name":"sockerless-e2e-build"}' \
      "$SIMULATOR_URL/storage/v1/b?project=sockerless-e2e" >/dev/null
    ;;
  azure-aca)
    subscription="00000000-0000-0000-0000-000000000001"
    group="sockerless-e2e"
    put_json \
      "$SIMULATOR_URL/subscriptions/$subscription/resourceGroups/$group/providers/Microsoft.Storage/storageAccounts/sockerlesse2e?api-version=2023-01-01" \
      '{"location":"eastus","sku":{"name":"Standard_LRS"},"kind":"StorageV2","properties":{}}'
    put_json \
      "$SIMULATOR_URL/subscriptions/$subscription/resourceGroups/$group/providers/Microsoft.App/managedEnvironments/sockerless-e2e?api-version=2024-03-01" \
      '{"location":"eastus","properties":{}}'
    ;;
  azure-azf)
    subscription="00000000-0000-0000-0000-000000000001"
    group="sockerless-e2e"
    put_json \
      "$SIMULATOR_URL/subscriptions/$subscription/resourceGroups/$group/providers/Microsoft.Storage/storageAccounts/sockerlesse2e?api-version=2023-01-01" \
      '{"location":"eastus","sku":{"name":"Standard_LRS"},"kind":"StorageV2","properties":{}}'
    ;;
  *)
    echo "ERROR: unknown SIMULATOR_SETUP=$SIMULATOR_SETUP" >&2
    exit 1
    ;;
esac

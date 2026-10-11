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
  # ARM_BEARER, when set by an Azure case below, carries the managed-identity
  # bearer the Azure Resource Manager control plane requires (see
  # azure_arm_bearer). The real ARM plane rejects an unauthenticated PUT; the
  # simulator now enforces the same. Non-Azure callers leave it unset.
  local auth_header=()
  if [[ -n "${ARM_BEARER:-}" ]]; then
    auth_header=(--header "Authorization: Bearer ${ARM_BEARER}")
  fi
  curl --fail --silent --show-error \
    --request PUT \
    --header "Content-Type: application/json" \
    ${auth_header[@]+"${auth_header[@]}"} \
    --data "$body" \
    "$url" >/dev/null
}

# azure_arm_bearer acquires an Azure Resource Manager bearer token from the
# simulator the exact way a managed-identity client does in production: an App
# Service MSI request against the identity endpoint (here the sim's /msi/token)
# for the ARM resource. The simulator mints a real, RS256-signed token whose
# `aud` is the management audience — the same token DefaultAzureCredential
# obtains inside the backend. It differs from real Azure only in coordinates
# (the endpoint base URL). Emits the raw access token on stdout.
azure_arm_bearer() {
  local base="$1"
  curl --fail --silent --show-error \
    --header "X-IDENTITY-HEADER: sim-identity-header" \
    "${base}/msi/token?resource=https://management.azure.com/" \
    | sed -n 's/.*"access_token"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p'
}

# hex_sha256 emits the lowercase hex SHA-256 of its stdin.
hex_sha256() {
  openssl dgst -sha256 -hex | sed 's/^.*= //'
}

# hmac_hex computes HMAC-SHA256(hexkey, data) and emits lowercase hex.
hmac_hex() {
  local hexkey="$1"
  printf '%s' "$2" | openssl dgst -sha256 -mac HMAC -macopt "hexkey:${hexkey}" | sed 's/^.*= //'
}

# aws_sigv4_request signs an AWS request with SigV4 the way a real AWS SDK / CLI
# client does and sends it. It differs from the real cloud ONLY in coordinates:
# the endpoint base URL (the simulator) and the seeded bootstrap administrator
# credential the simulator provisions (access key "test", secret "test", region
# us-east-1) — the same static credential the AWS backends configure. The
# simulator recomputes and verifies this signature at its control-plane
# chokepoint; an unsigned request is rejected with 403.
#
#   $1 method (POST for an awsJson operation, PUT for an S3 REST call)
#   $2 base URL (e.g. http://127.0.0.1:19320)
#   $3 path (/ for an awsJson operation)
#   $4 signing service (e.g. ecs, codebuild, s3)
#   $5 request body (may be empty)
#   $6 X-Amz-Target header value (awsJson operations only)
aws_sigv4_request() {
  local method="$1" base="$2" path="$3" service="$4" body="$5" target="${6:-}"
  local access_key="test" secret="test" region="us-east-1"
  local content_type="application/x-amz-json-1.1"

  local host="${base#http://}"
  host="${host#https://}"
  host="${host%%/*}"

  local amzdate datestamp
  amzdate="$(date -u +%Y%m%dT%H%M%SZ)"
  datestamp="$(date -u +%Y%m%d)"
  local payload_hash
  payload_hash="$(printf '%s' "$body" | hex_sha256)"

  local signed_headers="content-type;host;x-amz-content-sha256;x-amz-date"
  local canonical_headers="content-type:${content_type}
host:${host}
x-amz-content-sha256:${payload_hash}
x-amz-date:${amzdate}
"
  local target_header=()
  if [[ -n "$target" ]]; then
    signed_headers="${signed_headers};x-amz-target"
    canonical_headers="${canonical_headers}x-amz-target:${target}
"
    target_header=(--header "X-Amz-Target: ${target}")
  fi
  local canonical_request="${method}
${path}

${canonical_headers}
${signed_headers}
${payload_hash}"

  local scope="${datestamp}/${region}/${service}/aws4_request"
  local string_to_sign
  string_to_sign="AWS4-HMAC-SHA256
${amzdate}
${scope}
$(printf '%s' "$canonical_request" | hex_sha256)"

  local ksecret_hex
  ksecret_hex="$(printf 'AWS4%s' "$secret" | od -An -v -tx1 | tr -d ' \n')"
  local kdate kregion kservice ksigning signature
  kdate="$(hmac_hex "$ksecret_hex" "$datestamp")"
  kregion="$(hmac_hex "$kdate" "$region")"
  kservice="$(hmac_hex "$kregion" "$service")"
  ksigning="$(hmac_hex "$kservice" "aws4_request")"
  signature="$(hmac_hex "$ksigning" "$string_to_sign")"

  curl --fail --silent --show-error \
    --request "$method" \
    --header "Content-Type: ${content_type}" \
    ${target_header[@]+"${target_header[@]}"} \
    --header "X-Amz-Date: ${amzdate}" \
    --header "X-Amz-Content-Sha256: ${payload_hash}" \
    --header "Authorization: AWS4-HMAC-SHA256 Credential=${access_key}/${scope}, SignedHeaders=${signed_headers}, Signature=${signature}" \
    --data "$body" \
    "${base}${path}" >/dev/null
}

case "$SIMULATOR_SETUP" in
  aws-ecs)
    aws_sigv4_request POST "$SIMULATOR_URL" / ecs \
      '{"clusterName":"sockerless-e2e"}' \
      "AmazonEC2ContainerServiceV20141113.CreateCluster"
    ;;
  aws-lambda)
    # The build bucket and the AWS CodeBuild project every function's
    # bootstrap overlay is built with, as the Lambda Terraform module
    # provisions them.
    aws_sigv4_request PUT "$SIMULATOR_URL" /sockerless-e2e-build-context s3 ""
    aws_sigv4_request POST "$SIMULATOR_URL" / codebuild \
      '{"name":"sockerless-e2e-image-builder","source":{"type":"NO_SOURCE","buildspec":"version: 0.2\nphases:\n  build:\n    commands:\n      - docker build ."},"artifacts":{"type":"NO_ARTIFACTS"},"environment":{"type":"ARM_CONTAINER","image":"aws/codebuild/amazonlinux2-aarch64-standard:3.0","computeType":"BUILD_GENERAL1_MEDIUM","privilegedMode":true},"serviceRole":"arn:aws:iam::000000000000:role/sockerless-e2e-codebuild"}' \
      "CodeBuild_20161006.CreateProject"
    ;;
  gcp)
    # The simulator now verifies bearer tokens on its Cloud Storage data plane,
    # so obtain a real token from its GCE metadata server (the same coordinate a
    # workload uses on real GCE) and present it, exactly as the backend does.
    gcp_token="$(curl --fail --silent -H "Metadata-Flavor: Google" \
      "$SIMULATOR_URL/computeMetadata/v1/instance/service-accounts/default/token" | jq -r .access_token)"
    curl --fail --silent --show-error \
      --request POST \
      --header "Content-Type: application/json" \
      --header "Authorization: Bearer ${gcp_token}" \
      --data '{"name":"sockerless-e2e-build"}' \
      "$SIMULATOR_URL/storage/v1/b?project=sockerless-e2e" >/dev/null
    ;;
  azure-aca)
    subscription="00000000-0000-0000-0000-000000000001"
    group="sockerless-e2e"
    ARM_BEARER="$(azure_arm_bearer "$SIMULATOR_URL")"
    if [[ -z "$ARM_BEARER" ]]; then
      echo "ERROR: failed to acquire ARM bearer from $SIMULATOR_URL/msi/token" >&2
      exit 1
    fi
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
    ARM_BEARER="$(azure_arm_bearer "$SIMULATOR_URL")"
    if [[ -z "$ARM_BEARER" ]]; then
      echo "ERROR: failed to acquire ARM bearer from $SIMULATOR_URL/msi/token" >&2
      exit 1
    fi
    put_json \
      "$SIMULATOR_URL/subscriptions/$subscription/resourceGroups/$group/providers/Microsoft.Storage/storageAccounts/sockerlesse2e?api-version=2023-01-01" \
      '{"location":"eastus","sku":{"name":"Standard_LRS"},"kind":"StorageV2","properties":{}}'
    ;;
  *)
    echo "ERROR: unknown SIMULATOR_SETUP=$SIMULATOR_SETUP" >&2
    exit 1
    ;;
esac

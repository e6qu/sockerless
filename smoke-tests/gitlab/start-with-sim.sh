#!/usr/bin/env bash
set -euo pipefail

# Starts a cloud simulator and its backend in a single container.
# Used by docker-compose cloud backend overrides.

CLOUD="${CLOUD:-}"
BACKEND_ADDR="${BACKEND_ADDR:-:3375}"

wait_for_url() {
    local url="$1" max_wait="${2:-30}"
    local i=0
    while [ $i -lt $max_wait ]; do
        if curl -sf "$url" >/dev/null 2>&1; then
            return 0
        fi
        sleep 1
        i=$((i + 1))
    done
    echo "ERROR: Timed out waiting for $url"
    return 1
}

# aws_sigv4_post signs an AWS control-plane request (awsJson) with SigV4 and
# POSTs it, authenticating exactly as a real AWS SDK client does. The AWS
# simulator verifies the signature at its POST / control-plane chokepoint and
# rejects unsigned requests with 403 MissingAuthenticationToken, matching real
# AWS. This differs from a real-cloud call only in coordinates: the endpoint
# (local) and the simulator's seeded bootstrap admin credential (access
# key/secret "test"/"test", region us-east-1) — the same static credential the
# ECS backend uses.
# Args: <endpoint> <service> <x-amz-target> <json-payload>
aws_sigv4_post() {
    local endpoint="$1" service="$2" target="$3" payload="$4"
    local region="us-east-1" access_key="test" secret_key="test"
    local host amz_date datestamp
    host="$(printf '%s' "$endpoint" | sed -E 's#^https?://##; s#/.*$##')"
    amz_date="$(date -u +%Y%m%dT%H%M%SZ)"
    datestamp="$(date -u +%Y%m%d)"

    _sha256_hex() { openssl dgst -sha256 | sed 's/^.* //'; }
    _hmac_hex() { openssl dgst -sha256 -mac HMAC -macopt hexkey:"$1" | sed 's/^.* //'; }
    _str_hex() { printf '%s' "$1" | od -An -tx1 | tr -d ' \n'; }

    local payload_hash signed_headers canonical_headers canonical_request
    payload_hash="$(printf '%s' "$payload" | _sha256_hex)"
    signed_headers="content-type;host;x-amz-content-sha256;x-amz-date;x-amz-target"
    # Command substitution strips the trailing newline of the header block, so
    # the blank line the canonical-request spec requires between the headers and
    # the signed-headers list is written explicitly as the extra \n below.
    canonical_headers="$(printf 'content-type:application/x-amz-json-1.1\nhost:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\nx-amz-target:%s\n' \
        "$host" "$payload_hash" "$amz_date" "$target")"
    canonical_request="$(printf 'POST\n/\n\n%s\n\n%s\n%s' "$canonical_headers" "$signed_headers" "$payload_hash")"

    local cr_hash string_to_sign
    cr_hash="$(printf '%s' "$canonical_request" | _sha256_hex)"
    string_to_sign="$(printf 'AWS4-HMAC-SHA256\n%s\n%s/%s/%s/aws4_request\n%s' \
        "$amz_date" "$datestamp" "$region" "$service" "$cr_hash")"

    local k_date k_region k_service k_signing signature
    k_date="$(printf '%s' "$datestamp" | _hmac_hex "$(_str_hex "AWS4${secret_key}")")"
    k_region="$(printf '%s' "$region" | _hmac_hex "$k_date")"
    k_service="$(printf '%s' "$service" | _hmac_hex "$k_region")"
    k_signing="$(printf '%s' "aws4_request" | _hmac_hex "$k_service")"
    signature="$(printf '%s' "$string_to_sign" | _hmac_hex "$k_signing")"

    local authorization="AWS4-HMAC-SHA256 Credential=${access_key}/${datestamp}/${region}/${service}/aws4_request, SignedHeaders=${signed_headers}, Signature=${signature}"

    curl -sf -X POST "${endpoint}/" \
        -H "Content-Type: application/x-amz-json-1.1" \
        -H "X-Amz-Target: ${target}" \
        -H "X-Amz-Date: ${amz_date}" \
        -H "X-Amz-Content-Sha256: ${payload_hash}" \
        -H "Authorization: ${authorization}" \
        -d "$payload"
}

export SOCKERLESS_POLL_INTERVAL="500ms"

case "$CLOUD" in
    aws)
        SIM_LISTEN_ADDR=":4566" simulator-aws &
        wait_for_url "http://127.0.0.1:4566/health"
        # Create ECS cluster.
        # -f (via aws_sigv4_post's curl -sf) so a non-2xx CreateCluster aborts
        # (set -e) instead of starting the backend against a missing cluster.
        aws_sigv4_post "http://127.0.0.1:4566" "ecs" \
            "AmazonEC2ContainerServiceV20141113.CreateCluster" \
            '{"clusterName":"sim-cluster"}' >/dev/null
        export SOCKERLESS_ENDPOINT_URL="http://127.0.0.1:4566"
        export SOCKERLESS_ECS_CLUSTER="sim-cluster"
        export SOCKERLESS_ECS_SUBNETS="subnet-0123456789abcdef0"
        export SOCKERLESS_ECS_EXECUTION_ROLE_ARN="arn:aws:iam::000000000000:role/sim"
        exec sockerless-backend-ecs --addr "$BACKEND_ADDR" --log-level debug
        ;;
    gcp)
        # gRPC is a separate listener (Cloud Logging is its own API in
        # real GCP). Pin both ports explicitly so the backend can wire
        # SOCKERLESS_GCP_LOGADMIN_ENDPOINT to the gRPC port.
        SIM_LISTEN_ADDR=":4567" SIM_GCP_GRPC_PORT="4568" simulator-gcp &
        wait_for_url "http://127.0.0.1:4567/health"
        export SOCKERLESS_ENDPOINT_URL="http://127.0.0.1:4567"
        export SOCKERLESS_GCP_LOGADMIN_ENDPOINT="127.0.0.1:4568"
        export SOCKERLESS_GCR_PROJECT="sim-project"
        exec sockerless-backend-cloudrun --addr "$BACKEND_ADDR" --log-level debug
        ;;
    azure)
        SIM_LISTEN_ADDR=":4568" simulator-azure &
        wait_for_url "http://127.0.0.1:4568/health"
        export SOCKERLESS_ENDPOINT_URL="http://127.0.0.1:4568"
        export SOCKERLESS_ACA_SUBSCRIPTION_ID="00000000-0000-0000-0000-000000000001"
        export SOCKERLESS_ACA_RESOURCE_GROUP="sim-rg"
        exec sockerless-backend-aca --addr "$BACKEND_ADDR" --log-level debug
        ;;
    *)
        echo "ERROR: Unknown CLOUD: $CLOUD"
        exit 1
        ;;
esac

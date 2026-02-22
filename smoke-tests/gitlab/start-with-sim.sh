#!/usr/bin/env bash
set -euo pipefail

# Starts a cloud simulator and its backend in a single container.
# Used by docker-compose cloud backend overrides.

CLOUD="${CLOUD:-}"
BACKEND_ADDR="${BACKEND_ADDR:-:9100}"

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

case "$CLOUD" in
    aws)
        SIM_LISTEN_ADDR=":4566" simulator-aws &
        wait_for_url "http://127.0.0.1:4566/health"
        # Create ECS cluster
        curl -s -X POST http://127.0.0.1:4566/ \
            -H "Content-Type: application/x-amz-json-1.1" \
            -H "X-Amz-Target: AmazonEC2ContainerServiceV20141113.CreateCluster" \
            -d '{"clusterName":"sim-cluster"}' >/dev/null
        export SOCKERLESS_ENDPOINT_URL="http://127.0.0.1:4566"
        export SOCKERLESS_ECS_CLUSTER="sim-cluster"
        export SOCKERLESS_ECS_SUBNETS="subnet-sim"
        export SOCKERLESS_ECS_EXECUTION_ROLE_ARN="arn:aws:iam::000000000000:role/sim"
        exec sockerless-backend-ecs --addr "$BACKEND_ADDR" --log-level debug
        ;;
    gcp)
        SIM_LISTEN_ADDR=":4567" simulator-gcp &
        wait_for_url "http://127.0.0.1:4567/health"
        export SOCKERLESS_ENDPOINT_URL="http://127.0.0.1:4567"
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

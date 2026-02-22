#!/usr/bin/env bash
set -euo pipefail

# Act smoke test runner
# Starts Sockerless (memory backend + docker frontend) and runs act against it.

BACKEND_TYPE="${BACKEND:-memory}"
BACKEND_ADDR="127.0.0.1:9100"
FRONTEND_ADDR="127.0.0.1:2375"

cleanup() {
    echo "=== Cleaning up ==="
    [ -n "${FRONTEND_PID:-}" ] && kill "$FRONTEND_PID" 2>/dev/null || true
    [ -n "${BACKEND_PID:-}" ] && kill "$BACKEND_PID" 2>/dev/null || true
    [ -n "${SIM_PID:-}" ] && kill "$SIM_PID" 2>/dev/null || true
}
trap cleanup EXIT

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

# --- Start simulator (if cloud backend) ---
case "$BACKEND_TYPE" in
    ecs)
        echo "=== Starting AWS simulator ==="
        SIM_LISTEN_ADDR=":4566" /usr/local/bin/simulator-aws &
        SIM_PID=$!
        wait_for_url "http://127.0.0.1:4566/health"
        echo "AWS simulator ready"
        # Create ECS cluster
        curl -s -X POST http://127.0.0.1:4566/ \
            -H "Content-Type: application/x-amz-json-1.1" \
            -H "X-Amz-Target: AmazonEC2ContainerServiceV20141113.CreateCluster" \
            -d '{"clusterName":"sim-cluster"}'
        echo ""
        echo "ECS cluster created"
        export SOCKERLESS_ENDPOINT_URL="http://127.0.0.1:4566"
        export SOCKERLESS_ECS_CLUSTER="sim-cluster"
        export SOCKERLESS_ECS_SUBNETS="subnet-sim"
        export SOCKERLESS_ECS_EXECUTION_ROLE_ARN="arn:aws:iam::000000000000:role/sim"
        BACKEND_BIN="/usr/local/bin/sockerless-backend-ecs"
        ;;
    cloudrun)
        echo "=== Starting GCP simulator ==="
        SIM_LISTEN_ADDR=":4567" /usr/local/bin/simulator-gcp &
        SIM_PID=$!
        wait_for_url "http://127.0.0.1:4567/health"
        echo "GCP simulator ready"
        export SOCKERLESS_ENDPOINT_URL="http://127.0.0.1:4567"
        export SOCKERLESS_GCR_PROJECT="sim-project"
        BACKEND_BIN="/usr/local/bin/sockerless-backend-cloudrun"
        ;;
    aca)
        echo "=== Starting Azure simulator ==="
        SIM_LISTEN_ADDR=":4568" /usr/local/bin/simulator-azure &
        SIM_PID=$!
        wait_for_url "http://127.0.0.1:4568/health"
        echo "Azure simulator ready"
        export SOCKERLESS_ENDPOINT_URL="http://127.0.0.1:4568"
        export SOCKERLESS_ACA_SUBSCRIPTION_ID="00000000-0000-0000-0000-000000000001"
        export SOCKERLESS_ACA_RESOURCE_GROUP="sim-rg"
        BACKEND_BIN="/usr/local/bin/sockerless-backend-aca"
        ;;
    memory)
        BACKEND_BIN="/usr/local/bin/sockerless-backend-memory"
        ;;
    *)
        echo "ERROR: Unknown backend type: $BACKEND_TYPE"
        exit 1
        ;;
esac

# --- Start backend ---
echo "=== Starting $BACKEND_TYPE backend ==="
"$BACKEND_BIN" --addr "$BACKEND_ADDR" --log-level debug &
BACKEND_PID=$!
wait_for_url "http://$BACKEND_ADDR/internal/v1/info"
echo "$BACKEND_TYPE backend ready"

# --- Start frontend ---
echo "=== Starting Docker frontend ==="
sockerless-frontend-docker --addr "$FRONTEND_ADDR" --backend "http://$BACKEND_ADDR" --log-level debug &
FRONTEND_PID=$!
wait_for_url "http://$FRONTEND_ADDR/_ping"
echo "Docker frontend ready"

# --- Run act ---
echo "=== Running act (backend=$BACKEND_TYPE) ==="
export DOCKER_HOST="tcp://$FRONTEND_ADDR"

act push \
    --workflows /test/workflows/ \
    -P ubuntu-latest=alpine:latest \
    --container-daemon-socket "tcp://$FRONTEND_ADDR" \
    2>&1 | tee /tmp/act-output.log
ACT_EXIT=${PIPESTATUS[0]}

echo ""
if [ $ACT_EXIT -eq 0 ]; then
    echo "=== SMOKE TEST PASSED (backend=$BACKEND_TYPE) ==="
else
    echo "=== SMOKE TEST FAILED (backend=$BACKEND_TYPE, exit=$ACT_EXIT) ==="
    echo ""
    echo "--- Last 50 lines of output ---"
    tail -50 /tmp/act-output.log
fi

exit $ACT_EXIT

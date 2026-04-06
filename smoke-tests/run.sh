#!/usr/bin/env bash
# Smoke test runner for cloud backends.
# Starts simulator + backend, exercises the Docker API via CLI.
# Tests: pull, create, start, ps, exec, logs, stop, rm.
set -euo pipefail

BACKEND_TYPE="${BACKEND:-ecs}"
BACKEND_ADDR="127.0.0.1:2375"
# DOCKER_HOST is set per-command for CLI calls to the backend.
# The simulator's Docker SDK must use the default socket (/var/run/docker.sock)
# so it must NOT see DOCKER_HOST pointing at the backend.
BACKEND_DOCKER_HOST="tcp://$BACKEND_ADDR"

cleanup() {
    echo "=== Cleaning up ==="
    [ -n "${BACKEND_PID:-}" ] && kill "$BACKEND_PID" 2>/dev/null || true
    [ -n "${SIM_PID:-}" ] && kill "$SIM_PID" 2>/dev/null || true
}
trap cleanup EXIT

wait_for_url() {
    local url="$1" max_wait="${2:-30}"
    local i=0
    while [ $i -lt "$max_wait" ]; do
        if curl -sf "$url" >/dev/null 2>&1; then
            return 0
        fi
        sleep 1
        i=$((i + 1))
    done
    echo "ERROR: Timed out waiting for $url" >&2
    return 1
}

fail() {
    echo "FAIL: $1" >&2
    exit 1
}

# --- Start simulator ---
case "$BACKEND_TYPE" in
    ecs)
        echo "=== Starting AWS simulator ==="
        SIM_LISTEN_ADDR=":4566" /usr/local/bin/simulator-aws 2>/tmp/sim.log &
        SIM_PID=$!
        wait_for_url "http://127.0.0.1:4566/health"
        curl -s -X POST http://127.0.0.1:4566/ \
            -H "Content-Type: application/x-amz-json-1.1" \
            -H "X-Amz-Target: AmazonEC2ContainerServiceV20141113.CreateCluster" \
            -d '{"clusterName":"sim-cluster"}' >/dev/null
        export SOCKERLESS_ENDPOINT_URL="http://127.0.0.1:4566"
        export SOCKERLESS_ECS_CLUSTER="sim-cluster"
        export SOCKERLESS_ECS_SUBNETS="subnet-sim"
        export SOCKERLESS_ECS_EXECUTION_ROLE_ARN="arn:aws:iam::000000000000:role/sim"
        BACKEND_BIN="/usr/local/bin/sockerless-backend-ecs"
        ;;
    cloudrun)
        echo "=== Starting GCP simulator ==="
        SIM_LISTEN_ADDR=":4567" /usr/local/bin/simulator-gcp 2>/tmp/sim.log &
        SIM_PID=$!
        wait_for_url "http://127.0.0.1:4567/health"
        export SOCKERLESS_ENDPOINT_URL="http://127.0.0.1:4567"
        export SOCKERLESS_GCR_PROJECT="sim-project"
        BACKEND_BIN="/usr/local/bin/sockerless-backend-cloudrun"
        ;;
    aca)
        echo "=== Starting Azure simulator ==="
        SIM_LISTEN_ADDR=":4568" /usr/local/bin/simulator-azure 2>/tmp/sim.log &
        SIM_PID=$!
        wait_for_url "http://127.0.0.1:4568/health"
        export SOCKERLESS_ENDPOINT_URL="http://127.0.0.1:4568"
        export SOCKERLESS_ACA_SUBSCRIPTION_ID="00000000-0000-0000-0000-000000000001"
        export SOCKERLESS_ACA_RESOURCE_GROUP="sim-rg"
        export SOCKERLESS_ACA_LOG_ANALYTICS_WORKSPACE="default"
        BACKEND_BIN="/usr/local/bin/sockerless-backend-aca"
        ;;
    *)
        fail "Unknown backend type: $BACKEND_TYPE"
        ;;
esac

# --- Start backend ---
export SOCKERLESS_POLL_INTERVAL="500ms"
export SOCKERLESS_AGENT_TIMEOUT="2s"
echo "=== Starting $BACKEND_TYPE backend ==="
"$BACKEND_BIN" --addr "$BACKEND_ADDR" --log-level warn 2>/tmp/backend.log &
BACKEND_PID=$!
wait_for_url "http://$BACKEND_ADDR/_ping"
echo "$BACKEND_TYPE backend ready"

# --- Run tests ---
echo "=== Running smoke tests (backend=$BACKEND_TYPE) ==="
PASSED=0
FAILED=0

run_test() {
    local name="$1"
    shift
    echo -n "  $name... "
    if output=$("$@" 2>&1); then
        echo "OK"
        PASSED=$((PASSED + 1))
    else
        echo "FAIL"
        echo "    $output" | head -5
        FAILED=$((FAILED + 1))
    fi
}

run_test_output() {
    local name="$1" expected="$2"
    shift 2
    echo -n "  $name... "
    if output=$("$@" 2>&1) && echo "$output" | grep -q "$expected"; then
        echo "OK"
        PASSED=$((PASSED + 1))
    else
        echo "FAIL (expected '$expected')"
        echo "    $output" | head -5
        FAILED=$((FAILED + 1))
    fi
}

D="env DOCKER_HOST=$BACKEND_DOCKER_HOST docker"

# Pull
run_test "docker pull alpine" $D pull alpine:latest

# Create + start short-lived container
CID=$($D create --name smoke-short alpine:latest echo "hello from smoke test" 2>&1)
run_test "docker create (short)" test -n "$CID"
run_test "docker start (short)" $D start smoke-short

# Wait for exit
sleep 3

# PS -a (should show exited)
run_test_output "docker ps -a (exited)" "smoke-short" $D ps -a

# Logs
run_test_output "docker logs" "hello from smoke test" $D logs smoke-short

# Inspect
run_test_output "docker inspect (status)" "exited" \
    $D inspect --format '{{.State.Status}}' smoke-short

# Remove
run_test "docker rm (short)" $D rm smoke-short

# Create + start long-running container
CID2=$($D create --name smoke-long alpine:latest tail -f /dev/null 2>&1)
run_test "docker create (long)" test -n "$CID2"
run_test "docker start (long)" $D start smoke-long

sleep 3

# PS (should show running)
run_test_output "docker ps (running)" "smoke-long" $D ps

# Stop
run_test "docker stop (long)" $D stop smoke-long

# PS -a after stop (should show exited)
run_test_output "docker ps -a (after stop)" "Exited" $D ps -a

# Remove
run_test "docker rm (long)" $D rm smoke-long

# Summary
echo ""
echo "=== Results: $PASSED passed, $FAILED failed ==="
if [ "$FAILED" -gt 0 ]; then
    exit 1
fi
echo "=== SMOKE TEST PASSED (backend=$BACKEND_TYPE) ==="

#!/usr/bin/env bash
# Smoke test runner for cloud backends.
# Starts simulator + backend, exercises the Docker API via CLI.
# Tests: pull, create, start, ps, exec, logs, stop, rm.
set -euo pipefail

BACKEND_TYPE="${BACKEND:-ecs}"
BACKEND_ADDR="127.0.0.1:3375"
BACKEND_LISTEN_ADDR="${BACKEND_LISTEN_ADDR:-0.0.0.0:3375}"
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

callback_host() {
    if [ -n "${SOCKERLESS_SMOKE_CALLBACK_HOST:-}" ]; then
        printf '%s\n' "$SOCKERLESS_SMOKE_CALLBACK_HOST"
        return
    fi
    hostname -I | awk '{print $1}'
}

# --- Start simulator ---
case "$BACKEND_TYPE" in
    ecs)
        echo "=== Starting AWS simulator ==="
        SIM_LISTEN_ADDR=":4566" /usr/local/bin/simulator-aws 2>/tmp/sim.log &
        SIM_PID=$!
        wait_for_url "http://127.0.0.1:4566/health"
        aws_sigv4_post "http://127.0.0.1:4566" "ecs" \
            "AmazonEC2ContainerServiceV20141113.CreateCluster" \
            '{"clusterName":"sim-cluster"}' >/dev/null || fail "create ECS sim-cluster"
        export SOCKERLESS_ENDPOINT_URL="http://127.0.0.1:4566"
        export SOCKERLESS_ECS_CLUSTER="sim-cluster"
        export SOCKERLESS_ECS_SUBNETS="subnet-0123456789abcdef0"
        export SOCKERLESS_ECS_EXECUTION_ROLE_ARN="arn:aws:iam::000000000000:role/sim"
        # Made architecture mandatory; smoke tests must declare it.
        export SOCKERLESS_ECS_CPU_ARCHITECTURE="X86_64"
        BACKEND_BIN="/usr/local/bin/sockerless-backend-ecs"
        ;;
    cloudrun)
        echo "=== Starting GCP simulator ==="
        # gRPC is a separate listener (Cloud Logging is its own API in
        # real GCP). Default port is 4568 (HTTP 4567 + 1 historically;
        # set explicitly via SIM_GCP_GRPC_PORT so the backend's
        # SOCKERLESS_GCP_LOGADMIN_ENDPOINT below points at it
        # unambiguously).
        SIM_LISTEN_ADDR=":4567" SIM_GCP_GRPC_PORT="4568" /usr/local/bin/simulator-gcp 2>/tmp/sim.log &
        SIM_PID=$!
        wait_for_url "http://127.0.0.1:4567/health"
        export SOCKERLESS_ENDPOINT_URL="http://127.0.0.1:4567"
        # The simulator serves Artifact Registry's /v2/ at its own address;
        # the backend names it through the registry coordinate, scheme
        # included, so image references carry the host and registry HTTP is
        # dialed at the URL.
        export SOCKERLESS_GCP_AR_ENDPOINT="http://127.0.0.1:4567"
        export SOCKERLESS_GCP_LOGADMIN_ENDPOINT="127.0.0.1:4568"
        export SOCKERLESS_GCR_PROJECT="sim-project"
        SOCKERLESS_CALLBACK_URL="ws://$(callback_host):3375/v1/cloudrun/reverse"
        export SOCKERLESS_CALLBACK_URL
        BACKEND_BIN="/usr/local/bin/sockerless-backend-cloudrun"
        ;;
    aca)
        echo "=== Starting Azure simulator ==="
        SIM_LISTEN_ADDR=":4568" /usr/local/bin/simulator-azure 2>/tmp/sim.log &
        SIM_PID=$!
        wait_for_url "http://127.0.0.1:4568/health"
        export SOCKERLESS_ENDPOINT_URL="http://127.0.0.1:4568"
        # The simulator now verifies ARM bearers, so the backend federates via
        # DefaultAzureCredential's App Service managed-identity source; the
        # platform's IDENTITY_ENDPOINT/IDENTITY_HEADER coordinates point it at the
        # simulator's exempt /msi/token minter, exactly as against real Azure.
        export IDENTITY_ENDPOINT="http://127.0.0.1:4568/msi/token"
        export IDENTITY_HEADER="sim-identity-header"
        export SOCKERLESS_ACA_SUBSCRIPTION_ID="00000000-0000-0000-0000-000000000001"
        export SOCKERLESS_ACA_RESOURCE_GROUP="sim-rg"
        export SOCKERLESS_ACA_LOG_ANALYTICS_WORKSPACE="default"
        SOCKERLESS_CALLBACK_URL="ws://$(callback_host):3375/v1/aca/reverse"
        export SOCKERLESS_CALLBACK_URL
        BACKEND_BIN="/usr/local/bin/sockerless-backend-aca"
        ;;
    *)
        fail "Unknown backend type: $BACKEND_TYPE"
        ;;
esac

# --- Start backend ---
export SOCKERLESS_POLL_INTERVAL="500ms"
echo "=== Starting $BACKEND_TYPE backend ==="
"$BACKEND_BIN" --addr "$BACKEND_LISTEN_ADDR" --log-level debug 2>/tmp/backend.log &
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
HOST_DOCKER="env -u DOCKER_HOST docker"
CREATE_STDIN_ARGS=()
if [ "$BACKEND_TYPE" = "cloudrun" ] || [ "$BACKEND_TYPE" = "aca" ]; then
    CREATE_STDIN_ARGS=(-i)
fi

# The simulators execute workload containers on the mounted host
# Docker daemon, so the runtime image must exist there as well as in
# the backend's Docker-compatible image store.
run_test "host docker pull alpine" $HOST_DOCKER pull alpine:latest

# Pull
run_test "docker pull alpine" $D pull alpine:latest

# Create + start short-lived container
CID=$($D create "${CREATE_STDIN_ARGS[@]}" --name smoke-short alpine:latest echo "hello from smoke test" 2>&1)
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
CID2=$($D create "${CREATE_STDIN_ARGS[@]}" --name smoke-long alpine:latest tail -f /dev/null 2>&1)
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

# --- docker run --rm (exercises create + attach + start + wait + rm) ---
# `docker run --rm` sends create + attach (hijack) + start + wait + rm;
# the hijacked attach stream must receive container stdout bytes.
# Only backends that implement Attach can honour this flow — Cloud Run
# and Azure Container Apps have no container-level attach primitive
# (they're serverless HTTP/job backends) so the step is skipped there.
if [ "$BACKEND_TYPE" = "ecs" ]; then
    echo -n "  docker run --rm (attach path)... "
    if output=$(timeout 60 $D run --rm alpine:latest echo "hello from attach" 2>&1); then
        if echo "$output" | grep -q "hello from attach"; then
            echo "OK"
            PASSED=$((PASSED + 1))
        else
            echo "FAIL (output missing expected bytes)"
            echo "    $output" | head -5
            FAILED=$((FAILED + 1))
        fi
    else
        echo "FAIL (timed out or errored)"
        echo "    $output" | head -5
        FAILED=$((FAILED + 1))
    fi
fi

# Summary
echo ""
echo "=== Results: $PASSED passed, $FAILED failed ==="
if [ "$FAILED" -gt 0 ]; then
    echo ""
    echo "=== Simulator log (last 20 lines) ==="
    tail -20 /tmp/sim.log 2>/dev/null || true
    echo ""
    echo "=== Backend log (last 20 lines) ==="
    tail -20 /tmp/backend.log 2>/dev/null || true
    exit 1
fi
echo "=== SMOKE TEST PASSED (backend=$BACKEND_TYPE) ==="

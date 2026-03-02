#!/usr/bin/env bash
set -euo pipefail

# Per-cloud terraform integration test runner.
# Runs both backends for a cloud sharing a single simulator instance.
#
# Usage: ./run-cloud.sh <cloud>
#   cloud: aws | gcp | azure
#
# Optional env vars:
#   SKIP_SMOKE_TEST=1  — skip act smoke test (just test terraform apply/destroy)

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

CLOUD="${1:?Usage: $0 <cloud> (aws|gcp|azure)}"

# Map cloud to backends and port
case "$CLOUD" in
    aws)   BACKENDS=("ecs" "lambda")       ; SIM_PORT=4566 ;;
    gcp)   BACKENDS=("cloudrun" "gcf")     ; SIM_PORT=4567 ;;
    azure) BACKENDS=("aca" "azf")          ; SIM_PORT=4568 ;;
    *) echo "ERROR: Unknown cloud: $CLOUD"; exit 1 ;;
esac

SIM_DIR="$ROOT_DIR/simulators/$CLOUD"
BUILD_DIR="$ROOT_DIR/.build"
mkdir -p "$BUILD_DIR"

# --- Cleanup ---
SIM_PID=""

cleanup() {
    local exit_code=$?
    echo ""
    echo "=== Cleaning up shared $CLOUD simulator ==="
    [ -n "${SIM_PID:-}" ] && kill "$SIM_PID" 2>/dev/null || true
    exit "$exit_code"
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

# --- Build and start shared simulator ---
echo "=== Building $CLOUD simulator ==="
(cd "$SIM_DIR" && GOWORK=off go build -tags noui -o "$BUILD_DIR/simulator-$CLOUD" .)

echo "=== Starting shared $CLOUD simulator on :$SIM_PORT ==="
SIM_LISTEN_ADDR=":$SIM_PORT" "$BUILD_DIR/simulator-$CLOUD" &
SIM_PID=$!
wait_for_url "http://127.0.0.1:$SIM_PORT/health"
echo "$CLOUD simulator ready (PID=$SIM_PID)"

# --- Run each backend test ---
FAILED=0
for BACKEND in "${BACKENDS[@]}"; do
    echo ""
    echo "================================================================"
    echo "=== Running terraform integration test: $BACKEND"
    echo "================================================================"
    echo ""

    if SIM_PID_EXTERNAL="$SIM_PID" SIM_PORT="$SIM_PORT" \
       "$SCRIPT_DIR/run-test.sh" "$BACKEND"; then
        echo ""
        echo ">>> $BACKEND: PASSED"
    else
        echo ""
        echo ">>> $BACKEND: FAILED"
        FAILED=1
    fi
done

echo ""
echo "================================================================"
if [ $FAILED -eq 0 ]; then
    echo "=== ALL $CLOUD TERRAFORM INTEGRATION TESTS PASSED ==="
else
    echo "=== SOME $CLOUD TERRAFORM INTEGRATION TESTS FAILED ==="
    exit 1
fi

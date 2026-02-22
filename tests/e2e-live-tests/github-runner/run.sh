#!/usr/bin/env bash
set -euo pipefail

# E2E GitHub Actions runner test
# Starts Sockerless with the specified backend and runs act workflows against it.
#
# Usage: ./run.sh --backend <name> [--workflow <name>|all] [--mode simulator|live]

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# When running inside Docker, lib.sh is at /test/lib.sh
if [ -f "${SCRIPT_DIR}/lib.sh" ]; then
    source "${SCRIPT_DIR}/lib.sh"
elif [ -f "${SCRIPT_DIR}/../lib.sh" ]; then
    source "${SCRIPT_DIR}/../lib.sh"
fi

# --- Parse args ---
BACKEND=""
WORKFLOW="all"
MODE="simulator"

while [ $# -gt 0 ]; do
    case "$1" in
        --backend)   BACKEND="$2"; shift 2 ;;
        --workflow)  WORKFLOW="$2"; shift 2 ;;
        --mode)      MODE="$2"; shift 2 ;;
        *) log_error "Unknown arg: $1"; exit 1 ;;
    esac
done

if [ -z "$BACKEND" ]; then
    log_error "Usage: ./run.sh --backend <memory|ecs|lambda|cloudrun|gcf|aca|azf>"
    exit 1
fi

# --- Workflow discovery ---
WORKFLOW_DIR="${SCRIPT_DIR}/workflows"
if [ ! -d "$WORKFLOW_DIR" ]; then
    WORKFLOW_DIR="/test/workflows"
fi

ALL_WORKFLOWS="basic multi-step env-vars exit-codes multi-job container-action services large-output matrix custom-image working-dir outputs shell-features file-persistence job-outputs concurrent-jobs env-inheritance github-env step-outputs defaults-shell conditional-steps multi-job-data"

# --- Timestamp for logs ---
TS="$(date '+%Y%m%d-%H%M%S')"
LOG_DIR="${SCRIPT_DIR}/../logs"
mkdir -p "$LOG_DIR" 2>/dev/null || LOG_DIR="/tmp/e2e-logs"
mkdir -p "$LOG_DIR"

# --- Start backend ---
START_BACKEND="${SCRIPT_DIR}/start-backend.sh"
if [ ! -f "$START_BACKEND" ]; then
    START_BACKEND="${SCRIPT_DIR}/../start-backend.sh"
fi
if [ ! -f "$START_BACKEND" ]; then
    START_BACKEND="/test/start-backend.sh"
fi

log_info "Starting Sockerless with backend=$BACKEND mode=$MODE"

# Source start-backend.sh in a subshell-like way to get exports
# We need to eval it in the current shell to get env vars
BACKEND_ADDR="127.0.0.1:9100"
FRONTEND_ADDR="127.0.0.1:2375"
PIDFILE="/tmp/sockerless-e2e.pids"

cleanup() {
    local exit_code=$?
    log_info "Cleaning up..."
    cleanup_pids "$PIDFILE"
    exit "$exit_code"
}
trap cleanup EXIT

# Start the stack
source "$START_BACKEND" --backend "$BACKEND" --mode "$MODE" \
    --backend-addr "$BACKEND_ADDR" --frontend-addr "$FRONTEND_ADDR" --pidfile "$PIDFILE"

# --- Select workflows ---
if [ "$WORKFLOW" = "all" ]; then
    WORKFLOWS="$ALL_WORKFLOWS"
else
    WORKFLOWS="$WORKFLOW"
fi

# --- Run workflows ---
PASS=0
FAIL=0
SKIP=0
RESULTS=""

for wf in $WORKFLOWS; do
    # Route to WASM-compatible variant for memory backend
    VARIANT=$(get_test_variant "$BACKEND" "$wf")
    WF_FILE="${WORKFLOW_DIR}/${VARIANT}.yml"
    LOG_FILE="${LOG_DIR}/github-${BACKEND}-${wf}-${TS}.log"

    if [ ! -f "$WF_FILE" ]; then
        log_error "Workflow file not found: $WF_FILE"
        FAIL=$((FAIL + 1))
        RESULTS="${RESULTS}FAIL  ${wf} (file not found)\n"
        continue
    fi

    if [ "$VARIANT" != "$wf" ]; then
        log_info "Running workflow: $wf (variant: $VARIANT)"
    else
        log_info "Running workflow: $wf"
    fi

    # Run act
    set +e
    act push \
        --workflows "$WF_FILE" \
        -P ubuntu-latest=alpine:latest \
        --container-daemon-socket "tcp://$FRONTEND_ADDR" \
        2>&1 | tee "$LOG_FILE"
    ACT_EXIT=${PIPESTATUS[0]}
    set -e

    if [ $ACT_EXIT -eq 0 ]; then
        log_pass "$wf"
        PASS=$((PASS + 1))
        RESULTS="${RESULTS}PASS  ${wf}\n"
    else
        log_fail "$wf (exit=$ACT_EXIT)"
        FAIL=$((FAIL + 1))
        RESULTS="${RESULTS}FAIL  ${wf} (exit=$ACT_EXIT)\n"
    fi
done

# --- Summary ---
echo ""
echo "=============================="
echo "  E2E GitHub Runner Results"
echo "  Backend: $BACKEND"
echo "=============================="
echo "  PASS: $PASS"
echo "  FAIL: $FAIL"
echo "  SKIP: $SKIP"
echo "  Total: $((PASS + FAIL + SKIP))"
echo "=============================="
echo ""
printf "$RESULTS"

# Write summary file
SUMMARY_FILE="${LOG_DIR}/summary-github-${BACKEND}-${TS}.txt"
{
    echo "E2E GitHub Runner Results — Backend: $BACKEND"
    echo "Date: $(date)"
    echo "Mode: $MODE"
    echo ""
    echo "PASS: $PASS  FAIL: $FAIL  SKIP: $SKIP  Total: $((PASS + FAIL + SKIP))"
    echo ""
    printf "$RESULTS"
} > "$SUMMARY_FILE"

if [ $FAIL -gt 0 ]; then
    exit 1
fi
exit 0

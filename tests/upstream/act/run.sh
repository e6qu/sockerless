#!/usr/bin/env bash
set -euo pipefail

# Run nektos/act's upstream test suite against Sockerless.
#
# This is informational — failures are expected. The goal is to discover
# Docker API gaps, not to gate on passing.
#
# Usage: ./run.sh [--act-ref <tag>] [--test-filter <regex>] [--backend <name>] [--individual]
#
# Modes:
#   --individual       Run each subtest in its own go test invocation (isolates hangs)
#   --monolithic       Run all subtests in one go test invocation (faster, default for cloud)
#   (default)          Auto-detect: individual for memory, monolithic for cloud backends
#
# Inside Docker, the binaries are pre-built at /usr/local/bin/
# and the act source is at /act.

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# --- Defaults ---
ACT_REF="v0.2.84"
TEST_FILTER="TestRunEvent"
BACKEND="memory"
BACKEND_ADDR="127.0.0.1:9100"
FRONTEND_ADDR="127.0.0.1:2375"
PIDFILE="/tmp/upstream-act.pids"
ACT_SRC=""
RUN_MODE=""  # auto, individual, monolithic

# --- Parse args ---
while [ $# -gt 0 ]; do
    case "$1" in
        --act-ref)     ACT_REF="$2"; shift 2 ;;
        --test-filter) TEST_FILTER="$2"; shift 2 ;;
        --act-src)     ACT_SRC="$2"; shift 2 ;;
        --backend)     BACKEND="$2"; shift 2 ;;
        --individual)  RUN_MODE="individual"; shift ;;
        --monolithic)  RUN_MODE="monolithic"; shift ;;
        *) echo "Unknown arg: $1" >&2; exit 1 ;;
    esac
done

# Auto-detect run mode: individual for memory (WASM hangs), monolithic for cloud
if [ -z "$RUN_MODE" ]; then
    if [ "$BACKEND" = "memory" ]; then
        RUN_MODE="individual"
    else
        RUN_MODE="monolithic"
    fi
fi

# --- Logging ---
log() { echo "[$(date '+%H:%M:%S')] $*"; }

# --- Cleanup ---
cleanup() {
    local rc=$?
    log "Cleaning up..."
    if [ -f "$PIDFILE" ]; then
        while IFS= read -r pid; do
            [ -n "$pid" ] && kill "$pid" 2>/dev/null || true
        done < "$PIDFILE"
        rm -f "$PIDFILE"
    fi
    exit "$rc"
}
trap cleanup EXIT

# --- Clone act (if no source provided) ---
if [ -z "$ACT_SRC" ]; then
    if [ -d "/act" ]; then
        ACT_SRC="/act"
        log "Using pre-cloned act source at /act"
    else
        ACT_SRC="$(mktemp -d)"
        log "Cloning nektos/act@${ACT_REF} into ${ACT_SRC}"
        git clone --depth 1 --branch "$ACT_REF" https://github.com/nektos/act.git "$ACT_SRC"
    fi
fi

# --- Start Sockerless ---
if [ "$BACKEND" = "memory" ]; then
    # Simple direct startup for memory backend
    log "Starting Sockerless memory backend on $BACKEND_ADDR"
    sockerless-backend-memory --addr "$BACKEND_ADDR" --log-level warn &
    echo $! >> "$PIDFILE"

    for i in $(seq 1 30); do
        if curl -sf "http://$BACKEND_ADDR/internal/v1/info" >/dev/null 2>&1; then break; fi
        sleep 1
    done

    log "Starting Docker frontend on $FRONTEND_ADDR"
    sockerless-frontend-docker --addr "$FRONTEND_ADDR" --backend "http://$BACKEND_ADDR" --log-level warn &
    echo $! >> "$PIDFILE"

    for i in $(seq 1 30); do
        if curl -sf "http://$FRONTEND_ADDR/_ping" >/dev/null 2>&1; then break; fi
        sleep 1
    done
else
    # Use shared start-backend.sh for simulator backends
    log "Starting Sockerless $BACKEND backend via start-backend.sh"
    source "${SCRIPT_DIR}/start-backend.sh" \
        --backend "$BACKEND" \
        --backend-addr "$BACKEND_ADDR" \
        --frontend-addr "$FRONTEND_ADDR" \
        --pidfile "$PIDFILE"
fi

log "Sockerless stack ready ($BACKEND): DOCKER_HOST=tcp://$FRONTEND_ADDR"

# --- Run act tests ---
export DOCKER_HOST="tcp://$FRONTEND_ADDR"
export ACT_TEST_IMAGE="alpine:latest"

RESULTS_FILE="/tmp/upstream-act-results.log"
> "$RESULTS_FILE"

cd "$ACT_SRC"

if [ "$RUN_MODE" = "individual" ]; then
    # --- Individual mode: run each subtest in isolation ---
    log "Mode: INDIVIDUAL (per-subtest isolation, 3min timeout each)"
    log "Act source: $ACT_SRC (ref: $ACT_REF)"
    log "Backend: $BACKEND"
    log ""

    PER_TEST_TIMEOUT="3m"
    TOTAL_PASS=0
    TOTAL_FAIL=0
    TOTAL_SKIP=0
    TOTAL_TIMEOUT=0

    # Discover subtests by listing TestRunEvent
    log "Discovering subtests..."
    SUBTESTS_RAW=$(go test -list 'TestRunEvent$' -timeout 30s ./pkg/runner/ 2>&1 || true)
    # The -list output for subtests requires actually running the parent; instead parse testdata dirs
    # Act's TestRunEvent iterates over directories in pkg/runner/testdata/
    TESTDATA_DIR="${ACT_SRC}/pkg/runner/testdata"
    if [ ! -d "$TESTDATA_DIR" ]; then
        log "ERROR: testdata dir not found at $TESTDATA_DIR"
        exit 1
    fi

    # Get list of test directories (each becomes a subtest of TestRunEvent)
    SUBTESTS=()
    while IFS= read -r dir; do
        name=$(basename "$dir")
        # Skip hidden dirs
        [ "${name:0:1}" = "." ] && continue
        SUBTESTS+=("$name")
    done < <(find "$TESTDATA_DIR" -mindepth 1 -maxdepth 1 -type d | sort)

    log "Found ${#SUBTESTS[@]} subtests"

    # Also collect top-level test names (TestRunEventSecrets, etc.)
    TOP_TESTS=()
    TOP_TESTS_RAW=$(go test -list 'TestRunEvent' -timeout 30s ./pkg/runner/ 2>&1 | grep '^TestRunEvent' || true)
    while IFS= read -r t; do
        [ -z "$t" ] && continue
        # Skip plain TestRunEvent (it's the parent that runs subtests)
        [ "$t" = "TestRunEvent" ] && continue
        TOP_TESTS+=("$t")
    done <<< "$TOP_TESTS_RAW"

    if [ ${#TOP_TESTS[@]} -gt 0 ]; then
        log "Found ${#TOP_TESTS[@]} additional top-level tests: ${TOP_TESTS[*]}"
    fi

    # Run each subtest individually
    for subtest in "${SUBTESTS[@]}"; do
        # Escape special regex characters in subtest name
        escaped=$(printf '%s' "$subtest" | sed 's/[.[\*^$()+?{|]/\\&/g')
        run_name="TestRunEvent/${escaped}"

        log "--- Running: $run_name ---"
        SUBTEST_FILE="/tmp/upstream-act-subtest.log"

        set +e
        timeout --signal=KILL $((3 * 60 + 10)) \
            go test -run "^TestRunEvent\$/${escaped}\$" -v -count=1 -timeout "$PER_TEST_TIMEOUT" \
            ./pkg/runner/ > "$SUBTEST_FILE" 2>&1
        test_rc=$?
        set -e

        # Append to combined results
        cat "$SUBTEST_FILE" >> "$RESULTS_FILE"

        # Determine result
        if [ $test_rc -eq 137 ] || [ $test_rc -eq 124 ]; then
            # Killed by timeout
            log "  TIMEOUT: $subtest (killed after ${PER_TEST_TIMEOUT})"
            TOTAL_TIMEOUT=$((TOTAL_TIMEOUT + 1))
            TOTAL_FAIL=$((TOTAL_FAIL + 1))
            echo "    --- FAIL: TestRunEvent/$subtest (timeout)" >> "$RESULTS_FILE"
        elif grep -q "^[[:space:]]*--- PASS:" "$SUBTEST_FILE" 2>/dev/null; then
            log "  PASS: $subtest"
            TOTAL_PASS=$((TOTAL_PASS + 1))
        elif grep -q "^[[:space:]]*--- SKIP:" "$SUBTEST_FILE" 2>/dev/null; then
            log "  SKIP: $subtest"
            TOTAL_SKIP=$((TOTAL_SKIP + 1))
        else
            log "  FAIL: $subtest (exit=$test_rc)"
            TOTAL_FAIL=$((TOTAL_FAIL + 1))
        fi
    done

    # Run additional top-level tests
    for top_test in "${TOP_TESTS[@]}"; do
        log "--- Running: $top_test ---"
        SUBTEST_FILE="/tmp/upstream-act-subtest.log"

        set +e
        timeout --signal=KILL $((3 * 60 + 10)) \
            go test -run "^${top_test}\$" -v -count=1 -timeout "$PER_TEST_TIMEOUT" \
            ./pkg/runner/ > "$SUBTEST_FILE" 2>&1
        test_rc=$?
        set -e

        cat "$SUBTEST_FILE" >> "$RESULTS_FILE"

        if [ $test_rc -eq 137 ] || [ $test_rc -eq 124 ]; then
            log "  TIMEOUT: $top_test"
            TOTAL_TIMEOUT=$((TOTAL_TIMEOUT + 1))
            TOTAL_FAIL=$((TOTAL_FAIL + 1))
        elif grep -q "^--- PASS:" "$SUBTEST_FILE" 2>/dev/null; then
            log "  PASS: $top_test"
            TOTAL_PASS=$((TOTAL_PASS + 1))
        elif grep -q "^--- SKIP:" "$SUBTEST_FILE" 2>/dev/null; then
            log "  SKIP: $top_test"
            TOTAL_SKIP=$((TOTAL_SKIP + 1))
        else
            log "  FAIL: $top_test (exit=$test_rc)"
            TOTAL_FAIL=$((TOTAL_FAIL + 1))
        fi
    done

    TEST_EXIT=0

else
    # --- Monolithic mode: run all tests in one go test invocation ---
    log "Mode: MONOLITHIC (all subtests in one process)"
    log "Running act tests: go test -run $TEST_FILTER -v -timeout 60m ./pkg/runner/"
    log "Act source: $ACT_SRC (ref: $ACT_REF)"
    log "Backend: $BACKEND"

    set +e
    go test -run "$TEST_FILTER" -v -count=1 -timeout 60m ./pkg/runner/ 2>&1 | tee "$RESULTS_FILE"
    TEST_EXIT=$?
    set -e

    TOTAL_PASS=$(grep -c '^[[:space:]]*--- PASS:' "$RESULTS_FILE" 2>/dev/null || echo 0)
    TOTAL_FAIL=$(grep -c '^[[:space:]]*--- FAIL:' "$RESULTS_FILE" 2>/dev/null || echo 0)
    TOTAL_SKIP=$(grep -c '^[[:space:]]*--- SKIP:' "$RESULTS_FILE" 2>/dev/null || echo 0)
    TOTAL_TIMEOUT=0
fi

# --- Summary ---
log ""
log "=============================="
log "  Upstream Act Test Results"
log "  Act version: $ACT_REF"
log "  Backend: $BACKEND"
log "  Mode: $RUN_MODE"
log "=============================="

log "  PASS: $TOTAL_PASS"
log "  FAIL: $TOTAL_FAIL"
if [ "$TOTAL_TIMEOUT" -gt 0 ]; then
    log "  TIMEOUT: $TOTAL_TIMEOUT (included in FAIL)"
fi
log "  SKIP: $TOTAL_SKIP"
log "  go test exit: $TEST_EXIT"
log "=============================="
log ""

# --- Detailed results ---
log "=== PASSED subtests ==="
grep '^[[:space:]]*--- PASS:' "$RESULTS_FILE" | sed 's/^[[:space:]]*//' || true

log ""
log "=== FAILED subtests ==="
grep '^[[:space:]]*--- FAIL:' "$RESULTS_FILE" | sed 's/^[[:space:]]*//' || true

log ""
log "=== SKIPPED subtests ==="
grep '^[[:space:]]*--- SKIP:' "$RESULTS_FILE" | sed 's/^[[:space:]]*//' || true

# Copy results to a persistent location if available
if [ -d "/results" ]; then
    cp "$RESULTS_FILE" "/results/raw-output-${BACKEND}.log"
    log "Raw output saved to /results/raw-output-${BACKEND}.log"
fi

log ""
log "Full output in: $RESULTS_FILE"

# Always exit 0 — this is informational
exit 0

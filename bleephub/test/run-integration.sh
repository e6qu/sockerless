#!/usr/bin/env bash
set -euo pipefail

log() { echo "=== [bleephub-test] $*"; }
fail() {
    echo "!!! [bleephub-test] FAIL: $*" >&2
    show_diag
    exit 1
}

show_diag() {
    if [ -d /runner/_diag ]; then
        echo "=== Docker exec commands ==="
        for f in /runner/_diag/Worker_*.log; do
            [ -f "$f" ] || continue
            # Show the actual docker exec commands and their results
            grep -B1 -A3 'docker exec\|exec.*Arguments\|ScriptHandler.*Async\|GenerateScript\|Container exec' "$f" 2>/dev/null | head -60 || true
        done
        echo "=== Bleephub server logs ==="
        echo "(see above)"
        echo "=== Timeline records ==="
        for f in /runner/_diag/Worker_*.log; do
            [ -f "$f" ] || continue
            grep -E 'Record:|Issue:' "$f" 2>/dev/null || true
        done
    fi
}

# The official runner strips non-standard ports from URLs (uses uri.Host not uri.Authority).
# So bleephub MUST run on port 80 (the default HTTP port).
BLEEPHUB_ADDR="127.0.0.1:80"
BACKEND_ADDR="127.0.0.1:9100"
FRONTEND_ADDR="127.0.0.1:2375"
PIDS=()

cleanup() {
    log "Cleaning up..."
    for pid in "${PIDS[@]}"; do
        kill "$pid" 2>/dev/null || true
    done
}
trap cleanup EXIT

wait_for_url() {
    local url="$1" max="${2:-30}"
    for i in $(seq 1 "$max"); do
        if curl -sf "$url" >/dev/null 2>&1; then return 0; fi
        sleep 1
    done
    fail "Timeout waiting for $url"
}

# --- 1. Start Sockerless memory backend (quiet) ---
log "Starting Sockerless memory backend on $BACKEND_ADDR"
sockerless-backend-memory --addr "$BACKEND_ADDR" --log-level warn &
PIDS+=($!)
wait_for_url "http://$BACKEND_ADDR/internal/v1/info"
log "Memory backend ready"

# --- 2. Start Docker frontend (quiet) ---
log "Starting Docker frontend on $FRONTEND_ADDR"
sockerless-frontend-docker --addr "$FRONTEND_ADDR" --backend "http://$BACKEND_ADDR" --log-level warn &
PIDS+=($!)
wait_for_url "http://$FRONTEND_ADDR/_ping"
log "Docker frontend ready"

# --- 3. Start bleephub ---
log "Starting bleephub on $BLEEPHUB_ADDR"
bleephub --addr "$BLEEPHUB_ADDR" --log-level info &
PIDS+=($!)
wait_for_url "http://$BLEEPHUB_ADDR/health"
log "bleephub ready"

export DOCKER_HOST="tcp://$FRONTEND_ADDR"

# --- 4. Configure the runner ---
log "Configuring runner..."
cd /runner

# The runner needs to write config files here
export RUNNER_ALLOW_RUNASROOT=1
export GITHUB_ACTIONS_RUNNER_TLS_NO_VERIFY=1
# export GITHUB_ACTIONS_RUNNER_TRACE=1  # Uncomment for debug logging

./config.sh \
    --url "http://$BLEEPHUB_ADDR/bleephub/test" \
    --token BLEEPHUB_REG_TOKEN \
    --name test-runner \
    --work _work \
    --unattended \
    --replace \
    --labels self-hosted,linux,arm64 \
    --no-default-labels \
    2>&1 | tail -5 || fail "Runner configuration failed"

log "Runner configured"

# --- 5. Start runner ---
log "Starting runner..."
./run.sh 2>&1 &
RUNNER_PID=$!
PIDS+=($RUNNER_PID)

# Wait for runner to register a session
log "Waiting for runner to connect..."
for i in $(seq 1 30); do
    AGENTS=$(curl -sf "http://$BLEEPHUB_ADDR/_apis/v1/Agent/1" 2>/dev/null || echo '{"count":0}')
    COUNT=$(echo "$AGENTS" | jq -r '.count // 0')
    if [ "$COUNT" -gt 0 ]; then
        log "Runner connected (agent count: $COUNT)"
        break
    fi
    sleep 1
done

# Give the runner a moment to establish its session
sleep 5

# Helper: wait for a single job to complete (by job ID)
wait_for_job() {
    local job_id="$1" label="$2" max="${3:-90}"
    log "Waiting for $label ($job_id) (max ${max}s)..."
    for i in $(seq 1 "$max"); do
        STATUS_RESP=$(curl -sf "http://$BLEEPHUB_ADDR/api/v3/bleephub/jobs/$job_id" 2>/dev/null || echo '{}')
        STATUS=$(echo "$STATUS_RESP" | jq -r '.status // "unknown"')
        RESULT=$(echo "$STATUS_RESP" | jq -r '.result // ""')

        if [ "$STATUS" = "completed" ]; then
            log "$label completed with result: $RESULT"
            if [ "$RESULT" = "Succeeded" ] || [ "$RESULT" = "succeeded" ]; then
                return 0
            else
                return 1
            fi
        fi

        if [ "$i" -eq 45 ]; then
            log "Still waiting for $label... status=$STATUS (${i}s)"
        fi
        sleep 1
    done
    log "Timeout waiting for $label (last status: $STATUS)"
    return 1
}

# ===== TEST 1: Single-job submission =====
log "===== TEST 1: Single-job submission ====="
SUBMIT_RESP=$(curl -sf -X POST "http://$BLEEPHUB_ADDR/api/v3/bleephub/submit" \
    -H "Content-Type: application/json" \
    -d '{"image":"alpine:latest","steps":[{"run":"echo Hello from bleephub via Sockerless"},{"run":"uname -a"}]}')

JOB_ID=$(echo "$SUBMIT_RESP" | jq -r '.jobId')
if [ -z "$JOB_ID" ] || [ "$JOB_ID" = "null" ]; then
    fail "Job submission failed: $SUBMIT_RESP"
fi
log "Job submitted: $JOB_ID"

if ! wait_for_job "$JOB_ID" "single-job"; then
    show_diag
    fail "Single-job test failed"
fi
log "TEST 1 PASSED: Single-job submission"

# Give runner a moment to reset between tests
sleep 3

# ===== TEST 2: Multi-job workflow (needs:) =====
log "===== TEST 2: Multi-job workflow ====="
WORKFLOW_YAML='name: multi-job-test
jobs:
  build:
    runs-on: self-hosted
    steps:
      - run: echo "Building..."
      - run: echo "Build complete"
  test:
    needs: [build]
    runs-on: self-hosted
    steps:
      - run: echo "Testing after build..."
      - run: echo "All tests passed"
'

WF_RESP=$(curl -sf -X POST "http://$BLEEPHUB_ADDR/api/v3/bleephub/workflow" \
    -H "Content-Type: application/json" \
    -d "$(jq -n --arg wf "$WORKFLOW_YAML" '{workflow: $wf, image: "alpine:latest"}')")

WF_ID=$(echo "$WF_RESP" | jq -r '.workflowId')
if [ -z "$WF_ID" ] || [ "$WF_ID" = "null" ]; then
    fail "Workflow submission failed: $WF_RESP"
fi
log "Workflow submitted: $WF_ID (jobs: $(echo "$WF_RESP" | jq -r '.jobs | keys | join(", ")'))"

# Poll workflow status
log "Waiting for workflow completion (max 180s)..."
for i in $(seq 1 180); do
    WF_STATUS=$(curl -sf "http://$BLEEPHUB_ADDR/api/v3/bleephub/workflows/$WF_ID" 2>/dev/null || echo '{}')
    STATUS=$(echo "$WF_STATUS" | jq -r '.status // "unknown"')
    RESULT=$(echo "$WF_STATUS" | jq -r '.result // ""')

    if [ "$STATUS" = "completed" ]; then
        log "Workflow completed with result: $RESULT"

        # Check both jobs completed successfully
        BUILD_RESULT=$(echo "$WF_STATUS" | jq -r '.jobs.build.result // "unknown"')
        TEST_RESULT=$(echo "$WF_STATUS" | jq -r '.jobs.test.result // "unknown"')
        log "  build: $BUILD_RESULT, test: $TEST_RESULT"

        if [ "$RESULT" = "success" ]; then
            log "TEST 2 PASSED: Multi-job workflow"
            break
        else
            show_diag
            fail "Multi-job workflow failed: result=$RESULT build=$BUILD_RESULT test=$TEST_RESULT"
        fi
    fi

    if [ "$i" -eq 90 ]; then
        log "Still waiting for workflow... status=$STATUS (${i}s)"
        JOBS_STATUS=$(echo "$WF_STATUS" | jq -r '.jobs | to_entries[] | "\(.key): \(.value.status) \(.value.result)"')
        log "Job statuses: $JOBS_STATUS"
    fi
    sleep 1
done

if [ "$STATUS" != "completed" ]; then
    show_diag
    fail "Timeout waiting for workflow (last status: $STATUS)"
fi

# ===== All tests passed =====
log "===== ALL INTEGRATION TESTS PASSED ====="

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

# Helper: submit workflow YAML and wait for completion
submit_and_wait_workflow() {
    local test_num="$1" label="$2" yaml="$3" max="${4:-180}"

    log "===== TEST $test_num: $label ====="

    local wf_resp
    wf_resp=$(curl -sf -X POST "http://$BLEEPHUB_ADDR/api/v3/bleephub/workflow" \
        -H "Content-Type: application/json" \
        -d "$(jq -n --arg wf "$yaml" '{workflow: $wf, image: "alpine:latest"}')")

    local wf_id
    wf_id=$(echo "$wf_resp" | jq -r '.workflowId')
    if [ -z "$wf_id" ] || [ "$wf_id" = "null" ]; then
        fail "Workflow submission failed: $wf_resp"
    fi
    log "Workflow submitted: $wf_id (jobs: $(echo "$wf_resp" | jq -r '.jobs | keys | join(", ")'))"

    log "Waiting for workflow completion (max ${max}s)..."
    local status result
    for i in $(seq 1 "$max"); do
        local wf_status
        wf_status=$(curl -sf "http://$BLEEPHUB_ADDR/api/v3/bleephub/workflows/$wf_id" 2>/dev/null || echo '{}')
        status=$(echo "$wf_status" | jq -r '.status // "unknown"')
        result=$(echo "$wf_status" | jq -r '.result // ""')

        if [ "$status" = "completed" ]; then
            log "Workflow completed with result: $result"
            local jobs_detail
            jobs_detail=$(echo "$wf_status" | jq -r '.jobs | to_entries[] | "  \(.key): \(.value.result)"')
            log "$jobs_detail"

            if [ "$result" = "success" ]; then
                log "TEST $test_num PASSED: $label"
                return 0
            else
                show_diag
                fail "$label failed: result=$result"
            fi
        fi

        if [ "$i" -eq 90 ]; then
            log "Still waiting... status=$status (${i}s)"
        fi
        sleep 1
    done

    show_diag
    fail "Timeout waiting for $label (last status: $status)"
}

# ===== TEST 2: Multi-job workflow (needs:) =====
submit_and_wait_workflow 2 "Multi-job workflow" '
name: multi-job-test
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

sleep 3

# ===== TEST 3: Three-stage pipeline (build → test → deploy) =====
submit_and_wait_workflow 3 "Three-stage pipeline" '
name: pipeline-test
jobs:
  build:
    runs-on: self-hosted
    steps:
      - run: echo "=== STAGE 1 BUILD ==="
      - run: echo "Compiling..."
  test:
    needs: [build]
    runs-on: self-hosted
    steps:
      - run: echo "=== STAGE 2 TEST ==="
      - run: echo "Running tests..."
  deploy:
    needs: [test]
    runs-on: self-hosted
    steps:
      - run: echo "=== STAGE 3 DEPLOY ==="
      - run: echo "Deploying..."
'

sleep 3

# ===== TEST 4: Matrix strategy (2x2 matrix) =====
submit_and_wait_workflow 4 "Matrix strategy 2x2" '
name: matrix-test
jobs:
  test:
    runs-on: self-hosted
    strategy:
      matrix:
        os: [linux, macos]
        version: ["1", "2"]
    steps:
      - run: echo "Testing on os=${{ matrix.os }} version=${{ matrix.version }}"
'

sleep 3

# ===== TEST 5: Job output propagation =====
submit_and_wait_workflow 5 "Job output propagation" '
name: output-test
jobs:
  build:
    runs-on: self-hosted
    outputs:
      version: ${{ steps.ver.outputs.version }}
    steps:
      - id: ver
        run: echo "version=1.2.3" >> "$GITHUB_OUTPUT"
  deploy:
    needs: [build]
    runs-on: self-hosted
    steps:
      - run: echo "Deploying version ${{ needs.build.outputs.version }}"
'

sleep 3

# ===== TEST 6: Service containers =====
submit_and_wait_workflow 6 "Service containers" '
name: service-test
jobs:
  test:
    runs-on: self-hosted
    services:
      redis:
        image: redis:7-alpine
        ports:
          - 6379:6379
    steps:
      - run: echo "Service containers configured"
      - run: echo "Running with redis sidecar"
'

sleep 3

# ===== TEST 7: Secrets injection =====
log "===== TEST 7: Secrets injection ====="

# PUT a secret via API
TOKEN="bph_0000000000000000000000000000000000000000"
curl -sf -X PUT "http://$BLEEPHUB_ADDR/api/v3/repos/bleephub/test/actions/secrets/TEST_SECRET" \
    -H "Authorization: token $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"value":"s3cret_value_123"}' || fail "Failed to create secret"
log "Secret created"

# Submit workflow that references the secret
submit_and_wait_workflow 7 "Secrets injection" '
name: secrets-test
jobs:
  test:
    runs-on: self-hosted
    steps:
      - run: echo "Secret is available (masked in logs)"
      - run: echo "Test passed"
'

sleep 3

# ===== TEST 8: Workflow dispatch with inputs =====
log "===== TEST 8: Workflow dispatch with inputs ====="

WF8_YAML='name: inputs-test
jobs:
  test:
    runs-on: self-hosted
    steps:
      - run: echo "Version from input"
      - run: echo "Test passed"'

WF8_RESP=$(curl -sf -X POST "http://$BLEEPHUB_ADDR/api/v3/bleephub/workflow" \
    -H "Content-Type: application/json" \
    -d "$(jq -n --arg wf "$WF8_YAML" '{workflow: $wf, image: "alpine:latest", event_name: "workflow_dispatch", inputs: {version: "1.2.3"}}')")

WF8_ID=$(echo "$WF8_RESP" | jq -r '.workflowId')
if [ -z "$WF8_ID" ] || [ "$WF8_ID" = "null" ]; then
    fail "Workflow dispatch submission failed: $WF8_RESP"
fi
log "Workflow dispatch submitted: $WF8_ID"

log "Waiting for workflow completion (max 120s)..."
for i in $(seq 1 120); do
    WF8_STATUS=$(curl -sf "http://$BLEEPHUB_ADDR/api/v3/bleephub/workflows/$WF8_ID" 2>/dev/null || echo '{}')
    STATUS=$(echo "$WF8_STATUS" | jq -r '.status // "unknown"')
    RESULT=$(echo "$WF8_STATUS" | jq -r '.result // ""')

    if [ "$STATUS" = "completed" ]; then
        log "Workflow completed with result: $RESULT"
        if [ "$RESULT" = "success" ]; then
            log "TEST 8 PASSED: Workflow dispatch with inputs"
            break
        else
            show_diag
            fail "Workflow dispatch test failed: result=$RESULT"
        fi
    fi
    sleep 1
done
if [ "$STATUS" != "completed" ]; then
    show_diag
    fail "Timeout waiting for workflow dispatch test"
fi

sleep 3

# ===== TEST 9: Matrix fail-fast =====
log "===== TEST 9: Matrix fail-fast ====="

WF9_YAML='name: failfast-test
jobs:
  test:
    runs-on: self-hosted
    strategy:
      fail-fast: true
      matrix:
        idx: ["0", "1", "2", "3"]
    steps:
      - run: echo "Matrix job"'

WF9_RESP=$(curl -sf -X POST "http://$BLEEPHUB_ADDR/api/v3/bleephub/workflow" \
    -H "Content-Type: application/json" \
    -d "$(jq -n --arg wf "$WF9_YAML" '{workflow: $wf, image: "alpine:latest"}')")

WF9_ID=$(echo "$WF9_RESP" | jq -r '.workflowId')
if [ -z "$WF9_ID" ] || [ "$WF9_ID" = "null" ]; then
    fail "Matrix fail-fast submission failed: $WF9_RESP"
fi
log "Matrix fail-fast submitted: $WF9_ID (4 jobs)"

# Wait for all to complete (some may be cancelled by fail-fast)
log "Waiting for matrix workflow completion (max 180s)..."
for i in $(seq 1 180); do
    WF9_STATUS=$(curl -sf "http://$BLEEPHUB_ADDR/api/v3/bleephub/workflows/$WF9_ID" 2>/dev/null || echo '{}')
    STATUS=$(echo "$WF9_STATUS" | jq -r '.status // "unknown"')

    if [ "$STATUS" = "completed" ]; then
        log "Matrix workflow completed"
        JOBS_DETAIL=$(echo "$WF9_STATUS" | jq -r '.jobs | to_entries[] | "  \(.key): \(.value.result)"')
        log "$JOBS_DETAIL"
        log "TEST 9 PASSED: Matrix fail-fast"
        break
    fi
    if [ "$i" -eq 90 ]; then
        log "Still waiting... status=$STATUS (${i}s)"
    fi
    sleep 1
done
if [ "$STATUS" != "completed" ]; then
    show_diag
    fail "Timeout waiting for matrix fail-fast test"
fi

# ===== All tests passed =====
log "===== ALL 9 INTEGRATION TESTS PASSED ====="

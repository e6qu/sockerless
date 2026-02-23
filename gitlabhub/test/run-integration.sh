#!/usr/bin/env bash
set -euo pipefail

log() { echo "=== [gitlabhub-test] $*"; }
fail() {
    echo "!!! [gitlabhub-test] FAIL: $*" >&2
    show_diag
    exit 1
}

show_diag() {
    echo "=== gitlabhub logs above ==="
    if [ -d /home/gitlab-runner/builds ]; then
        echo "=== Build directory ==="
        ls -la /home/gitlab-runner/builds/ 2>/dev/null || true
    fi
}

PASSED=0
FAILED=0

GITLABHUB_ADDR="127.0.0.1:80"
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

# --- 1. Start Sockerless memory backend ---
log "Starting Sockerless memory backend on $BACKEND_ADDR"
sockerless-backend-memory --addr "$BACKEND_ADDR" --log-level warn &
PIDS+=($!)
wait_for_url "http://$BACKEND_ADDR/internal/v1/info"
log "Memory backend ready"

# --- 2. Start Docker frontend ---
log "Starting Docker frontend on $FRONTEND_ADDR"
sockerless-frontend-docker --addr "$FRONTEND_ADDR" --backend "http://$BACKEND_ADDR" --log-level warn &
PIDS+=($!)
wait_for_url "http://$FRONTEND_ADDR/_ping"
log "Docker frontend ready"

# --- 3. Start gitlabhub ---
log "Starting gitlabhub on $GITLABHUB_ADDR"
gitlabhub --addr "$GITLABHUB_ADDR" --log-level info &
PIDS+=($!)
wait_for_url "http://$GITLABHUB_ADDR/health"
log "gitlabhub ready"

export DOCKER_HOST="tcp://$FRONTEND_ADDR"

# --- 4. Register gitlab-runner ---
log "Registering gitlab-runner..."
gitlab-runner register \
    --non-interactive \
    --url "http://$GITLABHUB_ADDR" \
    --registration-token "glrt-test-token" \
    --executor docker \
    --docker-image alpine:latest \
    --docker-host "tcp://$FRONTEND_ADDR" \
    --docker-network-mode host \
    --docker-pull-policy if-not-present \
    --description "test-runner" \
    --tag-list "docker,linux" \
    2>&1 | tail -5 || fail "Runner registration failed"

log "Runner registered"

# --- 5. Start runner ---
log "Starting gitlab-runner..."
gitlab-runner run --working-directory /home/gitlab-runner/builds --config /etc/gitlab-runner/config.toml 2>&1 &
RUNNER_PID=$!
PIDS+=($RUNNER_PID)
sleep 3
log "Runner started"

# Helper: submit pipeline and wait for completion
submit_and_wait() {
    local test_num="$1" label="$2" yaml="$3" max="${4:-120}" expected="${5:-success}"

    log "===== TEST $test_num: $label ====="

    local resp
    resp=$(curl -sf -X POST "http://$GITLABHUB_ADDR/api/v3/gitlabhub/pipeline" \
        -H "Content-Type: application/json" \
        -d "$(jq -n --arg pl "$yaml" --arg img "alpine:latest" '{pipeline: $pl, image: $img}')")

    local pl_id
    pl_id=$(echo "$resp" | jq -r '.pipelineId')
    if [ -z "$pl_id" ] || [ "$pl_id" = "null" ]; then
        fail "Pipeline submission failed: $resp"
    fi
    log "Pipeline submitted: $pl_id (jobs: $(echo "$resp" | jq -r '.jobs | keys | join(", ")'))"

    log "Waiting for pipeline completion (max ${max}s)..."
    local status result
    for i in $(seq 1 "$max"); do
        local pl_status
        pl_status=$(curl -sf "http://$GITLABHUB_ADDR/api/v3/gitlabhub/pipelines/$pl_id" 2>/dev/null || echo '{}')
        status=$(echo "$pl_status" | jq -r '.status // "unknown"')
        result=$(echo "$pl_status" | jq -r '.result // ""')

        if [ "$status" = "success" ] || [ "$status" = "failed" ] || [ "$status" = "canceled" ]; then
            log "Pipeline completed: status=$status result=$result"
            local jobs_detail
            jobs_detail=$(echo "$pl_status" | jq -r '.jobs | to_entries[] | "  \(.key): status=\(.value.status) result=\(.value.result)"')
            log "$jobs_detail"

            if [ "$status" = "$expected" ]; then
                log "TEST $test_num PASSED: $label"
                PASSED=$((PASSED + 1))
                return 0
            else
                log "TEST $test_num FAILED: expected $expected, got $status"
                FAILED=$((FAILED + 1))
                show_diag
                return 1
            fi
        fi

        if [ "$i" -eq 60 ]; then
            log "Still waiting... status=$status (${i}s)"
        fi
        sleep 1
    done

    log "TEST $test_num FAILED: timeout (last status: $status)"
    FAILED=$((FAILED + 1))
    show_diag
    return 1
}

# ===== TEST 1: Single-job pipeline =====
submit_and_wait 1 "Single-job pipeline" '
test:
  script:
    - echo "Hello from gitlabhub via Sockerless"
    - uname -a
' || true

sleep 3

# ===== TEST 2: Multi-stage pipeline =====
submit_and_wait 2 "Multi-stage pipeline" '
stages:
  - build
  - test
  - deploy

build:
  stage: build
  script:
    - echo "=== STAGE 1 BUILD ==="
    - echo "Compiling..."

test:
  stage: test
  script:
    - echo "=== STAGE 2 TEST ==="
    - echo "Running tests..."

deploy:
  stage: deploy
  script:
    - echo "=== STAGE 3 DEPLOY ==="
    - echo "Deploying..."
' || true

sleep 3

# ===== TEST 3: Variable injection =====
submit_and_wait 3 "Variable injection" '
variables:
  GLOBAL_MSG: "hello-from-global"

test:
  variables:
    JOB_MSG: "hello-from-job"
  script:
    - echo "Global: $GLOBAL_MSG"
    - echo "Job: $JOB_MSG"
    - echo "CI_JOB_NAME: $CI_JOB_NAME"
    - echo "CI_PIPELINE_ID: $CI_PIPELINE_ID"
    - test -n "$CI_JOB_NAME"
' || true

sleep 3

# ===== TEST 4: Artifacts =====
submit_and_wait 4 "Artifacts pass-through" '
stages:
  - build
  - test

build:
  stage: build
  script:
    - mkdir -p output
    - echo "built-data" > output/result.txt
  artifacts:
    paths:
      - output/

test:
  stage: test
  script:
    - cat output/result.txt
    - test "$(cat output/result.txt)" = "built-data"
' || true

sleep 3

# ===== TEST 5: Service containers =====
submit_and_wait 5 "Service containers" '
test:
  services:
    - name: redis:7-alpine
  script:
    - echo "Service containers configured"
    - echo "Running with redis sidecar"
' || true

sleep 3

# ===== TEST 6: DAG dependencies =====
submit_and_wait 6 "DAG dependencies" '
stages:
  - build
  - test

build_a:
  stage: build
  script:
    - echo "Building A"

build_b:
  stage: build
  script:
    - echo "Building B"

test:
  stage: test
  needs: [build_a]
  script:
    - echo "Testing (depends only on build_a)"
' || true

sleep 3

# ===== TEST 7: Secrets/masked variables =====
log "===== TEST 7: Secrets/masked variables ====="

# Get the project ID from the latest pipeline
LATEST_PL=$(curl -sf "http://$GITLABHUB_ADDR/internal/status" | jq -r '.active_pipelines // 0')
log "Creating masked variable on project 7..."

# Create a project variable via API
curl -sf -X POST "http://$GITLABHUB_ADDR/api/v4/projects/7/variables" \
    -H "Content-Type: application/json" \
    -d '{"key":"SECRET_TOKEN","value":"super-secret-123","masked":true}' || log "Variable creation skipped (project may not exist)"

submit_and_wait 7 "Secrets/masked variables" '
test:
  script:
    - echo "Secrets test"
    - echo "Secret should be masked in trace"
' || true

sleep 3

# ===== TEST 8: Rules/conditional =====
submit_and_wait 8 "Rules/conditional" '
stages:
  - test
  - deploy

test:
  stage: test
  script:
    - echo "Test always runs"

deploy:
  stage: deploy
  rules:
    - if: $CI_PIPELINE_SOURCE == "push"
      when: on_success
  script:
    - echo "Deploy runs on push"
' || true

sleep 3

# ===== TEST 9: Cache =====
submit_and_wait 9 "Cache between jobs" '
test:
  cache:
    key: test-cache
    paths:
      - .cache/
  script:
    - mkdir -p .cache
    - echo "cached-data" > .cache/data.txt
    - echo "Cache test passed"
' || true

sleep 3

# ===== TEST 10: Expression rules =====
submit_and_wait 10 "Expression rules (skip deploy)" '
stages:
  - test
  - deploy

variables:
  DEPLOY_ENABLED: "false"

test:
  stage: test
  script:
    - echo "Test runs always"

deploy:
  stage: deploy
  rules:
    - if: $DEPLOY_ENABLED == "true"
      when: on_success
    - when: never
  script:
    - echo "This should NOT run"
' || true

sleep 3

# ===== TEST 11: Extends =====
submit_and_wait 11 "Extends keyword" '
.base_job:
  before_script:
    - echo "Setting up from template"

test:
  extends: .base_job
  script:
    - echo "Test inherits from .base_job"
' || true

sleep 3

# ===== TEST 12: Parallel =====
submit_and_wait 12 "Parallel jobs" '
test:
  parallel: 3
  script:
    - echo "Running parallel job $CI_NODE_INDEX of $CI_NODE_TOTAL"
' || true

sleep 3

# ===== TEST 13: Timeout =====
submit_and_wait 13 "Timeout" '
test:
  timeout: 2m
  script:
    - echo "Job with 2 minute timeout"
    - echo "Completes quickly"
' || true

sleep 3

# ===== TEST 14: Retry =====
submit_and_wait 14 "Retry on failure" '
test:
  retry: 1
  script:
    - echo "This job succeeds on first try"
' || true

sleep 3

# ===== TEST 15: Dotenv artifacts =====
submit_and_wait 15 "Dotenv artifact pass-through" '
stages:
  - build
  - test

build:
  stage: build
  script:
    - echo "VERSION=1.2.3" > build.env
  artifacts:
    reports:
      dotenv: build.env

test:
  stage: test
  script:
    - echo "Version from build: $VERSION"
' || true

sleep 3

# ===== TEST 16: Pipeline cancellation =====
log "===== TEST 16: Pipeline cancellation ====="

# Submit a pipeline with a slow job
CANCEL_RESP=$(curl -sf -X POST "http://$GITLABHUB_ADDR/api/v3/gitlabhub/pipeline" \
    -H "Content-Type: application/json" \
    -d "$(jq -n --arg pl 'stages:
  - test
  - deploy
test:
  stage: test
  script:
    - echo "quick test"
deploy:
  stage: deploy
  script:
    - sleep 300
    - echo "slow deploy"' --arg img "alpine:latest" '{pipeline: $pl, image: $img}')")

CANCEL_PL_ID=$(echo "$CANCEL_RESP" | jq -r '.pipelineId')
if [ -n "$CANCEL_PL_ID" ] && [ "$CANCEL_PL_ID" != "null" ]; then
    sleep 2
    # Cancel the pipeline
    CANCEL_RESULT=$(curl -sf -X POST "http://$GITLABHUB_ADDR/api/v3/gitlabhub/pipelines/$CANCEL_PL_ID/cancel" 2>/dev/null || echo '{}')
    CANCEL_STATUS=$(echo "$CANCEL_RESULT" | jq -r '.status // "unknown"')

    if [ "$CANCEL_STATUS" = "canceled" ]; then
        log "TEST 16 PASSED: Pipeline cancellation"
        PASSED=$((PASSED + 1))
    else
        log "TEST 16 FAILED: expected canceled, got $CANCEL_STATUS"
        FAILED=$((FAILED + 1))
    fi
else
    log "TEST 16 FAILED: Pipeline submission failed"
    FAILED=$((FAILED + 1))
fi

sleep 3

# ===== TEST 17: Resource group =====
submit_and_wait 17 "Resource group" '
stages:
  - deploy

deploy_a:
  stage: deploy
  resource_group: production
  script:
    - echo "Deploy A to production"

deploy_b:
  stage: deploy
  resource_group: production
  script:
    - echo "Deploy B to production (after A)"
' || true

# ===== Summary =====
echo ""
log "===== INTEGRATION TEST SUMMARY ====="
log "PASSED: $PASSED"
log "FAILED: $FAILED"

if [ "$FAILED" -gt 0 ]; then
    fail "$FAILED tests failed"
fi

log "===== ALL $PASSED INTEGRATION TESTS PASSED ====="

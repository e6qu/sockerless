#!/usr/bin/env bash
set -euo pipefail

# Run gitlab-ci-local as a CLI tool against Sockerless.
#
# NOTE: gitlab-ci-local's own test suite mocks Docker calls (initSpawnSpy),
# so running `bun test` would NOT exercise Sockerless. Instead, we run
# gitlab-ci-local as a CLI tool against sample .gitlab-ci.yml files with
# DOCKER_HOST pointing at Sockerless.
#
# Usage: ./run.sh [--backend <name>]

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# --- Defaults ---
BACKEND="memory"
BACKEND_ADDR="127.0.0.1:9100"
FRONTEND_ADDR="127.0.0.1:2375"
PIDFILE="/tmp/upstream-gcl.pids"
RESULTS_FILE="/tmp/upstream-gcl-results.log"

# --- Parse args ---
while [ $# -gt 0 ]; do
    case "$1" in
        --backend) BACKEND="$2"; shift 2 ;;
        *) echo "Unknown arg: $1" >&2; exit 1 ;;
    esac
done

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
export DOCKER_HOST="tcp://$FRONTEND_ADDR"

# --- Git config for gitlab-ci-local ---
git config --global user.name "test"
git config --global user.email "test@test.com"
git config --global init.defaultBranch main

# --- Test helpers ---
WORKDIR="$(mktemp -d)"
PASS=0
FAIL=0
TOTAL=0

make_test_dir() {
    local name="$1"
    local dir="$WORKDIR/$name"
    mkdir -p "$dir"
    echo "$dir"
}

init_git() {
    local dir="$1"
    cd "$dir" && git init -q && git add -A && git commit -q -m "init" 2>/dev/null
}

run_test() {
    local name="$1" dir="$2"
    TOTAL=$((TOTAL + 1))
    log "--- RUN: $name"

    local output rc
    set +e
    output=$(cd "$dir" && timeout 60 gitlab-ci-local --no-color 2>&1)
    rc=$?
    set -e

    echo "=== TEST: $name (exit=$rc) ===" >> "$RESULTS_FILE"
    echo "$output" >> "$RESULTS_FILE"
    echo "" >> "$RESULTS_FILE"

    if [ $rc -eq 0 ]; then
        PASS=$((PASS + 1))
        log "--- PASS: $name"
    else
        FAIL=$((FAIL + 1))
        log "--- FAIL: $name (exit=$rc)"
        echo "$output" | tail -5
    fi
}

# --- Test cases ---

# Test 1: Simple echo
TEST_DIR=$(make_test_dir echo)
cat > "$TEST_DIR/.gitlab-ci.yml" <<'YAML'
test-echo:
  image: alpine:latest
  script:
    - echo "Hello from Sockerless"
YAML
init_git "$TEST_DIR"
run_test "simple-echo" "$TEST_DIR"

# Test 2: Multi-step script
TEST_DIR=$(make_test_dir multi-step)
cat > "$TEST_DIR/.gitlab-ci.yml" <<'YAML'
test-multi:
  image: alpine:latest
  script:
    - echo "step 1"
    - echo "step 2"
    - echo "step 3"
YAML
init_git "$TEST_DIR"
run_test "multi-step" "$TEST_DIR"

# Test 3: Environment variables
TEST_DIR=$(make_test_dir env-vars)
cat > "$TEST_DIR/.gitlab-ci.yml" <<'YAML'
variables:
  MY_VAR: "hello-world"

test-env:
  image: alpine:latest
  script:
    - echo "MY_VAR=$MY_VAR"
YAML
init_git "$TEST_DIR"
run_test "env-vars" "$TEST_DIR"

# Test 4: Before/after scripts
TEST_DIR=$(make_test_dir before-after)
cat > "$TEST_DIR/.gitlab-ci.yml" <<'YAML'
test-hooks:
  image: alpine:latest
  before_script:
    - echo "before"
  script:
    - echo "main"
  after_script:
    - echo "after"
YAML
init_git "$TEST_DIR"
run_test "before-after-scripts" "$TEST_DIR"

# Test 5: Multiple jobs
TEST_DIR=$(make_test_dir multi-job)
cat > "$TEST_DIR/.gitlab-ci.yml" <<'YAML'
job-a:
  image: alpine:latest
  script:
    - echo "job A"

job-b:
  image: alpine:latest
  script:
    - echo "job B"
YAML
init_git "$TEST_DIR"
run_test "multiple-jobs" "$TEST_DIR"

# Test 6: Job with working directory
TEST_DIR=$(make_test_dir workdir)
cat > "$TEST_DIR/.gitlab-ci.yml" <<'YAML'
test-pwd:
  image: alpine:latest
  script:
    - pwd
YAML
init_git "$TEST_DIR"
run_test "working-directory" "$TEST_DIR"

# Test 7: Exit code handling
TEST_DIR=$(make_test_dir exitcode)
cat > "$TEST_DIR/.gitlab-ci.yml" <<'YAML'
test-exit:
  image: alpine:latest
  script:
    - "true"
YAML
init_git "$TEST_DIR"
run_test "exit-code-success" "$TEST_DIR"

# Test 8: Expected failure
TEST_DIR=$(make_test_dir fail)
cat > "$TEST_DIR/.gitlab-ci.yml" <<'YAML'
test-fail:
  image: alpine:latest
  script:
    - "false"
  allow_failure: true
YAML
init_git "$TEST_DIR"
run_test "allow-failure" "$TEST_DIR"

# Test 9: Stages
TEST_DIR=$(make_test_dir stages)
cat > "$TEST_DIR/.gitlab-ci.yml" <<'YAML'
stages:
  - build
  - test

build-job:
  stage: build
  image: alpine:latest
  script:
    - echo "building"

test-job:
  stage: test
  image: alpine:latest
  script:
    - echo "testing"
YAML
init_git "$TEST_DIR"
run_test "stages" "$TEST_DIR"

# Test 10: Variables expansion
TEST_DIR=$(make_test_dir var-expand)
cat > "$TEST_DIR/.gitlab-ci.yml" <<'YAML'
variables:
  BASE: "hello"
  DERIVED: "${BASE}-world"

test-expand:
  image: alpine:latest
  script:
    - echo "$DERIVED"
YAML
init_git "$TEST_DIR"
run_test "variable-expansion" "$TEST_DIR"

# Test 11: Rules
TEST_DIR=$(make_test_dir rules)
cat > "$TEST_DIR/.gitlab-ci.yml" <<'YAML'
test-rules:
  image: alpine:latest
  script:
    - echo "runs always"
  rules:
    - when: always
YAML
init_git "$TEST_DIR"
run_test "rules-always" "$TEST_DIR"

# Test 12: Artifacts (just the script part, no real artifact upload)
TEST_DIR=$(make_test_dir artifacts)
cat > "$TEST_DIR/.gitlab-ci.yml" <<'YAML'
test-artifacts:
  image: alpine:latest
  script:
    - mkdir -p output
    - echo "data" > output/result.txt
YAML
init_git "$TEST_DIR"
run_test "artifacts-script" "$TEST_DIR"

# Test 13: Different image
TEST_DIR=$(make_test_dir diff-image)
cat > "$TEST_DIR/.gitlab-ci.yml" <<'YAML'
test-image:
  image: debian:bookworm-slim
  script:
    - cat /etc/os-release | head -1
YAML
init_git "$TEST_DIR"
run_test "different-image" "$TEST_DIR"

# Test 14: Job-level variables
TEST_DIR=$(make_test_dir job-vars)
cat > "$TEST_DIR/.gitlab-ci.yml" <<'YAML'
test-job-vars:
  image: alpine:latest
  variables:
    JOB_VAR: "from-job"
  script:
    - echo "JOB_VAR=$JOB_VAR"
YAML
init_git "$TEST_DIR"
run_test "job-level-variables" "$TEST_DIR"

# Test 15: Needs/dependencies
TEST_DIR=$(make_test_dir needs)
cat > "$TEST_DIR/.gitlab-ci.yml" <<'YAML'
stages:
  - build
  - test

build-step:
  stage: build
  image: alpine:latest
  script:
    - echo "building"

test-step:
  stage: test
  image: alpine:latest
  needs:
    - build-step
  script:
    - echo "testing after build"
YAML
init_git "$TEST_DIR"
run_test "needs-dependency" "$TEST_DIR"

# Test 16: Script with pipes and redirection
TEST_DIR=$(make_test_dir pipes)
cat > "$TEST_DIR/.gitlab-ci.yml" <<'YAML'
test-pipes:
  image: alpine:latest
  script:
    - echo "hello world" | wc -w
    - echo "data" > /tmp/pipe-test.txt
    - cat /tmp/pipe-test.txt
    - test "$(cat /tmp/pipe-test.txt)" = "data"
YAML
init_git "$TEST_DIR"
run_test "pipes-and-redirect" "$TEST_DIR"

# Test 17: Parallel/matrix
TEST_DIR=$(make_test_dir parallel)
cat > "$TEST_DIR/.gitlab-ci.yml" <<'YAML'
test-matrix:
  image: alpine:latest
  parallel:
    matrix:
      - FLAVOR: [vanilla, chocolate]
  script:
    - echo "FLAVOR=$FLAVOR"
    - test -n "$FLAVOR"
YAML
init_git "$TEST_DIR"
run_test "parallel-matrix" "$TEST_DIR"

# Test 18: Multi-line heredoc script
TEST_DIR=$(make_test_dir heredoc)
cat > "$TEST_DIR/.gitlab-ci.yml" <<'YAML'
test-heredoc:
  image: alpine:latest
  script:
    - |
      LINE1="hello"
      LINE2="world"
      RESULT="${LINE1} ${LINE2}"
      echo "$RESULT"
      test "$RESULT" = "hello world"
YAML
init_git "$TEST_DIR"
run_test "heredoc-script" "$TEST_DIR"

# Test 19: Timeout with fast script
TEST_DIR=$(make_test_dir timeout)
cat > "$TEST_DIR/.gitlab-ci.yml" <<'YAML'
test-timeout:
  image: alpine:latest
  timeout: 2m
  script:
    - echo "fast script"
    - echo "timeout-ok"
YAML
init_git "$TEST_DIR"
run_test "timeout-fast" "$TEST_DIR"

# Test 20: Failure propagation via allow_failure
TEST_DIR=$(make_test_dir fail-prop)
cat > "$TEST_DIR/.gitlab-ci.yml" <<'YAML'
test-fail:
  image: alpine:latest
  script:
    - echo "about to fail"
    - "false"
  allow_failure: true
YAML
init_git "$TEST_DIR"
run_test "failure-allow-failure" "$TEST_DIR"

# Test 21: when:on_failure
TEST_DIR=$(make_test_dir when-fail)
cat > "$TEST_DIR/.gitlab-ci.yml" <<'YAML'
stages:
  - test
  - cleanup

test-job:
  stage: test
  image: alpine:latest
  script:
    - echo "test passes"

cleanup-job:
  stage: cleanup
  image: alpine:latest
  when: on_success
  script:
    - echo "cleanup runs on success"
YAML
init_git "$TEST_DIR"
run_test "when-on-success" "$TEST_DIR"

# Test 22: Extends
TEST_DIR=$(make_test_dir extends)
cat > "$TEST_DIR/.gitlab-ci.yml" <<'YAML'
.base-job:
  image: alpine:latest
  before_script:
    - echo "base-setup"

test-extends:
  extends: .base-job
  script:
    - echo "extended job"
YAML
init_git "$TEST_DIR"
run_test "extends-inheritance" "$TEST_DIR"

# Test 23: Large output
TEST_DIR=$(make_test_dir large-output)
cat > "$TEST_DIR/.gitlab-ci.yml" <<'YAML'
test-large:
  image: alpine:latest
  script:
    - for i in $(seq 1 200); do echo "line $i of 200"; done
    - echo "large-output-done"
YAML
init_git "$TEST_DIR"
run_test "large-output" "$TEST_DIR"

# Test 24: Script with arithmetic
TEST_DIR=$(make_test_dir arithmetic)
cat > "$TEST_DIR/.gitlab-ci.yml" <<'YAML'
test-arithmetic:
  image: alpine:latest
  script:
    - X=5
    - Y=$((X + 3))
    - test "$Y" = "8"
    - echo "arithmetic-ok"
YAML
init_git "$TEST_DIR"
run_test "arithmetic" "$TEST_DIR"

# Test 25: Complex variable interpolation
TEST_DIR=$(make_test_dir var-interp)
cat > "$TEST_DIR/.gitlab-ci.yml" <<'YAML'
variables:
  PREFIX: "app"
  VERSION: "1.0"

test-interp:
  image: alpine:latest
  variables:
    FULL_NAME: "${PREFIX}-${VERSION}"
  script:
    - echo "FULL_NAME=$FULL_NAME"
    - test "$FULL_NAME" = "app-1.0"
YAML
init_git "$TEST_DIR"
run_test "variable-interpolation" "$TEST_DIR"

# --- Print summary ---
log ""
log "=============================="
log "  gitlab-ci-local Test Results"
log "  Backend: $BACKEND"
log "=============================="
log "  PASS: $PASS"
log "  FAIL: $FAIL"
log "  TOTAL: $TOTAL"
log "=============================="
log ""

# Copy results
if [ -d "/results" ]; then
    cp "$RESULTS_FILE" "/results/raw-output-${BACKEND}.log"
    log "Raw output saved to /results/raw-output-${BACKEND}.log"
fi

# Always exit 0 — informational
exit 0

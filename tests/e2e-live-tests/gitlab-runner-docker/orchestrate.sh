#!/usr/bin/env bash
set -euo pipefail

# E2E GitLab Runner Orchestrator
# Waits for GitLab CE, then for each pipeline: creates a project, pushes
# the CI YAML, registers a runner, starts it, monitors the pipeline, and
# reports results. Supports running multiple pipelines in one invocation
# to avoid restarting GitLab CE for each test.
#
# Env vars:
#   GITLAB_URL          — GitLab base URL (default: http://gitlab)
#   GITLAB_ROOT_PASSWORD — root password
#   PIPELINE            — comma-separated pipeline names or "all" (default: basic)
#   SOCKERLESS_HOST     — Docker host for runner (default: tcp://sockerless-backend:2375)
#   GITLAB_TIMEOUT      — seconds to wait for GitLab readiness
#   PIPELINE_TIMEOUT    — seconds to wait for each pipeline completion

GITLAB_URL="${GITLAB_URL:-http://gitlab}"
GITLAB_TIMEOUT="${GITLAB_TIMEOUT:-600}"
PIPELINE_TIMEOUT="${PIPELINE_TIMEOUT:-300}"
ROOT_PASSWORD="${GITLAB_ROOT_PASSWORD:-sockerless-test-pw}"
PIPELINE_INPUT="${PIPELINE:-basic}"
SOCKERLESS_HOST="${SOCKERLESS_HOST:-tcp://sockerless-backend:2375}"

ALL_PIPELINES="basic multi-step env-vars exit-codes before-after multi-stage artifacts large-output parallel-jobs timeout services-wasm custom-image-wasm shell-features file-persistence env-inheritance complex-scripts variable-features job-artifacts large-script-output concurrent-lifecycle"

# Resolve pipeline list
if [ "$PIPELINE_INPUT" = "all" ]; then
    PIPELINE_LIST="$ALL_PIPELINES"
else
    PIPELINE_LIST=$(echo "$PIPELINE_INPUT" | tr ',' ' ')
fi

echo "=== E2E GitLab Runner Test ==="
echo "GitLab URL: $GITLAB_URL"
echo "Pipelines: $PIPELINE_LIST"

# --- Wait for GitLab to be ready ---
echo "=== Waiting for GitLab to be ready (timeout: ${GITLAB_TIMEOUT}s) ==="
i=0
while [ $i -lt "$GITLAB_TIMEOUT" ]; do
    status=$(curl -sf "${GITLAB_URL}/-/readiness" 2>/dev/null | jq -r '.status // empty' 2>/dev/null || true)
    if [ "$status" = "ok" ]; then
        echo "GitLab is ready (took ${i}s)"
        break
    fi
    if [ $((i % 30)) -eq 0 ] && [ $i -gt 0 ]; then
        echo "  still waiting... (${i}s)"
    fi
    sleep 5
    i=$((i + 5))
done
if [ $i -ge "$GITLAB_TIMEOUT" ]; then
    echo "ERROR: GitLab did not become ready within ${GITLAB_TIMEOUT}s"
    exit 1
fi

sleep 20

# --- Get OAuth access token ---
echo "=== Getting access token ==="
TOKEN_RESPONSE=$(curl -sf --request POST "${GITLAB_URL}/oauth/token" \
    --data "grant_type=password&username=root&password=${ROOT_PASSWORD}" 2>/dev/null || true)
ACCESS_TOKEN=$(echo "${TOKEN_RESPONSE:-}" | jq -r '.access_token // empty' 2>/dev/null || true)

if [ -z "$ACCESS_TOKEN" ]; then
    echo "ERROR: Failed to get OAuth token"
    echo "Response: ${TOKEN_RESPONSE:-none}"
    exit 1
fi
echo "Got OAuth access token"

api() {
    curl -s --header "Authorization: Bearer ${ACCESS_TOKEN}" "$@"
}

# --- Run a single pipeline ---
run_pipeline() {
    local pl="$1"
    local PIPELINE_FILE="/pipelines/${pl}.yml"

    echo ""
    echo "=========================================="
    echo "  Running pipeline: $pl"
    echo "=========================================="

    if [ ! -f "$PIPELINE_FILE" ]; then
        echo "ERROR: Pipeline file not found: $PIPELINE_FILE"
        return 1
    fi

    # Create project
    local PROJECT_NAME="e2e-test-${pl}-$(date +%s)"
    local PROJECT_RESPONSE
    PROJECT_RESPONSE=$(api --request POST "${GITLAB_URL}/api/v4/projects" \
        --data "name=${PROJECT_NAME}&initialize_with_readme=true&visibility=public" 2>/dev/null || true)
    local PROJECT_ID
    PROJECT_ID=$(echo "${PROJECT_RESPONSE:-}" | jq -r '.id // empty' 2>/dev/null || true)

    if [ -z "$PROJECT_ID" ]; then
        echo "ERROR: Failed to create project"
        echo "Response: ${PROJECT_RESPONSE:-none}"
        return 1
    fi
    echo "Created project ID: $PROJECT_ID (${PROJECT_NAME})"

    # Push pipeline YAML
    echo "Pushing .gitlab-ci.yml (from $PIPELINE_FILE)"
    local CI_CONTENT
    CI_CONTENT=$(cat "$PIPELINE_FILE" | jq -Rs .)
    local COMMIT_RESPONSE
    COMMIT_RESPONSE=$(api --request POST "${GITLAB_URL}/api/v4/projects/${PROJECT_ID}/repository/commits" \
        --header "Content-Type: application/json" \
        --data "{
            \"branch\": \"main\",
            \"commit_message\": \"Add CI config (${pl})\",
            \"actions\": [{
                \"action\": \"create\",
                \"file_path\": \".gitlab-ci.yml\",
                \"content\": ${CI_CONTENT}
            }]
        }" 2>/dev/null || true)

    local COMMIT_ID
    COMMIT_ID=$(echo "${COMMIT_RESPONSE:-}" | jq -r '.id // empty' 2>/dev/null || true)
    if [ -z "$COMMIT_ID" ]; then
        echo "ERROR: Failed to push .gitlab-ci.yml"
        echo "Response: ${COMMIT_RESPONSE:-none}"
        return 1
    fi
    echo "Pushed commit: ${COMMIT_ID:0:12}"

    # Create runner
    local RUNNER_RESPONSE
    RUNNER_RESPONSE=$(api --request POST "${GITLAB_URL}/api/v4/user/runners" \
        --data "runner_type=project_type&project_id=${PROJECT_ID}&description=e2e-runner-${pl}" 2>/dev/null || true)
    local RUNNER_TOKEN
    RUNNER_TOKEN=$(echo "${RUNNER_RESPONSE:-}" | jq -r '.token // empty' 2>/dev/null || true)

    if [ -z "$RUNNER_TOKEN" ]; then
        echo "New runner API failed, trying legacy registration..."
        local REG_TOKEN_RESPONSE
        REG_TOKEN_RESPONSE=$(api --request POST \
            "${GITLAB_URL}/api/v4/projects/${PROJECT_ID}/runners/reset_registration_token" 2>/dev/null || true)
        local REG_TOKEN
        REG_TOKEN=$(echo "${REG_TOKEN_RESPONSE:-}" | jq -r '.token // empty' 2>/dev/null || true)

        if [ -n "$REG_TOKEN" ]; then
            RUNNER_RESPONSE=$(curl -sf --request POST "${GITLAB_URL}/api/v4/runners" \
                --data "token=${REG_TOKEN}&description=e2e-runner-${pl}" 2>/dev/null || true)
            RUNNER_TOKEN=$(echo "${RUNNER_RESPONSE:-}" | jq -r '.token // empty' 2>/dev/null || true)
        fi
    fi

    if [ -z "$RUNNER_TOKEN" ]; then
        echo "ERROR: Failed to create/register runner"
        echo "Response: ${RUNNER_RESPONSE:-none}"
        return 1
    fi
    echo "Runner registered with token: ${RUNNER_TOKEN:0:8}..."

    # Write runner config
    cat > /etc/gitlab-runner/config.toml <<EOF
concurrent = 3
check_interval = 3

[[runners]]
  name = "e2e-runner-${pl}"
  url = "${GITLAB_URL}"
  token = "${RUNNER_TOKEN}"
  executor = "docker"
  [runners.docker]
    host = "${SOCKERLESS_HOST}"
    image = "alpine:latest"
    pull_policy = "always"
    tls_verify = false
    disable_cache = true
    volumes = []
    shm_size = 0
EOF

    # Start runner
    gitlab-runner run --config /etc/gitlab-runner/config.toml &
    local RUNNER_PID=$!
    sleep 5

    # Find pipeline
    local PIPELINES_JSON
    PIPELINES_JSON=$(api "${GITLAB_URL}/api/v4/projects/${PROJECT_ID}/pipelines" 2>/dev/null || true)
    local PIPELINE_ID
    PIPELINE_ID=$(echo "${PIPELINES_JSON:-[]}" | jq -r '.[0].id // empty' 2>/dev/null || true)

    if [ -z "$PIPELINE_ID" ]; then
        echo "No auto-triggered pipeline, creating one..."
        local PIPELINE_RESPONSE
        PIPELINE_RESPONSE=$(api --request POST "${GITLAB_URL}/api/v4/projects/${PROJECT_ID}/pipeline" \
            --data "ref=main" 2>/dev/null || true)
        PIPELINE_ID=$(echo "${PIPELINE_RESPONSE:-}" | jq -r '.id // empty' 2>/dev/null || true)
    fi

    if [ -z "$PIPELINE_ID" ]; then
        echo "ERROR: No pipeline found"
        kill $RUNNER_PID 2>/dev/null || true
        wait $RUNNER_PID 2>/dev/null || true
        return 1
    fi
    echo "Pipeline ID: $PIPELINE_ID"

    # Wait for pipeline
    echo "Waiting for pipeline to complete (timeout: ${PIPELINE_TIMEOUT}s)"
    local j=0
    local PIPELINE_STATUS=""
    while [ $j -lt "$PIPELINE_TIMEOUT" ]; do
        local PIPELINE_INFO
        PIPELINE_INFO=$(api "${GITLAB_URL}/api/v4/projects/${PROJECT_ID}/pipelines/${PIPELINE_ID}" 2>/dev/null || true)
        PIPELINE_STATUS=$(echo "${PIPELINE_INFO:-}" | jq -r '.status // empty' 2>/dev/null || true)

        case "$PIPELINE_STATUS" in
            success)
                echo "Pipeline succeeded! (took ${j}s)"
                break
                ;;
            failed|canceled|skipped)
                echo "Pipeline status: $PIPELINE_STATUS (after ${j}s)"
                local JOBS
                JOBS=$(api "${GITLAB_URL}/api/v4/projects/${PROJECT_ID}/pipelines/${PIPELINE_ID}/jobs" 2>/dev/null || true)
                echo "Jobs: $(echo "$JOBS" | jq -c '[.[] | {name, status}]' 2>/dev/null || echo 'unknown')"
                for JOB_ID in $(echo "$JOBS" | jq -r '.[] | select(.status == "failed") | .id' 2>/dev/null || true); do
                    local JOB_NAME
                    JOB_NAME=$(echo "$JOBS" | jq -r ".[] | select(.id == $JOB_ID) | .name" 2>/dev/null || echo "unknown")
                    echo "--- Job log: $JOB_NAME (last 50 lines) ---"
                    api "${GITLAB_URL}/api/v4/projects/${PROJECT_ID}/jobs/${JOB_ID}/trace" 2>/dev/null | tail -50 || true
                done
                break
                ;;
            *)
                if [ $((j % 15)) -eq 0 ]; then
                    echo "  Pipeline status: ${PIPELINE_STATUS:-pending} (${j}s)"
                fi
                ;;
        esac
        sleep 5
        j=$((j + 5))
    done

    # Cleanup runner
    kill $RUNNER_PID 2>/dev/null || true
    wait $RUNNER_PID 2>/dev/null || true

    if [ "$PIPELINE_STATUS" = "success" ]; then
        # For the timeout pipeline, verify per-job statuses:
        # should-succeed must have passed, should-timeout must have failed (timeout enforced)
        if [ "$pl" = "timeout" ]; then
            local JOBS
            JOBS=$(api "${GITLAB_URL}/api/v4/projects/${PROJECT_ID}/pipelines/${PIPELINE_ID}/jobs" 2>/dev/null || true)
            local TIMEOUT_JOB_STATUS
            TIMEOUT_JOB_STATUS=$(echo "$JOBS" | jq -r '.[] | select(.name == "should-timeout") | .status' 2>/dev/null || true)
            local SUCCEED_JOB_STATUS
            SUCCEED_JOB_STATUS=$(echo "$JOBS" | jq -r '.[] | select(.name == "should-succeed") | .status' 2>/dev/null || true)
            echo "  timeout pipeline job statuses: should-succeed=$SUCCEED_JOB_STATUS should-timeout=$TIMEOUT_JOB_STATUS"
            if [ "$TIMEOUT_JOB_STATUS" != "failed" ]; then
                echo "ERROR: should-timeout job was expected to fail but got: $TIMEOUT_JOB_STATUS"
                echo "=== PIPELINE FAILED: $pl (timeout not enforced) ==="
                return 1
            fi
            if [ "$SUCCEED_JOB_STATUS" != "success" ]; then
                echo "ERROR: should-succeed job was expected to succeed but got: $SUCCEED_JOB_STATUS"
                echo "=== PIPELINE FAILED: $pl (success job failed) ==="
                return 1
            fi
            echo "  timeout enforcement verified: should-timeout correctly failed"
        fi
        echo "=== PIPELINE PASSED: $pl ==="
        return 0
    elif [ $j -ge "$PIPELINE_TIMEOUT" ]; then
        echo "=== PIPELINE TIMEOUT: $pl (last status: ${PIPELINE_STATUS:-unknown}) ==="
        return 1
    else
        echo "=== PIPELINE FAILED: $pl (status: $PIPELINE_STATUS) ==="
        return 1
    fi
}

# --- Run all pipelines ---
PASS=0
FAIL=0
RESULTS=""

for pl in $PIPELINE_LIST; do
    if run_pipeline "$pl"; then
        PASS=$((PASS + 1))
        RESULTS="${RESULTS}PASS  ${pl}\n"
    else
        FAIL=$((FAIL + 1))
        RESULTS="${RESULTS}FAIL  ${pl}\n"
    fi
done

# --- Summary ---
echo ""
echo "=============================="
echo "  Orchestrator Results"
echo "=============================="
echo "  PASS: $PASS"
echo "  FAIL: $FAIL"
echo "  Total: $((PASS + FAIL))"
echo "=============================="
echo ""
printf "$RESULTS"

# Write results
{
    echo "PASS: $PASS  FAIL: $FAIL  Total: $((PASS + FAIL))"
    echo ""
    printf "$RESULTS"
} > /results/summary.txt

if [ $FAIL -gt 0 ]; then
    exit 1
fi
exit 0

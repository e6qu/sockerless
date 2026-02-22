#!/usr/bin/env bash
set -euo pipefail

# GitLab Runner Smoke Test Orchestrator
# Waits for GitLab CE to be ready, creates a project, registers a runner,
# triggers a pipeline, and waits for it to complete.

GITLAB_URL="${GITLAB_URL:-http://gitlab}"
GITLAB_TIMEOUT="${GITLAB_TIMEOUT:-600}"
PIPELINE_TIMEOUT="${PIPELINE_TIMEOUT:-300}"
ROOT_PASSWORD="${GITLAB_ROOT_PASSWORD:-sockerless-test-pw}"

echo "=== GitLab Runner Smoke Test ==="
echo "GitLab URL: $GITLAB_URL"

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

# Give GitLab a few more seconds to fully initialize
sleep 10

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

# Helper: all API calls use Bearer auth (no -f so we see error responses)
api() {
    curl -s --header "Authorization: Bearer ${ACCESS_TOKEN}" "$@"
}

# --- Create project ---
echo "=== Creating test project ==="
PROJECT_RESPONSE=$(api --request POST "${GITLAB_URL}/api/v4/projects" \
    --data "name=smoke-test&initialize_with_readme=true&visibility=public" 2>/dev/null || true)
PROJECT_ID=$(echo "${PROJECT_RESPONSE:-}" | jq -r '.id // empty' 2>/dev/null || true)

if [ -z "$PROJECT_ID" ]; then
    echo "ERROR: Failed to create project"
    echo "Response: ${PROJECT_RESPONSE:-none}"
    exit 1
fi
echo "Created project ID: $PROJECT_ID"

# --- Push .gitlab-ci.yml to the project ---
echo "=== Pushing .gitlab-ci.yml ==="
CI_CONTENT=$(cat /test-project/.gitlab-ci.yml | jq -Rs .)
COMMIT_RESPONSE=$(api --request POST "${GITLAB_URL}/api/v4/projects/${PROJECT_ID}/repository/commits" \
    --header "Content-Type: application/json" \
    --data "{
        \"branch\": \"main\",
        \"commit_message\": \"Add CI config\",
        \"actions\": [{
            \"action\": \"create\",
            \"file_path\": \".gitlab-ci.yml\",
            \"content\": ${CI_CONTENT}
        }]
    }" 2>/dev/null || true)

COMMIT_ID=$(echo "${COMMIT_RESPONSE:-}" | jq -r '.id // empty' 2>/dev/null || true)
if [ -z "$COMMIT_ID" ]; then
    echo "ERROR: Failed to push .gitlab-ci.yml"
    echo "Response: ${COMMIT_RESPONSE:-none}"
    exit 1
fi
echo "Pushed commit: $COMMIT_ID"

# --- Create runner ---
echo "=== Creating runner ==="
# GitLab 16+ runner creation API
RUNNER_RESPONSE=$(api --request POST "${GITLAB_URL}/api/v4/user/runners" \
    --data "runner_type=project_type&project_id=${PROJECT_ID}&description=smoke-runner" 2>/dev/null || true)
RUNNER_TOKEN=$(echo "${RUNNER_RESPONSE:-}" | jq -r '.token // empty' 2>/dev/null || true)

if [ -z "$RUNNER_TOKEN" ]; then
    echo "New runner API failed, trying legacy registration..."
    REG_TOKEN_RESPONSE=$(api --request POST \
        "${GITLAB_URL}/api/v4/projects/${PROJECT_ID}/runners/reset_registration_token" 2>/dev/null || true)
    REG_TOKEN=$(echo "${REG_TOKEN_RESPONSE:-}" | jq -r '.token // empty' 2>/dev/null || true)

    if [ -n "$REG_TOKEN" ]; then
        RUNNER_RESPONSE=$(curl -sf --request POST "${GITLAB_URL}/api/v4/runners" \
            --data "token=${REG_TOKEN}&description=smoke-runner" 2>/dev/null || true)
        RUNNER_TOKEN=$(echo "${RUNNER_RESPONSE:-}" | jq -r '.token // empty' 2>/dev/null || true)
    fi
fi

if [ -z "$RUNNER_TOKEN" ]; then
    echo "ERROR: Failed to create/register runner"
    echo "Response: ${RUNNER_RESPONSE:-none}"
    exit 1
fi
echo "Runner registered with token: ${RUNNER_TOKEN:0:8}..."

# --- Write runner config ---
echo "=== Configuring runner ==="
SOCKERLESS_HOST="${SOCKERLESS_HOST:-tcp://sockerless-frontend:2375}"
cat > /etc/gitlab-runner/config.toml <<EOF
concurrent = 1
check_interval = 3

[[runners]]
  name = "smoke-runner"
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

echo "Runner config written"

# --- Start runner in background ---
echo "=== Starting gitlab-runner ==="
gitlab-runner run --config /etc/gitlab-runner/config.toml &
RUNNER_PID=$!
sleep 5

# --- Trigger pipeline (or find auto-triggered one) ---
echo "=== Finding pipeline ==="
# The commit push may have auto-triggered a pipeline
PIPELINES=$(api "${GITLAB_URL}/api/v4/projects/${PROJECT_ID}/pipelines" 2>/dev/null || true)
PIPELINE_ID=$(echo "${PIPELINES:-[]}" | jq -r '.[0].id // empty' 2>/dev/null || true)

if [ -z "$PIPELINE_ID" ]; then
    echo "No auto-triggered pipeline, creating one..."
    PIPELINE_RESPONSE=$(api --request POST "${GITLAB_URL}/api/v4/projects/${PROJECT_ID}/pipeline" \
        --data "ref=main" 2>/dev/null || true)
    PIPELINE_ID=$(echo "${PIPELINE_RESPONSE:-}" | jq -r '.id // empty' 2>/dev/null || true)
fi

if [ -z "$PIPELINE_ID" ]; then
    echo "ERROR: No pipeline found"
    kill $RUNNER_PID 2>/dev/null || true
    exit 1
fi
echo "Pipeline ID: $PIPELINE_ID"

# --- Wait for pipeline completion ---
echo "=== Waiting for pipeline to complete (timeout: ${PIPELINE_TIMEOUT}s) ==="
i=0
PIPELINE_STATUS=""
while [ $i -lt "$PIPELINE_TIMEOUT" ]; do
    PIPELINE_INFO=$(api "${GITLAB_URL}/api/v4/projects/${PROJECT_ID}/pipelines/${PIPELINE_ID}" 2>/dev/null || true)
    PIPELINE_STATUS=$(echo "${PIPELINE_INFO:-}" | jq -r '.status // empty' 2>/dev/null || true)

    case "$PIPELINE_STATUS" in
        success)
            echo "Pipeline succeeded! (took ${i}s)"
            break
            ;;
        failed|canceled|skipped)
            echo "Pipeline status: $PIPELINE_STATUS (after ${i}s)"
            JOBS=$(api "${GITLAB_URL}/api/v4/projects/${PROJECT_ID}/pipelines/${PIPELINE_ID}/jobs" 2>/dev/null || true)
            echo "Jobs: $(echo "$JOBS" | jq -c '[.[] | {name, status}]' 2>/dev/null || echo 'unknown')"
            JOB_ID=$(echo "$JOBS" | jq -r '.[0].id // empty' 2>/dev/null || true)
            if [ -n "$JOB_ID" ]; then
                echo "--- Job log (last 50 lines) ---"
                api "${GITLAB_URL}/api/v4/projects/${PROJECT_ID}/jobs/${JOB_ID}/trace" 2>/dev/null | tail -50 || true
            fi
            break
            ;;
        *)
            if [ $((i % 15)) -eq 0 ]; then
                echo "  Pipeline status: ${PIPELINE_STATUS:-pending} (${i}s)"
            fi
            ;;
    esac
    sleep 5
    i=$((i + 5))
done

# --- Cleanup ---
kill $RUNNER_PID 2>/dev/null || true

if [ "$PIPELINE_STATUS" = "success" ]; then
    echo ""
    echo "=== GITLAB SMOKE TEST PASSED ==="
    exit 0
elif [ $i -ge "$PIPELINE_TIMEOUT" ]; then
    echo ""
    echo "=== GITLAB SMOKE TEST FAILED (timeout, last status: ${PIPELINE_STATUS:-unknown}) ==="
    exit 1
else
    echo ""
    echo "=== GITLAB SMOKE TEST FAILED (status: $PIPELINE_STATUS) ==="
    exit 1
fi

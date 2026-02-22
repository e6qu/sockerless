#!/usr/bin/env bash
set -euo pipefail

# E2E GitLab Runner test runner
# Builds Docker images, runs docker compose once (GitLab CE boots once),
# and the orchestrator runs all pipelines sequentially inside the container.
#
# Usage: ./run.sh --backend <name> [--pipeline <name>|all] [--mode simulator|live]

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# Source lib.sh for logging
if [ -f "${SCRIPT_DIR}/../lib.sh" ]; then
    source "${SCRIPT_DIR}/../lib.sh"
elif [ -f "${SCRIPT_DIR}/lib.sh" ]; then
    source "${SCRIPT_DIR}/lib.sh"
fi

# --- Parse args ---
BACKEND=""
PIPELINE="all"
MODE="simulator"

while [ $# -gt 0 ]; do
    case "$1" in
        --backend)   BACKEND="$2"; shift 2 ;;
        --pipeline)  PIPELINE="$2"; shift 2 ;;
        --mode)      MODE="$2"; shift 2 ;;
        *) log_error "Unknown arg: $1"; exit 1 ;;
    esac
done

if [ -z "$BACKEND" ]; then
    log_error "Usage: ./run.sh --backend <memory|ecs|lambda|cloudrun|gcf|aca|azf>"
    exit 1
fi

# --- Timestamp for logs ---
TS="$(date '+%Y%m%d-%H%M%S')"
LOG_DIR="${SCRIPT_DIR}/../logs"
mkdir -p "$LOG_DIR"
LOG_FILE="${LOG_DIR}/gitlab-${BACKEND}-${TS}.log"

# --- Build images ---
log_info "Building Docker images..."
# Use per-cloud Dockerfiles to avoid building all backends.
# Memory backend needs SOCKERLESS_SYNTHETIC=1 because gitlab-runner requires
# helper binaries (gitlab-runner-helper, gitlab-runner-build) that can't execute
# in the WASM sandbox. The WASM sandbox is validated by GitHub act runner tests instead.
case "$BACKEND" in
    memory)
        export DOCKERFILE_BACKEND="Dockerfile.backend-memory"
        export SOCKERLESS_SYNTHETIC=1
        ;;
    ecs|lambda)
        export DOCKERFILE_BACKEND="Dockerfile.backend-aws"
        ;;
    cloudrun|gcf)
        export DOCKERFILE_BACKEND="Dockerfile.backend-gcp"
        ;;
    aca|azf)
        export DOCKERFILE_BACKEND="Dockerfile.backend-azure"
        ;;
esac
(cd "$SCRIPT_DIR" && docker compose build)

# --- Clean up any previous run ---
(cd "$SCRIPT_DIR" && docker compose down -v --remove-orphans 2>/dev/null) || true
docker rm -f gitlab-runner-docker-sockerless-backend-1 gitlab-runner-docker-gitlab-1 gitlab-runner-docker-orchestrator-1 2>/dev/null || true

# --- Route pipelines through variant mapping ---
if [ "$PIPELINE" = "all" ]; then
    ALL_PIPELINES="basic multi-step env-vars exit-codes before-after multi-stage artifacts services large-output parallel-jobs custom-image timeout complex-scripts variable-features job-artifacts large-script-output concurrent-lifecycle"
    FILTERED=""
    for pl in $ALL_PIPELINES; do
        VARIANT=$(get_test_variant "$BACKEND" "$pl")
        FILTERED="${FILTERED:+$FILTERED,}$VARIANT"
    done
    PIPELINE_ARG="$FILTERED"
else
    PIPELINE_ARG="$PIPELINE"
fi
SKIP=0

# --- Run with docker compose ---
log_info "Starting GitLab CE + backend + orchestrator (pipelines: $PIPELINE_ARG)"
set +e
export BACKEND MODE
export PIPELINE="$PIPELINE_ARG"

# Start all services detached, wait for orchestrator via docker wait.
# - "docker compose up <service>" in foreground triggers a compose monitor panic with Podman
# - "docker logs -f" disconnects prematurely with Podman
# So we use docker wait (blocking) and collect logs after.
(cd "$SCRIPT_DIR" && docker compose up -d 2>&1)
sleep 2

ORCH_CONTAINER="gitlab-runner-docker-orchestrator-1"
log_info "Waiting for orchestrator (this will take a while)..."
COMPOSE_EXIT=$(docker wait "$ORCH_CONTAINER" 2>/dev/null || echo 1)
log_info "Orchestrator exited with code $COMPOSE_EXIT"

# Collect orchestrator and backend logs
docker logs "$ORCH_CONTAINER" >> "$LOG_FILE" 2>&1 || true
docker logs "$ORCH_CONTAINER" 2>&1 | tail -30
docker logs gitlab-runner-docker-sockerless-backend-1 >> "$LOG_FILE" 2>&1 || true

# --- Stop all containers ---
(cd "$SCRIPT_DIR" && docker compose down -v --remove-orphans 2>/dev/null) || true

# --- Extract results from orchestrator log ---
echo ""
if [ $COMPOSE_EXIT -eq 0 ]; then
    log_pass "All pipelines passed (backend=$BACKEND)"
else
    log_fail "Some pipelines failed (backend=$BACKEND, exit=$COMPOSE_EXIT)"
fi

# Show the orchestrator's summary from the log
echo ""
grep -A 100 "Orchestrator Results" "$LOG_FILE" 2>/dev/null || echo "(no summary found in log)"

# Write summary
SUMMARY_FILE="${LOG_DIR}/summary-gitlab-${BACKEND}-${TS}.txt"
{
    echo "E2E GitLab Runner Results — Backend: $BACKEND"
    echo "Date: $(date)"
    echo "Mode: $MODE"
    echo "Skipped: $SKIP"
    echo ""
    grep -A 100 "Orchestrator Results" "$LOG_FILE" 2>/dev/null || echo "(no summary)"
} > "$SUMMARY_FILE"

exit $COMPOSE_EXIT

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

# --- 6. Submit test job ---
log "Submitting test job..."
SUBMIT_RESP=$(curl -sf -X POST "http://$BLEEPHUB_ADDR/api/v3/bleephub/submit" \
    -H "Content-Type: application/json" \
    -d '{"image":"alpine:latest","steps":[{"run":"echo Hello from bleephub via Sockerless"},{"run":"uname -a"}]}')

JOB_ID=$(echo "$SUBMIT_RESP" | jq -r '.jobId')
if [ -z "$JOB_ID" ] || [ "$JOB_ID" = "null" ]; then
    fail "Job submission failed: $SUBMIT_RESP"
fi
log "Job submitted: $JOB_ID"

# --- 7. Wait for job completion ---
log "Waiting for job completion (max 90s)..."
for i in $(seq 1 90); do
    STATUS_RESP=$(curl -sf "http://$BLEEPHUB_ADDR/api/v3/bleephub/jobs/$JOB_ID" 2>/dev/null || echo '{}')
    STATUS=$(echo "$STATUS_RESP" | jq -r '.status // "unknown"')
    RESULT=$(echo "$STATUS_RESP" | jq -r '.result // ""')

    if [ "$STATUS" = "completed" ]; then
        log "Job completed with result: $RESULT"
        show_diag
        if [ "$RESULT" = "Succeeded" ] || [ "$RESULT" = "succeeded" ]; then
            log "SUCCESS: Job passed"
            exit 0
        else
            fail "Job completed but result was: $RESULT"
        fi
    fi

    # Show diagnostics at 45s
    if [ "$i" -eq 45 ]; then
        log "Still waiting... status=$STATUS (${i}s)"
        show_diag
    fi
    sleep 1
done

show_diag
fail "Timeout waiting for job to complete (last status: $STATUS)"

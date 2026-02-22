#!/usr/bin/env bash
set -euo pipefail

# Universal backend starter for E2E tests.
# Starts simulator (if needed) + backend + frontend, writes PIDs for cleanup.
#
# Usage: start-backend.sh --backend <name> [--mode simulator|live] [--backend-addr <host:port>] [--frontend-addr <host:port>]
#
# Exports:
#   DOCKER_HOST        — tcp://<frontend_addr>
#   FRONTEND_ADDR      — host:port of frontend
#   BACKEND_ADDR       — host:port of backend
#   PIDFILE            — path to PID file for cleanup

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/lib.sh"

# --- Parse args ---
BACKEND=""
MODE="simulator"
BACKEND_ADDR="127.0.0.1:9100"
FRONTEND_ADDR="127.0.0.1:2375"
PIDFILE="/tmp/sockerless-e2e.pids"

while [ $# -gt 0 ]; do
    case "$1" in
        --backend)      BACKEND="$2"; shift 2 ;;
        --mode)         MODE="$2"; shift 2 ;;
        --backend-addr) BACKEND_ADDR="$2"; shift 2 ;;
        --frontend-addr) FRONTEND_ADDR="$2"; shift 2 ;;
        --pidfile)      PIDFILE="$2"; shift 2 ;;
        *) log_error "Unknown arg: $1"; exit 1 ;;
    esac
done

if [ -z "$BACKEND" ]; then
    log_error "Usage: start-backend.sh --backend <memory|ecs|lambda|cloudrun|gcf|aca|azf>"
    exit 1
fi

# Clean any previous state
rm -f "$PIDFILE"

# --- Resolve binary paths ---
resolve_binary() {
    local name="$1"
    if [ -x "/usr/local/bin/$name" ]; then
        echo "/usr/local/bin/$name"
    else
        command -v "$name" 2>/dev/null || { log_error "Binary not found: $name"; return 1; }
    fi
}

# --- Start simulator (if needed and mode=simulator) ---
CLOUD=$(get_backend_cloud "$BACKEND")

if [ "$MODE" = "simulator" ] && [ -n "$CLOUD" ]; then
    SIM_PORT=$(get_sim_port "$BACKEND")
    start_simulator "$CLOUD" "$SIM_PORT"
    write_pid "$PIDFILE" "$SIM_PID"

    bootstrap_simulator "$BACKEND"
    get_backend_env "$BACKEND"
elif [ "$MODE" = "simulator" ] && [ -z "$CLOUD" ]; then
    # Memory backend, no simulator needed
    get_backend_env "$BACKEND"
else
    # Live mode — env vars must already be set
    log_info "Live mode: expecting env vars to be pre-configured"
fi

# --- Reverse agent callback URL (FaaS + container backends in sim mode) ---
if uses_reverse_agent "$BACKEND" && [ "$MODE" = "simulator" ]; then
    export SOCKERLESS_CALLBACK_URL="$(get_callback_url "$BACKEND_ADDR")"
    log_info "Reverse agent callback URL: $SOCKERLESS_CALLBACK_URL"
fi

# --- Start backend ---
BACKEND_BIN=$(get_backend_binary "$BACKEND")
BACKEND_BIN_PATH=$(resolve_binary "$BACKEND_BIN")

log_info "Starting $BACKEND backend on $BACKEND_ADDR"
"$BACKEND_BIN_PATH" --addr "$BACKEND_ADDR" --log-level debug &
BACKEND_PID=$!
write_pid "$PIDFILE" "$BACKEND_PID"
wait_for_url "http://$BACKEND_ADDR/internal/v1/info"
log_info "$BACKEND backend ready (PID=$BACKEND_PID)"

# --- Start frontend ---
FRONTEND_BIN_PATH=$(resolve_binary "sockerless-frontend-docker")

log_info "Starting Docker frontend on $FRONTEND_ADDR"
"$FRONTEND_BIN_PATH" --addr "$FRONTEND_ADDR" --backend "http://$BACKEND_ADDR" --log-level debug &
FRONTEND_PID=$!
write_pid "$PIDFILE" "$FRONTEND_PID"
wait_for_url "http://$FRONTEND_ADDR/_ping"
log_info "Docker frontend ready (PID=$FRONTEND_PID)"

# --- Export for callers ---
export DOCKER_HOST="tcp://$FRONTEND_ADDR"
export FRONTEND_ADDR
export BACKEND_ADDR
export PIDFILE

log_info "Sockerless stack ready: DOCKER_HOST=$DOCKER_HOST"

# If running as entrypoint (not sourced), stay alive
if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
    log_info "Running as entrypoint, waiting for processes..."
    wait
fi

#!/usr/bin/env bash
# Shared functions for E2E live tests

# --- Logging ---

_log() {
    local level="$1"; shift
    echo "[$(date '+%H:%M:%S')] [$level] $*"
}

log_info()  { _log "INFO" "$@"; }
log_error() { _log "ERROR" "$@" >&2; }
log_pass()  { _log "PASS" "$@"; }
log_fail()  { _log "FAIL" "$@"; }
log_skip()  { _log "SKIP" "$@"; }

# --- Wait for URL ---

wait_for_url() {
    local url="$1" max_wait="${2:-30}"
    local i=0
    while [ $i -lt "$max_wait" ]; do
        if curl -sf "$url" >/dev/null 2>&1; then
            return 0
        fi
        sleep 1
        i=$((i + 1))
    done
    log_error "Timed out waiting for $url (${max_wait}s)"
    return 1
}

# --- Simulator management ---

start_simulator() {
    local cloud="$1" port="$2"
    local bin="simulator-${cloud}"

    # Resolve binary: check /usr/local/bin first, then PATH
    local bin_path
    if [ -x "/usr/local/bin/$bin" ]; then
        bin_path="/usr/local/bin/$bin"
    else
        bin_path="$(command -v "$bin" 2>/dev/null || true)"
    fi

    if [ -z "$bin_path" ]; then
        log_error "Simulator binary not found: $bin"
        return 1
    fi

    log_info "Starting $cloud simulator on :${port}"
    SIM_LISTEN_ADDR=":${port}" "$bin_path" &
    SIM_PID=$!
    wait_for_url "http://127.0.0.1:${port}/health"
    log_info "$cloud simulator ready (PID=$SIM_PID)"
}

bootstrap_simulator() {
    local backend="$1"
    case "$backend" in
        ecs|lambda)
            # Create ECS cluster (needed by both ECS and Lambda for the simulator)
            log_info "Creating ECS cluster"
            curl -s -X POST http://127.0.0.1:4566/ \
                -H "Content-Type: application/x-amz-json-1.1" \
                -H "X-Amz-Target: AmazonEC2ContainerServiceV20141113.CreateCluster" \
                -d '{"clusterName":"sim-cluster"}' >/dev/null
            ;;
        # GCP and Azure simulators don't need pre-bootstrap
    esac
}

# --- Backend env vars ---

get_backend_env() {
    local backend="$1"
    local sim_port

    # Skip real registry config fetch in E2E tests (uses synthetic configs)
    export SOCKERLESS_SKIP_IMAGE_CONFIG="${SOCKERLESS_SKIP_IMAGE_CONFIG:-true}"

    case "$backend" in
        ecs)
            sim_port=4566
            export SOCKERLESS_ENDPOINT_URL="http://127.0.0.1:${sim_port}"
            export SOCKERLESS_ECS_CLUSTER="sim-cluster"
            export SOCKERLESS_ECS_SUBNETS="subnet-sim"
            export SOCKERLESS_ECS_EXECUTION_ROLE_ARN="arn:aws:iam::000000000000:role/sim"
            ;;
        lambda)
            sim_port=4566
            export SOCKERLESS_ENDPOINT_URL="http://127.0.0.1:${sim_port}"
            export SOCKERLESS_LAMBDA_ROLE_ARN="arn:aws:iam::000000000000:role/sim"
            ;;
        cloudrun)
            sim_port=4567
            export SOCKERLESS_ENDPOINT_URL="http://127.0.0.1:${sim_port}"
            export SOCKERLESS_GCR_PROJECT="sim-project"
            ;;
        gcf)
            sim_port=4567
            export SOCKERLESS_ENDPOINT_URL="http://127.0.0.1:${sim_port}"
            export SOCKERLESS_GCF_PROJECT="sim-project"
            export SOCKERLESS_GCF_SERVICE_ACCOUNT="sim@sim.iam.gserviceaccount.com"
            ;;
        aca)
            sim_port=4568
            export SOCKERLESS_ENDPOINT_URL="http://127.0.0.1:${sim_port}"
            export SOCKERLESS_ACA_SUBSCRIPTION_ID="00000000-0000-0000-0000-000000000001"
            export SOCKERLESS_ACA_RESOURCE_GROUP="sim-rg"
            ;;
        azf)
            sim_port=4568
            export SOCKERLESS_ENDPOINT_URL="http://127.0.0.1:${sim_port}"
            export SOCKERLESS_AZF_SUBSCRIPTION_ID="00000000-0000-0000-0000-000000000001"
            export SOCKERLESS_AZF_RESOURCE_GROUP="sim-rg"
            export SOCKERLESS_AZF_STORAGE_ACCOUNT="simstore"
            export SOCKERLESS_AZF_REGISTRY="sim.azurecr.io"
            export SOCKERLESS_AZF_APP_SERVICE_PLAN="/subscriptions/00000000-0000-0000-0000-000000000001/resourceGroups/sim-rg/providers/Microsoft.Web/serverfarms/sim-plan"
            ;;
        memory)
            # No simulator needed
            ;;
    esac
}

# --- Backend binary lookup ---

get_backend_binary() {
    local backend="$1"
    case "$backend" in
        memory)   echo "sockerless-backend-memory" ;;
        ecs)      echo "sockerless-backend-ecs" ;;
        lambda)   echo "sockerless-backend-lambda" ;;
        cloudrun) echo "sockerless-backend-cloudrun" ;;
        gcf)      echo "sockerless-backend-gcf" ;;
        aca)      echo "sockerless-backend-aca" ;;
        azf)      echo "sockerless-backend-azf" ;;
        *)        log_error "Unknown backend: $backend"; return 1 ;;
    esac
}

# --- Cloud mapping ---

get_backend_cloud() {
    local backend="$1"
    case "$backend" in
        ecs|lambda)     echo "aws" ;;
        cloudrun|gcf)   echo "gcp" ;;
        aca|azf)        echo "azure" ;;
        memory)         echo "" ;;
        *)              log_error "Unknown backend: $backend"; return 1 ;;
    esac
}

get_sim_port() {
    local backend="$1"
    case "$backend" in
        ecs|lambda)     echo "4566" ;;
        cloudrun|gcf)   echo "4567" ;;
        aca|azf)        echo "4568" ;;
        memory)         echo "" ;;
    esac
}

# --- FaaS detection ---

is_faas_backend() {
    local backend="$1"
    case "$backend" in
        lambda|gcf|azf) return 0 ;;
        *)              return 1 ;;
    esac
}

# --- Reverse agent detection (FaaS + container backends in sim mode) ---

uses_reverse_agent() {
    local backend="$1"
    case "$backend" in
        lambda|gcf|azf|ecs|cloudrun|aca) return 0 ;;
        *)                               return 1 ;;
    esac
}

# --- Callback URL for reverse agent backends ---

get_callback_url() {
    local backend_addr="$1"
    echo "http://${backend_addr}"
}

# --- PID file management ---

cleanup_pids() {
    local pidfile="${1:-/tmp/sockerless-e2e.pids}"
    if [ -f "$pidfile" ]; then
        while IFS= read -r pid; do
            [ -n "$pid" ] && kill "$pid" 2>/dev/null || true
        done < "$pidfile"
        rm -f "$pidfile"
    fi
}

write_pid() {
    local pidfile="$1" pid="$2"
    echo "$pid" >> "$pidfile"
}

# --- Test variant routing ---

# Returns the variant name for a test on a given backend.
# Memory backend uses WASM-compatible variants for tests that require
# network services or non-busybox binaries.
get_test_variant() {
    local backend="$1" test_name="$2"
    if [ "$backend" = "memory" ]; then
        case "$test_name" in
            services)          echo "services-wasm" ;;
            services-http)     echo "services-wasm" ;;
            custom-image)      echo "custom-image-wasm" ;;
            container-action)  echo "container-action-faas" ;;
            *)                 echo "$test_name" ;;
        esac
    elif uses_reverse_agent "$backend"; then
        # Reverse agent backends (FaaS + container) run agent on host, not in
        # a real container. Route tests that need container-specific tools to
        # compatible variants.
        case "$test_name" in
            services)          echo "services-wasm" ;;
            services-http)     echo "services-wasm" ;;
            custom-image)      echo "custom-image-wasm" ;;
            container-action)  echo "container-action-faas" ;;
            *)                 echo "$test_name" ;;
        esac
    else
        echo "$test_name"
    fi
}

# Legacy skip function — now always returns 1 (don't skip).
# Memory backend uses variant routing instead.
should_skip_for_faas() {
    return 1
}

ALL_BACKENDS="memory ecs lambda cloudrun gcf aca azf"

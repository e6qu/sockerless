#!/usr/bin/env bash
set -euo pipefail

# Terraform Integration Test Runner
#
# Runs a full terraform module against a local simulator, extracts outputs,
# starts the backend + frontend, runs an act smoke test, then destroys.
#
# Usage: ./run-test.sh <backend>
#   backend: ecs | lambda | cloudrun | gcf | aca | azf
#
# Optional env vars:
#   SKIP_SMOKE_TEST=1  — skip the act smoke test (just test terraform apply/destroy)
#   KEEP_STATE=1       — don't destroy after test (for debugging)
#   SIM_PID_EXTERNAL   — PID of already-running simulator (skip simulator start)
#   SIM_PORT           — port of already-running simulator

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

export TG_NON_INTERACTIVE=true

BACKEND="${1:?Usage: $0 <backend> (ecs|lambda|cloudrun|gcf|aca|azf)}"

# Map backend to cloud
case "$BACKEND" in
    ecs|lambda)     CLOUD="aws"   ; SIM_DEFAULT_PORT=4566 ;;
    cloudrun|gcf)   CLOUD="gcp"   ; SIM_DEFAULT_PORT=4567 ;;
    aca|azf)        CLOUD="azure" ; SIM_DEFAULT_PORT=4568 ;;
    *) echo "ERROR: Unknown backend: $BACKEND"; exit 1 ;;
esac

SIM_PORT="${SIM_PORT:-$SIM_DEFAULT_PORT}"
BACKEND_ADDR="127.0.0.1:9100"
FRONTEND_ADDR="127.0.0.1:2375"

# Paths
SIM_DIR="$ROOT_DIR/simulators/$CLOUD"
TG_DIR="$ROOT_DIR/terraform/environments/$BACKEND/simulator"
WORKFLOW_DIR="$ROOT_DIR/smoke-tests/act/workflows"

# Build output directory
BUILD_DIR="$ROOT_DIR/.build"
mkdir -p "$BUILD_DIR"

# --- Cleanup ---
SIM_PID=""
BACKEND_PID=""
FRONTEND_PID=""

cleanup() {
    local exit_code=$?
    echo ""
    echo "=== Cleaning up ==="
    [ -n "${FRONTEND_PID:-}" ] && kill "$FRONTEND_PID" 2>/dev/null || true
    [ -n "${BACKEND_PID:-}" ] && kill "$BACKEND_PID" 2>/dev/null || true

    # Destroy terraform state (unless KEEP_STATE is set)
    if [ "${KEEP_STATE:-}" != "1" ] && [ -d "$TG_DIR" ]; then
        echo "--- Destroying terraform resources ---"
        (cd "$TG_DIR" && terragrunt destroy -auto-approve 2>&1) || true
    fi

    # Only kill simulator if we started it
    if [ -z "${SIM_PID_EXTERNAL:-}" ] && [ -n "${SIM_PID:-}" ]; then
        kill "$SIM_PID" 2>/dev/null || true
    fi

    exit "$exit_code"
}
trap cleanup EXIT

wait_for_url() {
    local url="$1" max_wait="${2:-30}"
    local i=0
    while [ $i -lt $max_wait ]; do
        if curl -sf "$url" >/dev/null 2>&1; then
            return 0
        fi
        sleep 1
        i=$((i + 1))
    done
    echo "ERROR: Timed out waiting for $url"
    return 1
}

# --- Azure TLS cert generation ---
# The azurerm provider requires HTTPS for the metadata host endpoint.
# On macOS, Go uses Security.framework and ignores SSL_CERT_FILE, so
# Azure terraform integration tests are Linux/Docker-only.
CERT_DIR="$BUILD_DIR/certs"
SIM_SCHEME="http"

generate_tls_certs() {
    mkdir -p "$CERT_DIR"
    openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 \
        -keyout "$CERT_DIR/ca-key.pem" -out "$CERT_DIR/ca.pem" \
        -days 1 -nodes -subj "/CN=Test CA" 2>/dev/null

    openssl req -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 \
        -keyout "$CERT_DIR/server-key.pem" -out "$CERT_DIR/server.csr" \
        -nodes -subj "/CN=localhost" 2>/dev/null

    # Include wildcard SANs for storage data-plane subdomain routing:
    # The azurerm provider makes HTTPS calls to {account}.blob.localhost:4568 etc.
    openssl x509 -req -in "$CERT_DIR/server.csr" \
        -CA "$CERT_DIR/ca.pem" -CAkey "$CERT_DIR/ca-key.pem" -CAcreateserial \
        -out "$CERT_DIR/server-cert.pem" -days 1 \
        -extfile <(printf "subjectAltName=DNS:localhost,DNS:*.blob.localhost,DNS:*.file.localhost,DNS:*.queue.localhost,DNS:*.table.localhost,DNS:*.web.localhost,DNS:*.dfs.localhost,IP:127.0.0.1\nextendedKeyUsage=serverAuth") 2>/dev/null
}

# --- Step 1: Build simulator ---
if [ -z "${SIM_PID_EXTERNAL:-}" ]; then
    echo "=== Building $CLOUD simulator ==="
    (cd "$SIM_DIR" && GOWORK=off go build -tags noui -o "$BUILD_DIR/simulator-$CLOUD" .)
fi

# --- Step 2: Start simulator ---
if [ -z "${SIM_PID_EXTERNAL:-}" ]; then
    SIM_ENV=()
    SIM_ENV+=("SIM_LISTEN_ADDR=:$SIM_PORT")

    # Azure needs TLS for the azurerm provider metadata endpoint
    if [ "$CLOUD" = "azure" ]; then
        if [ "$(uname)" = "Darwin" ]; then
            echo "ERROR: Azure terraform integration tests require Linux (Docker)."
            echo "On macOS, Go uses Security.framework and ignores SSL_CERT_FILE."
            echo "Run with: make docker-tf-int-test-azure"
            exit 1
        fi
        echo "=== Generating TLS certificates ==="
        generate_tls_certs
        SIM_ENV+=("SIM_TLS_CERT=$CERT_DIR/server-cert.pem")
        SIM_ENV+=("SIM_TLS_KEY=$CERT_DIR/server-key.pem")
        SIM_SCHEME="https"
        # Export SSL_CERT_FILE now so curl trusts the CA for the health check
        export SSL_CERT_FILE="$CERT_DIR/ca.pem"

        # Start dnsmasq to resolve *.localhost → 127.0.0.1 for storage
        # data-plane subdomain routing. The azurerm provider makes HTTPS
        # calls to {account}.blob.localhost:4568 etc. Go's pure-Go DNS
        # resolver queries /etc/resolv.conf nameservers, so dnsmasq is
        # needed even though libnss-myhostname might be available.
        echo "=== Starting dnsmasq for *.localhost resolution ==="
        ORIG_NS=$(grep -m1 nameserver /etc/resolv.conf | awk '{print $2}')
        dnsmasq --listen-address=127.0.0.1 \
                --address=/localhost/127.0.0.1 \
                --server="${ORIG_NS:-8.8.8.8}"
        echo "nameserver 127.0.0.1" > /etc/resolv.conf
    fi

    echo "=== Starting $CLOUD simulator on :$SIM_PORT ==="
    env "${SIM_ENV[@]}" "$BUILD_DIR/simulator-$CLOUD" &
    SIM_PID=$!
    wait_for_url "$SIM_SCHEME://127.0.0.1:$SIM_PORT/health"
    echo "$CLOUD simulator ready (PID=$SIM_PID)"
else
    SIM_PID="$SIM_PID_EXTERNAL"
    echo "=== Using existing $CLOUD simulator on :$SIM_PORT (PID=$SIM_PID) ==="
fi

# --- Step 3: Set cloud-specific env vars for terraform ---
case "$CLOUD" in
    aws)
        export AWS_ACCESS_KEY_ID="test"
        export AWS_SECRET_ACCESS_KEY="test"
        export AWS_DEFAULT_REGION="us-east-1"
        ;;
    gcp)
        export GOOGLE_APPLICATION_CREDENTIALS=""
        ;;
    azure)
        # azurerm v3 uses ARM_METADATA_HOSTNAME (not ARM_METADATA_HOST which is for azurestack)
        export ARM_METADATA_HOSTNAME="localhost:$SIM_PORT"
        export ARM_TENANT_ID="00000000-0000-0000-0000-000000000000"
        export ARM_SUBSCRIPTION_ID="00000000-0000-0000-0000-000000000000"
        export ARM_CLIENT_ID="00000000-0000-0000-0000-000000000000"
        export ARM_CLIENT_SECRET="test"
        export SSL_CERT_FILE="$CERT_DIR/ca.pem"
        ;;
esac

# --- Step 4: Terragrunt apply ---
echo "=== Running terragrunt apply ($BACKEND) ==="
echo "    Working dir: $TG_DIR"
(cd "$TG_DIR" && terragrunt init 2>&1)
(cd "$TG_DIR" && terragrunt apply -auto-approve 2>&1)
echo "Terragrunt apply complete"

# --- Step 5: Extract outputs → env vars ---
echo "=== Extracting terraform outputs ==="
TF_OUTPUTS=$(cd "$TG_DIR" && terragrunt output -json 2>/dev/null)

# Helper: extract a single output value (strips quotes)
tf_output() {
    echo "$TF_OUTPUTS" | jq -r ".$1.value // empty"
}

# Common: simulator endpoint
export SOCKERLESS_ENDPOINT_URL="http://127.0.0.1:$SIM_PORT"

case "$BACKEND" in
    ecs)
        export SOCKERLESS_ECS_CLUSTER="$(tf_output ecs_cluster_name)"
        SUBNETS_JSON="$(tf_output private_subnet_ids)"
        export SOCKERLESS_ECS_SUBNETS="$(echo "$SUBNETS_JSON" | jq -r 'if type == "array" then join(",") else . end' 2>/dev/null || echo "$SUBNETS_JSON")"
        export SOCKERLESS_ECS_SECURITY_GROUPS="$(tf_output task_security_group_id)"
        export SOCKERLESS_ECS_TASK_ROLE_ARN="$(tf_output task_role_arn)"
        export SOCKERLESS_ECS_EXECUTION_ROLE_ARN="$(tf_output execution_role_arn)"
        export SOCKERLESS_ECS_LOG_GROUP="$(tf_output log_group_name)"
        export SOCKERLESS_AGENT_EFS_ID="$(tf_output efs_filesystem_id)"
        BACKEND_BIN_NAME="sockerless-backend-ecs"
        BACKEND_PKG="./backends/ecs/cmd/sockerless-backend-ecs"
        ;;
    lambda)
        export SOCKERLESS_LAMBDA_ROLE_ARN="$(tf_output execution_role_arn)"
        export SOCKERLESS_LAMBDA_LOG_GROUP="$(tf_output log_group_name)"
        BACKEND_BIN_NAME="sockerless-backend-lambda"
        BACKEND_PKG="./backends/lambda/cmd/sockerless-backend-lambda"
        ;;
    cloudrun)
        export SOCKERLESS_GCR_PROJECT="$(tf_output project_id)"
        export SOCKERLESS_GCR_REGION="$(tf_output region)"
        export SOCKERLESS_GCR_VPC_CONNECTOR="$(tf_output vpc_connector_name)"
        BACKEND_BIN_NAME="sockerless-backend-cloudrun"
        BACKEND_PKG="./backends/cloudrun/cmd/sockerless-backend-cloudrun"
        ;;
    gcf)
        export SOCKERLESS_GCF_PROJECT="$(tf_output project_id)"
        export SOCKERLESS_GCF_REGION="$(tf_output region)"
        export SOCKERLESS_GCF_SERVICE_ACCOUNT="$(tf_output service_account_email)"
        BACKEND_BIN_NAME="sockerless-backend-gcf"
        BACKEND_PKG="./backends/cloudrun-functions/cmd/sockerless-backend-gcf"
        ;;
    aca)
        export SOCKERLESS_ACA_SUBSCRIPTION_ID="${ARM_SUBSCRIPTION_ID:-00000000-0000-0000-0000-000000000000}"
        export SOCKERLESS_ACA_RESOURCE_GROUP="$(tf_output resource_group_name)"
        export SOCKERLESS_ACA_ENVIRONMENT="$(tf_output managed_environment_name)"
        export SOCKERLESS_ACA_LOCATION="$(tf_output location)"
        export SOCKERLESS_ACA_LOG_ANALYTICS_WORKSPACE="$(tf_output log_analytics_workspace_name)"
        export SOCKERLESS_ACA_STORAGE_ACCOUNT="$(tf_output storage_account_name)"
        BACKEND_BIN_NAME="sockerless-backend-aca"
        BACKEND_PKG="./backends/aca/cmd/sockerless-backend-aca"
        ;;
    azf)
        export SOCKERLESS_AZF_SUBSCRIPTION_ID="${ARM_SUBSCRIPTION_ID:-00000000-0000-0000-0000-000000000000}"
        export SOCKERLESS_AZF_RESOURCE_GROUP="$(tf_output resource_group_name)"
        export SOCKERLESS_AZF_LOCATION="$(tf_output location)"
        export SOCKERLESS_AZF_STORAGE_ACCOUNT="$(tf_output storage_account_name)"
        export SOCKERLESS_AZF_REGISTRY="$(tf_output acr_login_server)"
        export SOCKERLESS_AZF_APP_SERVICE_PLAN="$(tf_output app_service_plan_id)"
        export SOCKERLESS_AZF_LOG_ANALYTICS_WORKSPACE="$(tf_output log_analytics_workspace_id)"
        BACKEND_BIN_NAME="sockerless-backend-azf"
        BACKEND_PKG="./backends/azure-functions/cmd/sockerless-backend-azf"
        ;;
esac

echo "Exported env vars for $BACKEND backend"
env | grep "^SOCKERLESS_" | sort

# --- Step 6: Build and start backend ---
if [ "${SKIP_SMOKE_TEST:-}" != "1" ]; then
    echo ""
    echo "=== Building $BACKEND backend ==="
    (cd "$ROOT_DIR" && go build -o "$BUILD_DIR/$BACKEND_BIN_NAME" "$BACKEND_PKG")

    echo "=== Starting $BACKEND backend on $BACKEND_ADDR ==="
    "$BUILD_DIR/$BACKEND_BIN_NAME" --addr "$BACKEND_ADDR" --log-level debug &
    BACKEND_PID=$!
    wait_for_url "http://$BACKEND_ADDR/internal/v1/info"
    echo "$BACKEND backend ready (PID=$BACKEND_PID)"

    # --- Step 7: Build and start frontend ---
    echo "=== Building Docker frontend ==="
    (cd "$ROOT_DIR" && go build -o "$BUILD_DIR/sockerless-frontend-docker" ./frontends/docker/cmd)

    echo "=== Starting Docker frontend on $FRONTEND_ADDR ==="
    "$BUILD_DIR/sockerless-frontend-docker" --addr "$FRONTEND_ADDR" --backend "http://$BACKEND_ADDR" --log-level debug &
    FRONTEND_PID=$!
    wait_for_url "http://$FRONTEND_ADDR/_ping"
    echo "Docker frontend ready (PID=$FRONTEND_PID)"

    # --- Step 8: Run act smoke test ---
    echo ""
    echo "=== Running act smoke test (backend=$BACKEND) ==="
    export DOCKER_HOST="tcp://$FRONTEND_ADDR"

    act push \
        --workflows "$WORKFLOW_DIR/" \
        -P ubuntu-latest=alpine:latest \
        --container-daemon-socket "tcp://$FRONTEND_ADDR" \
        2>&1 | tee /tmp/act-tf-int-output.log
    ACT_EXIT=${PIPESTATUS[0]}

    echo ""
    if [ $ACT_EXIT -eq 0 ]; then
        echo "=== TERRAFORM INTEGRATION TEST PASSED (backend=$BACKEND) ==="
    else
        echo "=== TERRAFORM INTEGRATION TEST FAILED (backend=$BACKEND, exit=$ACT_EXIT) ==="
        echo ""
        echo "--- Last 50 lines of output ---"
        tail -50 /tmp/act-tf-int-output.log
        exit $ACT_EXIT
    fi
else
    echo ""
    echo "=== TERRAFORM APPLY/DESTROY TEST PASSED (backend=$BACKEND) ==="
    echo "(smoke test skipped — SKIP_SMOKE_TEST=1)"
fi

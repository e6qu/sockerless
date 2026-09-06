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
BACKEND_ADDR="127.0.0.1:3375"
# The backend listens on every interface: the workload containers' reverse-agent
# bootstraps dial it back at this host's address, not at loopback.
BACKEND_LISTEN_ADDR="0.0.0.0:3375"

# Paths
# Simulators come from the sockerless-cloud repository at the version pinned
# in tests/go.mod (tool directives); build through that module.
TESTS_DIR="$ROOT_DIR/tests"
TG_DIR="$ROOT_DIR/terraform/environments/$BACKEND/simulator"
# The Lambda environment reads the ECS environment's state for the EFS,
# subnets, security group, roles and log group the runner Lambda shares with
# ECS, so that environment is applied first and destroyed last.
ECS_TG_DIR="$ROOT_DIR/terraform/environments/ecs/simulator"
WORKFLOW_DIR="$ROOT_DIR/tests/terraform-integration/workflows"

# Build output directory
BUILD_DIR="$ROOT_DIR/.build"
mkdir -p "$BUILD_DIR"

# --- Cleanup ---
SIM_PID=""
BACKEND_PID=""

cleanup() {
    local exit_code=$?
    echo ""
    echo "=== Cleaning up ==="
    [ -n "${BACKEND_PID:-}" ] && kill "$BACKEND_PID" 2>/dev/null || true

    # Destroy terraform state (unless KEEP_STATE is set)
    if [ "${KEEP_STATE:-}" != "1" ] && [ -d "$TG_DIR" ]; then
        echo "--- Destroying terraform resources ---"
        (cd "$TG_DIR" && terragrunt destroy -auto-approve 2>&1) || true
        if [ "$BACKEND" = "lambda" ]; then
            (cd "$ECS_TG_DIR" && terragrunt destroy -auto-approve 2>&1) || true
        fi
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
    (cd "$TESTS_DIR" && go build -tags noui -o "$BUILD_DIR/simulator-$CLOUD" "github.com/e6qu/sockerless-cloud/simulator-$CLOUD")
fi

# --- Step 2: Start simulator ---
if [ -z "${SIM_PID_EXTERNAL:-}" ]; then
    SIM_ENV=()
    SIM_ENV+=("SIM_LISTEN_ADDR=:$SIM_PORT")

    # Azure needs TLS for the azurerm provider metadata endpoint
    if [ "$BACKEND" = "cloudrun" ] && [ "$(uname)" = "Darwin" ]; then
        echo "ERROR: the Cloud Run environment creates a Compute Engine network, which the simulator"
        echo "materializes with Linux network namespaces, bridges, veth pairs and nftables."
        echo "Run with: make tf-int-test-cloudrun  (Docker-based — see top-level Makefile)"
        exit 1
    fi
    if [ "$CLOUD" = "azure" ]; then
        if [ "$(uname)" = "Darwin" ]; then
            echo "ERROR: Azure terraform integration tests require Linux (Docker)."
            echo "On macOS, Go uses Security.framework and ignores SSL_CERT_FILE."
            echo "Run with: make tf-int-test-azure  (Docker-based — see top-level Makefile)"
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
        # On the host network the host's own resolver holds 127.0.0.1:53;
        # dnsmasq answers on the next loopback address, still resolving every
        # *.localhost name to 127.0.0.1.
        dnsmasq --listen-address=127.0.0.2 --bind-interfaces \
                --address=/localhost/127.0.0.1 \
                --server="${ORIG_NS:-8.8.8.8}"
        echo "nameserver 127.0.0.2" > /etc/resolv.conf
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
# The overlay images the FaaS and Container Apps backends build run on this
# host, so they are built for its platform.
case "$(uname -m)" in
    aarch64|arm64) WORKLOAD_PLATFORM="linux/arm64" ;;
    *) WORKLOAD_PLATFORM="linux/amd64" ;;
esac

case "$CLOUD" in
    aws)
        # The simulator's seeded administrator credential, the one every
        # AWS client of the simulator signs with.
        export AWS_ACCESS_KEY_ID="test"
        export AWS_SECRET_ACCESS_KEY="test"
        export AWS_DEFAULT_REGION="us-east-1"
        # The AWS CLI the ECS module's destroy-time sweep runs reaches the
        # simulator through the CLI's own endpoint coordinate.
        export AWS_ENDPOINT_URL="http://127.0.0.1:$SIM_PORT"
        ;;
    gcp)
        export SOCKERLESS_GCP_BUILD_PLATFORM="$WORKLOAD_PLATFORM"
        # Application Default Credentials reach the simulator's own Compute
        # Engine metadata server, the way a workload's credentials reach the
        # real one, through Google's non-default-metadata-host coordinate.
        unset GOOGLE_APPLICATION_CREDENTIALS
        export GCE_METADATA_HOST="127.0.0.1:$SIM_PORT"
        # The provider authenticates with an access token the simulator's own
        # OAuth 2.0 token endpoint issued for the JWT-bearer grant — the grant
        # a real terraform google provider performs — differing only in the
        # endpoint coordinate.
        GOOGLE_OAUTH_ACCESS_TOKEN=$(curl -sf -X POST "$SIM_SCHEME://127.0.0.1:$SIM_PORT/token" \
            --data-urlencode "grant_type=urn:ietf:params:oauth:grant-type:jwt-bearer" | jq -r '.access_token // empty')
        if [ -z "$GOOGLE_OAUTH_ACCESS_TOKEN" ]; then
            echo "ERROR: the simulator's token endpoint issued no access token"
            exit 1
        fi
        export GOOGLE_OAUTH_ACCESS_TOKEN
        # The project the environment applies into exists before Terraform
        # touches it, created through the Cloud Resource Manager API the way an
        # operator creates one; a project that already exists is the state
        # asked for.
        project_status=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$SIM_SCHEME://127.0.0.1:$SIM_PORT/v1/projects" \
            -H "Authorization: Bearer $GOOGLE_OAUTH_ACCESS_TOKEN" -H 'Content-Type: application/json' \
            -d '{"projectId":"sockerless-simulator","name":"sockerless-simulator"}')
        case "$project_status" in
            200|409) ;;
            *) echo "ERROR: create project sockerless-simulator: HTTP $project_status"; exit 1 ;;
        esac
        ;;
    azure)
        export SOCKERLESS_AZURE_BUILD_PLATFORM="$WORKLOAD_PLATFORM"
        # azurerm v3 uses ARM_METADATA_HOSTNAME (not ARM_METADATA_HOST which is for azurestack)
        export ARM_METADATA_HOSTNAME="localhost:$SIM_PORT"
        # The simulator's bootstrap application registration and tenant, the
        # client credential every Azure client of the simulator presents.
        export ARM_TENANT_ID="11111111-1111-1111-1111-111111111111"
        export ARM_SUBSCRIPTION_ID="00000000-0000-0000-0000-000000000001"
        export ARM_CLIENT_ID="test-client-id"
        export ARM_CLIENT_SECRET="test-client-secret"
        export SSL_CERT_FILE="$CERT_DIR/ca.pem"
        # The simulator serves Azure Container Registry's /v2/ at its own
        # address; the backend names it through the registry coordinate,
        # scheme included, so overlay image references carry that host and
        # the build, the push and the pull reach it there.
        export SOCKERLESS_AZURE_ACR_ENDPOINT="$SIM_SCHEME://127.0.0.1:$SIM_PORT"
        ;;
esac

# --- Step 4: Terragrunt apply ---
if [ "$BACKEND" = "lambda" ]; then
    echo "=== Running terragrunt apply (ecs, shared with lambda) ==="
    (cd "$ECS_TG_DIR" && terragrunt init 2>&1)
    (cd "$ECS_TG_DIR" && terragrunt apply -auto-approve 2>&1)
fi
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

# gcp_backend_coordinates exports the Google Cloud coordinates the backends
# take beside the API endpoint: the simulator serves Artifact Registry's /v2/
# at its own address (scheme included, the backend dials it), and Cloud
# Logging's gRPC API on the port above the next — a distinct API in real
# Google Cloud, so the backend takes it explicitly.
gcp_backend_coordinates() {
    export SOCKERLESS_GCP_AR_ENDPOINT="http://127.0.0.1:$SIM_PORT"
    export SOCKERLESS_GCP_LOGADMIN_ENDPOINT="127.0.0.1:$((SIM_PORT + 2))"
}

# callback_host is the address a workload container reaches this host at, for
# the reverse-agent bootstrap to dial back to the backend.
callback_host() {
    if [ -n "${SOCKERLESS_SMOKE_CALLBACK_HOST:-}" ]; then
        printf '%s\n' "$SOCKERLESS_SMOKE_CALLBACK_HOST"
        return
    fi
    hostname -I | awk '{print $1}'
}

# pull_host_image pulls an image on the host engine, waiting out the
# registry's rate limit: each refused attempt waits longer than the last,
# and the fifth refusal fails the run.
pull_host_image() {
    local image="$1" attempt wait=5 output
    for attempt in 1 2 3 4 5; do
        if output=$(env -u DOCKER_HOST docker pull -q "$image" 2>&1); then
            return 0
        fi
        printf '%s\n' "$output" >&2
        if [ "$attempt" = 5 ]; then
            echo "ERROR: pull $image failed after $attempt attempts" >&2
            return 1
        fi
        echo "pull $image refused (attempt $attempt); retrying in ${wait}s" >&2
        sleep "$wait"
        wait=$((wait * 3))
    done
}

# Common: simulator endpoint
export SOCKERLESS_ENDPOINT_URL="$SIM_SCHEME://127.0.0.1:$SIM_PORT"

case "$BACKEND" in
    ecs)
        # The architecture the Fargate tasks run on is the host's, which is
        # what the simulator runs them on.
        case "$(uname -m)" in
            aarch64|arm64) export SOCKERLESS_ECS_CPU_ARCHITECTURE="ARM64" ;;
            *) export SOCKERLESS_ECS_CPU_ARCHITECTURE="X86_64" ;;
        esac
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
        # The architecture the functions run on is the host's, which is what
        # the simulator runs them on.
        case "$(uname -m)" in
            aarch64|arm64) export SOCKERLESS_LAMBDA_ARCHITECTURE="arm64" ;;
            *) export SOCKERLESS_LAMBDA_ARCHITECTURE="x86_64" ;;
        esac
        export SOCKERLESS_CALLBACK_URL="ws://$(callback_host):3375/v1/lambda/reverse"
        ECS_OUTPUTS=$(cd "$ECS_TG_DIR" && terragrunt output -json 2>/dev/null)
        SOCKERLESS_LAMBDA_SUBNETS=$(echo "$ECS_OUTPUTS" | jq -r '.private_subnet_ids.value | join(",")')
        export SOCKERLESS_LAMBDA_SUBNETS
        SOCKERLESS_LAMBDA_SECURITY_GROUPS=$(echo "$ECS_OUTPUTS" | jq -r '.task_security_group_id.value')
        export SOCKERLESS_LAMBDA_SECURITY_GROUPS
        SOCKERLESS_LAMBDA_AGENT_EFS_ID=$(echo "$ECS_OUTPUTS" | jq -r '.efs_filesystem_id.value')
        export SOCKERLESS_LAMBDA_AGENT_EFS_ID
        export SOCKERLESS_LAMBDA_ROLE_ARN="$(tf_output execution_role_arn)"
        export SOCKERLESS_LAMBDA_LOG_GROUP="$(tf_output log_group_name)"
        BACKEND_BIN_NAME="sockerless-backend-lambda"
        BACKEND_PKG="./backends/lambda/cmd/sockerless-backend-lambda"
        ;;
    cloudrun)
        export SOCKERLESS_GCR_PROJECT="$(tf_output project_id)"
        export SOCKERLESS_GCR_REGION="$(tf_output region)"
        export SOCKERLESS_GCR_VPC_CONNECTOR="$(tf_output vpc_connector_name)"
        export SOCKERLESS_GCP_BUILD_BUCKET="$(tf_output build_context_bucket)"
        gcp_backend_coordinates
        export SOCKERLESS_CALLBACK_URL="ws://$(callback_host):3375/v1/cloudrun/reverse"
        export SOCKERLESS_CLOUDRUN_BOOTSTRAP="/opt/sockerless/sockerless-cloudrun-bootstrap"
        BACKEND_BIN_NAME="sockerless-backend-cloudrun"
        BACKEND_PKG="./backends/cloudrun/cmd/sockerless-backend-cloudrun"
        ;;
    gcf)
        export SOCKERLESS_GCF_PROJECT="$(tf_output project_id)"
        export SOCKERLESS_GCF_REGION="$(tf_output region)"
        export SOCKERLESS_GCF_SERVICE_ACCOUNT="$(tf_output service_account_email)"
        export SOCKERLESS_GCP_BUILD_BUCKET="$(tf_output build_context_bucket)"
        gcp_backend_coordinates
        export SOCKERLESS_CALLBACK_URL="ws://$(callback_host):3375/v1/gcf/reverse"
        export SOCKERLESS_GCF_BOOTSTRAP="/opt/sockerless/sockerless-gcf-bootstrap"
        BACKEND_BIN_NAME="sockerless-backend-gcf"
        BACKEND_PKG="./backends/cloudrun-functions/cmd/sockerless-backend-gcf"
        ;;
    aca)
        # The managed-identity coordinate the Azure platform injects into a
        # Container Apps or Functions container; the backend's
        # DefaultAzureCredential acquires a real bearer from it — here the
        # simulator's /msi/token, against real Azure the platform's own.
        export IDENTITY_ENDPOINT="$SOCKERLESS_ENDPOINT_URL/msi/token"
        export IDENTITY_HEADER="sim-identity-header"
        export SOCKERLESS_ACA_SUBSCRIPTION_ID="${ARM_SUBSCRIPTION_ID:-00000000-0000-0000-0000-000000000000}"
        export SOCKERLESS_ACA_RESOURCE_GROUP="$(tf_output resource_group_name)"
        export SOCKERLESS_ACA_ENVIRONMENT="$(tf_output managed_environment_name)"
        export SOCKERLESS_ACA_LOCATION="$(tf_output location)"
        export SOCKERLESS_ACA_LOG_ANALYTICS_WORKSPACE="$(tf_output log_analytics_workspace_name)"
        export SOCKERLESS_ACA_STORAGE_ACCOUNT="$(tf_output storage_account_name)"
        # The registry the overlay build lands in and the blob container its
        # build context is uploaded to, which Azure Container Registry Tasks
        # builds from.
        export SOCKERLESS_AZURE_ACR_NAME="$(tf_output acr_name)"
        export SOCKERLESS_AZURE_BUILD_STORAGE_ACCOUNT="$(tf_output storage_account_name)"
        export SOCKERLESS_AZURE_BUILD_CONTAINER="$(tf_output build_container_name)"
        export SOCKERLESS_CALLBACK_URL="ws://$(callback_host):3375/v1/aca/reverse"
        export SOCKERLESS_ACA_BOOTSTRAP="/opt/sockerless/sockerless-cloudrun-bootstrap"
        # act keeps its job container alive and execs every step into it,
        # which needs the bootstrap in the container; the backend runs
        # containers as Container Apps with the bootstrap baked in, as the
        # GitHub Actions runbook prescribes, rather than as plain Jobs.
        export SOCKERLESS_ACA_USE_APP=1
        BACKEND_BIN_NAME="sockerless-backend-aca"
        BACKEND_PKG="./backends/aca/cmd/sockerless-backend-aca"
        ;;
    azf)
        # The managed-identity coordinate the Azure platform injects into a
        # Container Apps or Functions container; the backend's
        # DefaultAzureCredential acquires a real bearer from it — here the
        # simulator's /msi/token, against real Azure the platform's own.
        export IDENTITY_ENDPOINT="$SOCKERLESS_ENDPOINT_URL/msi/token"
        export IDENTITY_HEADER="sim-identity-header"
        export SOCKERLESS_AZF_SUBSCRIPTION_ID="${ARM_SUBSCRIPTION_ID:-00000000-0000-0000-0000-000000000000}"
        export SOCKERLESS_AZF_RESOURCE_GROUP="$(tf_output resource_group_name)"
        export SOCKERLESS_AZF_LOCATION="$(tf_output location)"
        export SOCKERLESS_AZF_STORAGE_ACCOUNT="$(tf_output storage_account_name)"
        export SOCKERLESS_AZF_REGISTRY="$(tf_output acr_login_server)"
        # The blob container the overlay build context is uploaded to, which
        # Azure Container Registry Tasks builds from.
        export SOCKERLESS_AZURE_BUILD_STORAGE_ACCOUNT="$(tf_output storage_account_name)"
        export SOCKERLESS_AZURE_BUILD_CONTAINER="$(tf_output build_container_name)"
        export SOCKERLESS_AZF_APP_SERVICE_PLAN="$(tf_output app_service_plan_id)"
        export SOCKERLESS_AZF_LOG_ANALYTICS_WORKSPACE="$(tf_output log_analytics_workspace_id)"
        export SOCKERLESS_CALLBACK_URL="ws://$(callback_host):3375/v1/azf/reverse"
        export SOCKERLESS_AZF_BOOTSTRAP="/opt/sockerless/sockerless-azf-bootstrap"
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
    (cd "$ROOT_DIR" && go build -tags noui -o "$BUILD_DIR/$BACKEND_BIN_NAME" "$BACKEND_PKG")

    echo "=== Starting $BACKEND backend on $BACKEND_ADDR ==="
    "$BUILD_DIR/$BACKEND_BIN_NAME" --addr "$BACKEND_LISTEN_ADDR" --log-level debug &
    BACKEND_PID=$!
    wait_for_url "http://$BACKEND_ADDR/_ping"
    echo "$BACKEND backend ready (PID=$BACKEND_PID)"

    # --- Step 7: Run act smoke test ---
    echo ""
    echo "=== Running act smoke test (backend=$BACKEND) ==="
    # The simulator serves the workflow's image from what the host engine
    # holds under the upstream's own name — the way the smoke tests pull it on
    # the host first — from the Docker Library mirror on the Amazon ECR
    # Public Gallery, which rate-limits anonymous pulls from one address;
    # five cells pull it at once, so a throttled pull is retried with a
    # growing wait.
    pull_host_image public.ecr.aws/docker/library/alpine:latest
    env -u DOCKER_HOST docker tag public.ecr.aws/docker/library/alpine:latest alpine:latest
    export DOCKER_HOST="tcp://$BACKEND_ADDR"

    # --reuse keeps the job container after the run, so a failure can show
    # the workload's own output below.
    act push \
        --workflows "$WORKFLOW_DIR/" \
        -P ubuntu-latest=alpine:latest \
        --container-daemon-socket "tcp://$BACKEND_ADDR" \
        --reuse \
        2>&1 | tee /tmp/act-tf-int-output.log
    ACT_EXIT=${PIPESTATUS[0]}
    if [ "$ACT_EXIT" -ne 0 ]; then
        echo "--- act job container logs ---"
        for c in $(docker ps -a --filter name=act- --format '{{.ID}}'); do
            docker logs "$c" 2>&1 | tail -40 || true
        done
        # The workloads the simulator ran are containers on the host engine,
        # labelled with the simulator's host; their own output shows what a
        # bootstrap did before or instead of dialling back.
        echo "--- simulator workload containers on the host engine ---"
        for c in $(env -u DOCKER_HOST docker ps -a --filter label=sockerless-sim-host --format '{{.ID}}'); do
            env -u DOCKER_HOST docker inspect --format '{{.Id}} {{.Name}} {{.Config.Image}} {{.State.Status}} exit={{.State.ExitCode}}' "$c" 2>&1 || true
            env -u DOCKER_HOST docker logs "$c" 2>&1 | tail -40 || true
        done
    fi
    docker ps -a --filter name=act- --format '{{.ID}}' | xargs -r docker rm -f >/dev/null 2>&1 || true

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

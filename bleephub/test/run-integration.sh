#!/usr/bin/env bash
set -euo pipefail

log() { echo "=== [bleephub-test] $*"; }
fail() {
    echo "!!! [bleephub-test] FAIL: $*" >&2
    show_diag
    if [ "${BLEEPHUB_HOLD:-}" = "1" ]; then
        echo "!!! [bleephub-test] BLEEPHUB_HOLD=1 — stack held for inspection (sim :4566, backend :3375); ctrl-c / docker rm -f to release" >&2
        sleep infinity
    fi
    exit 1
}

show_diag() {
    # Scope the backend log to the ACTIVE backend (not a `sockerless-backend-*`
    # glob): a stale log from a prior run in a reused data dir would otherwise
    # bleed into this run's failure dump and mislead debugging.
    for lf in "${LOG_DIR:-/tmp}"/simulator-*.log "${LOG_DIR:-/tmp}"/sockerless-backend-"${BLEEPHUB_BACKEND:-*}".log; do
        if [ -f "$lf" ]; then
            echo "=== tail $lf ==="
            tail -40 "$lf"
        fi
    done
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
#
# Tests 1-11 run in HOST MODE (jobContainer null — what real GitHub
# sends for jobs without `container:`): the runner executes steps
# directly in this container, exercising the full protocol surface.
# Tests 12+ are CONTAINER-MODE: jobs declaring `container:` (and
# `services:`) dispatch through sockerless-backend-ecs to the AWS
# simulator; the workload containers run on the host engine (mounted
# docker.sock) and share the runner's workspace through the sim-EFS
# host dir, exactly the runner-as-cloud-task data plane the live cells
# use. TEST 14 closes the control-plane loop: the github-runner
# dispatcher polls bleephub for the queued job and spawns the runner
# itself.
BLEEPHUB_ADDR="127.0.0.1:80"
PIDS=()

cleanup() {
    log "Cleaning up..."
    for pid in "${PIDS[@]}"; do
        kill "$pid" 2>/dev/null || true
    done
    # Reap sim-spawned workload containers on the HOST engine — they
    # outlive the sim when a run dies mid-test (the sim exits with this
    # container; its workloads don't).
    if [ -S /var/run/docker.sock ]; then
        for _ in 1 2 3; do
            ids=$(docker ps -aq --filter label=sockerless-sim 2>/dev/null || true)
            [ -z "$ids" ] && break
            echo "$ids" | xargs docker rm -f >/dev/null 2>&1 || true
        done
    fi
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

# --- 1. Start bleephub ---
log "Starting bleephub on $BLEEPHUB_ADDR"
# The admin token has no default — the binary fails loudly if this is unset.
export BLEEPHUB_ADMIN_TOKEN="bleephub-admin-token-00000000000000000000"
# Job messages must carry a server URL every runner can reach — the
# dispatcher-spawned runner lives on the HOST engine and dials
# host.docker.internal:80 (published), while the resident runner lives
# in THIS container; the hosts entry points the name back at ourselves
# so one URL serves both (the GitHub Enterprise Server external-URL model).
export BLEEPHUB_EXTERNAL_URL="http://host.docker.internal"
echo "127.0.0.1 host.docker.internal" >> /etc/hosts
bleephub --addr ":80" --log-level info &
PIDS+=($!)
wait_for_url "http://$BLEEPHUB_ADDR/health"
log "bleephub ready"

log "Creating the GitHub Actions test repository"
if ! curl -sf -X POST "http://$BLEEPHUB_ADDR/api/v3/user/repos" \
    -H "Authorization: token $BLEEPHUB_ADMIN_TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"name":"test","auto_init":true}' >/dev/null; then
    curl -sf "http://$BLEEPHUB_ADDR/api/v3/repos/admin/test" \
        -H "Authorization: token $BLEEPHUB_ADMIN_TOKEN" >/dev/null || fail "create test repository"
fi

# --- 2. Start the cloud simulator + sockerless backend (per BLEEPHUB_BACKEND) ---
# SOCKERLESS_HARNESS_DATA_DIR is a host directory mounted into this container
# at the SAME path: the sim resolves cloud storage (EFS access points / Azure
# Files shares) to host dirs and the HOST engine bind-mounts them into workload
# containers, so the paths must be valid on both sides. Each provision_<backend>
# exports WORK_DIR + EXT_DIR (the runner workspace + externals host dirs), starts
# its sim + backend on :3375, and stages the runner externals.
# Local podman-machine note: relabel the data root once per machine so the
# label-disabled harness and the confined sim-spawned workloads can both write it:
#   podman machine ssh "sudo chcon -R -t container_file_t -l s0 $SOCKERLESS_HARNESS_DATA_DIR"
: "${SOCKERLESS_HARNESS_DATA_DIR:?SOCKERLESS_HARNESS_DATA_DIR must be set to the identical-path host mount (see the Makefile bleephub-runner-docker-test* target)}"

provision_ecs() {
    # --- 2. Start the AWS simulator + sockerless-backend-ecs ---
    # SIM_EFS_DATA_DIR must be a host directory mounted into this container
    # at the SAME path (see the Makefile target): the sim resolves EFS
    # access points to host dirs and the HOST engine bind-mounts them into
    # workload containers, so the paths must be valid on both sides.
    SIM_EFS_DATA_DIR="$SOCKERLESS_HARNESS_DATA_DIR"
    export SIM_EFS_DATA_DIR
    # Local podman-machine note: the VM enforces SELinux, and sim-spawned
    # workloads run confined (container_t) while this harness runs
    # label-disabled — relabel the EFS root once per machine so both sides
    # can write it: podman machine ssh "sudo chcon -R -t container_file_t -l s0 $SIM_EFS_DATA_DIR"
    # CI (Docker, no SELinux) needs nothing.
    export AWS_ACCESS_KEY_ID=sim AWS_SECRET_ACCESS_KEY=sim AWS_REGION=us-east-1
    SIM_ADDR="127.0.0.1:4566"

    log "Starting simulator-aws on $SIM_ADDR"
    LOG_DIR="$SIM_EFS_DATA_DIR/logs"
    mkdir -p "$LOG_DIR"
    simulator-aws --addr "$SIM_ADDR" >"$LOG_DIR/simulator-aws.log" 2>&1 &
    PIDS+=($!)
    wait_for_url "http://$SIM_ADDR/health"

    log "Bootstrapping sim: ECS cluster + EFS workspace"
    curl -sf -X POST "http://$SIM_ADDR/"     -H "Content-Type: application/x-amz-json-1.1"     -H "X-Amz-Target: AmazonEC2ContainerServiceV20141113.CreateCluster"     -d '{"clusterName":"sim-cluster"}' >/dev/null || fail "create ECS cluster"

    # EFS filesystem + two access points: the runner workspace and the
    # runner externals — the same shape the live cell terraform provisions.
    FS_ID=$(curl -sf -X POST "http://$SIM_ADDR/2015-02-01/file-systems"     -H 'Content-Type: application/json'     -d '{"CreationToken":"bleephub-runner"}' | jq -r '.FileSystemId // empty')
    [ -n "$FS_ID" ] || fail "create EFS filesystem"
    WS_AP_ID=$(curl -sf -X POST "http://$SIM_ADDR/2015-02-01/access-points"     -H 'Content-Type: application/json'     -d "{\"ClientToken\":\"ws\",\"FileSystemId\":\"$FS_ID\",\"RootDirectory\":{\"Path\":\"/runner-ws\"}}" | jq -r '.AccessPointId // empty')
    [ -n "$WS_AP_ID" ] || fail "create workspace access point"
    EXT_AP_ID=$(curl -sf -X POST "http://$SIM_ADDR/2015-02-01/access-points"     -H 'Content-Type: application/json'     -d "{\"ClientToken\":\"ext\",\"FileSystemId\":\"$FS_ID\",\"RootDirectory\":{\"Path\":\"/runner-externals\"}}" | jq -r '.AccessPointId // empty')
    [ -n "$EXT_AP_ID" ] || fail "create externals access point"

    # The runner's workspace lives ON the workspace access point (the
    # runner-as-cloud-task shape: the cell runner-task mounts EFS at its
    # work dir). Externals are staged onto their access point so job
    # containers see the same node toolchain the runner uses.
    WORK_DIR="$SIM_EFS_DATA_DIR/$FS_ID/runner-ws"
    EXT_DIR="$SIM_EFS_DATA_DIR/$FS_ID/runner-externals"
    mkdir -p "$WORK_DIR" "$EXT_DIR"
    log "Staging runner externals onto EFS ($EXT_DIR)…"
    cp -a /runner/externals/. "$EXT_DIR/"

    case "$(uname -m)" in
        x86_64)        ECS_ARCH=X86_64 ;;
        aarch64|arm64) ECS_ARCH=ARM64 ;;
        *) fail "unsupported arch $(uname -m)" ;;
    esac

    export SOCKERLESS_ENDPOINT_URL="http://$SIM_ADDR"
    export SOCKERLESS_ECS_CLUSTER=sim-cluster
    export SOCKERLESS_ECS_SUBNETS=subnet-0123456789abcdef0
    export SOCKERLESS_ECS_EXECUTION_ROLE_ARN=arn:aws:iam::000000000000:role/sim
    export SOCKERLESS_ECS_CPU_ARCHITECTURE="$ECS_ARCH"
    # Workload containers run on the HOST engine and dial back through the
    # published backend port (the sim adds host.docker.internal:host-gateway
    # to task containers).
    export SOCKERLESS_CALLBACK_URL=http://host.docker.internal:3375
    export SOCKERLESS_AUTO_AGENT_BIN=/usr/local/bin/sockerless-agent
    # The runner's container-job binds translate onto the shared volumes:
    # workspace-root and externals map to their access points; sub-paths
    # under the workspace drop (the parent mount covers them);
    # /var/run/docker.sock drops.
    export SOCKERLESS_ECS_SHARED_VOLUMES="runner-ws=${WORK_DIR}=${WS_AP_ID}=${FS_ID},runner-externals=/runner/externals=${EXT_AP_ID}=${FS_ID}"

    log "Starting sockerless-backend-ecs on :3375"
    sockerless-backend-ecs --addr :3375 --log-level debug >"$LOG_DIR/sockerless-backend-ecs.log" 2>&1 &
    PIDS+=($!)
    wait_for_url "http://127.0.0.1:3375/_ping"
    log "sockerless-backend-ecs ready (shared volumes: workspace + externals)"
}

provision_aca() {
    SIM_AZURE_FILES_DATA_DIR="$SOCKERLESS_HARNESS_DATA_DIR"
    export SIM_AZURE_FILES_DATA_DIR
    SIM_ADDR="127.0.0.1:4568"
    # The backend reaches the sim via the `localhost` hostname (not the
    # 127.0.0.1 literal) so the storage account's advertised blob endpoint
    # is `<account>.blob.localhost:<port>` — a name that resolves to
    # loopback on Linux, where the azblob context upload then lands. (The IP
    # literal would advertise an unroutable `<account>.blob.127.0.0.1`.)
    local sim_endpoint="http://localhost:4568"
    local sub="00000000-0000-0000-0000-000000000001"
    local rg="sim-rg" acct="simstorage" env="sockerless" acr="simacr"
    case "$(uname -m)" in
        aarch64|arm64) local build_platform="linux/arm64" ;;
        *)             local build_platform="linux/amd64" ;;
    esac

    LOG_DIR="$SOCKERLESS_HARNESS_DATA_DIR/logs"
    mkdir -p "$LOG_DIR"

    # The ACR-Tasks overlay build uploads its context to blob storage via
    # the storage account's advertised endpoint. Pin that endpoint to a
    # deterministic `<account>.blob.localhost` host (independent of which
    # Host header reaches the sim) and make it resolve to loopback inside
    # this container — `*.localhost` is not special-cased by the container
    # resolver, so it needs an explicit hosts entry. This is how the harness
    # realizes Azure storage DNS locally; the backend just uses whatever ARM
    # advertises.
    export SIM_AZURE_ARM_EXTERNAL_DATA_PLANE_URLS_JSON='{"storage":{"blob":"http://{account}.blob.localhost:{port}/"}}'
    if ! grep -q "${acct}.blob.localhost" /etc/hosts 2>/dev/null; then
        echo "127.0.0.1 ${acct}.blob.localhost" >>/etc/hosts || fail "add storage host alias"
    fi

    log "Starting simulator-azure on :4568 (all interfaces, so the published registry port reaches it)"
    simulator-azure --addr ":4568" >"$LOG_DIR/simulator-azure.log" 2>&1 &
    PIDS+=($!)
    wait_for_url "http://$SIM_ADDR/health"

    log "Bootstrapping sim: storage account + managed environment"
    curl -sf -X PUT "http://$SIM_ADDR/subscriptions/$sub/resourceGroups/$rg/providers/Microsoft.Storage/storageAccounts/$acct?api-version=2023-01-01" \
        -H 'Content-Type: application/json' \
        -d '{"location":"eastus","sku":{"name":"Standard_LRS"},"kind":"StorageV2","properties":{}}' >/dev/null || fail "create storage account"
    curl -sf -X PUT "http://$SIM_ADDR/subscriptions/$sub/resourceGroups/$rg/providers/Microsoft.App/managedEnvironments/$env?api-version=2024-03-01" \
        -H 'Content-Type: application/json' \
        -d '{"location":"eastus","properties":{}}' >/dev/null || fail "create managed environment"
    # ACR + a build-context blob container for the App-overlay bootstrap
    # image: backend-aca builds the reverse-agent overlay via ACR Tasks
    # (uploads the context to this container, then scheduleRun → docker
    # build on the host engine).
    curl -sf -X PUT "http://$SIM_ADDR/subscriptions/$sub/resourceGroups/$rg/providers/Microsoft.ContainerRegistry/registries/$acr?api-version=2023-07-01" \
        -H 'Content-Type: application/json' \
        -d '{"location":"eastus","sku":{"name":"Basic"},"properties":{}}' >/dev/null || fail "create ACR registry"
    curl -sf -X PUT "http://$SIM_ADDR/build-context?restype=container" \
        -H "Host: ${acct}.blob.localhost:4568" >/dev/null || fail "create build-context container"

    # The runner workspace + externals live on Azure Files shares; the sim
    # materialises each (account, share) at $DATA_DIR/<account>/<share>.
    WORK_DIR="$SIM_AZURE_FILES_DATA_DIR/$acct/runner-ws"
    EXT_DIR="$SIM_AZURE_FILES_DATA_DIR/$acct/runner-externals"
    mkdir -p "$WORK_DIR" "$EXT_DIR"
    log "Staging runner externals onto Azure Files ($EXT_DIR)…"
    cp -a /runner/externals/. "$EXT_DIR/"

    export SOCKERLESS_ENDPOINT_URL="$sim_endpoint"
    export SOCKERLESS_ACA_SUBSCRIPTION_ID="$sub"
    export SOCKERLESS_ACA_RESOURCE_GROUP="$rg"
    export SOCKERLESS_ACA_STORAGE_ACCOUNT="$acct"
    export SOCKERLESS_ACA_LOG_ANALYTICS_WORKSPACE="default"
    export SOCKERLESS_ACA_ENVIRONMENT="$env"
    # Container-mode jobs need a long-lived container with reverse-agent
    # exec, i.e. the ACA App path: backend-aca injects the reverse-agent
    # bootstrap by building an overlay image through ACR Tasks (App overlay,
    # SOCKERLESS_ACA_USE_APP=1), then runs the App.
    export SOCKERLESS_ACA_USE_APP=1
    export SOCKERLESS_AZURE_ACR_NAME="$acr"
    export SOCKERLESS_AZURE_BUILD_STORAGE_ACCOUNT="$acct"
    export SOCKERLESS_AZURE_BUILD_CONTAINER="build-context"
    export SOCKERLESS_AZURE_BUILD_PLATFORM="$build_platform"
    # The overlay image is built, pushed, and pulled at this registry
    # endpoint — the sim's /v2/ published to the host engine at
    # 127.0.0.1:5000 (a loopback host the engine treats as insecure, so no
    # daemon registry config is needed). ACR Tasks does a real `docker push`
    # here; the ACA App run does a real `docker pull` from here — registry
    # and compute stay agnostic, connected only by the /v2/ API.
    export SOCKERLESS_AZURE_ACR_ENDPOINT="127.0.0.1:5000"
    # ACA exec/attach is via the reverse agent: the overlay bootstrap inside
    # the App container dials back to the backend's reverse endpoint.
    export SOCKERLESS_CALLBACK_URL="ws://host.docker.internal:3375/v1/aca/reverse"
    export SOCKERLESS_ACA_BOOTSTRAP=/usr/local/bin/sockerless-cloudrun-bootstrap
    export SOCKERLESS_AUTO_AGENT_BIN=/usr/local/bin/sockerless-agent
    # Runner container-job binds translate onto the shared Azure Files volumes.
    export SOCKERLESS_ACA_SHARED_VOLUMES="runner-ws=${WORK_DIR}=runner-ws=azure-files-ephemeral,runner-externals=/runner/externals=runner-externals=azure-files-ephemeral"

    log "Starting sockerless-backend-aca on :3375"
    sockerless-backend-aca --addr :3375 --log-level debug >"$LOG_DIR/sockerless-backend-aca.log" 2>&1 &
    PIDS+=($!)
    wait_for_url "http://127.0.0.1:3375/_ping"
    log "sockerless-backend-aca ready (shared volumes: workspace + externals)"
}

provision_azf() {
    # Azure Functions deploys container-mode jobs as Linux Function App sites
    # whose sitecontainers run the workload. backend-azf builds the
    # reverse-agent overlay through ACR Tasks (the same build→push→pull as aca),
    # deploys the site, and exec/attaches over the reverse agent. A `services:`
    # container deploys as a sibling Function App site on the per-build network,
    # reached by name through Azure Private DNS — azf's faithful network
    # primitive (cloud-dns discovery), matching aca's per-build network. The
    # runner workspace + externals are shared via Azure-Files-ephemeral volumes.
    SIM_AZURE_FILES_DATA_DIR="$SOCKERLESS_HARNESS_DATA_DIR"
    export SIM_AZURE_FILES_DATA_DIR
    SIM_ADDR="127.0.0.1:4568"
    # As with aca, the backend reaches the sim via the `localhost` hostname so
    # the storage account's advertised blob endpoint resolves to loopback.
    local sim_endpoint="http://localhost:4568"
    local sub="00000000-0000-0000-0000-000000000001"
    local rg="sim-rg" acct="simstorage" plan="sockerless-plan" acr="simacr"
    case "$(uname -m)" in
        aarch64|arm64) local build_platform="linux/arm64" ;;
        *)             local build_platform="linux/amd64" ;;
    esac

    LOG_DIR="$SOCKERLESS_HARNESS_DATA_DIR/logs"
    mkdir -p "$LOG_DIR"

    # The ACR-Tasks overlay build uploads its context to blob storage via the
    # account's advertised endpoint — pin it to a deterministic
    # `<account>.blob.localhost` resolved to loopback inside this container.
    export SIM_AZURE_ARM_EXTERNAL_DATA_PLANE_URLS_JSON='{"storage":{"blob":"http://{account}.blob.localhost:{port}/"}}'
    if ! grep -q "${acct}.blob.localhost" /etc/hosts 2>/dev/null; then
        echo "127.0.0.1 ${acct}.blob.localhost" >>/etc/hosts || fail "add storage host alias"
    fi

    log "Starting simulator-azure on :4568 (all interfaces, so the published registry port reaches it)"
    simulator-azure --addr ":4568" >"$LOG_DIR/simulator-azure.log" 2>&1 &
    PIDS+=($!)
    wait_for_url "http://$SIM_ADDR/health"

    log "Bootstrapping sim: storage account + App Service plan + ACR"
    curl -sf -X PUT "http://$SIM_ADDR/subscriptions/$sub/resourceGroups/$rg/providers/Microsoft.Storage/storageAccounts/$acct?api-version=2023-01-01" \
        -H 'Content-Type: application/json' \
        -d '{"location":"eastus","sku":{"name":"Standard_LRS"},"kind":"StorageV2","properties":{}}' >/dev/null || fail "create storage account"
    # azf's host primitive is an App Service plan (Microsoft.Web/serverfarms),
    # not a managed environment: `services:` siblings deploy as Function App
    # sites on this plan / per-build VNet.
    curl -sf -X PUT "http://$SIM_ADDR/subscriptions/$sub/resourceGroups/$rg/providers/Microsoft.Web/serverfarms/$plan?api-version=2023-12-01" \
        -H 'Content-Type: application/json' \
        -d '{"location":"eastus","sku":{"name":"EP1","tier":"ElasticPremium"},"kind":"linux","properties":{"reserved":true}}' >/dev/null || fail "create App Service plan"
    curl -sf -X PUT "http://$SIM_ADDR/subscriptions/$sub/resourceGroups/$rg/providers/Microsoft.ContainerRegistry/registries/$acr?api-version=2023-07-01" \
        -H 'Content-Type: application/json' \
        -d '{"location":"eastus","sku":{"name":"Basic"},"properties":{}}' >/dev/null || fail "create ACR registry"
    curl -sf -X PUT "http://$SIM_ADDR/build-context?restype=container" \
        -H "Host: ${acct}.blob.localhost:4568" >/dev/null || fail "create build-context container"

    # The runner workspace + externals live on Azure Files shares; the sim
    # materialises each (account, share) at $DATA_DIR/<account>/<share>.
    WORK_DIR="$SIM_AZURE_FILES_DATA_DIR/$acct/runner-ws"
    EXT_DIR="$SIM_AZURE_FILES_DATA_DIR/$acct/runner-externals"
    mkdir -p "$WORK_DIR" "$EXT_DIR"
    log "Staging runner externals onto Azure Files ($EXT_DIR)…"
    cp -a /runner/externals/. "$EXT_DIR/"

    export SOCKERLESS_ENDPOINT_URL="$sim_endpoint"
    export SOCKERLESS_AZF_SUBSCRIPTION_ID="$sub"
    export SOCKERLESS_AZF_RESOURCE_GROUP="$rg"
    export SOCKERLESS_AZF_STORAGE_ACCOUNT="$acct"
    export SOCKERLESS_AZF_LOG_ANALYTICS_WORKSPACE="default"
    export SOCKERLESS_AZF_APP_SERVICE_PLAN="/subscriptions/$sub/resourceGroups/$rg/providers/Microsoft.Web/serverfarms/$plan"
    export SOCKERLESS_AZF_REGISTRY="${acr}.azurecr.io"
    # azf composes the per-build network + service discovery from real Azure
    # primitives: a VNet + Microsoft.Web/serverFarms-delegated subnet + a linked
    # Private DNS zone, with each site VNet-integrated and its --network-alias
    # registered as a Private DNS CNAME.
    export SOCKERLESS_AZF_NETWORK_DISCOVERY="cloud-dns"
    export SOCKERLESS_AZURE_ACR_NAME="$acr"
    export SOCKERLESS_AZURE_BUILD_STORAGE_ACCOUNT="$acct"
    export SOCKERLESS_AZURE_BUILD_CONTAINER="build-context"
    export SOCKERLESS_AZURE_BUILD_PLATFORM="$build_platform"
    export SOCKERLESS_AZURE_ACR_ENDPOINT="127.0.0.1:5000"
    export SOCKERLESS_CALLBACK_URL="ws://host.docker.internal:3375/v1/azf/reverse"
    export SOCKERLESS_AZF_BOOTSTRAP=/usr/local/bin/sockerless-azf-bootstrap
    export SOCKERLESS_AUTO_AGENT_BIN=/usr/local/bin/sockerless-agent
    # Runner container-job binds translate onto the shared Azure Files volumes.
    export SOCKERLESS_AZF_SHARED_VOLUMES="runner-ws=${WORK_DIR}=runner-ws=azure-files-ephemeral,runner-externals=/runner/externals=runner-externals=azure-files-ephemeral"

    log "Starting sockerless-backend-azf on :3375"
    sockerless-backend-azf --addr :3375 --log-level debug >"$LOG_DIR/sockerless-backend-azf.log" 2>&1 &
    PIDS+=($!)
    wait_for_url "http://127.0.0.1:3375/_ping"
    log "sockerless-backend-azf ready (shared volumes: workspace + externals)"
}

# write_simulator_sa_json writes a service-account JSON whose token_uri points
# at the simulator's /token endpoint. The backend's Google Cloud Go clients
# parse the real, freshly generated RSA private key, self-sign a JWT, and
# exchange it at token_uri for an access token, exactly as against Google Cloud,
# differing only in the endpoint coordinate.
write_simulator_sa_json() {
    local out="$1" token_uri="$2" project="$3"
    local key
    key=$(openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 2>/dev/null) || fail "generate SA RSA key"
    jq -n --arg pk "$key" --arg tu "$token_uri" --arg proj "$project" '{
        type: "service_account",
        project_id: $proj,
        private_key_id: "sim-key",
        private_key: $pk,
        client_email: ("sockerless-runner@" + $proj + ".iam.gserviceaccount.com"),
        client_id: "111111111111111111111",
        auth_uri: "https://accounts.google.com/o/oauth2/auth",
        token_uri: $tu,
        auth_provider_x509_cert_url: "https://www.googleapis.com/oauth2/v1/certs",
        universe_domain: "googleapis.com"
    }' > "$out" || fail "write SA JSON"
}

provision_cloudrun() {
    # The Cloud Run backend shares the runner workspace via GCS
    # snapshot-sync (gcs-sync): unlike ECS/ACA's live EFS / Azure Files
    # mount, the workspace mount inside the job container is an empty
    # tmpfs; the bootstrap restores it from GCS before each exec and
    # persists it back after, and the backend syncs the same GCS object
    # to/from the runner's local --work dir. The sim materialises each
    # GCS bucket at $SIM_GCS_DATA_DIR/<bucket> on the host engine.
    SIM_GCS_DATA_DIR="$SOCKERLESS_HARNESS_DATA_DIR"
    export SIM_GCS_DATA_DIR
    SIM_ADDR="127.0.0.1:4567"
    local sim_grpc_port=4577
    local sim_grpc_addr="127.0.0.1:${sim_grpc_port}"
    local project="sim-project"
    local build_bucket="sockerless-build"
    local ws_bucket="sockerless-runner-ws"
    local ext_bucket="sockerless-runner-externals"
    case "$(uname -m)" in
        aarch64|arm64) local build_platform="linux/arm64" ;;
        *)             local build_platform="linux/amd64" ;;
    esac

    LOG_DIR="$SOCKERLESS_HARNESS_DATA_DIR/logs"
    mkdir -p "$LOG_DIR"

    # The sim serves the Cloud Logging admin read API over gRPC on a
    # separate port; the backend reads container logs from it.
    export SIM_GCP_GRPC_PORT="$sim_grpc_port"
    log "Starting simulator-gcp on :4567 (gRPC :$sim_grpc_port)"
    simulator-gcp --addr ":4567" >"$LOG_DIR/simulator-gcp.log" 2>&1 &
    PIDS+=($!)
    wait_for_url "http://$SIM_ADDR/health"

    log "Bootstrapping sim: GCS buckets (build + workspace + externals)"
    local b
    for b in "$build_bucket" "$ws_bucket" "$ext_bucket"; do
        curl -sf -X POST "http://$SIM_ADDR/storage/v1/b?project=$project" \
            -H 'Content-Type: application/json' -d "{\"name\":\"$b\"}" >/dev/null \
            || fail "create GCS bucket $b"
    done

    # Google Cloud auth: simulator service-account JSON with token_uri as the
    # simulator's /token coordinate.
    local sa_json="$SOCKERLESS_HARNESS_DATA_DIR/cloudrun-sa.json"
    write_simulator_sa_json "$sa_json" "http://$SIM_ADDR/token" "$project"
    export GOOGLE_APPLICATION_CREDENTIALS="$sa_json"

    # The runner workspace + externals are LOCAL dirs synced to/from GCS
    # around each container-job exec (contrast ECS/ACA, where they live
    # directly on the cloud mount).
    WORK_DIR="$SOCKERLESS_HARNESS_DATA_DIR/runner-ws"
    EXT_DIR="$SOCKERLESS_HARNESS_DATA_DIR/runner-externals"
    mkdir -p "$WORK_DIR" "$EXT_DIR"
    log "Staging runner externals ($EXT_DIR)…"
    cp -a /runner/externals/. "$EXT_DIR/"

    export SOCKERLESS_ENDPOINT_URL="http://$SIM_ADDR"
    export SOCKERLESS_GCP_LOGADMIN_ENDPOINT="$sim_grpc_addr"
    export SOCKERLESS_GCR_PROJECT="$project"
    export SOCKERLESS_GCR_REGION="us-central1"
    export SOCKERLESS_GCP_BUILD_BUCKET="$build_bucket"
    export SOCKERLESS_GCP_BUILD_PLATFORM="$build_platform"
    export SOCKERLESS_POLL_INTERVAL=500ms
    # Container-mode jobs need a long-lived container with reverse-agent
    # exec: the Cloud Run backend injects the reverse-agent bootstrap by
    # building an overlay image via Cloud Build, pushing it to Artifact
    # Registry, and pulling it on the Cloud Run task. The overlay image
    # is built/pushed/pulled at this registry endpoint — the sim's /v2/
    # published to the host engine at 127.0.0.1:5000 (a loopback host the
    # engine treats as insecure). Cloud Build does a real `docker push`
    # here; the Cloud Run run does a real `docker pull` — registry and
    # compute stay agnostic, connected only by the /v2/ API.
    export SOCKERLESS_GCP_AR_ENDPOINT="127.0.0.1:5000"
    export SOCKERLESS_CLOUDRUN_BOOTSTRAP=/usr/local/bin/sockerless-cloudrun-bootstrap
    # The overlay pull + Service start + bootstrap dial-back must complete
    # within this window. Kept below the 300s per-job wait so a genuine
    # reverse-agent registration failure surfaces as "did not register"
    # rather than being masked by the job timeout (status=running).
    # NOTE: on a freshly-created podman machine the sim-registry insecure
    # drop-in (`bleephub-sim-registry-trust`) is not honored by the build
    # path until the podman service reloads — `podman machine stop && start`
    # once after creating the machine, or the overlay `FROM` pull fails with
    # "http: server gave HTTP response to HTTPS client".
    export SOCKERLESS_CLOUDRUN_BOOTSTRAP_TIMEOUT_SEC=180
    export SOCKERLESS_AUTO_AGENT_BIN=/usr/local/bin/sockerless-agent
    # Cloud Run exec/attach is via the reverse agent: the overlay
    # bootstrap inside the task dials back to the backend's reverse
    # endpoint.
    export SOCKERLESS_CALLBACK_URL="ws://host.docker.internal:3375/v1/cloudrun/reverse"
    # The in-container bootstrap's gcs-sync workspace restore/save reaches the
    # sim's storage through the published sim port on the host gateway — the
    # same host.docker.internal path the reverse-agent callback uses (the
    # backend's in-container SOCKERLESS_ENDPOINT_URL is not workload-reachable).
    # Injected into the task as STORAGE_EMULATOR_HOST. The sim's /storage/v1/
    # API shares the mux published at 127.0.0.1:5000 → host.docker.internal:5000
    # from inside a workload container.
    export SOCKERLESS_GCS_WORKLOAD_ENDPOINT="host.docker.internal:5000"
    # Service containers (TEST 13) run as Cloud Run Services discovered
    # over Cloud DNS via the VPC connector.
    export SOCKERLESS_GCR_USE_SERVICE=1
    export SOCKERLESS_GCR_VPC_CONNECTOR="projects/$project/locations/us-central1/connectors/sim-connector"
    # Runner container-job binds translate onto the gcs-sync shared volumes.
    export SOCKERLESS_GCP_SHARED_VOLUMES="runner-ws=${WORK_DIR}=${ws_bucket}=gcs-sync,runner-externals=/runner/externals=${ext_bucket}=gcs-sync"

    # Stage workload base images into the host daemon. The Cloud Run
    # overlay build rewrites a Docker Hub base (e.g. alpine:3.20) to the
    # AR docker-hub pull-through ref via the registry coordinate; the sim
    # serves that ref by hydrating from the local daemon, so the base
    # must be present locally. Pull from the ECR public gallery (no Docker
    # Hub anonymous-pull throttle) and tag under the bare Docker Hub name
    # the pull-through maps to.
    log "Staging workload base images (alpine, nginx) into the host daemon…"
    local img src
    for img in "alpine:3.20" "nginx:alpine"; do
        src="public.ecr.aws/docker/library/$img"
        local ok=""
        for attempt in 1 2 3 4 5; do
            if docker pull -q "$src" >/dev/null 2>&1; then ok=1; break; fi
            sleep "$((attempt * 3))"
        done
        [ -n "$ok" ] || fail "pull base image $src"
        docker tag "$src" "$img" || fail "tag base image $img"
    done

    log "Starting sockerless-backend-cloudrun on :3375"
    sockerless-backend-cloudrun --addr :3375 --log-level debug >"$LOG_DIR/sockerless-backend-cloudrun.log" 2>&1 &
    PIDS+=($!)
    wait_for_url "http://127.0.0.1:3375/_ping"
    log "sockerless-backend-cloudrun ready (gcs-sync workspace + externals)"
}

provision_gcf() {
    # The Cloud Run Functions (GCF Gen2) backend deploys container-jobs as
    # multi-container Cloud Run Service revisions (Functions Gen2 build on
    # Cloud Run), so it shares the runner workspace via GCS snapshot-sync
    # exactly like the Cloud Run cell: the workspace mount in the job
    # container is an empty tmpfs; the bootstrap restores it from GCS before
    # each exec and persists it back after, and the backend syncs the same
    # GCS object to/from the runner's local --work dir. The sim materialises
    # each GCS bucket at $SIM_GCS_DATA_DIR/<bucket> on the host engine.
    SIM_GCS_DATA_DIR="$SOCKERLESS_HARNESS_DATA_DIR"
    export SIM_GCS_DATA_DIR
    SIM_ADDR="127.0.0.1:4567"
    local sim_grpc_port=4577
    local sim_grpc_addr="127.0.0.1:${sim_grpc_port}"
    local project="sim-project"
    local build_bucket="sockerless-build"
    local ws_bucket="sockerless-runner-ws"
    local ext_bucket="sockerless-runner-externals"
    case "$(uname -m)" in
        aarch64|arm64) local build_platform="linux/arm64" ;;
        *)             local build_platform="linux/amd64" ;;
    esac

    LOG_DIR="$SOCKERLESS_HARNESS_DATA_DIR/logs"
    mkdir -p "$LOG_DIR"

    export SIM_GCP_GRPC_PORT="$sim_grpc_port"
    log "Starting simulator-gcp on :4567 (gRPC :$sim_grpc_port)"
    simulator-gcp --addr ":4567" >"$LOG_DIR/simulator-gcp.log" 2>&1 &
    PIDS+=($!)
    wait_for_url "http://$SIM_ADDR/health"

    log "Bootstrapping sim: GCS buckets (build + workspace + externals)"
    local b
    for b in "$build_bucket" "$ws_bucket" "$ext_bucket"; do
        curl -sf -X POST "http://$SIM_ADDR/storage/v1/b?project=$project" \
            -H 'Content-Type: application/json' -d "{\"name\":\"$b\"}" >/dev/null \
            || fail "create GCS bucket $b"
    done

    # Google Cloud auth: simulator service-account JSON with token_uri as the
    # simulator's /token coordinate.
    local sa_json="$SOCKERLESS_HARNESS_DATA_DIR/gcf-sa.json"
    write_simulator_sa_json "$sa_json" "http://$SIM_ADDR/token" "$project"
    export GOOGLE_APPLICATION_CREDENTIALS="$sa_json"

    WORK_DIR="$SOCKERLESS_HARNESS_DATA_DIR/runner-ws"
    EXT_DIR="$SOCKERLESS_HARNESS_DATA_DIR/runner-externals"
    mkdir -p "$WORK_DIR" "$EXT_DIR"
    log "Staging runner externals ($EXT_DIR)…"
    cp -a /runner/externals/. "$EXT_DIR/"

    export SOCKERLESS_ENDPOINT_URL="http://$SIM_ADDR"
    export SOCKERLESS_GCP_LOGADMIN_ENDPOINT="$sim_grpc_addr"
    export SOCKERLESS_GCF_PROJECT="$project"
    export SOCKERLESS_GCF_REGION="us-central1"
    export SOCKERLESS_GCP_BUILD_BUCKET="$build_bucket"
    export SOCKERLESS_GCP_BUILD_PLATFORM="$build_platform"
    export SOCKERLESS_POLL_INTERVAL=500ms
    # Container-mode jobs need a long-lived container with reverse-agent
    # exec: the GCF backend injects the reverse-agent bootstrap by building
    # an overlay image via Cloud Build, pushing it to Artifact Registry, and
    # pulling it on the Cloud Run Service revision the function is backed by.
    # The overlay is built/pushed/pulled at the sim's /v2/ published to the
    # host engine at 127.0.0.1:5000 (a loopback host the engine treats as
    # insecure), connected to compute only by the /v2/ API coordinate. On a
    # freshly-created podman machine the sim-registry insecure drop-in is not
    # honored by the build path until the podman service reloads —
    # `podman machine stop && start` once after creating the machine.
    export SOCKERLESS_GCP_AR_ENDPOINT="127.0.0.1:5000"
    export SOCKERLESS_GCF_BOOTSTRAP=/usr/local/bin/sockerless-gcf-bootstrap
    # Kept below the 300s per-job wait so a genuine reverse-agent
    # registration failure surfaces as "did not register" rather than being
    # masked by the job timeout (status=running).
    export SOCKERLESS_GCF_BOOTSTRAP_TIMEOUT_SEC=180
    # GCF exec/attach is via the reverse agent: the overlay bootstrap inside
    # the function dials back to the backend's reverse endpoint.
    export SOCKERLESS_CALLBACK_URL="ws://host.docker.internal:3375/v1/gcf/reverse"
    # The in-container bootstrap's gcs-sync workspace restore/save reaches the
    # sim's storage through the published sim port on the host gateway — the
    # same host.docker.internal path the reverse-agent callback uses (the
    # backend's in-container SOCKERLESS_ENDPOINT_URL is not workload-reachable).
    # Injected into the workload as STORAGE_EMULATOR_HOST.
    export SOCKERLESS_GCS_WORKLOAD_ENDPOINT="host.docker.internal:5000"
    # Service containers (TEST 13) run as Cloud Run Service revision sidecars
    # sharing loopback with the job container, discovered via /etc/hosts.
    export SOCKERLESS_GCF_VPC_CONNECTOR="projects/$project/locations/us-central1/connectors/sim-connector"
    # Runner container-job binds translate onto the gcs-sync shared volumes.
    export SOCKERLESS_GCP_SHARED_VOLUMES="runner-ws=${WORK_DIR}=${ws_bucket}=gcs-sync,runner-externals=/runner/externals=${ext_bucket}=gcs-sync"

    log "Staging workload base images (alpine, nginx) into the host daemon…"
    local img src
    for img in "alpine:3.20" "nginx:alpine"; do
        src="public.ecr.aws/docker/library/$img"
        local ok=""
        for attempt in 1 2 3 4 5; do
            if docker pull -q "$src" >/dev/null 2>&1; then ok=1; break; fi
            sleep "$((attempt * 3))"
        done
        [ -n "$ok" ] || fail "pull base image $src"
        docker tag "$src" "$img" || fail "tag base image $img"
    done

    log "Starting sockerless-backend-gcf on :3375"
    sockerless-backend-gcf --addr :3375 --log-level debug >"$LOG_DIR/sockerless-backend-gcf.log" 2>&1 &
    PIDS+=($!)
    wait_for_url "http://127.0.0.1:3375/_ping"
    log "sockerless-backend-gcf ready (gcs-sync workspace + externals)"
}

case "${BLEEPHUB_BACKEND:-ecs}" in
    ecs) provision_ecs ;;
    aca) provision_aca ;;
    azf) provision_azf ;;
    cloudrun) provision_cloudrun ;;
    gcf) provision_gcf ;;
    *) fail "unsupported BLEEPHUB_BACKEND: ${BLEEPHUB_BACKEND} (ecs|aca|azf|cloudrun|gcf)" ;;
esac

# --- 4. Configure the runner ---
log "Configuring runner..."
cd /runner

# The runner needs to write config files here
export RUNNER_ALLOW_RUNASROOT=1
export GITHUB_ACTIONS_RUNNER_TLS_NO_VERIFY=1
# export GITHUB_ACTIONS_RUNNER_TRACE=1  # Uncomment for debug logging

./config.sh \
    --url "$BLEEPHUB_EXTERNAL_URL/admin/test" \
    --token BLEEPHUB_REG_TOKEN \
    --name test-runner \
    --work "$WORK_DIR" \
    --unattended \
    --replace \
    --labels self-hosted,linux,arm64 \
    --no-default-labels \
    2>&1 | tail -5 || fail "Runner configuration failed"

log "Runner configured"

# --- 5. Start runner ---
log "Starting runner (DOCKER_HOST → sockerless-backend-ecs)..."
DOCKER_HOST=tcp://127.0.0.1:3375 ./run.sh 2>&1 &
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

api_get() {
    local path="$1"
    curl -sf -H "Authorization: token $BLEEPHUB_ADMIN_TOKEN" "http://$BLEEPHUB_ADDR$path"
}

api_post() {
    local path="$1" body="${2:-}"
    if [ -z "$body" ]; then
        body='{}'
    fi
    curl -sf -X POST -H "Authorization: token $BLEEPHUB_ADMIN_TOKEN" \
        -H "Content-Type: application/json" \
        -d "$body" "http://$BLEEPHUB_ADDR$path"
}

put_workflow_file() {
    local filename="$1" yaml="$2" encoded
    encoded=$(printf "%s\n" "$yaml" | base64 | tr -d '\n')
    curl -sf -X PUT -H "Authorization: token $BLEEPHUB_ADMIN_TOKEN" \
        -H "Content-Type: application/json" \
        -d "$(jq -n --arg msg "Add $filename" --arg content "$encoded" '{message: $msg, content: $content, branch: "main"}')" \
        "http://$BLEEPHUB_ADDR/api/v3/repos/admin/test/contents/.github/workflows/$filename" >/dev/null
}

dispatchable_workflow_yaml() {
    local yaml="$1"
    if printf "%s\n" "$yaml" | grep -Eq '^[[:space:]]*on:'; then
        printf "%s\n" "$yaml"
    else
        printf "on: workflow_dispatch\n%s\n" "$yaml"
    fi
}

latest_workflow_run_id() {
    api_get "/api/v3/repos/admin/test/actions/runs?event=workflow_dispatch&per_page=20" \
        | jq -r '.workflow_runs[0].id // 0'
}

LAST_WORKFLOW_RUN_ID=""
submit_workflow_dispatch() {
    local test_num="$1" yaml="$2" inputs_json="${3:-}"
    local filename="test-${test_num}.yml"
    local before_id run_id
    if [ -z "$inputs_json" ]; then
        inputs_json='{}'
    fi
    before_id=$(latest_workflow_run_id)
    put_workflow_file "$filename" "$(dispatchable_workflow_yaml "$yaml")" || fail "put workflow file $filename"
    api_post "/api/v3/repos/admin/test/actions/workflows/$filename/dispatches" \
        "$(jq -n --arg ref main --argjson inputs "$inputs_json" '{ref: $ref, inputs: $inputs}')" >/dev/null \
        || fail "dispatch workflow file $filename"

    log "Workflow dispatch accepted for $filename"
    for i in $(seq 1 30); do
        run_id=$(api_get "/api/v3/repos/admin/test/actions/runs?event=workflow_dispatch&per_page=20" \
            | jq -r --arg before "$before_id" --arg path ".github/workflows/$filename" \
                '[.workflow_runs[] | select((.id | tostring) != $before and .path == $path)][0].id // empty')
        if [ -n "$run_id" ]; then
            LAST_WORKFLOW_RUN_ID="$run_id"
            log "Workflow run queued: $run_id"
            return 0
        fi
        sleep 1
    done
    fail "workflow dispatch did not create a visible run for $filename"
}

wait_for_workflow_run() {
    local run_id="$1" label="$2" max="${3:-180}" expected="${4:-success}"
    local status conclusion run_status jobs_detail
    log "Waiting for $label ($run_id) (max ${max}s)..."
    for i in $(seq 1 "$max"); do
        run_status=$(api_get "/api/v3/repos/admin/test/actions/runs/$run_id" 2>/dev/null || echo '{}')
        status=$(echo "$run_status" | jq -r '.status // "unknown"')
        conclusion=$(echo "$run_status" | jq -r '.conclusion // ""')

        if [ "$status" = "completed" ]; then
            log "Workflow completed with conclusion: $conclusion"
            jobs_detail=$(api_get "/api/v3/repos/admin/test/actions/runs/$run_id/jobs" \
                | jq -r '.jobs[] | "  \(.name): \(.conclusion // .status)"')
            log "$jobs_detail"
            if [ "$expected" = "any" ] || [ "$conclusion" = "$expected" ]; then
                return 0
            fi
            return 1
        fi

        if [ "$i" -eq 90 ]; then
            log "Still waiting... status=$status (${i}s)"
        fi
        sleep 1
    done
    log "Timeout waiting for $label (last status: $status)"
    return 1
}

# Helper: submit workflow YAML through GitHub Actions workflow dispatch and wait for completion.
submit_and_wait_workflow() {
    local test_num="$1" label="$2" yaml="$3" max="${4:-180}" inputs_json="${5:-}"
    if [ -z "$inputs_json" ]; then
        inputs_json='{}'
    fi

    log "===== TEST $test_num: $label ====="
    submit_workflow_dispatch "$test_num" "$yaml" "$inputs_json"
    if wait_for_workflow_run "$LAST_WORKFLOW_RUN_ID" "$label" "$max"; then
        log "TEST $test_num PASSED: $label"
        return 0
    fi
    show_diag
    fail "$label failed"
}

# Iteration aid: BLEEPHUB_TEST_FROM=N starts at test N. CI runs everything.
TEST_FROM="${BLEEPHUB_TEST_FROM:-1}"

run_test() {
    [ "$TEST_FROM" -le "$1" ]
}

# ===== TEST 1: Single-job GitHub Actions workflow =====
if run_test 1; then
submit_and_wait_workflow 1 "Single-job GitHub Actions workflow" '
name: single-job-test
jobs:
  test:
    runs-on: self-hosted
    steps:
      - run: echo Hello from bleephub host mode
      - run: uname -a
'

# Give runner a moment to reset between tests
sleep 3
fi


# ===== TEST 2: Multi-job workflow (needs:) =====
if run_test 2; then
submit_and_wait_workflow 2 "Multi-job workflow" '
name: multi-job-test
jobs:
  build:
    runs-on: self-hosted
    steps:
      - run: echo "Building..."
      - run: echo "Build complete"
  test:
    needs: [build]
    runs-on: self-hosted
    steps:
      - run: echo "Testing after build..."
      - run: echo "All tests passed"
'

sleep 3
fi

# ===== TEST 3: Three-stage pipeline (build → test → deploy) =====
if run_test 3; then
submit_and_wait_workflow 3 "Three-stage pipeline" '
name: pipeline-test
jobs:
  build:
    runs-on: self-hosted
    steps:
      - run: echo "=== STAGE 1 BUILD ==="
      - run: echo "Compiling..."
  test:
    needs: [build]
    runs-on: self-hosted
    steps:
      - run: echo "=== STAGE 2 TEST ==="
      - run: echo "Running tests..."
  deploy:
    needs: [test]
    runs-on: self-hosted
    steps:
      - run: echo "=== STAGE 3 DEPLOY ==="
      - run: echo "Deploying..."
'

sleep 3
fi

# ===== TEST 4: Matrix strategy (2x2 matrix) =====
if run_test 4; then
submit_and_wait_workflow 4 "Matrix strategy 2x2" '
name: matrix-test
jobs:
  test:
    runs-on: self-hosted
    strategy:
      matrix:
        os: [linux, macos]
        version: ["1", "2"]
    steps:
      - run: echo "Testing on os=${{ matrix.os }} version=${{ matrix.version }}"
'

sleep 3
fi

# ===== TEST 5: Job output propagation =====
if run_test 5; then
submit_and_wait_workflow 5 "Job output propagation" '
name: output-test
jobs:
  build:
    runs-on: self-hosted
    outputs:
      version: ${{ steps.ver.outputs.version }}
    steps:
      - id: ver
        run: echo "version=1.2.3" >> "$GITHUB_OUTPUT"
  deploy:
    needs: [build]
    runs-on: self-hosted
    steps:
      - run: echo "Deploying version ${{ needs.build.outputs.version }}"
'

sleep 3
fi

# (Service containers are real containers — like container-mode jobs
# they need an engine and are gated on the bind-mount→EFS work in
# docs/GITHUB_RUNNER.md; this host-mode harness skips them.)

# ===== TEST 7: Secrets injection =====
if run_test 7; then
log "===== TEST 7: Secrets injection ====="

# PUT a secret with the REAL wire contract: fetch the public key and
# libsodium-seal the value, exactly like GitHub CLI and the GitHub REST
# application programming interface documentation require.
TOKEN="bleephub-admin-token-00000000000000000000"
PUBKEY_JSON=$(curl -sf -H "Authorization: token $TOKEN" \
    "http://$BLEEPHUB_ADDR/api/v3/repos/admin/test/actions/secrets/public-key") || fail "public-key fetch failed"
KEY_ID=$(echo "$PUBKEY_JSON" | jq -r .key_id)
SEALED=$(echo "$PUBKEY_JSON" | jq -r .key | python3 -c '
import sys, base64
from nacl.public import PublicKey, SealedBox
pub = PublicKey(base64.b64decode(sys.stdin.read().strip()))
print(base64.b64encode(SealedBox(pub).encrypt(b"s3cret_value_123")).decode())
') || fail "sealing failed"
curl -sf -X PUT "http://$BLEEPHUB_ADDR/api/v3/repos/admin/test/actions/secrets/TEST_SECRET" \
    -H "Authorization: token $TOKEN" \
    -H "Content-Type: application/json" \
    -d "$(jq -n --arg ev "$SEALED" --arg kid "$KEY_ID" '{encrypted_value: $ev, key_id: $kid}')" \
    || fail "Failed to create secret"
log "Secret created (sealed box)"

# The job asserts the decrypted secret VALUE reaches the secrets context.
submit_and_wait_workflow 7 "Secrets injection" '
name: secrets-test
jobs:
  test:
    runs-on: self-hosted
    steps:
      - run: test "${{ secrets.TEST_SECRET }}" = "s3cret_value_123"
      - run: echo "Secret value verified"
'

sleep 3
fi

# ===== TEST 8: Workflow dispatch with inputs =====
if run_test 8; then
submit_and_wait_workflow 8 "Workflow dispatch with inputs" '
name: inputs-test
on:
  workflow_dispatch:
    inputs:
      version:
        description: Version under test
        required: true
jobs:
  test:
    runs-on: self-hosted
    steps:
      - run: test "${{ inputs.version }}" = "1.2.3"
      - run: echo "Input value verified"
' 120 '{"version":"1.2.3"}'

sleep 3
fi

# ===== TEST 9: Matrix fail-fast =====
if run_test 9; then
submit_and_wait_workflow 9 "Matrix fail-fast" '
name: failfast-test
jobs:
  test:
    runs-on: self-hosted
    strategy:
      fail-fast: true
      matrix:
        idx: ["0", "1", "2", "3"]
    steps:
      - run: echo "Matrix job"
'
fi

# ===== TEST 10: Composite action hosted ON bleephub =====
if run_test 10; then
log "===== TEST 10: Composite action from a bleephub-hosted repo ====="

curl -sf -X POST "http://$BLEEPHUB_ADDR/api/v3/user/repos" \
    -H "Authorization: token $BLEEPHUB_ADMIN_TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"name":"hello-action"}' >/dev/null || fail "create action repo"

ACT_DIR=$(mktemp -d)
git -C "$ACT_DIR" init -q -b main
mkdir -p "$ACT_DIR"
cat > "$ACT_DIR/action.yml" <<'YAML'
name: hello-composite
runs:
  using: composite
  steps:
    - run: echo "composite step one"
      shell: bash
    - run: test "composite" = "composite"
      shell: bash
YAML
git -C "$ACT_DIR" add action.yml
git -C "$ACT_DIR" -c user.email=t@t -c user.name=t commit -q -m "composite action"
git -C "$ACT_DIR" push -q "http://admin:$BLEEPHUB_ADMIN_TOKEN@$BLEEPHUB_ADDR/admin/hello-action" main || fail "push action repo"
log "Composite action repo pushed"

submit_and_wait_workflow 10 "Composite action" '
name: composite-test
jobs:
  test:
    runs-on: self-hosted
    steps:
      - uses: admin/hello-action@main
      - run: echo "after composite"
'

sleep 3
fi

# ===== TEST 11: Cancellation of a running job =====
if run_test 11; then
log "===== TEST 11: Cancellation reaches the running job ====="

WF11_YAML='name: cancel-test
jobs:
  slow:
    runs-on: self-hosted
    steps:
      - run: sleep 300
  cleanup:
    needs: [slow]
    if: always()
    runs-on: self-hosted
    steps:
      - run: echo "cleanup ran"'

submit_workflow_dispatch 11 "$WF11_YAML"
WF11_ID="$LAST_WORKFLOW_RUN_ID"

log "Waiting for the slow job to start on the runner..."
for i in $(seq 1 60); do
    SLOW_STATUS=$(api_get "/api/v3/repos/admin/test/actions/runs/$WF11_ID/jobs" \
        | jq -r '.jobs[] | select(.name == "slow") | .status // "unknown"')
    [ "$SLOW_STATUS" = "in_progress" ] && break
    sleep 1
done
[ "$SLOW_STATUS" = "in_progress" ] || fail "slow job never started (status: $SLOW_STATUS)"
log "Slow job running — cancelling the workflow"

api_post "/api/v3/repos/admin/test/actions/runs/$WF11_ID/cancel" >/dev/null || fail "cancel request failed"

log "Waiting for cancellation to settle (runner must abort the sleep)..."
for i in $(seq 1 90); do
    WF11_STATUS=$(api_get "/api/v3/repos/admin/test/actions/runs/$WF11_ID" 2>/dev/null || echo '{}')
    STATUS=$(echo "$WF11_STATUS" | jq -r '.status // "unknown"')
    [ "$STATUS" = "completed" ] && break
    sleep 1
done
[ "$STATUS" = "completed" ] || fail "cancelled workflow never completed (the runner kept the job)"

RESULT=$(echo "$WF11_STATUS" | jq -r '.conclusion')
WF11_JOBS=$(api_get "/api/v3/repos/admin/test/actions/runs/$WF11_ID/jobs")
SLOW_RESULT=$(echo "$WF11_JOBS" | jq -r '.jobs[] | select(.name == "slow") | .conclusion // empty')
CLEANUP_RESULT=$(echo "$WF11_JOBS" | jq -r '.jobs[] | select(.name == "cleanup") | .conclusion // empty')
[ "$RESULT" = "cancelled" ] || fail "run result=$RESULT, want cancelled"
[ "$SLOW_RESULT" = "cancelled" ] || fail "slow result=$SLOW_RESULT, want cancelled"
[ "$CLEANUP_RESULT" = "success" ] || fail "always() cleanup result=$CLEANUP_RESULT, want success (must run after cancel)"
log "TEST 11 PASSED: Cancellation (run cancelled, always() cleanup ran)"

sleep 3
fi

# ===== TEST 12: Container-mode job through the selected backend =====
# The job declares `container:` — the runner creates the job container
# via its DOCKER_HOST (sockerless-backend-ecs), which dispatches it as
# a cloud-native workload on the host engine. The runner's workspace bind
# translates to the shared EFS volume; steps run via docker exec.
if run_test 12; then
log "===== TEST 12: Container-mode job ====="

submit_and_wait_workflow 12 "Container-mode job" '
name: container-test
jobs:
  ctr:
    runs-on: self-hosted
    container: alpine:3.20
    steps:
      - run: grep -qi alpine /etc/os-release
      - run: echo "container-proof-payload" > "$GITHUB_WORKSPACE/proof.txt"
      - run: test "$(cat "$GITHUB_WORKSPACE/proof.txt")" = "container-proof-payload"
' 300

# Data-plane assertion: the file written INSIDE the job container (a
# cloud-native workload on the host engine) must be visible in the runner
# workspace on the shared EFS volume — the exact sharing contract the
# runner-as-cloud-task topology depends on.
PROOF=$(find "$WORK_DIR" -name proof.txt -exec cat {} \; 2>/dev/null | head -1)
[ "$PROOF" = "container-proof-payload" ] || fail "workspace not shared: proof.txt not found under $WORK_DIR (got: '$PROOF')"
log "Workspace sharing verified: container-written file visible on the runner EFS workspace"

sleep 3
fi

# ===== TEST 13: Service container reachable from the job container =====
if run_test 13; then
log "===== TEST 13: Service container (nginx) reachable by alias ====="

submit_and_wait_workflow 13 "Service container" '
name: services-test
jobs:
  svc:
    runs-on: self-hosted
    container: alpine:3.20
    services:
      web:
        image: nginx:alpine
    steps:
      - run: for i in $(seq 1 30); do wget -qO- http://web/ >/tmp/idx.html 2>/dev/null && break; sleep 1; done
      - run: grep -qi nginx /tmp/idx.html
' 300

sleep 3
fi

# ===== TEST 14: Dispatcher-in-the-loop runner spawn (runner-as-task) =====
# The control-plane half of the topology: a job is queued whose labels
# no resident runner satisfies; github-runner-dispatcher-aws polls
# bleephub (--api-base), mints a registration token, and spawns an
# ephemeral runner container on the host engine; that runner registers,
# takes the job, and completes.
if run_test 14; then
log "===== TEST 14: Dispatcher spawns the runner for a queued job ====="

# Spawn image: one thin layer over the harness image (already on the
# host from the make target's build) with the spawn entrypoint.
docker build -q -t bleephub-spawn-runner:local -f /test/Dockerfile.spawn-runner /test >/dev/null \
    || fail "build spawn-runner image"

cat > /tmp/dispatcher.toml <<EOF
[[label]]
name        = "dispatched"
docker_host = "unix:///var/run/docker.sock"
image       = "bleephub-spawn-runner:local"
EOF

WF14_YAML='name: dispatched-test
jobs:
  hello:
    runs-on: [self-hosted, dispatched]
    steps:
      - run: echo "ran on a dispatcher-spawned runner"
      - run: test -n "$RUNNER_NAME"'

submit_workflow_dispatch 14 "$WF14_YAML"
WF14_ID="$LAST_WORKFLOW_RUN_ID"
log "Workflow queued (no resident runner carries the 'dispatched' label)"

log "Running dispatcher --once against bleephub..."
github-runner-dispatcher-aws \
    --repo admin/test \
    --token "$BLEEPHUB_ADMIN_TOKEN" \
    --api-base "http://$BLEEPHUB_ADDR/api/v3" \
    --config /tmp/dispatcher.toml \
    --once 2>&1 | sed 's/^/[dispatcher] /' || fail "dispatcher --once failed"

log "Waiting for the dispatcher-spawned runner to complete the job (max 300s)..."
for i in $(seq 1 300); do
    WF14_STATUS=$(api_get "/api/v3/repos/admin/test/actions/runs/$WF14_ID" 2>/dev/null || echo '{}')
    STATUS=$(echo "$WF14_STATUS" | jq -r '.status // "unknown"')
    RESULT=$(echo "$WF14_STATUS" | jq -r '.conclusion // ""')
    if [ "$STATUS" = "completed" ]; then break; fi
    if [ "$i" -eq 120 ]; then log "Still waiting... status=$STATUS (${i}s)"; fi
    sleep 1
done
[ "$STATUS" = "completed" ] || fail "dispatched job never completed (status: $STATUS)"
[ "$RESULT" = "success" ] || fail "dispatched job result=$RESULT, want success"
log "TEST 14 PASSED: dispatcher-spawned runner executed the queued job"
fi

log "===== ALL 14 INTEGRATION TESTS PASSED ====="

#!/usr/bin/env bash
# bleeplab GitLab docker-executor integration harness. A real gitlab-runner
# registers against the bleeplab control-plane simulator and runs CI jobs
# through a docker executor whose `--docker-host` is a sockerless backend —
# exactly as it would against gitlab.com with a cloud DOCKER_HOST. The job +
# helper containers dispatch through sockerless to the cloud simulator and run
# on the host engine (mounted docker.sock), the runner-as-cloud-task data plane
# the live GitLab cells use.
set -euo pipefail

log() { echo "=== [bleeplab-test] $*"; }
fail() {
    echo "!!! [bleeplab-test] FAIL: $*" >&2
    show_diag
    if [ "${BLEEPLAB_HOLD:-}" = "1" ]; then
        echo "!!! [bleeplab-test] BLEEPLAB_HOLD=1 — stack held for inspection (bleeplab :8929, backend :3375); ctrl-c / docker rm -f to release" >&2
        sleep infinity
    fi
    exit 1
}

show_diag() {
    for lf in "${LOG_DIR:-/tmp}"/simulator-*.log "${LOG_DIR:-/tmp}"/bleeplab.log "${LOG_DIR:-/tmp}"/gitlab-runner.log; do
        if [ -f "$lf" ]; then
            echo "=== tail $lf ==="
            tail -80 "$lf"
        fi
    done
    # Full backend log — the cloud-dns / VNet / service-discovery flow needs the
    # whole API sequence, not a tail.
    bl="${LOG_DIR:-/tmp}/sockerless-backend-${BLEEPLAB_BACKEND:-}.log"
    if [ -f "$bl" ]; then
        echo "=== FULL $bl ==="
        cat "$bl"
    fi
}

PIDS=()
cleanup() {
    log "Cleaning up..."
    for pid in "${PIDS[@]}"; do kill "$pid" 2>/dev/null || true; done
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
    for _ in $(seq 1 "$max"); do
        if curl -sf "$url" >/dev/null 2>&1; then return 0; fi
        sleep 1
    done
    fail "Timeout waiting for $url"
}

# bleeplab control-plane API helper.
BL=http://127.0.0.1:8929
bl() { # METHOD PATH [JSON]
    curl -sf -X "$1" "$BL$2" -H 'Content-Type: application/json' ${3:+-d "$3"}
}

# ── Provision the sim-backed sockerless backend (ECS) ──────────────────
provision_ecs() {
    SIM_EFS_DATA_DIR="$SOCKERLESS_HARNESS_DATA_DIR"
    export SIM_EFS_DATA_DIR
    export AWS_ACCESS_KEY_ID=sim AWS_SECRET_ACCESS_KEY=sim AWS_REGION=us-east-1
    SIM_ADDR="127.0.0.1:4566"
    LOG_DIR="$SIM_EFS_DATA_DIR/logs"
    mkdir -p "$LOG_DIR"

    log "Starting simulator-aws on $SIM_ADDR"
    simulator-aws --addr "$SIM_ADDR" >"$LOG_DIR/simulator-aws.log" 2>&1 &
    PIDS+=($!)
    wait_for_url "http://$SIM_ADDR/health"

    log "Bootstrapping sim: ECS cluster + EFS workspace"
    curl -sf -X POST "http://$SIM_ADDR/" -H "Content-Type: application/x-amz-json-1.1" \
        -H "X-Amz-Target: AmazonEC2ContainerServiceV20141113.CreateCluster" \
        -d '{"clusterName":"sim-cluster"}' >/dev/null || fail "create ECS cluster"

    FS_ID=$(curl -sf -X POST "http://$SIM_ADDR/2015-02-01/file-systems" -H 'Content-Type: application/json' \
        -d '{"CreationToken":"bleeplab-runner"}' | jq -r '.FileSystemId // empty')
    [ -n "$FS_ID" ] || fail "create EFS filesystem"
    WS_AP_ID=$(curl -sf -X POST "http://$SIM_ADDR/2015-02-01/access-points" -H 'Content-Type: application/json' \
        -d "{\"ClientToken\":\"ws\",\"FileSystemId\":\"$FS_ID\",\"RootDirectory\":{\"Path\":\"/runner-ws\"}}" | jq -r '.AccessPointId // empty')
    [ -n "$WS_AP_ID" ] || fail "create workspace access point"

    WORK_DIR="$SIM_EFS_DATA_DIR/$FS_ID/runner-ws"
    mkdir -p "$WORK_DIR"

    case "$(uname -m)" in
        x86_64)        ECS_ARCH=X86_64; WORKLOAD_ARCH=amd64 ;;
        aarch64|arm64) ECS_ARCH=ARM64;  WORKLOAD_ARCH=arm64 ;;
        *) fail "unsupported arch $(uname -m)" ;;
    esac

    # The sim runs ECS tasks on the host engine, so workloads are host-arch.
    # Image manifest selection (incl. the arch-specific gitlab-runner-helper
    # tag) must match — otherwise an arm64-only helper has no amd64 entry.
    export SOCKERLESS_WORKLOAD_ARCH="$WORKLOAD_ARCH"
    export SOCKERLESS_ENDPOINT_URL="http://$SIM_ADDR"
    export SOCKERLESS_ECS_CLUSTER=sim-cluster
    export SOCKERLESS_ECS_SUBNETS=subnet-0123456789abcdef0
    export SOCKERLESS_ECS_EXECUTION_ROLE_ARN=arn:aws:iam::000000000000:role/sim
    export SOCKERLESS_ECS_CPU_ARCHITECTURE="$ECS_ARCH"
    export SOCKERLESS_CALLBACK_URL=http://host.docker.internal:3375
    export SOCKERLESS_AUTO_AGENT_BIN=/usr/local/bin/sockerless-agent
    # gitlab-runner build-container binds (e.g. its build/cache dirs) translate
    # onto the EFS workspace access point.
    export SOCKERLESS_ECS_SHARED_VOLUMES="runner-ws=${WORK_DIR}=${WS_AP_ID}=${FS_ID}"

    log "Starting sockerless-backend-ecs on :3375"
    sockerless-backend-ecs --addr :3375 --log-level debug >"$LOG_DIR/sockerless-backend-ecs.log" 2>&1 &
    PIDS+=($!)
    wait_for_url "http://127.0.0.1:3375/_ping"
    log "sockerless-backend-ecs ready"
}

# write_fake_sa_json writes a service-account JSON whose token_uri points at the
# sim's /token endpoint, so the backend's google clients sign + exchange against
# the sim exactly as against real GCP (differing only in coordinates).
write_fake_sa_json() {
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

# ── Provision the sim-backed sockerless backend (Cloud Run) ────────────
provision_cloudrun() {
    # Unlike ECS's live EFS mount, the Cloud Run backend shares the runner
    # workspace via GCS snapshot-sync (gcs-sync): the workspace mount inside
    # the job container is an empty tmpfs the bootstrap restores from GCS
    # before each exec and persists back after; the backend syncs the same
    # GCS object to/from the runner's local --work dir. The sim materialises
    # each GCS bucket at $SIM_GCS_DATA_DIR/<bucket> on the host engine.
    SIM_GCS_DATA_DIR="$SOCKERLESS_HARNESS_DATA_DIR"
    export SIM_GCS_DATA_DIR
    SIM_ADDR="127.0.0.1:4567"
    local sim_grpc_port=4577
    local project="sim-project"
    local build_bucket="sockerless-build"
    local ws_bucket="sockerless-runner-ws"
    # gitlab-runner picks its helper image arch from the DOCKER_HOST's reported
    # arch (docker /version), and the sim selects image manifests by
    # SOCKERLESS_WORKLOAD_ARCH — both must match the host the workloads run on.
    # gitlab's helper tag uses `x86_64`/`arm64`.
    local build_platform helper_arch workload_arch
    case "$(uname -m)" in
        aarch64|arm64) build_platform="linux/arm64"; helper_arch="arm64";  workload_arch="arm64" ;;
        *)             build_platform="linux/amd64"; helper_arch="x86_64"; workload_arch="amd64" ;;
    esac
    export SOCKERLESS_WORKLOAD_ARCH="$workload_arch"

    LOG_DIR="$SOCKERLESS_HARNESS_DATA_DIR/logs"
    mkdir -p "$LOG_DIR"

    # The sim serves the Cloud Logging admin read API over gRPC on a separate
    # port; the backend reads container logs from it.
    export SIM_GCP_GRPC_PORT="$sim_grpc_port"
    log "Starting simulator-gcp on :4567 (gRPC :$sim_grpc_port)"
    simulator-gcp --addr ":4567" >"$LOG_DIR/simulator-gcp.log" 2>&1 &
    PIDS+=($!)
    wait_for_url "http://$SIM_ADDR/health"

    log "Bootstrapping sim: GCS buckets (build + workspace)"
    local b
    for b in "$build_bucket" "$ws_bucket"; do
        curl -sf -X POST "http://$SIM_ADDR/storage/v1/b?project=$project" \
            -H 'Content-Type: application/json' -d "{\"name\":\"$b\"}" >/dev/null \
            || fail "create GCS bucket $b"
    done

    # GCP auth: a fake SA JSON whose token_uri is the sim's /token.
    local sa_json="$SOCKERLESS_HARNESS_DATA_DIR/cloudrun-sa.json"
    write_fake_sa_json "$sa_json" "http://$SIM_ADDR/token" "$project"
    export GOOGLE_APPLICATION_CREDENTIALS="$sa_json"

    # The gitlab-runner build workspace is a LOCAL dir synced to/from GCS
    # around each container-job exec (unlike ECS, where it lives on the EFS
    # mount). gitlab-runner has no externals tree (that is github-runner only).
    WORK_DIR="$SOCKERLESS_HARNESS_DATA_DIR/runner-ws"
    mkdir -p "$WORK_DIR"

    export SOCKERLESS_ENDPOINT_URL="http://$SIM_ADDR"
    export SOCKERLESS_GCP_LOGADMIN_ENDPOINT="127.0.0.1:${sim_grpc_port}"
    export SOCKERLESS_GCR_PROJECT="$project"
    export SOCKERLESS_GCR_REGION="us-central1"
    export SOCKERLESS_GCP_BUILD_BUCKET="$build_bucket"
    export SOCKERLESS_GCP_BUILD_PLATFORM="$build_platform"
    export SOCKERLESS_POLL_INTERVAL=500ms
    # The reverse-agent overlay is built via Cloud Build, pushed to Artifact
    # Registry, and pulled on the Cloud Run task — all at this registry
    # coordinate: the sim's /v2/ published to the host engine at
    # 127.0.0.1:5000. A real docker push (build) + pull (run); registry and
    # compute stay agnostic, connected only by the /v2/ API.
    export SOCKERLESS_GCP_AR_ENDPOINT="127.0.0.1:5000"
    export SOCKERLESS_CLOUDRUN_BOOTSTRAP=/usr/local/bin/sockerless-cloudrun-bootstrap
    # The overlay pull + Service start + bootstrap dial-back must complete
    # within this window (kept below the per-job wait so a real reverse-agent
    # registration failure surfaces as "did not register").
    # NOTE: on a freshly-created podman machine the sim-registry insecure
    # drop-in is not honored by the build path until the podman service
    # reloads — `podman machine stop && start` once, or the overlay `FROM`
    # pull fails with "http: server gave HTTP response to HTTPS client".
    export SOCKERLESS_CLOUDRUN_BOOTSTRAP_TIMEOUT_SEC=180
    export SOCKERLESS_AUTO_AGENT_BIN=/usr/local/bin/sockerless-agent
    # Cloud Run exec/attach is via the reverse agent: the overlay bootstrap
    # inside the task dials back to the backend's reverse endpoint.
    export SOCKERLESS_CALLBACK_URL="ws://host.docker.internal:3375/v1/cloudrun/reverse"
    # The in-container bootstrap's gcs-sync workspace restore/save reaches the
    # sim's storage through the published sim port on the host gateway (the
    # backend's in-container SOCKERLESS_ENDPOINT_URL is not workload-reachable).
    # Injected into the task as STORAGE_EMULATOR_HOST.
    export SOCKERLESS_GCS_WORKLOAD_ENDPOINT="host.docker.internal:5000"
    # gitlab-runner cycles its OpenStdin container across stages; the backend
    # keeps the Cloud Run Service alive between stages (backend_impl.go). The
    # `services:` container runs as a Cloud Run Service discovered over Cloud
    # DNS via the VPC connector.
    export SOCKERLESS_GCR_USE_SERVICE=1
    export SOCKERLESS_GCR_VPC_CONNECTOR="projects/$project/locations/us-central1/connectors/sim-connector"
    # gitlab-runner build-container binds translate onto the gcs-sync workspace.
    export SOCKERLESS_GCP_SHARED_VOLUMES="runner-ws=${WORK_DIR}=${ws_bucket}=gcs-sync"

    # Stage workload base images into the host daemon: the Cloud Run overlay
    # build rewrites a Docker Hub base (alpine/redis) to the AR docker-hub
    # pull-through ref the sim serves by hydrating from the local daemon, so
    # the base must be present locally. Pull from the ECR gallery (no Docker
    # Hub throttle) and tag under the bare Docker Hub name the pull-through maps.
    log "Staging workload base images (alpine, redis) into the host daemon…"
    local img src ok
    for img in "alpine:3.20" "redis:7-alpine"; do
        src="public.ecr.aws/docker/library/$img"
        ok=""
        for attempt in 1 2 3 4 5; do
            if docker pull -q "$src" >/dev/null 2>&1; then ok=1; break; fi
            sleep "$((attempt * 3))"
        done
        [ -n "$ok" ] || fail "pull base image $src"
        docker tag "$src" "$img" || fail "tag base image $img"
    done

    # The gitlab-runner-helper (git clone + artifact-transfer container) is also
    # overlay-built on Cloud Run, so the sim's gitlab-registry pull-through must
    # hydrate it from the local daemon. Stage the arch-matched tag straight from
    # registry.gitlab.com (not Docker-Hub-throttled); the version must match the
    # gitlab-runner binary baked into this image.
    local runner_ver helper_ref
    runner_ver=$(gitlab-runner --version 2>/dev/null | sed -n 's/^Version:[[:space:]]*//p' | head -1)
    [ -n "$runner_ver" ] || fail "determine gitlab-runner version"
    helper_ref="registry.gitlab.com/gitlab-org/gitlab-runner/gitlab-runner-helper:${helper_arch}-v${runner_ver}"
    log "Staging gitlab-runner-helper ($helper_ref)…"
    ok=""
    for attempt in 1 2 3 4 5; do
        if docker pull -q "$helper_ref" >/dev/null 2>&1; then ok=1; break; fi
        sleep "$((attempt * 3))"
    done
    [ -n "$ok" ] || fail "pull gitlab-runner-helper $helper_ref"

    log "Starting sockerless-backend-cloudrun on :3375"
    sockerless-backend-cloudrun --addr :3375 --log-level debug >"$LOG_DIR/sockerless-backend-cloudrun.log" 2>&1 &
    PIDS+=($!)
    wait_for_url "http://127.0.0.1:3375/_ping"
    log "sockerless-backend-cloudrun ready (gcs-sync workspace)"
}

# ── Provision the sim-backed sockerless backend (Cloud Run Functions) ──
provision_gcf() {
    # GCF (Cloud Run Functions, Gen2) deploys container-jobs as multi-container
    # Cloud Run Service revisions (Functions Gen2 build on Cloud Run), so it
    # shares the runner workspace via GCS snapshot-sync exactly like the Cloud
    # Run cell. A `services:` container co-deploys as a revision sidecar sharing
    # loopback with the job container (the BUG-1781 gcf network-pod path), not a
    # separate Service — so SOCKERLESS_GCR_USE_SERVICE is NOT set here.
    SIM_GCS_DATA_DIR="$SOCKERLESS_HARNESS_DATA_DIR"
    export SIM_GCS_DATA_DIR
    SIM_ADDR="127.0.0.1:4567"
    local sim_grpc_port=4577
    local project="sim-project"
    local build_bucket="sockerless-build"
    local ws_bucket="sockerless-runner-ws"
    # gitlab-runner picks its helper image arch from the DOCKER_HOST's reported
    # arch (docker /version); the sim selects manifests by SOCKERLESS_WORKLOAD_ARCH.
    local build_platform helper_arch workload_arch
    case "$(uname -m)" in
        aarch64|arm64) build_platform="linux/arm64"; helper_arch="arm64";  workload_arch="arm64" ;;
        *)             build_platform="linux/amd64"; helper_arch="x86_64"; workload_arch="amd64" ;;
    esac
    export SOCKERLESS_WORKLOAD_ARCH="$workload_arch"

    LOG_DIR="$SOCKERLESS_HARNESS_DATA_DIR/logs"
    mkdir -p "$LOG_DIR"

    export SIM_GCP_GRPC_PORT="$sim_grpc_port"
    log "Starting simulator-gcp on :4567 (gRPC :$sim_grpc_port)"
    simulator-gcp --addr ":4567" >"$LOG_DIR/simulator-gcp.log" 2>&1 &
    PIDS+=($!)
    wait_for_url "http://$SIM_ADDR/health"

    log "Bootstrapping sim: GCS buckets (build + workspace)"
    local b
    for b in "$build_bucket" "$ws_bucket"; do
        curl -sf -X POST "http://$SIM_ADDR/storage/v1/b?project=$project" \
            -H 'Content-Type: application/json' -d "{\"name\":\"$b\"}" >/dev/null \
            || fail "create GCS bucket $b"
    done

    local sa_json="$SOCKERLESS_HARNESS_DATA_DIR/gcf-sa.json"
    write_fake_sa_json "$sa_json" "http://$SIM_ADDR/token" "$project"
    export GOOGLE_APPLICATION_CREDENTIALS="$sa_json"

    WORK_DIR="$SOCKERLESS_HARNESS_DATA_DIR/runner-ws"
    mkdir -p "$WORK_DIR"

    export SOCKERLESS_ENDPOINT_URL="http://$SIM_ADDR"
    export SOCKERLESS_GCP_LOGADMIN_ENDPOINT="127.0.0.1:${sim_grpc_port}"
    export SOCKERLESS_GCF_PROJECT="$project"
    export SOCKERLESS_GCF_REGION="us-central1"
    export SOCKERLESS_GCP_BUILD_BUCKET="$build_bucket"
    export SOCKERLESS_GCP_BUILD_PLATFORM="$build_platform"
    export SOCKERLESS_POLL_INTERVAL=500ms
    # Overlay built via Cloud Build → Artifact Registry, pulled on the Cloud Run
    # Service revision the function is backed by — all at the sim's /v2/
    # published to the host engine at 127.0.0.1:5000.
    export SOCKERLESS_GCP_AR_ENDPOINT="127.0.0.1:5000"
    export SOCKERLESS_GCF_BOOTSTRAP=/usr/local/bin/sockerless-gcf-bootstrap
    export SOCKERLESS_GCF_BOOTSTRAP_TIMEOUT_SEC=180
    export SOCKERLESS_AUTO_AGENT_BIN=/usr/local/bin/sockerless-agent
    # GCF exec/attach is via the reverse agent: the overlay bootstrap inside the
    # function dials back to the backend's reverse endpoint.
    export SOCKERLESS_CALLBACK_URL="ws://host.docker.internal:3375/v1/gcf/reverse"
    export SOCKERLESS_GCS_WORKLOAD_ENDPOINT="host.docker.internal:5000"
    # `services:` containers co-deploy as revision sidecars sharing loopback,
    # discovered via /etc/hosts.
    export SOCKERLESS_GCF_VPC_CONNECTOR="projects/$project/locations/us-central1/connectors/sim-connector"
    export SOCKERLESS_GCP_SHARED_VOLUMES="runner-ws=${WORK_DIR}=${ws_bucket}=gcs-sync"

    log "Staging workload base images (alpine, redis) into the host daemon…"
    local img src ok
    for img in "alpine:3.20" "redis:7-alpine"; do
        src="public.ecr.aws/docker/library/$img"
        ok=""
        for attempt in 1 2 3 4 5; do
            if docker pull -q "$src" >/dev/null 2>&1; then ok=1; break; fi
            sleep "$((attempt * 3))"
        done
        [ -n "$ok" ] || fail "pull base image $src"
        docker tag "$src" "$img" || fail "tag base image $img"
    done

    # The gitlab-runner-helper is overlay-built on Cloud Run too, so the sim's
    # gitlab-registry pull-through must hydrate it from the local daemon.
    local runner_ver helper_ref
    runner_ver=$(gitlab-runner --version 2>/dev/null | sed -n 's/^Version:[[:space:]]*//p' | head -1)
    [ -n "$runner_ver" ] || fail "determine gitlab-runner version"
    helper_ref="registry.gitlab.com/gitlab-org/gitlab-runner/gitlab-runner-helper:${helper_arch}-v${runner_ver}"
    log "Staging gitlab-runner-helper ($helper_ref)…"
    ok=""
    for attempt in 1 2 3 4 5; do
        if docker pull -q "$helper_ref" >/dev/null 2>&1; then ok=1; break; fi
        sleep "$((attempt * 3))"
    done
    [ -n "$ok" ] || fail "pull gitlab-runner-helper $helper_ref"

    log "Starting sockerless-backend-gcf on :3375"
    sockerless-backend-gcf --addr :3375 --log-level debug >"$LOG_DIR/sockerless-backend-gcf.log" 2>&1 &
    PIDS+=($!)
    wait_for_url "http://127.0.0.1:3375/_ping"
    log "sockerless-backend-gcf ready (gcs-sync workspace)"
}

# stage_gitlab_helper pulls the arch-matched gitlab-runner-helper into the host
# daemon so the azure/gcp sim's registry pull-through can hydrate it when the
# backend overlay-builds the reverse-agent bootstrap FROM it (ACR Tasks / Cloud
# Build). ECS skips this (no overlay — the runner pulls the helper directly).
stage_gitlab_helper() { # helper_arch
    local runner_ver helper_ref
    runner_ver=$(gitlab-runner --version 2>/dev/null | sed -n 's/^Version:[[:space:]]*//p' | head -1)
    [ -n "$runner_ver" ] || fail "determine gitlab-runner version"
    helper_ref="registry.gitlab.com/gitlab-org/gitlab-runner/gitlab-runner-helper:${1}-v${runner_ver}"
    log "Staging gitlab-runner-helper ($helper_ref)…"
    local attempt
    for attempt in 1 2 3 4 5; do
        if docker pull -q "$helper_ref" >/dev/null 2>&1; then return 0; fi
        sleep "$((attempt * 3))"
    done
    fail "pull gitlab-runner-helper $helper_ref"
}

provision_aca() {
    # ACA deploys container-jobs as Container Apps (the App path,
    # SOCKERLESS_ACA_USE_APP=1): backend-aca builds the reverse-agent overlay via
    # ACR Tasks (uploads the build context to a blob container, scheduleRun →
    # docker build on the host engine, push to the sim's /v2/), then runs the
    # App and exec/attaches over the reverse agent. A `services:` container
    # co-deploys as a sibling Container App sharing the per-build network. The
    # runner workspace is shared via an Azure-Files-ephemeral volume.
    SIM_AZURE_FILES_DATA_DIR="$SOCKERLESS_HARNESS_DATA_DIR"
    export SIM_AZURE_FILES_DATA_DIR
    SIM_ADDR="127.0.0.1:4568"
    local sim_endpoint="http://localhost:4568"
    local sub="00000000-0000-0000-0000-000000000001"
    local rg="sim-rg" acct="simstorage" env="sockerless" acr="simacr"
    local build_platform helper_arch workload_arch
    case "$(uname -m)" in
        aarch64|arm64) build_platform="linux/arm64"; helper_arch="arm64";  workload_arch="arm64" ;;
        *)             build_platform="linux/amd64"; helper_arch="x86_64"; workload_arch="amd64" ;;
    esac
    export SOCKERLESS_WORKLOAD_ARCH="$workload_arch"

    LOG_DIR="$SOCKERLESS_HARNESS_DATA_DIR/logs"
    mkdir -p "$LOG_DIR"

    # The ACR-Tasks overlay build uploads its context to blob storage via the
    # storage account's advertised endpoint. Pin that to a deterministic
    # `<account>.blob.localhost` host and resolve it to loopback inside this
    # container (`*.localhost` is not special-cased by the container resolver).
    export SIM_AZURE_ARM_EXTERNAL_DATA_PLANE_URLS_JSON='{"storage":{"blob":"http://{account}.blob.localhost:{port}/"}}'
    if ! grep -q "${acct}.blob.localhost" /etc/hosts 2>/dev/null; then
        echo "127.0.0.1 ${acct}.blob.localhost" >>/etc/hosts || fail "add storage host alias"
    fi

    log "Starting simulator-azure on :4568 (all interfaces, so the published registry port reaches it)"
    simulator-azure --addr ":4568" >"$LOG_DIR/simulator-azure.log" 2>&1 &
    PIDS+=($!)
    wait_for_url "http://$SIM_ADDR/health"

    log "Bootstrapping sim: storage account + managed environment + ACR"
    curl -sf -X PUT "http://$SIM_ADDR/subscriptions/$sub/resourceGroups/$rg/providers/Microsoft.Storage/storageAccounts/$acct?api-version=2023-01-01" \
        -H 'Content-Type: application/json' \
        -d '{"location":"eastus","sku":{"name":"Standard_LRS"},"kind":"StorageV2","properties":{}}' >/dev/null || fail "create storage account"
    curl -sf -X PUT "http://$SIM_ADDR/subscriptions/$sub/resourceGroups/$rg/providers/Microsoft.App/managedEnvironments/$env?api-version=2024-03-01" \
        -H 'Content-Type: application/json' \
        -d '{"location":"eastus","properties":{}}' >/dev/null || fail "create managed environment"
    curl -sf -X PUT "http://$SIM_ADDR/subscriptions/$sub/resourceGroups/$rg/providers/Microsoft.ContainerRegistry/registries/$acr?api-version=2023-07-01" \
        -H 'Content-Type: application/json' \
        -d '{"location":"eastus","sku":{"name":"Basic"},"properties":{}}' >/dev/null || fail "create ACR registry"
    curl -sf -X PUT "http://$SIM_ADDR/build-context?restype=container" \
        -H "Host: ${acct}.blob.localhost:4568" >/dev/null || fail "create build-context container"

    WORK_DIR="$SIM_AZURE_FILES_DATA_DIR/$acct/runner-ws"
    mkdir -p "$WORK_DIR"

    export SOCKERLESS_ENDPOINT_URL="$sim_endpoint"
    export SOCKERLESS_ACA_SUBSCRIPTION_ID="$sub"
    export SOCKERLESS_ACA_RESOURCE_GROUP="$rg"
    export SOCKERLESS_ACA_STORAGE_ACCOUNT="$acct"
    export SOCKERLESS_ACA_LOG_ANALYTICS_WORKSPACE="default"
    export SOCKERLESS_ACA_ENVIRONMENT="$env"
    export SOCKERLESS_ACA_USE_APP=1
    export SOCKERLESS_AZURE_ACR_NAME="$acr"
    export SOCKERLESS_AZURE_BUILD_STORAGE_ACCOUNT="$acct"
    export SOCKERLESS_AZURE_BUILD_CONTAINER="build-context"
    export SOCKERLESS_AZURE_BUILD_PLATFORM="$build_platform"
    export SOCKERLESS_AZURE_ACR_ENDPOINT="127.0.0.1:5000"
    export SOCKERLESS_CALLBACK_URL="ws://host.docker.internal:3375/v1/aca/reverse"
    export SOCKERLESS_ACA_BOOTSTRAP=/usr/local/bin/sockerless-cloudrun-bootstrap
    export SOCKERLESS_AUTO_AGENT_BIN=/usr/local/bin/sockerless-agent
    export SOCKERLESS_ACA_SHARED_VOLUMES="runner-ws=${WORK_DIR}=runner-ws=azure-files-ephemeral"

    stage_gitlab_helper "$helper_arch"

    log "Starting sockerless-backend-aca on :3375"
    sockerless-backend-aca --addr :3375 --log-level debug >"$LOG_DIR/sockerless-backend-aca.log" 2>&1 &
    PIDS+=($!)
    wait_for_url "http://127.0.0.1:3375/_ping"
    log "sockerless-backend-aca ready (azure-files-ephemeral workspace)"
}

provision_azf() {
    # Azure Functions deploys container-jobs as Linux Function App sites whose
    # sitecontainers run the workload (SOCKERLESS_AZF_REGISTRY overlay built via
    # ACR Tasks, same build→push→pull as aca): backend-azf builds the
    # reverse-agent overlay, deploys the site, and exec/attaches over the
    # reverse agent. A `services:` container deploys as a sibling site on the
    # per-build network, reachable by name through Azure Private DNS (cloud-dns
    # discovery — azf's faithful network primitive, matching aca). The runner
    # workspace is shared via an Azure-Files-ephemeral volume.
    SIM_AZURE_FILES_DATA_DIR="$SOCKERLESS_HARNESS_DATA_DIR"
    export SIM_AZURE_FILES_DATA_DIR
    SIM_ADDR="127.0.0.1:4568"
    local sim_endpoint="http://localhost:4568"
    local sub="00000000-0000-0000-0000-000000000001"
    local rg="sim-rg" acct="simstorage" plan="sockerless-plan" acr="simacr"
    local build_platform helper_arch workload_arch
    case "$(uname -m)" in
        aarch64|arm64) build_platform="linux/arm64"; helper_arch="arm64";  workload_arch="arm64" ;;
        *)             build_platform="linux/amd64"; helper_arch="x86_64"; workload_arch="amd64" ;;
    esac
    export SOCKERLESS_WORKLOAD_ARCH="$workload_arch"

    LOG_DIR="$SOCKERLESS_HARNESS_DATA_DIR/logs"
    mkdir -p "$LOG_DIR"

    # ACR-Tasks overlay build uploads its context to blob storage via the
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
    curl -sf -X PUT "http://$SIM_ADDR/subscriptions/$sub/resourceGroups/$rg/providers/Microsoft.Web/serverfarms/$plan?api-version=2023-12-01" \
        -H 'Content-Type: application/json' \
        -d '{"location":"eastus","sku":{"name":"EP1","tier":"ElasticPremium"},"kind":"linux","properties":{"reserved":true}}' >/dev/null || fail "create App Service plan"
    curl -sf -X PUT "http://$SIM_ADDR/subscriptions/$sub/resourceGroups/$rg/providers/Microsoft.ContainerRegistry/registries/$acr?api-version=2023-07-01" \
        -H 'Content-Type: application/json' \
        -d '{"location":"eastus","sku":{"name":"Basic"},"properties":{}}' >/dev/null || fail "create ACR registry"
    curl -sf -X PUT "http://$SIM_ADDR/build-context?restype=container" \
        -H "Host: ${acct}.blob.localhost:4568" >/dev/null || fail "create build-context container"

    WORK_DIR="$SIM_AZURE_FILES_DATA_DIR/$acct/runner-ws"
    mkdir -p "$WORK_DIR"

    export SOCKERLESS_ENDPOINT_URL="$sim_endpoint"
    export SOCKERLESS_AZF_SUBSCRIPTION_ID="$sub"
    export SOCKERLESS_AZF_RESOURCE_GROUP="$rg"
    export SOCKERLESS_AZF_STORAGE_ACCOUNT="$acct"
    export SOCKERLESS_AZF_LOG_ANALYTICS_WORKSPACE="default"
    export SOCKERLESS_AZF_APP_SERVICE_PLAN="/subscriptions/$sub/resourceGroups/$rg/providers/Microsoft.Web/serverfarms/$plan"
    export SOCKERLESS_AZF_REGISTRY="${acr}.azurecr.io"
    export SOCKERLESS_AZF_NETWORK_DISCOVERY="cloud-dns"
    export SOCKERLESS_AZURE_ACR_NAME="$acr"
    export SOCKERLESS_AZURE_BUILD_STORAGE_ACCOUNT="$acct"
    export SOCKERLESS_AZURE_BUILD_CONTAINER="build-context"
    export SOCKERLESS_AZURE_BUILD_PLATFORM="$build_platform"
    export SOCKERLESS_AZURE_ACR_ENDPOINT="127.0.0.1:5000"
    export SOCKERLESS_CALLBACK_URL="ws://host.docker.internal:3375/v1/azf/reverse"
    export SOCKERLESS_AZF_BOOTSTRAP=/usr/local/bin/sockerless-azf-bootstrap
    export SOCKERLESS_AUTO_AGENT_BIN=/usr/local/bin/sockerless-agent
    export SOCKERLESS_AZF_SHARED_VOLUMES="runner-ws=${WORK_DIR}=runner-ws=azure-files-ephemeral"

    stage_gitlab_helper "$helper_arch"

    log "Starting sockerless-backend-azf on :3375"
    sockerless-backend-azf --addr :3375 --log-level debug >"$LOG_DIR/sockerless-backend-azf.log" 2>&1 &
    PIDS+=($!)
    wait_for_url "http://127.0.0.1:3375/_ping"
    log "sockerless-backend-azf ready (azure-files-ephemeral workspace)"
}

BLEEPLAB_BACKEND="${BLEEPLAB_BACKEND:-ecs}"
case "$BLEEPLAB_BACKEND" in
    ecs)      provision_ecs ;;
    cloudrun) provision_cloudrun ;;
    gcf)      provision_gcf ;;
    aca)      provision_aca ;;
    azf)      provision_azf ;;
    *) fail "unsupported BLEEPLAB_BACKEND: $BLEEPLAB_BACKEND (ecs|cloudrun|gcf|aca|azf)" ;;
esac

# ── Start bleeplab ─────────────────────────────────────────────────────
echo "127.0.0.1 host.docker.internal" >> /etc/hosts
# The runner clones git_info.repo_url from inside the job/helper container,
# which reaches bleeplab via host.docker.internal (not the harness loopback the
# runner process uses for the control-plane API).
export BLEEPLAB_EXTERNAL_URL="http://host.docker.internal:8929"
log "Starting bleeplab on :8929 (git external URL $BLEEPLAB_EXTERNAL_URL)"
bleeplab --addr :8929 --log-level info >"$LOG_DIR/bleeplab.log" 2>&1 &
PIDS+=($!)
wait_for_url "$BL/health"

# ── Stage workload images on the host engine ───────────────────────────
# The aws sim runs ECS tasks as host docker containers, which pull images
# directly. Pre-pull alpine from the ECR gallery (no Docker Hub throttle) and
# tag it; the gitlab-runner helper comes from registry.gitlab.com (not
# throttled) and the runner pulls it on first use.
log "Staging workload images (alpine, redis) on the host engine…"
stage_image() { # gallery-ref local-tag
    for attempt in 1 2 3 4 5; do
        if docker pull -q "$1" >/dev/null 2>&1; then
            docker tag "$1" "$2"
            return 0
        fi
        sleep "$((attempt * 3))"
    done
    fail "stage image $1"
}
stage_image public.ecr.aws/docker/library/alpine:3.20 alpine:3.20
# redis backs the `services:` job — a real CI service container the build job
# connects to by network alias over the per-build pod network.
stage_image public.ecr.aws/docker/library/redis:7-alpine redis:7-alpine

# ── Create the project, CI config, runner, and pipeline ────────────────
log "Creating project + .gitlab-ci.yml + runner + pipeline"
PID=$(bl POST /api/v4/projects '{"name":"demo"}' | jq -r '.id')
[ -n "$PID" ] || fail "create project"

# A real arithmetic calculator the CI job compiles from the cloned source and
# runs — genuine build + execute work (not an echo), proving the clone delivers
# usable source and the cloud workload compiles + runs it correctly.
CALC_C=$(cat <<'EOF'
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

static long apply(long a, const char *op, long b, int *err) {
    *err = 0;
    if (!strcmp(op, "+")) return a + b;
    if (!strcmp(op, "-")) return a - b;
    if (!strcmp(op, "*") || !strcmp(op, "x")) return a * b;
    if (!strcmp(op, "/")) { if (b == 0) { *err = 2; return 0; } return a / b; }
    if (!strcmp(op, "%")) { if (b == 0) { *err = 2; return 0; } return a % b; }
    *err = 1;
    return 0;
}

static int selftest(void) {
    struct { long a; const char *op; long b; long want; } c[] = {
        {3, "+", 4, 7}, {10, "-", 3, 7}, {6, "x", 7, 42},
        {20, "/", 5, 4}, {17, "%", 5, 2}, {-3, "+", 8, 5},
    };
    size_t n = sizeof(c) / sizeof(c[0]);
    for (size_t i = 0; i < n; i++) {
        int err;
        long got = apply(c[i].a, c[i].op, c[i].b, &err);
        if (err || got != c[i].want) {
            fprintf(stderr, "selftest FAIL: %ld %s %ld = %ld (want %ld)\n",
                    c[i].a, c[i].op, c[i].b, got, c[i].want);
            return 1;
        }
    }
    printf("CALC-SELFTEST-OK (%zu cases)\n", n);
    return 0;
}

int main(int argc, char **argv) {
    int quiet = 0, i = 1;
    if (i < argc && (!strcmp(argv[i], "-q") || !strcmp(argv[i], "--value"))) { quiet = 1; i++; }
    if (!quiet && i < argc && !strcmp(argv[i], "--selftest")) return selftest();
    if (argc - i != 3) {
        fprintf(stderr, "usage: %s [-q] <a> <op> <b> | --selftest\n", argv[0]);
        return 64;
    }
    char *ea, *eb;
    long a = strtol(argv[i], &ea, 10);
    long b = strtol(argv[i + 2], &eb, 10);
    if (*ea || *eb) { fprintf(stderr, "non-integer operand\n"); return 65; }
    int err;
    long r = apply(a, argv[i + 1], b, &err);
    if (err == 1) { fprintf(stderr, "unknown operator %s\n", argv[i + 1]); return 66; }
    if (err == 2) { fprintf(stderr, "division by zero\n"); return 67; }
    if (quiet) printf("%ld\n", r);
    else printf("%ld %s %ld = %ld\n", a, argv[i + 1], b, r);
    return 0;
}
EOF
)

# GIT_STRATEGY: clone — the runner clones the project repo bleeplab serves over
# smart-HTTP into CI_PROJECT_DIR, exactly as against gitlab.com. The build job
# compiles calc.c with gcc and publishes the `calc` binary as an artifact; the
# test job downloads that artifact (no recompile — it has no toolchain) and
# verifies real arithmetic, proving cross-stage artifact passing end to end.
CI='stages: [build, test, integration]
build-job:
  stage: build
  image: alpine:3.20
  variables:
    GIT_STRATEGY: "clone"
  script:
    - echo "BLEEPLAB-BUILD on $(uname -m)"
    - apk add --no-cache gcc musl-dev
    - gcc -O2 -Wall -Werror -o calc calc.c
    - ./calc --selftest
    - ./calc 6 x 7
    - echo BLEEPLAB-BUILD-OK
  artifacts:
    paths:
      - calc
test-job:
  stage: test
  image: alpine:3.20
  variables:
    GIT_STRATEGY: "clone"
  script:
    - echo "BLEEPLAB-TEST consuming the build artifact (no toolchain, no recompile)"
    - test -x ./calc && echo ARTIFACT-CALC-PRESENT
    - ./calc --selftest
    - test "$(./calc 7 + 4)" = "7 + 4 = 11"
    - test "$(./calc 100 / 7)" = "100 / 7 = 14"
    - test "$(./calc 17 % 5)" = "17 % 5 = 2"
    - acc=0; for i in $(seq 1 100); do acc=$(./calc -q $acc + $i); done; echo "SUM 1..100 = $acc"; test "$acc" = "5050"
    - echo BLEEPLAB-TEST-OK
service-job:
  stage: integration
  image: alpine:3.20
  variables:
    GIT_STRATEGY: "none"
    FF_NETWORK_PER_BUILD: "true"
  services:
    - name: redis:7-alpine
      alias: redis
  script:
    - echo "BLEEPLAB-SERVICE connecting to the redis service by alias over the pod network"
    - apk add --no-cache redis
    - for i in $(seq 1 30); do redis-cli -h redis ping >/dev/null 2>&1 && break; sleep 1; done
    - test "$(redis-cli -h redis ping)" = "PONG"
    - redis-cli -h redis set bleeplab 42 >/dev/null
    - test "$(redis-cli -h redis get bleeplab)" = "42"
    - echo BLEEPLAB-SERVICE-OK'
# Commit the CI config + calculator source via the bleeplab commits API (the
# repo bleeplab then serves over git; JSON-safe via jq).
jq -n --arg ci "$CI" --arg calc "$CALC_C" \
    '{branch:"main",actions:[{file_path:".gitlab-ci.yml",content:$ci},{file_path:"calc.c",content:$calc}]}' \
    | curl -sf -X POST "$BL/api/v4/projects/$PID/repository/commits" -H 'Content-Type: application/json' -d @- >/dev/null \
    || fail "commit .gitlab-ci.yml + calc.c"

TOKEN=$(bl POST /api/v4/user/runners '{"runner_type":"project_type"}' | jq -r '.token')
[ -n "$TOKEN" ] || fail "create runner"
PLID=$(bl POST "/api/v4/projects/$PID/pipeline" '{"ref":"main"}' | jq -r '.id')
[ -n "$PLID" ] || fail "create pipeline"
log "project=$PID runner=$TOKEN pipeline=$PLID"

# ── Run the gitlab-runner against the sockerless backend ───────────────
# The runner's URL must be reachable from BOTH the runner process (here, the
# harness host) AND the job/helper containers (where the artifacts
# uploader/downloader and git client run): host.docker.internal resolves to the
# harness loopback via /etc/hosts here, and to the host gateway (published
# :8929) from the containers. So a single coordinate works everywhere.
cat > /tmp/gitlab-runner-config.toml <<EOF
concurrent = 1
check_interval = 1

[[runners]]
  name = "bleeplab-$BLEEPLAB_BACKEND"
  url = "$BLEEPLAB_EXTERNAL_URL"
  token = "$TOKEN"
  executor = "docker"
  [runners.docker]
    host = "tcp://127.0.0.1:3375"
    image = "alpine:3.20"
    pull_policy = ["if-not-present"]
    privileged = false
EOF

log "Starting gitlab-runner (docker executor → sockerless-backend-ecs)"
gitlab-runner run --config /tmp/gitlab-runner-config.toml >"$LOG_DIR/gitlab-runner.log" 2>&1 &
PIDS+=($!)

# ── Wait for the pipeline to finish ────────────────────────────────────
log "Waiting for pipeline $PLID to complete…"
STATUS=""
# Three stages (build, test, integration), each pulling images + apk-installing
# toolchains, comfortably exceed a 240s budget — poll up to ~7 min.
for _ in $(seq 1 210); do
    STATUS=$(bl GET "/api/v4/projects/$PID/pipelines/$PLID" '' | jq -r '.status')
    case "$STATUS" in
        success) log "TEST 1 PASSED: GitLab pipeline succeeded on sockerless-$BLEEPLAB_BACKEND"; break ;;
        failed)
            DBG_TRACE=$(mktemp)
            for JID in $(bl GET "/api/v4/projects/$PID/pipelines/$PLID/jobs" '' | jq -r '.[].id' 2>/dev/null); do
                printf '\n===== job %s trace =====\n' "$JID" >> "$DBG_TRACE"
                bl GET "/api/v4/projects/$PID/jobs/$JID/trace" '' >> "$DBG_TRACE" 2>/dev/null
            done
            fail "pipeline failed (status=failed); job traces:\n$(cat "$DBG_TRACE")"
            ;;
    esac
    sleep 2
done
[ "$STATUS" = "success" ] || fail "pipeline did not finish (last status=$STATUS)"

# ── Assert the calculator was compiled + run correctly in the cloud workload ──
# Concatenate every job's trace, then require the real build/run evidence: the
# compiled self-test, a live multiplication, the folded sum, and both stage
# markers. These only appear if gcc compiled the cloned calc.c and the binary
# executed with correct arithmetic on the sockerless ECS backend.
# Aggregate every job's trace into a file (robust against transient fetch
# blips and large/binary trace bodies that command-substitution mishandles).
TRACE_FILE=$(mktemp)
for attempt in 1 2 3 4 5; do
    : > "$TRACE_FILE"
    JIDS=$(bl GET "/api/v4/projects/$PID/pipelines/$PLID/jobs" '' | jq -r '.[].id' 2>/dev/null)
    for JID in $JIDS; do
        bl GET "/api/v4/projects/$PID/jobs/$JID/trace" '' >> "$TRACE_FILE" 2>/dev/null
        printf '\n' >> "$TRACE_FILE"
    done
    # Done once every job's terminal marker is present (or the last attempt).
    if grep -qF "BLEEPLAB-SERVICE-OK" "$TRACE_FILE" || [ "$attempt" = 5 ]; then
        break
    fi
    sleep 2
done
for marker in \
    "CALC-SELFTEST-OK" \
    "6 x 7 = 42" \
    "BLEEPLAB-BUILD-OK" \
    "ARTIFACT-CALC-PRESENT" \
    "SUM 1..100 = 5050" \
    "BLEEPLAB-TEST-OK" \
    "BLEEPLAB-SERVICE-OK"; do
    grep -qF "$marker" "$TRACE_FILE" || fail "job trace missing expected marker '$marker'; full trace:\n$(cat "$TRACE_FILE")"
done
rm -f "$TRACE_FILE"
log "TEST 2 PASSED: gcc-compiled calc.c ran in the cloud workload (self-test, 6 x 7 = 42, sum 1..100 = 5050)"
log "TEST 3 PASSED: the test stage consumed the build stage's calc artifact (no recompile)"
log "TEST 4 PASSED: the integration stage reached the redis service container by alias over the per-build pod network (PING/SET/GET)"

log "===== ALL bleeplab-$BLEEPLAB_BACKEND INTEGRATION TESTS PASSED ====="

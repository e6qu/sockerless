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
    for lf in "${LOG_DIR:-/tmp}"/simulator-*.log "${LOG_DIR:-/tmp}"/sockerless-backend-"${BLEEPLAB_BACKEND:-*}".log "${LOG_DIR:-/tmp}"/bleeplab.log "${LOG_DIR:-/tmp}"/gitlab-runner.log; do
        if [ -f "$lf" ]; then
            echo "=== tail $lf ==="
            tail -50 "$lf"
        fi
    done
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

BLEEPLAB_BACKEND="${BLEEPLAB_BACKEND:-ecs}"
case "$BLEEPLAB_BACKEND" in
    ecs) provision_ecs ;;
    *) fail "unsupported BLEEPLAB_BACKEND: $BLEEPLAB_BACKEND (ecs)" ;;
esac

# ── Start bleeplab ─────────────────────────────────────────────────────
echo "127.0.0.1 host.docker.internal" >> /etc/hosts
log "Starting bleeplab on :8929"
bleeplab --addr :8929 --log-level info >"$LOG_DIR/bleeplab.log" 2>&1 &
PIDS+=($!)
wait_for_url "$BL/health"

# ── Stage workload images on the host engine ───────────────────────────
# The aws sim runs ECS tasks as host docker containers, which pull images
# directly. Pre-pull alpine from the ECR gallery (no Docker Hub throttle) and
# tag it; the gitlab-runner helper comes from registry.gitlab.com (not
# throttled) and the runner pulls it on first use.
log "Staging workload image (alpine) on the host engine…"
for attempt in 1 2 3 4 5; do
    if docker pull -q public.ecr.aws/docker/library/alpine:3.20 >/dev/null 2>&1; then
        docker tag public.ecr.aws/docker/library/alpine:3.20 alpine:3.20
        break
    fi
    sleep "$((attempt * 3))"
done

# ── Create the project, CI config, runner, and pipeline ────────────────
log "Creating project + .gitlab-ci.yml + runner + pipeline"
PID=$(bl POST /api/v4/projects '{"name":"demo"}' | jq -r '.id')
[ -n "$PID" ] || fail "create project"

CI='stages: [build, test]
build-job:
  stage: build
  image: alpine:3.20
  variables:
    GIT_STRATEGY: "none"
  script:
    - echo BLEEPLAB-ECS-BUILD-OK
    - cat /etc/os-release | head -1
test-job:
  stage: test
  image: alpine:3.20
  variables:
    GIT_STRATEGY: "none"
  script:
    - echo BLEEPLAB-ECS-TEST-OK'
# Commit the CI config via the bleeplab commits API (JSON-safe via jq).
jq -n --arg c "$CI" '{branch:"main",actions:[{file_path:".gitlab-ci.yml",content:$c}]}' \
    | curl -sf -X POST "$BL/api/v4/projects/$PID/repository/commits" -H 'Content-Type: application/json' -d @- >/dev/null \
    || fail "commit .gitlab-ci.yml"

TOKEN=$(bl POST /api/v4/user/runners '{"runner_type":"project_type"}' | jq -r '.token')
[ -n "$TOKEN" ] || fail "create runner"
PLID=$(bl POST "/api/v4/projects/$PID/pipeline" '{"ref":"main"}' | jq -r '.id')
[ -n "$PLID" ] || fail "create pipeline"
log "project=$PID runner=$TOKEN pipeline=$PLID"

# ── Run the gitlab-runner against the sockerless backend ───────────────
cat > /tmp/gitlab-runner-config.toml <<EOF
concurrent = 1
check_interval = 1

[[runners]]
  name = "bleeplab-ecs"
  url = "$BL"
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
for _ in $(seq 1 120); do
    STATUS=$(bl GET "/api/v4/projects/$PID/pipelines/$PLID" '' | jq -r '.status')
    case "$STATUS" in
        success) log "TEST 1 PASSED: GitLab pipeline succeeded on sockerless-$BLEEPLAB_BACKEND"; break ;;
        failed)  fail "pipeline failed (status=failed)" ;;
    esac
    sleep 2
done
[ "$STATUS" = "success" ] || fail "pipeline did not finish (last status=$STATUS)"

# ── Assert the job trace shows the script ran in the cloud workload ────
JID=$(bl GET "/api/v4/projects/$PID/pipelines/$PLID/jobs" '' | jq -r '.[0].id')
TRACE=$(bl GET "/api/v4/projects/$PID/jobs/$JID/trace" '')
echo "$TRACE" | grep -q 'BLEEPLAB-ECS' || fail "job trace missing expected script output:\n$TRACE"

log "===== ALL bleeplab-ecs INTEGRATION TESTS PASSED ====="

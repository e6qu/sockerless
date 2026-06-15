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
    - echo "BLEEPLAB-ECS-BUILD on $(uname -m)"
    - apk add --no-cache gcc musl-dev
    - gcc -O2 -Wall -Werror -o calc calc.c
    - ./calc --selftest
    - ./calc 6 x 7
    - echo BLEEPLAB-ECS-BUILD-OK
  artifacts:
    paths:
      - calc
test-job:
  stage: test
  image: alpine:3.20
  variables:
    GIT_STRATEGY: "clone"
  script:
    - echo "BLEEPLAB-ECS-TEST consuming the build artifact (no toolchain, no recompile)"
    - test -x ./calc && echo ARTIFACT-CALC-PRESENT
    - ./calc --selftest
    - test "$(./calc 7 + 4)" = "7 + 4 = 11"
    - test "$(./calc 100 / 7)" = "100 / 7 = 14"
    - test "$(./calc 17 % 5)" = "17 % 5 = 2"
    - acc=0; for i in $(seq 1 100); do acc=$(./calc -q $acc + $i); done; echo "SUM 1..100 = $acc"; test "$acc" = "5050"
    - echo BLEEPLAB-ECS-TEST-OK
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
    - echo "BLEEPLAB-ECS-SERVICE connecting to the redis service by alias over the pod network"
    - apk add --no-cache redis
    - for i in $(seq 1 30); do redis-cli -h redis ping >/dev/null 2>&1 && break; sleep 1; done
    - test "$(redis-cli -h redis ping)" = "PONG"
    - redis-cli -h redis set bleeplab 42 >/dev/null
    - test "$(redis-cli -h redis get bleeplab)" = "42"
    - echo BLEEPLAB-ECS-SERVICE-OK'
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
  name = "bleeplab-ecs"
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
        failed)  fail "pipeline failed (status=failed)" ;;
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
    if grep -qF "BLEEPLAB-ECS-SERVICE-OK" "$TRACE_FILE" || [ "$attempt" = 5 ]; then
        break
    fi
    sleep 2
done
for marker in \
    "CALC-SELFTEST-OK" \
    "6 x 7 = 42" \
    "BLEEPLAB-ECS-BUILD-OK" \
    "ARTIFACT-CALC-PRESENT" \
    "SUM 1..100 = 5050" \
    "BLEEPLAB-ECS-TEST-OK" \
    "BLEEPLAB-ECS-SERVICE-OK"; do
    grep -qF "$marker" "$TRACE_FILE" || fail "job trace missing expected marker '$marker'; full trace:\n$(cat "$TRACE_FILE")"
done
rm -f "$TRACE_FILE"
log "TEST 2 PASSED: gcc-compiled calc.c ran in the cloud workload (self-test, 6 x 7 = 42, sum 1..100 = 5050)"
log "TEST 3 PASSED: the test stage consumed the build stage's calc artifact (no recompile)"
log "TEST 4 PASSED: the integration stage reached the redis service container by alias over the per-build pod network (PING/SET/GET)"

log "===== ALL bleeplab-ecs INTEGRATION TESTS PASSED ====="

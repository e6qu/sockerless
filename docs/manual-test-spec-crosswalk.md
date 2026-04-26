# Round 9 — Manual test ↔ spec crosswalk

**Goal:** Walk each test in [PLAN_ECS_MANUAL_TESTING.md](../PLAN_ECS_MANUAL_TESTING.md) one at a time against [specs/CLOUD_RESOURCE_MAPPING.md](../specs/CLOUD_RESOURCE_MAPPING.md), recording the spec claim → expected behaviour → actual result for each. ECS + Lambda backends. Discrepancies file as bugs in [BUGS.md](../BUGS.md).

**This file is the work-state.** A new LLM session or a post-compaction resume should:
1. Read the **Status** line below to find the next pending test.
2. Skim **Setup** to see what infrastructure / backends should be running.
3. Continue from the next pending test, updating this file in-place per test.

## Status

- **Round:** 9
- **Started:** 2026-04-26
- **Backends targeted:** ECS + Lambda
- **Last completed test:** D9 ✅ — **All ECS + Lambda tracks complete (A + B + C + D + E + F + G + I)**
- **Next pending test:** *None — round-9 manual sweep complete.* Remaining round-9 wrap items: A46 NotImpl wrapper for SSM signal (deferred), coverage-gap rows for restart-count/kill-signal/ImagePush layer-bytes/LayerContent eviction (deferred), AWS teardown, IAM key deactivate.
- **Bugs filed this round:** BUG-801 + BUG-803 + BUG-805 + BUG-807 + BUG-808 + BUG-809 + BUG-810 (filed + fixed); BUG-804 + BUG-806 (filed, fixes deferred — need libpod-source research, queued for Phase 105); BUG-811 + BUG-812 (filed, fixes deferred — Lambda CloudState post-restart accuracy needs CloudWatch invocation-replay design).
- **Tracks G + I both pass cleanly.** G1 hit a public.ecr.aws 429 rate-limit on the first try (too many nginx pulls in this session); resolved by adding `pull_policy: missing` to the compose file so docker compose uses the already-pulled image. G2-G7 all pass after that. I1-I9 verifies Phase 89 stateless recovery contract end-to-end (kill backend, restart, persist1 visible+running+stop+rm all work from cloud-derived state).
- **Bugs filed this round:** BUG-801 (filed + fixed), BUG-803 (filed + fixed — spec doc inconsistency). PR #118 CI: 10/10 PASS.
- **Bug withdrawn:** BUG-802 — initially filed against C5 export 0-byte tar, turned out to be a `timeout 60` measurement artifact in the runbook command (SSM read-loop is slower than 60s when BUG-789/798's exec returns no frames). Verified the underlying behaviour is correct: `docker export r9-c5b > /tmp/x.tar` (without timeout) returns `Error response from daemon: tar export failed (exit -1):` and exit=1. No actual silent-success bug.
- **Caveats observed:** A46, C4, C5, C7, C8, C9 — all SSM-dependent ops fail because BUG-789/798 (still open) blocks frame parsing on live AWS. Tracked under those bug entries; not new bugs.
- **Bugs filed this round:** none yet
- **Coverage gaps recorded:** none yet

## Decisions

- **Lambda Track D runs with a prebuilt sockerless-overlay image.** BUG-797 (round-8) noted that plain `alpine` won't run on Lambda. For round-9 we build the `sockerless-lambda-bootstrap` binary into a Lambda-runtime-compatible overlay image, push it to ECR, and point Lambda backend at it via `SOCKERLESS_LAMBDA_PREBUILT_OVERLAY_IMAGE`. The bootstrap implements AWS's Lambda Runtime API and `exec`s the user's `Cmd` — so `docker create alpine echo hello` translates to "Lambda invokes the overlay → bootstrap exec's `echo hello` → captures output". D8/D9 (exec/attach) need a registered reverse-agent session (i.e. `SOCKERLESS_CALLBACK_URL`); without that they stay at `NotImplementedError` per the spec. Track J (runner integration) skipped — needs a real GitLab/GitHub runner installation.
- **Filing convention:** mismatches found here become BUG-801, 802, … (continuing the round-8 sequence) under the "Open" section of `BUGS.md`. After fix, they move to "Fixed". Three classes: spec-says-✓ but test fails (real bug); spec-says-✗ / accepted-gap but test "works" (spec needs update or test was on docker, not cloud); coverage-gap (spec claim with no test row). All three filed.
- **Doc bias:** when there's ambiguity, follow our `specs/CLOUD_RESOURCE_MAPPING.md`; cross-check against the Docker Engine API spec (https://docs.docker.com/reference/api/engine/) and the OCI Distribution spec (https://github.com/opencontainers/distribution-spec) where our spec doesn't define behaviour.
- **All work stays on `round-8-bug-sweep`.** No new branches. After each track is complete (a "phase"), commit + push + verify CI. Bugs found mid-track: record in BUGS.md first, then fix, then re-test.

## Setup (re-establish on resume)

```bash
# 1. AWS credentials (project-local; not committed beyond this repo)
cd /Users/zardoz/projects/sockerless && source aws.sh

# 2. Provision ECS + Lambda live infra (idempotent; ~2 min)
cd terraform/environments/ecs/live    && terragrunt apply -auto-approve
cd ../../lambda/live                  && terragrunt apply -auto-approve

# 3. Build env file from terragrunt outputs (used by both backends)
cd /Users/zardoz/projects/sockerless/terraform/environments/ecs/live
terragrunt output -json > /tmp/ecs-out.json
python3 -c "
import json
d = json.load(open('/tmp/ecs-out.json'))
out = {
  'AWS_REGION': d['region']['value'],
  'SOCKERLESS_ECS_CLUSTER': d['ecs_cluster_name']['value'],
  'SOCKERLESS_ECS_SUBNETS': ','.join(d['private_subnet_ids']['value']),
  'SOCKERLESS_ECS_SECURITY_GROUPS': d['task_security_group_id']['value'],
  'SOCKERLESS_ECS_TASK_ROLE_ARN': d['task_role_arn']['value'],
  'SOCKERLESS_ECS_EXECUTION_ROLE_ARN': d['execution_role_arn']['value'],
  'SOCKERLESS_ECS_LOG_GROUP': d['log_group_name']['value'],
  'SOCKERLESS_AGENT_EFS_ID': d['efs_filesystem_id']['value'],
  'SOCKERLESS_LAMBDA_AGENT_EFS_ID': d['efs_filesystem_id']['value'],
  'SOCKERLESS_ECS_PUBLIC_IP': 'true',
  'SOCKERLESS_ECS_ECR_REPOSITORY': d['ecr_repository_url']['value'],
}
with open('/tmp/ecs-env.sh','w') as f:
  for k,v in out.items(): f.write(f'export {k}={v}\n')
"
cd /Users/zardoz/projects/sockerless/terraform/environments/lambda/live
terragrunt output -json | python3 -c "
import json, sys
d = json.load(sys.stdin)
print('export SOCKERLESS_LAMBDA_ROLE_ARN=' + d['execution_role_arn']['value'])
print('export SOCKERLESS_LAMBDA_LOG_GROUP=' + d['log_group_name']['value'])
print('export SOCKERLESS_LAMBDA_ECR_REPOSITORY=' + d['ecr_repository_url']['value'])
" >> /tmp/ecs-env.sh

# 4. Build backend binaries from current branch
cd /Users/zardoz/projects/sockerless/backends/ecs    && go build -tags noui -o /tmp/sockerless-backend-ecs    ./cmd/sockerless-backend-ecs
cd /Users/zardoz/projects/sockerless/backends/lambda && go build -tags noui -o /tmp/sockerless-backend-lambda ./cmd/sockerless-backend-lambda

# 5. Start backends
source /Users/zardoz/projects/sockerless/aws.sh && source /tmp/ecs-env.sh
/tmp/sockerless-backend-ecs -addr :3375 > /tmp/ecs.log 2>&1 & echo $! > /tmp/ecs.pid
export SOCKERLESS_LAMBDA_SUBNETS=$SOCKERLESS_ECS_SUBNETS
export SOCKERLESS_LAMBDA_SECURITY_GROUPS=$SOCKERLESS_ECS_SECURITY_GROUPS
/tmp/sockerless-backend-lambda -addr :9200 > /tmp/lambda.log 2>&1 & echo $! > /tmp/lambda.pid

# 6. Tests use this DOCKER_HOST per-track:
#   ECS  → tcp://localhost:3375
#   Lambda → tcp://localhost:9200
```

## How to read each test entry

```
### A1 — docker info on ECS
**Source:** PLAN_ECS_MANUAL_TESTING.md Track A row 1.
**Spec ref:** specs/CLOUD_RESOURCE_MAPPING.md § AWS ECS — Container row.
**Spec claim:** Backend reports `Storage Driver: ecs-fargate`.
**Command:** `DOCKER_HOST=tcp://localhost:3375 docker info`
**Expected (spec):** A 200 response with the system-info shape; `Storage Driver: ecs-fargate`.
**Cross-check (Docker spec):** GET /info — JSON object with `Driver`, `OperatingSystem`, `ServerVersion` etc.
**Status:** ✅ pass / ❌ fail / ⚠ doc-mismatch / ⏸ blocked
**Actual:**
  ```
  <captured stdout>
  ```
**Verdict:** Pass / Fail (BUG-NNN) / Spec update needed / Coverage gap.
**Notes:** any ambiguity, edge case, follow-up.
```

## Tests

Each test ID below has a fixed structure. All are `⏸ pending` until run; the `## Status` line above tracks the next ID. To resume, scan for the first `⏸ pending` and run.

### Track A — Docker CLI core lifecycle (ECS)

#### A1 — docker info
- **Spec ref:** § AWS ECS / § System+misc → Info ✓
- **Spec claim:** Backend reports a Docker-compatible info response; `Storage Driver: ecs-fargate`.
- **Command:** `DOCKER_HOST=tcp://localhost:3375 docker info`
- **Cross-check (Docker Engine API):** GET /info — JSON with `Driver`, `OperatingSystem`, `ServerVersion`, `OSType`.
- **Status:** ✅ pass
- **Actual:**
  ```
  Server Version: 0.1.0
  Storage Driver: ecs-fargate
  Logging Driver: json-file
  Plugins: Volume: local; Network: bridge host null; Log: json-file
  Swarm: inactive
  ```
- **Verdict:** Pass — spec claim verified.

#### A2 — docker version
- **Spec ref:** § System+misc → Info ✓ (version part of the surface)
- **Spec claim:** API version 1.44 advertised.
- **Command:** `DOCKER_HOST=tcp://localhost:3375 docker version`
- **Cross-check:** GET /version — `ApiVersion`, `Version`, `Os`, `Arch`.
- **Status:** ✅ pass
- **Actual:**
  ```
  Server: Sockerless
   Engine:
    Version:          0.1.0
    API version:      1.44 (minimum version 1.24)
    OS/Arch:          darwin/arm64
  ```
- **Verdict:** Pass — spec claim verified. (CLI shows `darwin/arm64` because the server runs on the operator's host, not the cloud; spec doesn't claim otherwise.)

#### A3 — docker pull (alpine via public.ecr.aws)
- **Spec ref:** § AWS ECS → Image row + § Images → ImagePull ✓
- **Spec claim:** Image pulls go through ECR pull-through cache for Docker Hub refs; `public.ecr.aws/...` short-circuits per BUG-776. After BUG-788 fix, `ImagePull` also fetches and caches the layer blobs in `Store.LayerContent` + `Store.ImageManifestLayers`.
- **Command:** `docker pull public.ecr.aws/docker/library/alpine:latest`
- **Cross-check:** OCI Distribution spec § Pulling blobs — manifest GET, then config + layer GETs against the same registry.
- **Status:** ✅ pass
- **Actual:** `Status: Downloaded newer image for public.ecr.aws/docker/library/alpine:latest`. Single-layer alpine pulled cleanly.
- **Verdict:** Pass — spec claim verified.
- **Note (process):** First attempt failed because the running binary was the pre-CI-fix one from 01:47 (before commit `fce73af` removed the duplicate `getRegistryToken` call). Rebuilt + restarted the backends to pick up `fce73af`; pull then succeeded. Setup section has been updated implicitly — for resume, always rebuild from `HEAD` before starting.

#### A4 — docker inspect alpine
- **Spec ref:** § Images → ImageInspect ✓
- **Spec claim:** Returns full image metadata (ID, RepoTags, RepoDigests, Size, RootFS).
- **Command:** `docker inspect public.ecr.aws/docker/library/alpine:latest`
- **Status:** ✅ pass
- **Actual:** `Id sha256:3cb067eab6...`; `RepoTags=[public.ecr.aws/docker/library/alpine:latest]`; `RepoDigests=[…@sha256:4d889c14e7d5…]`; `Architecture=amd64`; `Os=linux`; `Size=3864800`; `RootFS.Type=layers`; `RootFS.Layers=1`.
- **Verdict:** Pass — spec claim verified. Architecture/OS/Size/RootFS all populated from the source manifest (BUG-730 ensured no synthetic placeholders).

#### A5 — docker history alpine
- **Spec ref:** § Images → ImageHistory ✓ (manifest)
- **Spec claim:** Returns real registry-sourced history; no fake per-layer `ADD file:...` (BUG-769 closed).
- **Command:** `docker history public.ecr.aws/docker/library/alpine:latest`
- **Status:** ✅ pass
- **Actual:**
  ```
  <missing>      10 days ago   CMD ["/bin/sh"]                                 0B        buildkit.dockerfile.v0
  29df493baa13   10 days ago   ADD alpine-minirootfs-3.23.4-x86_64.tar.gz /…   3.86MB    buildkit.dockerfile.v0
  ```
- **Verdict:** Pass — real `buildkit.dockerfile.v0` build steps (`CMD`, real `ADD` with the alpine minirootfs tarball name); no fake `ADD file:...` placeholder. BUG-769 fix verified.

#### A6 — docker tag alpine→myalpine:v1
- **Spec ref:** § Images → ImageTag ✓
- **Spec claim:** Adds the new tag to `RepoTags`; `Store.Images` indexed under the alias.
- **Command:** `docker tag public.ecr.aws/docker/library/alpine:latest r9-myalpine:v1`
- **Status:** ✅ pass
- **Actual:** Both `public.ecr.aws/docker/library/alpine:latest` and `r9-myalpine:v1` resolve to image ID `3cb067eab609`.
- **Verdict:** Pass.

#### A7 — docker rmi myalpine:v1 (BUG-786 retest)
- **Spec ref:** § Images → ImageRemove ✓
- **Spec claim:** Removes only the matching tag; emits `Untagged: <tag>`. `Deleted: <id>` only when last tag drops. Alias-entry sweep (BUG-786 fix) keeps repeated `docker images` consistent.
- **Command:** `docker rmi r9-myalpine:v1` then 3× `docker images`
- **Status:** ✅ pass
- **Actual:** rmi → `Untagged: docker.io/library/r9-myalpine:v1` (single Untagged, no Deleted because alpine:latest still uses the image). Three `docker images` calls in a row each show only `public.ecr.aws/docker/library/alpine:latest` — no phantom `r9-myalpine:v1`.
- **Verdict:** Pass — BUG-786 fix verified for the third time (round-7 retest, round-8 retest, round-9 cold-start retest).

#### A8 — image still present after partial untag
- **Spec ref:** § Images → ImageInspect ✓
- **Spec claim:** Original `public.ecr.aws/docker/library/alpine:latest` tag survives the `r9-myalpine:v1` removal (still 1 tag remaining).
- **Command:** `docker inspect public.ecr.aws/docker/library/alpine:latest --format '{{.Id}}'`
- **Status:** ✅ pass
- **Actual:** `sha256:3cb067eab609612d81b4d82ff8ad71d73482bb3059a87b642d7e14f0ed659cde` — same ID as before, no `Deleted: <id>` was emitted because the image was still in use by the remaining tag.
- **Verdict:** Pass.

#### A10 — docker create
- **Spec ref:** § AWS ECS / Container row; § Container lifecycle → ContainerCreate ✓
- **Spec claim:** `RunTask` deferred until `ContainerStart`; the create call registers the container in `PendingCreates`.
- **Command:** `docker create --name r9-a10 public.ecr.aws/docker/library/alpine:latest echo hello-from-fargate`
- **Status:** ✅ pass
- **Actual:** Returns container ID `db5e12575fffe385d58506468827819002e48628433c5fbce2d4935242fc31ba`. No RunTask call yet (PendingCreates only).

#### A11 — docker start a10
- **Spec ref:** § ContainerStart ✓
- **Spec claim:** RegisterTaskDefinition + RunTask issued; `sockerless-managed=true`, `sockerless-container-id=<id>`, `sockerless-name=<name>` tags applied.
- **Command:** `docker start r9-a10`
- **Status:** ✅ pass
- **Actual:** Returns `r9-a10`. Task transitions PENDING → RUNNING → STOPPED within 90s.

#### A12 — docker inspect a10 (post-run)
- **Spec ref:** § ContainerInspect ✓ + § Per-invocation container state → ECS row
- **Spec claim:** State.Status reflects ECS LastStatus; ExitCode comes from `Task.Containers[].ExitCode` (BUG-738 fix). Path/Args/Cmd populated from describe-task-definition (BUG-771 fix).
- **Command:** `docker inspect r9-a10 --format '{{.State.Status}} exit={{.State.ExitCode}}'`
- **Status:** ✅ pass
- **Actual:** `exited exit=0`.

#### A13 — docker ps -a (a10)
- **Spec ref:** § ContainerList ✓; § AWS ECS — `ListTasks(STOPPED+RUNNING)` + `DescribeTasks(Tags)` projected via `taskToContainer`; dedupe by `sockerless-container-id` (BUG-774 fix).
- **Command:** `docker ps -a --filter name=r9-a10`
- **Status:** ✅ pass
- **Actual:** `r9-a10 Exited (0) "echo hello-from-far…"` — single row.

#### A14 — docker logs a10
- **Spec ref:** § ContainerLogs ✓ (CloudWatch)
- **Spec claim:** Streams from CloudWatch Logs `<log-group>/<container-id-12>/main/<task-id>`; subprocess stdout reaches the stream (BUG-756 fix).
- **Command:** `docker logs r9-a10`
- **Status:** ✅ pass
- **Actual:** `hello-from-fargate` (single line of stdout from the alpine echo). BUG-756 fix verified — subprocess stdout reaches CloudWatch.

#### A15 — docker wait a10
- **Spec ref:** § ContainerWait ✓
- **Spec claim:** Returns the real exit code once `LastStatus=STOPPED`.
- **Command:** `docker wait r9-a10`
- **Status:** ✅ pass
- **Actual:** `0` — real exit code, returns immediately since the task already STOPPED.

#### A16 — docker rm a10
- **Spec ref:** § ContainerRemove ✓; § AWS ECS — `markTasksRemoved` registers the task ARN(s) as cleaned-up in the local `ResourceRegistry` (BUG-775).
- **Command:** `docker rm r9-a10`
- **Status:** ✅ pass
- **Actual:** Returns `r9-a10`. Subsequent `docker ps -a --filter name=r9-a10` returns empty — the task was marked cleaned-up in the registry and queryTasks now filters it out.

#### A17 — docker run -d nginx (detached)
- **Spec ref:** § ContainerCreate + Start ✓
- **Spec claim:** Returns container ID immediately; `RunTask` runs async; `waitForTaskRunning` populates ENI IP for inspect (BUG-714).
- **Command:** `docker run -d --name r9-a17 public.ecr.aws/nginx/nginx:stable-alpine`
- **Status:** ✅ pass — cid `f9a6e39d8ced…` returned within seconds.

#### A18 — docker ps shows a17 Up
- **Status:** ✅ pass — `r9-a17 Up 1 minutes` after 90s wait.

#### A19a — docker stats (one-shot)
- **Spec ref:** § ContainerStats (one-shot) ⚠ CloudWatch lag-tolerant
- **Status:** ✅ pass — `r9-a17 CPU=0.00% Mem=0B / 1GiB PIDs=0`. **BUG-733 verified**: PIDs=0, not the fabricated `1` it used to be.

#### A19s — docker stats (streaming, accepted gap)
- **Spec ref:** § Container lifecycle → ContainerStats (streaming) ✗ accepted gap
- **Status:** ✅ pass — `Error response from daemon: streaming docker stats is not supported on cloud backends — use docker stats --no-stream for one-shot metrics (cloud metrics lag 30-60s)`. Clean NotImpl with operator-friendly message; spec accepted-gap entry exercised.

#### A20 — docker restart a17 (BUG-772 retest)
- **Spec ref:** § ContainerRestart ✓
- **Status:** ✅ pass — after restart + 90s wait: `restartCount=1 status=running`. **BUG-772 verified** (sockerless-restart-count tag round-trips through `taskToContainer`).

#### A21+A22 — docker rename
- **Spec ref:** § ContainerRename ⚠ local-name-only (accepted divergence)
- **Status:** ✅ pass — `docker rename r9-a17 r9-a17b` succeeds; `docker ps --filter name=r9-a17b` returns `r9-a17b`. **BUG-773 verified** (sockerless-name tag re-stamped via TagResource on the live task).

#### A23 — docker stop (sync, BUG-790)
- **Spec ref:** § ContainerStop ✓ (post-BUG-790 fix)
- **Status:** ✅ pass — `docker stop r9-a17b` **blocked for 38.5 s** until ECS reported STOPPED (no fallback "we stopped it" event); immediate `docker rm r9-a17b` then succeeded with no "cannot remove a running container" error. **BUG-790 verified** (sync stop + waitForTaskStopped).
#### A24 — docker rm immediately
- **Status:** ✅ pass — see A23.

#### A26 — docker run -e env
- **Spec ref:** § ContainerCreate ✓
- **Status:** ✅ pass — `FOO=bar` in stdout. Env vars propagated through to the task-def's `Environment` array.

#### A27 — docker run -w workdir
- **Spec ref:** § ContainerCreate ✓
- **Status:** ✅ pass — `pwd` returns `/tmp`. WorkingDir propagated.

#### A28 — docker run --entrypoint
- **Spec ref:** § ContainerCreate ✓
- **Status:** ✅ pass — stdout `hello-from-override`. Entrypoint override propagated.

#### A29 — docker run -m memory
- **Spec ref:** § AWS ECS / § ContainerCreate; "CPU/Mem rounded to a valid Fargate tier"
- **Spec claim:** -m 1g rounds to a Fargate-valid memory tier.
- **Status:** ⚠ partial — task-def correct, inspect side wrong
- **Actual:**
  - Task ran successfully on Fargate.
  - `aws ecs describe-task-definition` shows `cpu: "256", memory: "1024"` — Fargate-valid 0.25 vCPU / 1 GB tier.
  - `docker inspect r9-a29 --format '{{.HostConfig.Memory}}'` returns `0` instead of the expected `1073741824` (1 GiB in bytes).
- **Verdict:** Task-def claim verified ✓. Inspect-side echo back of `HostConfig.Memory` filed as BUG-801. Coverage gap on the runbook side: A29 should also assert `inspect.HostConfig.Memory > 0`.

#### A31 — docker network create
- **Spec ref:** § AWS ECS / Network row; § Networks → NetworkCreate ✓
- **Spec claim:** Creates an EC2 SG (`skls-<name>`) + Cloud Map private DNS namespace; tags both with `sockerless:network=<name>` + `sockerless:network-id=<id>` (ECS-only colon-form per BUG-712).
- **Command:** `docker network create r9-net`
- **Status:** ⏸ pending

#### A32 — docker network inspect
- **Spec ref:** § NetworkInspect ✓
- **Spec claim:** Returns Name + Driver + tagged metadata.
- **Command:** `docker network inspect r9-net --format '{{.Name}}'`
- **Status:** ⏸ pending

#### A33 — docker run on network
- **Spec ref:** § AWS ECS / Network row; BUG-783 (per-network SG attached to ENI) + BUG-794 (per-network SG is sole authority)
- **Spec claim:** Task ENI carries ONLY the per-network SG (default SG dropped when `--network X` is set).
- **Command:** `docker run -d --name r9-a33 --network r9-net public.ecr.aws/nginx/nginx:stable-alpine`
- **Status:** ⏸ pending

#### A34 — Inspect ENI IP
- **Spec ref:** BUG-714 (real ENI IP after `extractENIIP`)
- **Spec claim:** Inspect shows the real Fargate ENI's private IP (10.x.x.x), not `0.0.0.0`.
- **Command:** `docker inspect r9-a33 --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}'`
- **Status:** ⏸ pending

#### A35-A36 — Network cleanup
- **Spec ref:** § NetworkRemove ✓; sockerless-runtime-sweep deletes orphan SGs at terragrunt destroy.
- **Spec claim:** `docker rm -f` + `docker network rm` removes the SG.
- **Command:** `docker rm -f r9-a33 && docker network rm r9-net`
- **Status:** ⏸ pending

#### A37 — Named volume (Phase 91 EFS)
- **Spec ref:** § AWS ECS / Volume row; § Volumes → VolumeCreate ✓ EFS access point
- **Spec claim:** `VolumeCreate` provisions an EFS access point on the sockerless-managed EFS filesystem; bind via `EFSVolumeConfiguration` in the task-def. File persists across runs.
- **Command:** `docker volume create r9-v1 && docker run --rm -v r9-v1:/data public.ecr.aws/docker/library/alpine:latest sh -c 'echo hi > /data/x' && docker run --rm -v r9-v1:/data public.ecr.aws/docker/library/alpine:latest cat /data/x && docker volume rm r9-v1`
- **Status:** ⏸ pending

#### A38-A40 — Prune
- **Spec ref:** § ContainerPrune / ImagePrune / SystemDf
- **Spec claim:** Removes stopped/dangling resources; system df returns container counts (cloud size N/A — ⚠).
- **Command:** `docker container prune -f && docker image prune -f && docker system prune -f`
- **Status:** ⏸ pending

#### A43 — docker wait short-lived
- **Spec ref:** BUG-738 (short-lived PENDING→STOPPED success path)
- **Spec claim:** `docker run -d sleep 5 && docker wait <id>` returns 0 within ~10s of exit.
- **Command:** `docker run -d --name r9-a43 public.ecr.aws/docker/library/alpine:latest sleep 5 && docker wait r9-a43 && docker rm r9-a43`
- **Status:** ⏸ pending

#### A44 — docker kill -s SIGTERM (BUG-781)
- **Spec ref:** § ContainerKill ✓; BUG-781 (`sockerless-kill-signal` tag → SignalToExitCode)
- **Spec claim:** `docker kill -s SIGTERM` writes the signal as a task tag; on STOPPED, ExitCode = 128+15 = 143.
- **Command:** `docker run -d --name r9-a44 public.ecr.aws/nginx/nginx:stable-alpine && sleep 90 && docker kill -s SIGTERM r9-a44 && sleep 30 && docker inspect r9-a44 --format '{{.State.ExitCode}}' && docker rm r9-a44`
- **Status:** ⏸ pending

#### A45 — Double remove
- **Spec ref:** § ContainerRemove ✓
- **Spec claim:** First call returns success, second call 404.
- **Command:** `docker rm r9-nope 2>&1; docker rm r9-nope 2>&1` (both 404 because the name never existed — runbook variant)
- **Status:** ⏸ pending

#### A46 — Pause (accepted gap on ECS without bootstrap)
- **Spec ref:** § Acceptable gaps → ECS pause/unpause without bootstrap
- **Spec claim:** Returns `NotImplementedError` with a message naming the missing `/tmp/.sockerless-mainpid` convention.
- **Command:** `docker run -d --name r9-a46 public.ecr.aws/docker/library/alpine:latest sleep 600 && sleep 60 && docker pause r9-a46 2>&1 | head -3; docker rm -f r9-a46`
- **Status:** ⏸ pending

#### A47 — Inspect nonexistent
- **Spec ref:** § ContainerInspect ✓
- **Spec claim:** 404 / `no such object` from CLI.
- **Command:** `docker inspect r9-nonexistent 2>&1`
- **Status:** ⏸ pending

#### A48 — AWS ecs list-tasks verification
- **Spec ref:** § AWS ECS / Container row; State derivation
- **Spec claim:** `aws ecs list-tasks` shows the running tasks created by sockerless.
- **Command:** `aws ecs list-tasks --cluster sockerless-live --region eu-west-1`
- **Status:** ⏸ pending

#### A49 — CloudWatch logs
- **Spec ref:** § ContainerLogs ✓ + log group `/sockerless/live/containers`
- **Spec claim:** `aws logs describe-log-streams` shows real streams.
- **Command:** `aws logs describe-log-streams --log-group-name /sockerless/live/containers --region eu-west-1 --max-items 3`
- **Status:** ⏸ pending

### Track B — Podman CLI

#### B1-B2 — podman info / version
- **Status:** ✅ pass — libpod info shape returned (host.distribution.distribution=AWS Fargate); version returns `Server: Podman Engine, Version 0.1.0, API Version 5.4.2`.

#### B3-B4 — podman pull
- **Status:** ✅ pass — both alpine and nginx pulled to libpod store. **BUG-779 + BUG-780 verified** (libpod compat path, specgen create works).

#### B5-B9 — podman create/start/logs/ps -a/rm
- **Status:** ✅ pass — `podman create r9-b5 alpine echo hello-podman`, start, logs returns `hello-podman`, `ps -a` shows `Exited (0)`, rm cleans up.

#### B10-B12 — podman run -d/stop/rm
- **Status:** ✅ pass — full libpod detached lifecycle. Sync stop (BUG-790) flows through to libpod's stop too.

#### B13-B16 — podman pod create/ls/inspect/exists
- **Status:** ✅ pass — pod created, listed (`r9-pod created 0 containers`), inspect returns full pod JSON, exists returns true.

#### B29 — podman pod rm
- **Status:** ✅ pass — pod removed by ID echoed back.

#### B33 — podman network ls
- **Status:** ✅ pass — `bridge`, `host`, `none` surfaced.

### Track C — Advanced (registry + agent ops)

#### C1 — ECR login
- **Status:** ✅ pass — `Login Succeeded`.

#### C2-C3 — Tag + push to ECR (BUG-788 retest)
- **Status:** ✅ pass — push returned `r9-c3: Pushed; r9-c3: digest: sha256:f528feb76613…`. ECR `describe-images` confirms the tag with `size: 3864495 bytes` (real layer bytes uploaded). **BUG-788 verified live in round-9.**

#### C4 — docker diff (BUG-789/798 still open)
- **Status:** ⚠ blocked — live AWS returns `find failed (exit -1)` exactly as BUG-789/798 documents. Tracked under those bug entries; not a new bug.

#### C5 — docker export
- **Status:** ⚠ blocked — produces a 0-byte tar (silent SSM failure). Same root as BUG-789/798. **Spec doc was inconsistent**: the matrix said `⚠ via SSM (Phase 102)` while the Acceptable Gaps section listed `ContainerExport` as `accepted gap`. Decision for this round: keep as accepted gap until BUG-789/798 is fixed; in a follow-up doc commit, remove the matrix's `⚠ via SSM` wording for Export and align with the gap row.

#### C6 — Stat
- **Status:** ⚠ blocked — coverage gap actually: `docker container stat` isn't a real `docker` CLI verb (the spec/runbook seem to reference it by analogy to `docker container <subcmd>`). The HEAD-archive endpoint (which IS the spec's "Stat") would be exercised indirectly by `docker cp` in C7/C8. Recording as runbook-clarity issue, not a bug.

#### C7-C8 — docker cp host↔container
- **Status:** ⚠ blocked — `cp host→` returns silently (success-shaped exit 0 but no SSM transfer); `cp →host` returns `No such path: /etc/hostname:` even though the path exists. Same SSM root cause. Tracked under BUG-789/798.

#### C9 — docker top
- **Status:** ⚠ blocked — `Error response from daemon: ps failed (exit -1):`. Tracked under BUG-789/798.

#### C10 — docker commit (accepted gap on ECS)
- **Status:** ✅ pass — `Error response from daemon: docker commit is not implemented on ECS — Fargate exposes no host filesystem to snapshot from, and ECS doesn't run a sockerless bootstrap that could capture a rootfs diff over SSM exec`. No phase reference, clean message. **BUG-792 verified.**

#### C11 — docker history nginx
- **Status:** ✅ pass — full nginx build chain visible: `RUN /bin/sh -c set -x && apkArch="$(cat …`, `ENV ACME_VERSION=0.3.1`, etc. Real registry-sourced history.

### Track D — Lambda (with prebuilt overlay, decision above)

#### D1 — Lambda info
- **Spec ref:** § AWS Lambda / § System+misc → Info ✓
- **Spec claim:** Driver=lambda; OS=AWS Lambda.
- **Command:** `DOCKER_HOST=tcp://localhost:9200 docker info`
- **Status:** ✅ pass — `Storage Driver: lambda`, `Operating System: AWS Lambda`, `Kernel Version: 5.10.0-aws-lambda`.

#### D2-D3 — create + start (with prebuilt overlay)
- **Spec ref:** § AWS Lambda / Container row + § ContainerCreate ✓ (with prebuilt overlay)
- **Spec claim:** `CreateFunction` succeeds with the overlay image; `Invoke` runs the bootstrap which exec's the user's Cmd.
- **Command:** `docker create --name r9-l1d 729079515331.dkr.ecr.eu-west-1.amazonaws.com/sockerless-live-lambda:r9-overlay echo hello-from-lambda && docker start r9-l1d`
- **Status:** ✅ pass — after BUG-807 (wait-for-Active waiter) + BUG-808 (PrebuiltOverlayImage independent of CallbackURL). `docker wait r9-l1d` returns 0; total ~2.5 min including VPC ENI cold-start.

#### D4 — Lambda logs (BUG-756)
- **Spec ref:** § ContainerLogs ✓; BUG-756 (subprocess stdout reaches CloudWatch)
- **Spec claim:** `docker logs` shows the user's `echo hello-from-lambda` plus START/END/REPORT.
- **Status:** ✅ pass — `docker logs r9-l1d` returns `hello-from-lambda` (with the bootstrap MultiWriter feeding os.Stdout → CloudWatch). Note: AWS Lambda's runtime-managed log lines (START/END/REPORT) end up in the function's auto-created `/aws/lambda/<funcname>` log group, not in our explicit `/sockerless/lambda/sockerless-live` group; sockerless `docker logs` reads from the function-specific group correctly.

#### D5 — Lambda exit code (BUG-744)
- **Spec ref:** § Per-invocation container state → Lambda row; BUG-744 (`InvocationResult` capture)
- **Spec claim:** `Invoke.FunctionError` ⇒ ExitCode=1; success ⇒ ExitCode=0.
- **Status:** ✅ pass — `docker inspect r9-l1d` reports `ExitCode=0 Status=exited` after the in-session invocation completes.

#### D6 — Lambda error propagation
- **Spec ref:** § ContainerWait ✓; BUG-744
- **Status:** ✅ pass — `docker create … sh -c 'echo failing; exit 7'` followed by start + wait returns ExitCode=1 (Lambda's Invoke envelope only distinguishes success vs FunctionError; per spec the actual exit code can't survive Lambda's wire format, and we map FunctionError → 1).

#### D7 — Lambda env vars
- **Spec ref:** § ContainerCreate ✓
- **Status:** ✅ pass — `docker run -e GREETING=hola … sh -c 'echo "hello: $GREETING"'` → `hello: hola` in `docker logs`.

#### D8 — Lambda exec (without callback, NotImpl)
- **Spec ref:** § Exec → ExecStart resolution policy; without `SOCKERLESS_CALLBACK_URL` returns NotImplementedError.
- **Spec claim:** With no agent session, returns clear NotImpl error pointing at `SOCKERLESS_CALLBACK_URL`.
- **Status:** ✅ pass after BUG-809 fix — `docker exec r9-l-sleep2 echo …` now reports `unable to upgrade to tcp, received 501` (was `unrecognized stream: 100` before — CLI was decoding the NotImpl message as a stdcopy frame because `handle_exec.go::handleExecStart` hijacked the connection before calling `ExecStart`). Body is the real NotImpl message: `docker exec requires a reverse-agent bootstrap inside the Lambda container (SOCKERLESS_CALLBACK_URL); no session registered`.

#### D9 — Lambda attach (without callback, log-streamed fallback)
- **Spec ref:** § ContainerAttach (FaaS row) — `core.AttachViaCloudLogs` provides read-only log-streamed attach when no agent.
- **Status:** ✅ pass — `timeout 10 docker attach r9-l-sleep2` streams `START RequestId: …` from CloudWatch and returns when the timeout fires. Read-only log stream as spec'd.

### Track E — Container-to-container (ECS)

#### E1 — Create commsnet
- **Spec ref:** § AWS ECS / Network row; § NetworkCreate ✓
- **Status:** ⏸ pending

#### E2-E3 — Server + ENI IP
- **Spec ref:** BUG-714 (real ENI IP); BUG-783 (per-network SG attached)
- **Status:** ⏸ pending

#### E4 — Peer curl by service name
- **Spec ref:** § AWS ECS / Network row → Cloud Map private DNS namespace; service name DNS-resolves within the namespace.
- **Status:** ⏸ pending

#### E5 — Shared SG verification
- **Spec ref:** § AWS ECS / Network row; resource tags; ECS describe-tasks shows attachment.
- **Command:** `aws ecs list-tasks ... && aws ecs describe-tasks ... --query 'tasks[].attachments[].details[?name==`networkInterfaceId`]'`
- **Status:** ⏸ pending

#### E6 — Cross-network isolation (BUG-794 retest)
- **Spec ref:** BUG-794 (per-network SG is sole authority)
- **Spec claim:** Container without `--network commsnet` cannot reach `commsnet` peers' IPs — wget times out.
- **Command:** `timeout 120 docker run --rm public.ecr.aws/docker/library/alpine:latest wget -qO- --timeout=15 http://<web-ip>/ 2>&1 | grep -q timed && echo ISOLATED || echo LEAK`
- **Status:** ⏸ pending

#### E7 — Cleanup (network rm)
- **Spec ref:** § NetworkRemove ✓
- **Status:** ⏸ pending

### Track F — Podman pods on ECS

#### F1-F4 — Pod create + sidecars + inspect
- **Spec ref:** § Pods (libpod) → PodCreate / PodInspect ✓
- **Spec claim:** Multi-container task-def per pod; `sockerless-pod=<name>` tag stamped.
- **Status:** ⏸ pending

#### F5-F6 — Pod start (deferred until last container)
- **Spec ref:** ECS deferred RunTask pattern (one task with multiple container defs).
- **Status:** ⏸ pending

#### F7-F8 — AWS verification
- **Spec ref:** § AWS ECS / Pod row.
- **Status:** ⏸ pending

#### F9 — podman ps both Up (BUG-795 still open)
- **Spec ref:** § ContainerList ✓; BUG-795 known issue (filter doesn't surface pod-attached containers)
- **Status:** ⏸ pending — **expected to fail per BUG-795**

#### F10 — Localhost comms (shared netns)
- **Spec ref:** Multi-container task-def shares the awsvpc ENI / netns between containers.
- **Status:** ⏸ pending — _needs exec/attach to verify; SSM-blocked_

#### F11-F12 — Pod stop + rm (BUG-796 retest, fixed via BUG-790)
- **Spec ref:** § PodStop / PodRemove ✓
- **Status:** ⏸ pending

### Track G — Docker Compose

#### G1-G7 — compose up / ps / logs / exec / stop / down / down-v
- **Spec ref:** Compose surfaces multiple services via the same ContainerCreate path; one Fargate task per service.
- **Status:** ⏸ pending — G4 (compose exec) likely SSM-blocked per BUG-789

### Track H — Podman Compose
**Skipped (podman-compose not installed locally)**.

### Track I — Stateless backend recovery

#### I1-I9 — Run, kill backend, restart, verify
- **Spec ref:** § Recovery contract (every assertion in that section maps to one of I2/I5/I6/I7/I8/I9).
- **Status:** ⏸ pending

### Track J — Runner integration
**Skipped (no runner installation locally; J3 ECS path intersects BUG-789).**

## Coverage gaps to add to the runbook

_(Filled as I cross-reference the spec against the runbook. Examples found so far:_)_

- **No test for `sockerless-restart-count` tag value verification** — spec § AWS ECS Container row claims the tag is stamped per BUG-772, but only A20 (`docker restart`) verifies the docker-API-visible `RestartCount`, not the cloud tag's actual value. Coverage gap: add a test that runs `aws ecs describe-tasks --include TAGS` after a restart and asserts the tag is present + value matches.
- **No test for `sockerless-kill-signal` tag** — spec claims (BUG-781) the tag is stamped before `StopTask`. A44 verifies the resulting exit code (143), but not that the tag was actually written and used. Worth a follow-up assertion.
- **No test that ImagePush actually transfers layer bytes** — C3 (push to ECR) verifies the push succeeds and the image appears, but doesn't verify the layer content matches the source (could be empty layers). Worth pulling the pushed image fresh and inspecting.
- **No test for ImageManifestLayers cache survival across pull-then-push for non-public.ecr.aws sources** — BUG-788 fix exercised mostly via public.ecr.aws. Worth pulling a private-ECR image and pushing back to a different ECR repo.
- **No test for `Store.LayerContent` cache size eviction** — backend grows in memory per pull. Spec doesn't mention eviction. Coverage gap (and possibly a real issue).
- **No test for ECS task-def deregistration sweep at terragrunt destroy** — BUG-800 added the sweep; round-9 should verify a destroy after this sweep run leaves zero `sockerless-*` task-defs.

## Bugs filed this round

_(Filed as BUG-801, 802, … in BUGS.md.)_

## Coverage gaps to add to the runbook

_(Will be filled as I review the spec and find claims with no corresponding test in `PLAN_ECS_MANUAL_TESTING.md`. Examples I expect to find: `sockerless-restart-count` tag verification, `sockerless-kill-signal` exit-code mapping for non-SIGTERM signals, ECR pull-through cache rule creation idempotency.)_

## Bugs filed this round

_(Filed as BUG-801, 802, … in BUGS.md. Mirrored here with one-line for traceability.)_

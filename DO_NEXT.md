# Do Next

Resume pointer. Updated after every task. Roadmap detail in [PLAN.md](PLAN.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md); bug log in [BUGS.md](BUGS.md); runner wiring in [docs/RUNNERS.md](docs/RUNNERS.md).

## Branch state

- `main` synced with `origin/main` at PR #121 merge.
- `origin-gitlab/main` mirrors `origin/main` (in sync as of 2026-04-27).
- **`phase-110-runner-integration`** — active. PR #122 in flight. Phase 110b architecture work landing on this branch (Phase 110a deferred to a follow-on once 110b proves the architecture).

## ⚠ Active blockers (must clear before resuming cell-2/3/4 work)

1. **BUG-873 — Lambda image-mode requires Docker schema 2 + Runtime API client at ENTRYPOINT.** Cell 2 currently fails at `lambda.CreateFunction` with `image manifest, config or layer media type ... is not supported` (alpine carries `vnd.oci.image.index.v1+json`). Even when that's fixed, alpine's `tail -f /dev/null` ENTRYPOINT never calls `/next` so the function would be unresponsive. Architectural fix routes ALL Lambda CreateFunction through the existing `BuildAndPushOverlayImage` overlay-inject path (extending it to use `awscommon.CodeBuildService` when no local docker daemon is available). Tracked as **Phase 115** in PLAN.md.
2. **BUG-868 — gitlab-runner `start-attach-script` per-command lifecycle vs Fargate non-restartable task.** Each script step does `docker start <helper>` + `docker attach`; Fargate tasks can't restart, so sockerless re-launches a fresh task per /start, but the task entrypoint exits after the first script. Fix: keep the helper task alive with a synthetic entrypoint (`tail -f /dev/null`), route each /start's script through SSM ExecuteCommand. Tracked as **Phase 114** in PLAN.md.

Cell 1 (GH × ECS) is GREEN and stays green — no action needed there. PR #122 CI is GREEN at `88aca1e`. Latest pushes `99c8ca0` + `b3be64f` close BUG-869/870/871/872 and surface BUG-873.

## Operational state — 2026-04-29 ~00:00 UTC

- **AWS creds:** ⚠ expired; was active via `aws.sh` (root `729079515331`). Refresh before resuming.
- **Live AWS infra: UP in eu-west-1.** ECS cluster `sockerless-live` (35 base + 4 runner-extension resources). Lambda live env (8 resources). EFS `fs-069c02e0e8823b64e` with two access points: `runner_workspace=fsap-0f60e569bae585f25`, `runner_externals=fsap-0ff9f9686208c4ed7`.
- **Runner-task ECS task definition:** `sockerless-live-runner:2` registered in eu-west-1. Single-container Fargate (1024 CPU / 2048 MB, X86_64). Image: `729079515331.dkr.ecr.eu-west-1.amazonaws.com/sockerless-live:runner-amd64` (latest digest pushed to ECR; `LABEL com.sockerless.ecs.task-definition-family=sockerless-runner`). EFS volumes mounted at `/home/runner/_work` and `/home/runner/externals`; entrypoint pre-populates externals on first start (tar pipe).
- **Sockerless code changes (BUG-850..853 — all on this branch):**
  1. `Config.SharedVolumes` + bind-mount → EFS translation in `backends/ecs/backend_impl.go`. Sub-path drop. `/var/run/docker.sock` drop.
  2. `accessPointForVolume` short-circuits to `SharedVolume.AccessPointID` when the named volume matches a configured shared volume.
  3. ECS server overrides `s.Drivers.Network` with `SyntheticNetworkDriver` (metadata-only — Fargate has its own netns, Linux netns is the wrong abstraction for cloud).
  4. Sub-tasks include the operator's default SG alongside per-network SG (so EFS mount targets stay reachable).
  5. `cloudExecStart` waits for `ExecuteCommandAgent.LastStatus == RUNNING` before issuing `ExecuteCommand`.
- **PR-#122 commits:** working tree has all the above plus state-doc updates; ready to commit + push.

## Phase 110b — Cell 1 status: ✅ GREEN

**Successful run:** https://github.com/e6qu/sockerless/actions/runs/25052661438 (commit `7362197` pushed 2026-04-28).

All 6 workflow steps passed. `container: alpine:latest` directive flowed through sockerless's bind-mount → EFS translation; sub-task spawned in Fargate with shared EFS access points; `docker exec` succeeded after the new ExecuteCommandAgent-ready wait.

Iteration history (recorded for future debugging):
- 25049909614 — Initialize containers failed: bind mount rejection (BUG-850 not yet shipped).
- 25051339655 — Initialize failed: netns (BUG-851).
- 25051469196 — Initialize failed: EFS mount timeout from sub-task (BUG-852).
- 25051866900 — Initialize ✓; Run echo failed exit 255 (BUG-853).
- 25052043048 — Initialize ✓; exec failed: missing `ecs:ExecuteCommand` IAM perm.
- 25052216785, 25052362819 — same exec-agent-not-ready failure (BUG-853 confirmed, fix not yet shipped).
- 25052661438 — **GREEN** — first run with the BUG-853 wait fix shipped.

## 4-cell verification status (2026-04-29 ~10:08 UTC)

| Cell | Status | Latest evidence URL | Next |
|---|---|---|---|
| 1 GH × ECS | ✅ PASS | https://github.com/e6qu/sockerless/actions/runs/25075259911 | re-run during sweep once 2/3/4 verified |
| 2 GH × Lambda | 🟡 5 walls past, blocked at BUG-873 | https://github.com/e6qu/sockerless/actions/runs/25102901975 (failed at OCI manifest) | implement Phase 115 (always-on overlay-inject via CodeBuild) |
| 3 GL × ECS | 🟡 progresses past cleanup_file_variables | https://gitlab.com/e6qu/sockerless/-/jobs/14137580070 | implement Phase 114 (long-lived helper task + SSM ExecuteCommand) |
| 4 GL × Lambda | ⏸ inherits BUG-868 + BUG-873 | n/a | unblocks once Phases 111 + 112 land |

## CI status

✅ **PR #122 CI fully GREEN** as of commit `88aca1e` (BUG-866 v2): all 10 jobs PASS — lint, test, test (e2e), sim (aws/gcp/azure), ui, build-check, smoke, terraform.

## Bugs shipped this iteration (PR #122)

- BUG-859 (H, ECS attach stdin)
- BUG-860 (H, Lambda attach stdin)
- BUG-861 (H, Lambda externals shared-volume entry — symptom of BUG-862)
- BUG-862 (CRITICAL, runner-Lambda baked wrong backend — codified class-of-bug rule)
- BUG-863 (M, integration / smoke / test arch env var missing)
- BUG-864 (L, terraform-test substring-match false positive)
- BUG-865 (H, image-resolve routes locally-built images through Public Gallery)
- BUG-866 (H, deferred-stdin path entered too eagerly — v1 fall-through, v2 only-when-pipe-loaded)
- BUG-869 (H, CodeBuild buildspec produced OCI manifest; Lambda image-mode rejects)
- BUG-870 (H, EFS access-point ARN lookup filtered by `sockerless-managed` tag — operator-provisioned APs lacked it)
- BUG-871 (H, Lambda single-FSC + `/mnt/...` mount path constraint — collapse + BIND_LINKS bootstrap symlinks + EFS subpath in SharedVolume)
- BUG-872 (H, pull-through cache prefix mismatch with ECS — derive prefix the same way both backends do)

## Up next on this branch — Phase 115 (BUG-873) and Phase 114 (BUG-868)

Phase 115 — Lambda image-mode requires Docker schema 2 manifests AND Runtime API client at the entrypoint. Cell 2's alpine image fails both. Architectural fix: route ALL Lambda CreateFunction calls through `BuildAndPushOverlayImage` overlay-inject, swapping its `os/exec docker build` for `awscommon.CodeBuildService` so it works inside the runner-Lambda. Cache converted images by source-content hash. Implementation steps:

1. Refactor `BuildAndPushOverlayImage` in `backends/lambda/image_inject.go`: accept a `core.CloudBuildService` dependency. When available, build via CodeBuild (already wired via `s.images.BuildService` in `server.go:72-76`); else fall back to local docker.
2. `backend_impl.go` create flow: drop the no-CallbackURL default branch. Always go through overlay-inject.
3. New ECR repo (`sockerless-live-overlay`) for converted images, tag = sha256 of `BaseImageRef + AgentBinaryPath + BootstrapBinaryPath + UserEntrypoint + UserCmd`. Skip rebuild on cache hit.
4. `specs/CLOUD_RESOURCE_MAPPING.md` Lambda mapping row: extend with "Lambda images go through overlay-inject; OCI inputs auto-converted to Docker schema 2 by the overlay build."

Phase 114 — gitlab-runner `start-attach-script` per-command lifecycle. Each script step does `docker start <helper>` + `docker attach`. On Fargate the task entrypoint runs once and exits — gitlab-runner expects the helper to stay running. Fix: keep the task alive with synthetic `tail -f /dev/null`-style entrypoint, route each /start's script through SSM ExecuteCommand. Implementation steps:

1. Add a "long-lived helper" mode to ECS ContainerStart when ECSState.OpenStdin is true and the gitlab-runner -predefined suffix is absent (i.e. user-script container).
2. First /start: run a task whose entrypoint is `sh -c 'while sleep 60; do :; done'`; record the task ARN.
3. Subsequent /start cycles for the same container ID: skip RunTask; use SSM ExecuteCommand to run the buffered stdin bytes as a script in the existing task.
4. /attach reads from SSM session output (already implemented for `docker exec`).
5. Container stop: `ecs.StopTask`.

After Phases 111 + 112 land, all 4 cells should reach GREEN.

## Original cell-2 unblock recipe (now superseded by Phase 115)

Source-side corrections shipped through commit `b3be64f`. Full runner hurdle catalog (15 closed + 8 predicted) in [docs/RUNNERS.md § Runner hurdles](docs/RUNNERS.md) — that's where future-debugging starts.

The operator-driven runtime steps below remain accurate as the live-infra prep needed for any cell-2 verification run after Phase 115 lands:

### Step 1 — Apply terraform (cells 2 + 4 prep)

```bash
cd /Users/zardoz/projects/sockerless/terraform/environments/lambda/live
source aws.sh
terragrunt apply
```

Provisions on `sockerless-live`:
- `sockerless-live-image-builder` CodeBuild project (linux/amd64 standard, privileged docker, inline buildspec)
- `sockerless-live-build-context` S3 bucket (24-hour lifecycle on `build-context/` prefix)
- IAM role `sockerless-live-codebuild-role` (S3 read + ECR push + CloudWatch Logs)
- Updates `sockerless-live-runner` Lambda: ECS dispatch IAM perms → Lambda dispatch perms; env vars all `SOCKERLESS_LAMBDA_*` (workspace + externals SHARED_VOLUMES, plus CODEBUILD_PROJECT + BUILD_BUCKET).

### Step 2 — Rebuild runner-Lambda image (cell 2 unblock)

No local Docker daemon needed:

```bash
cd /Users/zardoz/projects/sockerless/tests/runners/github/dockerfile-lambda
make codebuild-update
```

Pipeline: `make stage` (cross-compile linux/amd64 backend + agent + bootstrap into the build context) → `make upload-context` (tar + S3 upload) → `make codebuild-build` (start CodeBuild + poll every 10 s until SUCCEEDED) → `make update-function` (`aws lambda update-function-code --publish`) → `make wait` (`aws lambda wait function-updated-v2`).

Local-Docker alternative if preferred: `make all`.

### Step 3 — Restart sockerless backends (cells 3 + 4 unblock)

```bash
kill 75092 70870
source /Users/zardoz/projects/sockerless/aws.sh
source /tmp/ecs-env.sh
nohup /tmp/sockerless-backend-ecs    -addr :3375 -log-level debug \
    >>/tmp/sockerless-ecs.log    2>&1 &
source /tmp/lambda-env.sh
nohup /tmp/sockerless-backend-lambda -addr :3376 -log-level debug \
    >>/tmp/sockerless-lambda.log 2>&1 &
curl -s http://localhost:3375/_ping; echo
curl -s http://localhost:3376/_ping; echo
```

The macOS-arm64 binaries at `/tmp/sockerless-backend-{ecs,lambda}` were rebuilt this session and contain BUG-859 / BUG-860 fixes.

### Step 3a — Cell-4 prerequisite: agent + bootstrap on disk

The laptop sockerless-backend-lambda's image-inject path needs the agent + bootstrap binaries available locally. Pick one:

```bash
# Option A: copy the linux/amd64 binaries to /opt/sockerless/
sudo mkdir -p /opt/sockerless && \
  sudo cp /Users/zardoz/projects/sockerless/tests/runners/github/dockerfile-lambda/sockerless-agent /opt/sockerless/ && \
  sudo cp /Users/zardoz/projects/sockerless/tests/runners/github/dockerfile-lambda/sockerless-lambda-bootstrap /opt/sockerless/

# Option B: append env vars to /tmp/lambda-env.sh before re-sourcing it in step 3:
cat >> /tmp/lambda-env.sh <<'EOF'
export SOCKERLESS_AGENT_BINARY=/Users/zardoz/projects/sockerless/tests/runners/github/dockerfile-lambda/sockerless-agent
export SOCKERLESS_LAMBDA_BOOTSTRAP=/Users/zardoz/projects/sockerless/tests/runners/github/dockerfile-lambda/sockerless-lambda-bootstrap
export SOCKERLESS_CODEBUILD_PROJECT=sockerless-live-image-builder
export SOCKERLESS_BUILD_BUCKET=sockerless-live-build-context
EOF
```

### Step 4 — 4-cell verification sweep

Tell me when steps 1-3 are done and I'll fire all four cells:

```bash
go test -v -tags github_runner_live -run TestGitHub_ECS_Hello    -timeout 30m ./tests/runners/github
go test -v -tags github_runner_live -run TestGitHub_Lambda_Hello -timeout 30m ./tests/runners/github
go test -v -tags gitlab_runner_live -run TestGitLab_ECS_Hello    -timeout 30m ./tests/runners/gitlab
go test -v -tags gitlab_runner_live -run TestGitLab_Lambda_Hello -timeout 30m ./tests/runners/gitlab
```

I'll capture all four run / pipeline URLs back into this doc. Phase 110 closes when all four are GREEN with their evidence URLs recorded.

### After all four cells GREEN

5. Update `docs/runner-capability-matrix.md`: TBD → PASS for cells 1-4.
6. Phase 110b dispatcher wiring: ECR push pipeline for the dispatcher's own runner image; end-to-end harness wiring through the dispatcher binary (vs the current per-cell direct dispatch).
7. **Tear down live AWS** at session end (`terragrunt destroy` from both `terraform/environments/{ecs,lambda}/live`).

## Sockerless restart command

```bash
kill 75092 70870
source /Users/zardoz/projects/sockerless/aws.sh
source /tmp/ecs-env.sh
nohup /tmp/sockerless-backend-ecs    -addr :3375 -log-level debug \
    >>/tmp/sockerless-ecs.log    2>&1 &
source /tmp/lambda-env.sh
nohup /tmp/sockerless-backend-lambda -addr :3376 -log-level debug \
    >>/tmp/sockerless-lambda.log 2>&1 &
curl -s http://localhost:3375/_ping; echo
curl -s http://localhost:3376/_ping; echo
```

The macOS-arm64 binaries at `/tmp/sockerless-backend-{ecs,lambda}` were rebuilt this session and contain BUG-859 / BUG-860 fixes.

## Resume notes

- **Live infra is UP** — re-run `terragrunt destroy` when done with the session.
- Sockerless ECS backend running locally on `:3375` (laptop), Lambda on `:3376`. Both in `eu-west-1`.
- Runner image already pushed to ECR; ECS task def revision 2 active.
- `gh auth token` keychain-backed; GitLab PAT in `security` keychain.
- The architecture proven by run 25052661438 generalizes to Lambda once SharedVolumes is mirrored. The user said "no fakes / no fallbacks / no workarounds" — Lambda work should follow the same shape.

## Bug log this session (PR #122)

860 fixed (BUG-845..860). 2 open: BUG-861 (Lambda externals shared-volume entry) + BUG-862 (CRITICAL — backend ↔ host primitive mismatch, runner-Lambda baked the ECS backend). Both ship as part of the same cell-2 fix round (rebuild runner-Lambda image with sockerless-backend-lambda + apply new terraform). Class-of-bug rule documented at top of [BUGS.md](BUGS.md): cross-cloud-primitive baking is a P0.

| # | Sev | Area | One-liner |
|---|-----|------|-----------|
| 845 | M | terraform | Lambda live env was us-east-1 → realigned to eu-west-1 + sockerless-tf-state. |
| 846 | M | image-resolve | Docker Hub PAT path replaced with AWS Public Gallery routing. |
| 847 | L | tests/runners | GH runner asset URL `darwin` → `osx`; bumped 2.319.1 → 2.334.0. |
| 848 | M | ecs/lambda | `docker info` Architecture from required `SOCKERLESS_*_ARCHITECTURE` env vars. |
| 849 | M | tests/runners | Drop broken `--add-host host-gateway`; install docker CLI in runner image. |
| 850 | H | ecs (bind mounts) | `Config.SharedVolumes` + bind-mount → EFS translation; sub-path drop; docker.sock drop. |
| 851 | M | ecs (network) | Override `s.Drivers.Network` with metadata-only synthetic; netns is wrong for Fargate. |
| 852 | M | ecs (network) | Sub-tasks need operator default SG too (EFS mount target allow-list). |
| 853 | H | ecs (exec) | Wait for `ExecuteCommandAgent.LastStatus == RUNNING` before `ExecuteCommand`. |
| 854 | M | ecs (image-resolve) | sha256-only refs no longer misroute through `public.ecr.aws/docker/library/sha256:...`; resolve via local Store or surface clear error. |
| 855 | M | aws-common (volumes) | EFS access-point path overflow on long volume names — fall back to `/sockerless/v/<sha256[:16]>`. |
| 856 | M | terraform / runner-lambda | `SOCKERLESS_ECS_SHARED_VOLUMES` aligned to Lambda's `/tmp/runner-state/...` paths. |
| 857 | M | tests/runners (gitlab) | gitlab-runner-helper image pre-pushed to ECR + Basic-auth-direct routing for ECR-shaped registries. |
| 858 | M | ecs (container lifecycle) | `ContainerStart` falls back to `ResolveContainerAuto` for STOPPED-then-restarted containers; PendingCreates preserved through `waitForTaskRunning`. |
| 859 | H | ecs (attach stdin) | `ecsStdinAttachDriver` captures `docker attach` stdin into a per-cycle `stdinPipe`; `launchAfterStdin` defers RunTask until stdin EOF then bakes the script into the task definition's `Entrypoint=[sh,-c]` + `Cmd=[<script>]`. ECSState gains `OpenStdin` so per-cycle restarts (gitlab-runner reuses container ID across script steps) survive PendingCreates churn. |
| 860 | H | lambda (attach stdin) | Mirror of BUG-859 for Lambda: `lambdaStdinAttachDriver` captures stdin → buffered → `lambda.Invoke` Payload (the bootstrap pipes Payload to user entrypoint as stdin, so `Cmd=[sh]` runs the script). LambdaState gains `OpenStdin`. |
| 861 | H | runner-lambda image + lambda backend | ⚠ open. Cell 2 fail surfaced "host bind mounts not supported on ECS backend" for `/tmp/runner-state/externals:/__e:ro`. Root cause was BUG-862 (wrong backend baked in); fix lands together with BUG-862's terraform `SOCKERLESS_LAMBDA_SHARED_VOLUMES` carrying both workspace + externals paths. |
| 862 | **CRITICAL** | architecture / runner-lambda | ⚠ open. Runner-Lambda image baked `sockerless-backend-ecs` and dispatched `container:` sub-tasks via `ecs.RunTask` to Fargate — backend ↔ host primitive mismatch. Project rule (now top of BUGS.md + MEMORY.md + CLOUD_RESOURCE_MAPPING.md universal rule #9): each backend runs on its own native primitive. Source fixed (Dockerfile, bootstrap, terraform IAM + env vars, agent + bootstrap binaries staged into image); awaits image rebuild + `terragrunt apply`. |

## Cross-links

- Roadmap: [PLAN.md](PLAN.md)
- Phase roll-up: [STATUS.md](STATUS.md)
- Narrative: [WHAT_WE_DID.md](WHAT_WE_DID.md)
- Bug log: [BUGS.md](BUGS.md)
- Runner wiring: [docs/RUNNERS.md](docs/RUNNERS.md)

## Standing rules (carry forward)

- **No fakes, no fallbacks, no workarounds** — every gap is a real bug with a real fix. (User reaffirmed several times this session.)
- **Sim parity per commit** — any new SDK call updates `specs/SIM_PARITY_MATRIX.md` + adds the sim handler.
- **State save after every major piece of work** (PLAN / STATUS / WHAT_WE_DID / DO_NEXT / BUGS) — mandatory at ~80% context.
- **Never merge PRs** — user handles all merges.
- **Branch hygiene** — rebase on `origin/main` before push.
- **`github-runner-dispatcher` is sockerless-agnostic** — pure Docker SDK / CLI client.

# Do Next

Resume pointer. Updated after every task. Roadmap detail in [PLAN.md](PLAN.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md); bug log in [BUGS.md](BUGS.md); runner wiring in [docs/RUNNERS.md](docs/RUNNERS.md).

## Branch state

- `main` synced with `origin/main` at PR #121 merge.
- `origin-gitlab/main` mirrors `origin/main` (in sync as of 2026-04-27).
- **`phase-110-runner-integration`** — active. PR #122 in flight. Phase 110b architecture work landing on this branch (Phase 110a deferred to a follow-on once 110b proves the architecture).

## Operational state — 2026-04-29 ~00:00 UTC

- **AWS creds:** active via `aws.sh` (root `729079515331`).
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

## 4-cell verification status (2026-04-29)

| Cell | Status | Run / Blocker |
|---|---|---|
| 1 GH × ECS | ✅ PASS | https://github.com/e6qu/sockerless/actions/runs/25075259911 (2026-04-28 20:13 UTC) |
| 2 GH × Lambda | ❌ FAIL → architecture corrected | Run 25075247501 surfaced BUG-861 → BUG-862. Source fixed; **needs runner-Lambda image rebuild** (Docker daemon required) + `terragrunt apply` for new IAM/env vars. |
| 3 GL × ECS | blocked | Sockerless ECS PID 75092 still on pre-BUG-859 binary (mmap'd). User must `kill 75092` and relaunch from `/tmp/sockerless-backend-ecs` (now contains the fix). |
| 4 GL × Lambda | blocked | Sockerless Lambda PID 70870 still on pre-BUG-860 binary. User must `kill 70870` and relaunch from `/tmp/sockerless-backend-lambda`. |

## Up next on this branch — paths forward to all-cells-GREEN

Source-side corrections shipped through commit `8c70d1a`. Full runner hurdle catalog (15 closed + 8 predicted) in [docs/RUNNERS.md § Runner hurdles](docs/RUNNERS.md) — that's where future-debugging starts.

Remaining work is operator-driven runtime steps in this exact order:

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

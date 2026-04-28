# Do Next

Resume pointer. Updated after every task. Roadmap detail in [PLAN.md](PLAN.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md); bug log in [BUGS.md](BUGS.md); runner wiring in [docs/RUNNERS.md](docs/RUNNERS.md).

## Branch state

- `main` synced with `origin/main` at PR #121 merge.
- `origin-gitlab/main` mirrors `origin/main` (in sync as of 2026-04-27).
- **`phase-110-runner-integration`** — active. PR #122 in flight. Phase 110b architecture work landing on this branch (Phase 110a deferred to a follow-on once 110b proves the architecture).

## Operational state — 2026-04-28 ~15:30 UTC

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

## Up next on this branch

1. **Debug BUG-858** — gitlab-runner cell 3 fails on the predefined container's `docker exec` with "No such container". Sockerless's `queryTasks` tags + filters look correct; the exec-create handler resolves through `ResolveContainerAuto` which checks PendingCreates → CloudState → Store. PendingCreates is deleted in `ContainerStart` line 351 before `waitForTaskRunning`. Check:
   - Whether the gitlab-runner predefined container's ECS task actually transitions to RUNNING (perhaps the helper-image bind-mount/EFS config is wrong).
   - Whether `queryTasks` is returning STOPPED tasks but `queryTasks` filter is excluding them somehow.
   - Whether the runner's exec ID format differs from sockerless's.
2. **Cell 2 — GitHub × Lambda.** Mirror the ECS architecture for the Lambda backend:
   - `backends/lambda/config.go`: add `SharedVolumes` field + parse + lookup helpers (copy from `backends/ecs/config.go`).
   - `backends/lambda/backend_impl.go` (or wherever ContainerCreate is): same bind-mount → EFS-volume translation as ECS, plus sub-path drop + docker.sock drop. Lambda's volume code maps to `FileSystemConfig` (Lambda's EFS attachment shape) instead of `EFSVolumeConfiguration` (ECS).
   - Cross-compile Lambda backend binary; build a Lambda-runtime container image (different shape from ECS — needs the AWS Lambda Runtime Interface Emulator or the runner runs as the Lambda handler). One option: use the AWS-provided `aws-lambda-runtime-interface-emulator` so the same actions/runner image works as a Lambda function.
   - Push to ECR.
   - Define a Lambda function in Terraform (`terraform/modules/lambda/runner.tf`) with `FileSystemConfig` for the workspace + externals EFS access points. Lambda function role with `lambda:InvokeFunction`, EFS mount perms, ECS RunTask + EC2 perms (so sockerless inside the Lambda can dispatch sub-tasks to Fargate).
   - Harness change: `runLambdaRunnerTask` shells out to `aws lambda invoke` with the per-cell registration token. Caveat: Lambda 15-min hard cap — cell 2 is restricted to short workflows.
2. **Cells 3 + 4 (GitLab)** — gitlab-runner master runs locally on the laptop. No sockerless changes needed. Just an explicit run from `tests/runners/gitlab/`.
3. **`github-runner-dispatcher` top-level Go module** — Phase 110a deliverable. Sockerless-agnostic Docker-API client. Skeleton + smoke test against local Podman.
4. **Tear down live AWS** at session end (`terragrunt destroy` from both `terraform/environments/{ecs,lambda}/live`).

## Resume notes

- **Live infra is UP** — re-run `terragrunt destroy` when done with the session.
- Sockerless ECS backend running locally on `:3375` (laptop), Lambda on `:3376`. Both in `eu-west-1`.
- Runner image already pushed to ECR; ECS task def revision 2 active.
- `gh auth token` keychain-backed; GitLab PAT in `security` keychain.
- The architecture proven by run 25052661438 generalizes to Lambda once SharedVolumes is mirrored. The user said "no fakes / no fallbacks / no workarounds" — Lambda work should follow the same shape.

## Bug log this session (PR #122)

All resolved.

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

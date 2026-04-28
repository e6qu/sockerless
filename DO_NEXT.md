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

## Phase 110b — Cell 1 status (in flight, multiple iterations recorded above)

Past workflow runs against the new architecture (oldest → newest):
- https://github.com/e6qu/sockerless/actions/runs/25049909614 — `Initialize containers` failed: bind mount rejection (BUG-850 not yet implemented).
- https://github.com/e6qu/sockerless/actions/runs/25051339655 — Initialize failed: netns creation (BUG-851 fix not yet shipped to image).
- https://github.com/e6qu/sockerless/actions/runs/25051469196 — Initialize failed: EFS mount timeout from sub-task (BUG-852 fix not yet shipped).
- https://github.com/e6qu/sockerless/actions/runs/25051866900 — Initialize containers ✓; `Run echo` step failed exit 255 (BUG-853 — exec agent not ready yet, fix not yet shipped).
- https://github.com/e6qu/sockerless/actions/runs/25052043048 — Initialize ✓; exec failed: missing `ecs:ExecuteCommand` IAM permission (added to runner-task IAM role).
- https://github.com/e6qu/sockerless/actions/runs/25052216785 — Initialize ✓; exec failed: `InvalidParameterException ... execute command agent isn't running` (BUG-853 confirmed, fix in progress).
- https://github.com/e6qu/sockerless/actions/runs/25052362819 — same as above; agent-ready wait fix not yet shipped.
- **In flight:** https://github.com/e6qu/sockerless/actions/runs/25052661438 — first run with the agent-ready wait fix. Result will appear here once it completes.

## Up next on this branch

1. **Verify cell 1 green** — `https://github.com/e6qu/sockerless/actions/runs/25052661438` should show `success` once the SSM agent comes up and the exec succeeds.
2. **Commit + push** all the sockerless + Terraform + Dockerfile + entrypoint + harness + state-doc changes (the workspace currently has many uncommitted files).
3. **Apply same architecture to Lambda backend** (cell 2 — GitHub × Lambda). Lambda has FileSystemConfigs equivalent to ECS volumes; bind-mount-via-EFS translation should plug in symmetrically. Caveat from PLAN.md: cell 2 is restricted to non-`container:` workflows or short jobs (Lambda 15-min hard cap).
4. **Cells 3 + 4 (GitLab × ECS, Lambda)** — GitLab side has no architectural blockers; runs locally. Just needs an explicit run.
5. **`github-runner-dispatcher` top-level Go module** — production-shape dispatcher. Sockerless-agnostic Docker-API client. Phase 110a deliverable.
6. **Tear down live AWS** at session end via `terragrunt destroy`.

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

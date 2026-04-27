# Do Next

Resume pointer. Updated after every task. Roadmap detail lives in [PLAN.md](PLAN.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md); bug log in [BUGS.md](BUGS.md); runner wiring in [docs/RUNNERS.md](docs/RUNNERS.md).

## Branch state

- `main` synced with `origin/main` at PR #121 merge.
- `origin-gitlab/main` mirrors `origin/main` (in sync as of 2026-04-27 — pre-push hook now mirror-aware via `PRE_COMMIT_REMOTE_NAME`).
- **`phase-110-runner-integration`** — open as the active branch. Phase 110 = real GitHub Actions + GitLab Runner integration against ECS + Lambda backends, plus a live-AWS manual test pass to feed into the harness.

## Phase 110 progress (2026-04-27 session)

✓ State-doc compression (BUGS 3-section restructure, STATUS/WHAT_WE_DID/DO_NEXT compressed).
✓ Phase 110 plan in PLAN.md (4-cell matrix; token strategy; execution sequence).
✓ Phase 111 + Phase 112 planned (workload identity; instance metadata services).
✓ `docs/RUNNERS.md` canonical wiring guide; linked from README.
✓ Pre-push hooks mirror-aware via `PRE_COMMIT_REMOTE_NAME`; `origin-gitlab` mirror in sync.
✓ Harness refactored — `tests/runners/internal/{tokens.sh,tokens.go,runners.go}` + 4 test cells (`TestGitHub_{ECS,Lambda}_Hello`, `TestGitLab_{ECS,Lambda}_Hello`); compiles clean with build tags.
✓ All 3 operator prereqs satisfied: AWS creds active via `aws.sh`; `gh` PAT has `workflow` scope; `gitlab-runner` installed; GitLab PAT in keychain.
✓ Live AWS infra provisioned (eu-west-1): ECS cluster `sockerless-live`, Lambda execution role, EFS, ECR, NAT Gateway, VPC.
✓ ECS sockerless backend daemon running on `tcp://localhost:3375`.
✓ BUG-845 fixed (Lambda live env was us-east-1; realigned to eu-west-1 to share ECS subnets per runbook).
✓ BUG-846 fixed in code (Docker Hub creds prereq documented + terraform passthrough); operator setup still required.

⏳ **Blocker — Docker Hub credentials in Secrets Manager.** First `docker run alpine:latest` against the ECS backend fails with `UnsupportedUpstreamRegistryException` because the ECR pull-through cache requires Docker Hub credentials. Per BUG-708 the backend surfaces this clearly (no fallback). Per BUG-846 (just fixed) the prereq is now documented in [`manual-tests/01-infrastructure.md`](manual-tests/01-infrastructure.md) § "Prerequisite: Docker Hub credentials in Secrets Manager".

   **What unblocks:** operator runs the new prereq section once — mints a Docker Hub PAT (https://hub.docker.com/settings/security, scope: Public Repo Read-only), creates a Secrets Manager secret named `ecr-pullthroughcache/sockerless-dockerhub` (the prefix is required by the AWSServiceRoleForECRPullThroughCache role), exports the ARN as `SOCKERLESS_ECR_DOCKERHUB_CREDENTIAL_ARN`, restarts the ECS backend daemon. Next session picks up from there.

⏸ **Live infra still up** in eu-west-1. NAT Gateway is ~$0.05/hr; tear down via `terragrunt destroy` from `terraform/environments/{ecs,lambda}/live` whenever convenient. Self-sufficient teardown via `null_resource sockerless_runtime_sweep` (BUG-819).

## Up next on this branch (Phase 110 remaining)

1. **Operator: provision Docker Hub creds secret** per the new doc.
2. **Re-run smoke** — `docker run --rm alpine:latest echo hi` against `tcp://localhost:3375`. Expected: alpine task runs to completion in Fargate, exits 0.
3. **Walk the relevant tracks of [`manual-tests/02-aws-runbook.md`](manual-tests/02-aws-runbook.md)** in the time the operator allocates. Fix-as-you-go bugs to BUGS.md.
4. **4-cell runner harness** end-to-end:
   - `go test -tags github_runner_live -run TestGitHub_ECS_Hello -timeout 30m ./tests/runners/github`
   - `go test -tags github_runner_live -run TestGitHub_Lambda_Hello -timeout 30m ./tests/runners/github`
   - `go test -tags gitlab_runner_live -run TestGitLab_ECS_Hello -timeout 30m ./tests/runners/gitlab`
   - `go test -tags gitlab_runner_live -run TestGitLab_Lambda_Hello -timeout 30m ./tests/runners/gitlab`
5. **Tear down live AWS** via `terragrunt destroy`.

## Operational state

- **AWS creds:** active via `. /Users/zardoz/projects/sockerless/aws.sh` (root account `729079515331`).
- **GitHub PAT:** keychain-backed via `gh`; scopes include `workflow`; registration-token mint smoke-tested.
- **GitLab PAT:** keychain entry `sockerless-gl-pat` present; scopes include `api`, `create_runner`, `manage_runner`; `gitlab-runner` v18.11.1 installed.
- **Runner registration tokens:** ephemeral; minted per harness run, deleted on exit. Never on disk.
- **Pre-push hooks** mirror-aware (`PRE_COMMIT_REMOTE_NAME`); pre-commit was reinstalled via `pipx reinstall pre-commit` after `brew install gitlab-runner` broke its python venv symlink.
- **PR #122**: open against `phase-110-runner-integration` (TBD — open after this commit).

## Cross-links

- Roadmap: [PLAN.md](PLAN.md)
- Phase roll-up: [STATUS.md](STATUS.md)
- Narrative: [WHAT_WE_DID.md](WHAT_WE_DID.md)
- Bug log: [BUGS.md](BUGS.md)
- Runner wiring: [docs/RUNNERS.md](docs/RUNNERS.md)
- Architecture: [specs/SOCKERLESS_SPEC.md](specs/SOCKERLESS_SPEC.md), [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md), [specs/BACKEND_STATE.md](specs/BACKEND_STATE.md), [specs/SIM_PARITY_MATRIX.md](specs/SIM_PARITY_MATRIX.md)
- Manual-test runbooks: [manual-tests/](manual-tests/)

## Standing rules (carry forward)

- **No fakes, no fallbacks, no workarounds** — every gap is a real bug, every bug ships a real fix in the same session.
- **Sim parity per commit** — any new SDK call added to a backend must update [specs/SIM_PARITY_MATRIX.md](specs/SIM_PARITY_MATRIX.md) and add the sim handler in the same commit.
- **State save after every task** — PLAN / STATUS / WHAT_WE_DID / DO_NEXT / BUGS / memory.
- **Never merge PRs** — user handles all merges; agent only creates PRs and waits for CI.
- **Branch hygiene** — rebase PR branch on `origin/main` before push; sync local `main` after push.

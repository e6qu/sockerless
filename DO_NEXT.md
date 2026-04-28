# Do Next

Resume pointer. Updated after every task. Roadmap detail lives in [PLAN.md](PLAN.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md); bug log in [BUGS.md](BUGS.md); runner wiring in [docs/RUNNERS.md](docs/RUNNERS.md).

## Branch state

- `main` synced with `origin/main` at PR #121 merge.
- `origin-gitlab/main` mirrors `origin/main` (in sync as of 2026-04-27 — pre-push hook now mirror-aware via `PRE_COMMIT_REMOTE_NAME`).
- **`phase-110-runner-integration`** — open as the active branch. PR #122 in flight. **Phase 110 split into 110a + 110b** (architecture decision recorded 2026-04-28; see [PLAN.md § Phase 110](PLAN.md) and [WHAT_WE_DID.md § Phase 110](WHAT_WE_DID.md) for the rationale).

## Operational state — 2026-04-28 ~12:45 UTC

- **AWS creds:** active via `. /Users/zardoz/projects/sockerless/aws.sh` (root account `729079515331`).
- **Live AWS infra: UP in eu-west-1.** Re-provisioned this session via `terragrunt apply` from both `terraform/environments/{ecs,lambda}/live`. ECS = 35 resources (VPC + NAT + cluster + EFS + ECR + IAM + Cloud Map). Lambda = 8 resources (IAM execution role + ECR repo + log group). NAT Gateway runs ~$0.045/hr — tear down via `terragrunt destroy` when the session ends.
- **Sockerless backends RUNNING.**
  - ECS daemon on `tcp://localhost:3375` (cluster `sockerless-live`).
  - Lambda daemon on `tcp://localhost:3376` (eu-west-1, sharing the ECS VPC subnets per BUG-845).
  - Required env vars: `SOCKERLESS_ECS_CPU_ARCHITECTURE=X86_64` / `SOCKERLESS_LAMBDA_ARCHITECTURE=x86_64` (BUG-848, no defaults).
  - Smoke verified: `DOCKER_HOST=tcp://localhost:3375 docker run --rm alpine:latest echo hi` exits 0 from a Fargate task — confirms BUG-846 (AWS Public Gallery routing) holds after re-provisioning.
- **Podman machine running** (applehv VM). Used for local-Podman dispatcher testing in 110a; not used for cells 3 + 4 (gitlab-runner master is darwin-native binary).

## Phase 110a — pending tasks (close PR #122)

1. **Run cell 3 — GitLab × ECS.** No new code; the harness already runs `gitlab-runner` locally with the docker executor pointed at sockerless on `:3375`. Mints a runner authentication token via `POST /api/v4/user/runners`, registers the runner, commits a per-cell pipeline YAML on a throwaway branch, triggers the pipeline, polls to success, then unregisters + deletes runner + branch.
   ```bash
   go test -v -tags gitlab_runner_live -run TestGitLab_ECS_Hello -timeout 25m ./tests/runners/gitlab/
   ```
2. **Run cell 4 — GitLab × Lambda.** Same harness, points at `:3376`. Lambda's 15-minute hard cap applies per job.
   ```bash
   go test -v -tags gitlab_runner_live -run TestGitLab_Lambda_Hello -timeout 25m ./tests/runners/gitlab/
   ```
3. **Scaffold `github-runner-dispatcher` top-level module.** New top-level Go module at the repo root (own `go.mod`, builds independently). Coupled **only to the public Docker API / CLI** — zero awareness of sockerless. Components:
   - `github-runner-dispatcher/cmd/github-runner-dispatcher/main.go` — entry point with mandatory `--repo` flag (no default).
   - `github-runner-dispatcher/pkg/poller/poller.go` — short-poller (`/repos/{repo}/actions/runs?status=queued` + per-run `/jobs`) every 15 s. Stateless dedup via seen-set with 5-min TTL.
   - `github-runner-dispatcher/pkg/spawner/spawner.go` — Docker SDK client doing `docker run --pull never <runner-image-uri> -e RUNNER_REG_TOKEN=… -e RUNNER_LABELS=…`. `DOCKER_HOST` env dictates target daemon.
   - `github-runner-dispatcher/pkg/config/config.go` — TOML loader at `~/.sockerless/dispatcher/config.toml` mapping label → `{daemon URL, runner image URI}`. CLI flags can override.
   - Explicit scope verification at startup via `gh api /` checking the PAT scopes; fail loud with full instructions on missing scopes.
   - Sockerless-daemon-liveness check: skip the poll cycle if `DOCKER_HOST` is unreachable; log warning, don't crash.
4. **Smoke-test the dispatcher** against local Podman with the existing `sockerless-actions-runner:local` image. No ECR push needed for 110a.
5. **Update [docs/RUNNERS.md](docs/RUNNERS.md)** — document the dispatcher's role, the 110a/110b split, and what each half ships.
6. **Update [docs/runner-capability-matrix.md](docs/runner-capability-matrix.md)** — fill in cells 3 + 4 with `PASS` (per the runs above); cells 1 + 2 stay `TBD` until 110b.
7. **Tear down live AWS** (operator-scheduled) via `terragrunt destroy` from both terragrunt envs.

## Phase 110b — queued (next branch)

Not started; landing target is a fresh branch + PR after 110a closes. See [PLAN.md § Phase 110b](PLAN.md). Headlines:
- Bind-mount → EFS translation feature in sockerless ECS + Lambda backends.
- New `sockerless-runner` ECR repo via Terraform.
- Runner image push (with `LABEL com.sockerless.ecs.task-definition-family=sockerless-runner`).
- Pre-registered `sockerless-runner` ECS task definition (multi-container: runner + sockerless sidecar; EFS-backed workspace) in Terraform under `terraform/environments/runner/live/`.
- Lambda function definition for cell 2 (with `FileSystemConfigs` for EFS).
- Cells 1 + 2 wired via the dispatcher.

## Bug log this session (PR #122)

All resolved.

- ✓ BUG-845 — Lambda live env was us-east-1; realigned to eu-west-1 + sockerless-tf-state.
- ✓ BUG-846 — Docker Hub PAT path replaced with AWS Public Gallery routing in `resolveImageURI`. Verified live: `docker run --rm alpine:latest echo hi` → exit 0 from Fargate.
- ✓ BUG-847 — GH runner asset URL (`darwin` → `osx`) + version bump (2.319.1 → 2.334.0). Reusable in 110b.
- ✓ BUG-848 — `docker info` reported hardcoded `amd64`; now reflects configured `RuntimePlatform.CpuArchitecture` (ECS) / `Architectures` (Lambda). Required env vars: `SOCKERLESS_ECS_CPU_ARCHITECTURE` (X86_64/ARM64) + `SOCKERLESS_LAMBDA_ARCHITECTURE` (x86_64/arm64). No defaults — Validate() rejects empty.
- ✓ BUG-849 — runner image fixes: drop `--add-host host-gateway` (broken on Podman 5.x — auto-resolved aliases work without it); install docker CLI in the runner image so `container:` directive can do its docker create + exec.

The bind-mount limitation (`container:` directive's host bind mounts of `/home/runner/_work`) is **not a bug of BUG-849** — it's the architectural mismatch between GitHub's worker-pattern runner and Fargate's task-isolated filesystem. Resolved in 110b via the bind-mount → EFS translation feature.

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
- **`github-runner-dispatcher` is sockerless-agnostic** — pure Docker SDK / CLI client. Sockerless integration is invisible to it; encoded in image labels + pre-registered ECS task definitions.

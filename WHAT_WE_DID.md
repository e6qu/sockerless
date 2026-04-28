# Sockerless — What We Built

Docker-compatible REST API that runs containers on cloud backends (ECS, Lambda, Cloud Run, GCF, ACA, AZF) or local Docker. 7 backends, 3 cloud simulators, validated against SDKs / CLIs / Terraform. Designed to power CI runners (GitHub Actions + GitLab Runner) on cloud serverless capacity — see [docs/RUNNERS.md](docs/RUNNERS.md).

See [STATUS.md](STATUS.md) for the current phase roll-up, [BUGS.md](BUGS.md) for the bug log (per-bug fix detail), [PLAN.md](PLAN.md) for the roadmap, [DO_NEXT.md](DO_NEXT.md) for the resume pointer, [specs/](specs/) for architecture (start with [specs/SOCKERLESS_SPEC.md](specs/SOCKERLESS_SPEC.md), [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md), [specs/BACKEND_STATE.md](specs/BACKEND_STATE.md), [specs/SIM_PARITY_MATRIX.md](specs/SIM_PARITY_MATRIX.md)).

This file keeps narrative / "why we did it" context that doesn't live in BUGS.md or git log. Per-bug detail belongs in [BUGS.md](BUGS.md) — don't duplicate it here.

## Phase 110 — runner integration (in flight, PR #122)

Architecture exploration that landed two important separations:

**GitLab vs GitHub runner — dispatcher pattern vs worker pattern.** GitLab Runner is a *dispatcher*: the master polls GitLab and uses the docker executor's `docker create + docker exec` to spawn the job container. The master is just a docker client; it never bind-mounts its own filesystem; it can run anywhere with `--docker-host` pointing at sockerless. **Cells 3 + 4 need zero new sockerless code.** GitHub Actions Runner is a *worker*: it *is* the workspace. For `container:` jobs it does `docker create -v /home/runner/_work:/__w …` — host bind mounts that assume a shared filesystem with the spawned container. On Fargate two tasks don't share filesystems by default. **Cells 1 + 2 require both a topology change (runner-as-ECS-task with EFS-backed workspace) and a sockerless feature (bind-mount → EFS translation).**

**`github-runner-dispatcher` is sockerless-agnostic.** A new top-level Go module (own `go.mod`, separate dep tree) that speaks only the public Docker API / CLI. Pointed at local Podman it spawns runners locally; pointed at sockerless via `DOCKER_HOST` it spawns runners in Fargate. The dispatcher doesn't know sockerless exists — same `docker run` call, different daemon. Sockerless's role (sidecar injection, EFS bind-mount translation) is invisible to the dispatcher; it's encoded in (a) image labels set at runner-image build time and (b) a pre-registered ECS task definition that sockerless's ECS backend recognizes via the image label and dispatches to.

**Static task definition for the runner-task.** The runner-task's shape (multi-container: runner + sockerless sidecar; EFS-backed workspace; IAM role; log config) lives in Terraform as a pre-registered ECS task definition with a stable ARN. Sockerless's ECS backend, when it sees an image with `LABEL com.sockerless.ecs.task-definition-family=sockerless-runner`, calls `RunTask --task-definition sockerless-runner:LATEST` with per-job container-override env vars (`REG_TOKEN` / `LABELS` / `RUNNER_NAME`) — no dynamic task-def composition. Operator owns the runner-task spec; sockerless just dispatches. (Job-tasks the runner subsequently spawns inside the workflow keep dynamic composition; that's where the bind-mount-via-EFS feature plugs in.)

Splits into two PRs: 110a (cells 3 + 4 + dispatcher skeleton — closes PR #122) and 110b (sockerless EFS feature + runner image push to ECR + cells 1 + 2). See [PLAN.md § Phase 110](PLAN.md) for the full plan, [docs/RUNNERS.md](docs/RUNNERS.md) for token strategy + wiring.

**Bugs surfaced + fixed in 110a (PR #122):** BUG-845 (Lambda live env was us-east-1; realigned to eu-west-1 + sockerless-tf-state), BUG-846 (Docker Hub PAT path replaced with AWS Public Gallery routing for `alpine`-style library refs — verified live: `docker run alpine echo hi` exits 0 from Fargate), BUG-847 (GH runner asset URL `darwin` → `osx`; pinned 2.319.1 → 2.334.0), BUG-848 (`docker info` reported hardcoded `amd64` — now reflects required `SOCKERLESS_ECS_CPU_ARCHITECTURE` / `SOCKERLESS_LAMBDA_ARCHITECTURE` env vars; ECS RuntimePlatform + Lambda Architectures wired through), BUG-849 (Linux runner container approach: `--add-host host-gateway` syntax fails on Podman 5.x because Podman natively provides `host.docker.internal` and `host.containers.internal` aliases; drop the `--add-host` flag entirely + install docker CLI in the runner image so `container:` directive can do its docker create + exec).

## Phase 109 — strict cloud-API fidelity sweep (PR #121, merged 2026-04-27)

19 audit items closed. Triggered by PR #120 CI failures that traced back to synthetic responses. Goal: every sim slice sockerless touches behaves like the real cloud — same wire shape, same validation rules, same state transitions, same SDK / CLI / Terraform-provider compatibility.

**Why these mattered for runner work.** The runner phases (106/107/110) drive workloads at much higher fidelity than the SDK/CLI matrix did. Every fake the runner trips becomes a live-cloud bug. Stamping them out in the sim now keeps the runner integration (Phase 110) from chasing wire-format mismatches under load.

**Closures, grouped by cloud:**

- **AWS** — Lambda VpcConfig from real subnet CIDR; `awsRegion()` / `awsAccountID()` env-var-configurable identity; Secrets Manager + SSM Parameter Store + KMS Encrypt/Decrypt + DynamoDB (with Terraform state-lock semantics — `attribute_not_exists(LockID)` succeeds first time, `ConditionalCheckFailedException` on contention).
- **GCP** — `compute.firewalls`, `compute.routers` + Cloud NAT, `iam.serviceAccounts.generateAccessToken`, operations endpoint persistence (no synthetic `done=true` for unknown ops).
- **Azure** — IMDS metadata token endpoint, Blob Container ARM control plane, NSG rule priority+direction uniqueness, Private DNS AAAA/CNAME/MX/PTR/SRV/TXT records, NAT Gateways + Route Tables, `Azure-AsyncOperation` polling for Container Apps + Jobs, Key Vault (ARM control + data plane subdomain routing), ARM `SystemData.createdAt` preserved across updates (lastModifiedAt stamped fresh).
- **No-fakes audit on test fixtures** — clean. All hardcoded IDs are sim-pre-registered defaults, configuration values, or intentional negative-test inputs.

The pattern across all 19: handlers that returned hardcoded values, accepted invalid input, or skipped validation real cloud APIs enforce. Fixed by making each handler walk the real-cloud shape, including the failure modes (e.g., DynamoDB ConditionalCheckFailedException, ARM `SystemData.createdAt` preservation).

## Phase 108 — sim-parity matrix audit + Phase 106/107 harness scaffolding (PR #120, merged 2026-04-27)

Cumulative 22 bug closures + framework + matrix work.

- **Phase 104 framework migration complete.** 13 typed driver interfaces + `DriverContext` envelope + `Driver.Describe()` composition rule + `SOCKERLESS_<BACKEND>_<DIMENSION>` override resolver. Every dispatch site (Exec, Attach, Logs, Signal, ProcList, FSDiff, FSRead/Write/Export, Commit, Build, Registry) flows through `TypedDriverSet`. Cloud-native typed drivers across every backend — 44/91 cells in the per-backend matrix are cloud-native, bypassing api.Backend; the rest stay on legacy adapters whose api.Backend method already does the cloud-native thing. Per-backend default-driver matrix in [specs/DRIVERS.md](specs/DRIVERS.md).
- **Type tightening underway.** `core.ImageRef` domain object (`{Domain, Path, Tag, Digest}` + `ParseImageRef` + `String()`) lands at the typed `RegistryDriver.Push/Pull` boundary. Handlers parse once at dispatch; the typed driver receives a structured value.
- **Phase 105 waves 1-3.** Golden shape tests for 8 libpod handlers: `pod inspect` + `pod stop`/`kill` Errs serialization + `info`, `containers/json`, rm-report, `images/pull` stream, `networks/json`, `volumes/json`, `system/df`.
- **Phase 108 closed (77/77 ✓).** [`specs/SIM_PARITY_MATRIX.md`](specs/SIM_PARITY_MATRIX.md) audit walked all 77 cloud-API rows (33 AWS / 16 GCP / 28 Azure). Standing rule strengthened (PLAN.md principle #10): any new SDK call added to a backend must update the matrix + add the sim handler in the same commit.
- **Phase 106/107 harnesses shipped.** [`tests/runners/github/harness_test.go`](tests/runners/github/harness_test.go) and [`tests/runners/gitlab/harness_test.go`](tests/runners/gitlab/harness_test.go) — build-tag-gated end-to-end harnesses. Live-cloud runs against real repos pending Phase 110.

**Repo-wide cleanup also landed.** All `Phase NN` / `BUG-NNN` references stripped from Go source comments + docs (the metadata stays in BUGS.md / git log / PR descriptions). Manual-tests directory consolidated; redundant simulator-parity docs deleted; 633 task-archive `.md` files dropped from `_tasks/done/`.

## Round-7 / Round-8 / Round-9 live-AWS sweeps (PRs #117, #118)

Three rounds of live-AWS testing in `eu-west-1` against ECS + Lambda, replaying [manual-tests/02-aws-runbook.md](manual-tests/02-aws-runbook.md) against [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md). 46 bugs closed total (BUG-770..819).

**Headlines:**
- **Round-7 (PR #117).** ImageRemove correctness, ECS task lifecycle (rename, restart, kill-signal mapping), libpod compat, OCI push auth + config-blob, Lambda bootstrap PID + heartbeat, registry persistence robustness.
- **Round-8 + Round-9 (PR #118).** Real registry-to-registry layer mirror (BUG-788, closes 4 retroactive bugs); live SSM frame capture → exit-code marker; sync `docker stop`; per-network SG isolation; Lambda Active-waiter; per-cloud `null_resource sockerless_runtime_sweep` so `terragrunt destroy` is self-sufficient.

These rounds proved the live-AWS path before Phase 110 starts integrating real CI runners against it.

## Older closed phases (compressed)

Per-bug detail in BUGS.md, code-level detail in `git log`.

| Phase(s) | Headline | PR |
|---|---|---|
| 96 / 98 / 99 / 100 / 101 / 102 + 13-bug audit | Reverse-agent + SSM machinery for `docker top / stat / cp / get-archive / put-archive / export / diff / commit / pause`. Shared `core.ReverseAgentRegistry` + `HandleReverseAgentWS`. Sim parity for cloud-native exec/attach. | #115 |
| 91–95 | Real per-cloud volumes — `docker volume create` provisions EFS access points (AWS), GCS buckets (GCP), Azure Files shares (Azure). FaaS invocation-lifecycle tracker + GCP label-value charset compliance. | #114 |
| 87 / 88 / 89 / 90 | Cloud Run Services + ACA Apps (internal-ingress workloads, peers via Cloud DNS / Private DNS CNAMEs). Stateless audit + no-fakes sweep. | #113 |
| 86 | Simulator parity + Lambda agent-as-handler. Pre-commit contract: every new sim handler needs SDK+CLI+terraform coverage. | #112 |

Earlier phases (≤ Phase 85) are summarised in PR descriptions and git log.

## Stack & structure

- **Simulators** — `simulators/{aws,gcp,azure}/`, separate Go modules. `simulators/<cloud>/shared/` for container + network helpers; `sdk-tests/` / `cli-tests/` / `terraform-tests/` for external validation.
- **Backends** — 7 backends (`backends/docker`, `backends/ecs`, `backends/lambda`, `backends/cloudrun`, `backends/cloudrun-functions`, `backends/aca`, `backends/azure-functions`). Each a separate Go module. Cloud-common shared: `backends/{aws,gcp,azure}-common/`. Core driver + shared types: `backends/core/`.
- **Agent** — `agent/` with sub-commands for the in-container driver + Lambda bootstrap. Shared simulator library: `github.com/sockerless/simulator`.
- **Frontend** — Docker REST API. `cmd/sockerless/` zero-dep CLI. UI SPA at `ui/` (Bun / React 19 / Vite / React Router 7 / TanStack / Tailwind 4 / Turborepo), embedded via Go `!noui` build tag.
- **Tests** — `tests/` for cross-backend e2e, `tests/upstream/` for external-suite replays (act, gitlab-ci-local), `tests/runners/{github,gitlab}/` for real-runner harnesses (build-tag-gated), `tests/terraform-integration/`, `smoke-tests/` for per-cloud Docker-backed smokes.

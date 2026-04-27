# Sockerless — What We Built

Docker-compatible REST API that runs containers on cloud backends (ECS, Lambda, Cloud Run, GCF, ACA, AZF) or local Docker. 7 backends, 3 cloud simulators, validated against SDKs / CLIs / Terraform. Designed to power CI runners (GitHub Actions + GitLab Runner) on cloud serverless capacity — see [docs/RUNNERS.md](docs/RUNNERS.md).

See [STATUS.md](STATUS.md) for the current phase roll-up, [BUGS.md](BUGS.md) for the bug log (per-bug fix detail), [PLAN.md](PLAN.md) for the roadmap, [DO_NEXT.md](DO_NEXT.md) for the resume pointer, [specs/](specs/) for architecture (start with [specs/SOCKERLESS_SPEC.md](specs/SOCKERLESS_SPEC.md), [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md), [specs/BACKEND_STATE.md](specs/BACKEND_STATE.md), [specs/SIM_PARITY_MATRIX.md](specs/SIM_PARITY_MATRIX.md)).

This file keeps narrative / "why we did it" context that doesn't live in BUGS.md or git log. Per-bug detail belongs in [BUGS.md](BUGS.md) — don't duplicate it here.

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

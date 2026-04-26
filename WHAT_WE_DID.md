# Sockerless — What We Built

Docker-compatible REST API that runs containers on cloud backends (ECS, Lambda, Cloud Run, GCF, ACA, AZF) or local Docker. 7 backends, 3 cloud simulators, validated against SDKs / CLIs / Terraform.

See [STATUS.md](STATUS.md) for the current phase roll-up, [BUGS.md](BUGS.md) for the bug log (per-bug fix detail), [PLAN.md](PLAN.md) for the roadmap, [DO_NEXT.md](DO_NEXT.md) for the resume pointer, [specs/](specs/) for architecture (start with [specs/SOCKERLESS_SPEC.md](specs/SOCKERLESS_SPEC.md), [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md), [specs/BACKEND_STATE.md](specs/BACKEND_STATE.md), [specs/SIM_PARITY_MATRIX.md](specs/SIM_PARITY_MATRIX.md)).

This file keeps narrative / "why we did it" context that doesn't live in BUGS.md or git log. Per-bug detail belongs in [BUGS.md](BUGS.md) — don't duplicate it here.

## Post-PR-#118 audit + Phase 104 + 105 + 108 (PR #120 — open)

PR #118 merged the round-8 + round-9 live-AWS sweep. The post-merge audit pass on this branch records every previously-open or "known-issue" bug as a real fix per the no-defer / no-fakes / no-fallbacks rule, and ships **Phase 104 skeleton + lifts 1+2**, **Phase 105 waves 1-3** golden tests, and **Phase 108 closure** (sim-parity matrix at 77/77 ✓) on the same branch.

- **22 bug closures.** BUG-802 + 638/640/646/648 retro + 804/806 + 820..831 + 832..835. Full per-bug detail in [BUGS.md](BUGS.md).
- **Phase 104 skeleton + lifts 1 and 2.** 13 typed driver interfaces (Exec / Attach / FSRead / FSWrite / FSDiff / FSExport / Commit / Build / Stats / ProcList / Logs / Signal / Registry) plus the `DriverContext` envelope, the `Driver.Describe()` composition rule, and the `SOCKERLESS_<BACKEND>_<DIMENSION>` override resolver. **Lift 1 (Exec)**: `WrapLegacyExec` in `driver_adapt_exec.go`. **Lift 2 (Attach)**: `WrapLegacyAttach` plus `NewCloudLogsAttachDriver` (lifts `core.AttachViaCloudLogs`). No behaviour change yet — backends keep their existing impls; opting in to `DriverSet104.{Exec,Attach}` is the next step.
- **Phase 105 waves 1-3.** Golden shape tests for **8** libpod handlers: `pod inspect` (BUG-804) + `pod stop`/`kill` Errs serialization (BUG-806) + `info`, `containers/json`, rm-report, `images/pull` stream, `networks/json`, `volumes/json`, `system/df`. The shape tests pin top-level shape (object vs array vs stream) plus every required field name, so podman-CLI compatibility regressions surface at CI time instead of in live sweeps.
- **Phase 108 closed (77/77 ✓).** [`specs/SIM_PARITY_MATRIX.md`](specs/SIM_PARITY_MATRIX.md) audit walked all 77 cloud-API rows (33 AWS / 16 GCP / 28 Azure). Closed BUG-832 (sim/aws ECS TagResource), BUG-833 (sim/gcp v2 Cloud Run Services), BUG-834 (sim/azure ContainerApps Apps surface), BUG-835 (sim/azure WebApps.UpdateAzureStorageAccounts). Every fix pinned with SDK + CLI tests at the wire-format the backend uses. Standing project rule strengthened (now PLAN.md principle #10): any new SDK call added to a backend must update the matrix + add the sim handler in the same commit.

**Why these audit findings mattered.** Several were silent failures the existing test matrix couldn't see — backends used best-effort tagging (BUG-832), or feature flags (`UseService`/`UseApp`) had no integration coverage against the sim (BUG-833/834), or the synthetic-data sweep surfaced fields that read as real-but-fabricated (BUG-820..831). Recording each as a bug + a real fix makes the project state honest about what works and removes the temptation to revert to "it works against the sim" as a proxy for "it works."

**New phases queued in this PR** ([PLAN.md](PLAN.md)):

- **Phase 106** — Real GitHub Actions runner integration via `actions/runner` + DOCKER_HOST → sockerless. ECS + Lambda first. Architecture decision recorded in PLAN.md: per-backend daemon (v1) → label-based dispatch via Phase 68 (v2).
- **Phase 107** — Real GitLab Runner docker-executor → sockerless. Uses the `origin-gitlab` mirror (CI must be enabled mirror-side) or self-hosted GitLab CE in a test container. Same coverage shape as 106 plus `dind` sub-test.

## Round-7 / Round-8 / Round-9 live-AWS sweeps (PRs #117, #118)

Three rounds of live-AWS testing in `eu-west-1` against ECS + Lambda, replaying the per-test crosswalk in [PLAN_ECS_MANUAL_TESTING.md](PLAN_ECS_MANUAL_TESTING.md) against [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md). Working state archived in [docs/manual-test-spec-crosswalk.md](docs/manual-test-spec-crosswalk.md).

**Round-7 (PR #117).** 16 bugs closed (BUG-770..785). Categories: ImageRemove correctness, ECS task lifecycle (rename, restart, kill-signal mapping, removal-via-registry), libpod compat (specgen create, container list, normalised times), OCI push auth + config-blob, Lambda bootstrap PID publishing + heartbeat mutex, registry persistence robustness.

**Round-8 + Round-9 (PR #118).** 30 bugs closed (BUG-786..819). Headlines:
- **BUG-788** — registry-to-registry layer mirror. `ImagePull` now downloads layer blobs into `Store.LayerContent` and records `[]ManifestLayerEntry`; `OCIPush` uses source compressed digests verbatim. Closes 4 retroactive "known-issue" bugs (BUG-638/640/646/648).
- **BUG-789/798** — live SSM frame capture → exit-code marker. Real ECS exec via SSM was missing the exit-code byte; now extracts it from the SSM stream.
- **BUG-790** — sync `docker stop` blocks until ECS observes STOPPED, so immediate `docker rm` succeeds.
- **BUG-794** — cross-network isolation. Per-network SG is the sole authority for containers with `--network X`.
- **BUG-799/800** — stateless invariant restored. Recovery skips STOPPED tasks; `core.ResourceRegistry` Save/Load collapsed to no-ops; 11 stale `sockerless-registry.json` files swept.
- **BUG-815/816/817** — `sh -c` exec wrap, busybox-compat find, stat tab format.
- **BUG-807..812** — Lambda track: wait-for-Active waiter, PrebuiltOverlayImage independence from CallbackURL, ExecStart hijack-before-error, stale "loaded from disk" log, tag-based InvocationResult persistence + replay, LastModified RFC3339Nano conversion.
- **BUG-819** — per-cloud `null_resource sockerless_runtime_sweep` so `terragrunt destroy` is self-sufficient on every backend.

Live AWS torn down post-merge.

## Older closed phases (compressed)

Newest first; per-bug detail in BUGS.md, code-level detail in `git log`.

| Phase(s) | Headline | PR | Date |
|---|---|---|---|
| 96 / 98 / 98b / 99 / 100 / 101 / 102 + 13-bug audit | Reverse-agent + SSM machinery for `docker top / stat / cp / get-archive / put-archive / export / diff / commit / pause`. Shared `core.ReverseAgentRegistry` + `HandleReverseAgentWS`; CR/ACA/GCF/AZF mount `/v1/<backend>/reverse`; ECS parity via SSM ExecuteCommand. Sim parity for cloud-native exec/attach. | #115 | 2026-04-24 |
| 91 / 92 / 93 / 94 / 94b | Real per-cloud volumes — `docker volume create` provisions EFS access points (AWS), GCS buckets (GCP), Azure Files shares (Azure). | #114 | 2026-04-21 |
| 95 | FaaS invocation-lifecycle tracker — `core.InvocationResult` captures per-container exit code + finished-at + error at the invocation source. | #114 | 2026-04-21 |
| 97 | GCP label-value charset compliance — charset-safe label encoding + annotation routing for non-conforming values. | #114 | 2026-04-21 |
| 89 / 90 | Stateless audit + no-fakes sweep. Every cloud backend derives state from cloud actuals; project-wide audit of workarounds, silent substitutions, placeholder fields. | #113 | 2026-04-21 |
| 87 / 88 | Cloud Run Services + ACA Apps — internal-ingress workloads with VPC connector / managed environment; peers resolve via Cloud DNS / Private DNS CNAMEs. | #113 | 2026-04-21 |
| 86 | Simulator parity + Lambda agent-as-handler. Every cloud-API slice sockerless depends on is a first-class slice in its per-cloud simulator. Pre-commit contract: every new sim handler needs SDK+CLI+terraform coverage. | #112 | 2026-04-20 |

## Stack & structure

- **Simulators** — `simulators/{aws,gcp,azure}/`, separate Go modules. `simulators/<cloud>/shared/` for container + network helpers; `sdk-tests/` / `cli-tests/` / `terraform-tests/` for external validation.
- **Backends** — 7 backends (`backends/docker`, `backends/ecs`, `backends/lambda`, `backends/cloudrun`, `backends/cloudrun-functions`, `backends/aca`, `backends/azure-functions`). Each a separate Go module. Cloud-common shared: `backends/{aws,gcp,azure}-common/`. Core driver + shared types: `backends/core/`.
- **Agent** — `agent/` with sub-commands for the in-container driver + Lambda bootstrap. Shared simulator library: `github.com/sockerless/simulator`.
- **Frontend** — Docker REST API. `cmd/sockerless/` zero-dep CLI. UI SPA at `ui/` (Bun / React 19 / Vite / React Router 7 / TanStack / Tailwind 4 / Turborepo), embedded via Go `!noui` build tag.
- **Tests** — `tests/` for cross-backend e2e, `tests/upstream/` for external-suite replays (act, gitlab-ci-local), `tests/e2e-live-tests/` for runner orchestration, `tests/terraform-integration/`, `smoke-tests/` for per-cloud Docker-backed smokes.

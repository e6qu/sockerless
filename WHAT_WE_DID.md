# Sockerless — What We Built

Docker-compatible REST API that runs containers on cloud backends (ECS, Lambda, Cloud Run, GCF, ACA, AZF) or local Docker. 7 backends, 3 cloud simulators, validated against SDKs / CLIs / Terraform. Designed to power CI runners (GitHub Actions + GitLab Runner) on cloud serverless capacity — see [docs/RUNNERS.md](docs/RUNNERS.md).

State [STATUS.md](STATUS.md) · roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · architecture [specs/](specs/) (start with [specs/SOCKERLESS_SPEC.md](specs/SOCKERLESS_SPEC.md), [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md), [specs/BACKEND_STATE.md](specs/BACKEND_STATE.md), [specs/SIM_PARITY_MATRIX.md](specs/SIM_PARITY_MATRIX.md)).

This file keeps narrative — *why* we did each phase, what was surprising, what blocked. Per-bug detail belongs in [BUGS.md](BUGS.md); code-level detail in `git log`.

## 2026-05-09 — Phase 128 Runner job timeout (PR pending)

First of the post-Phase-135 ordered roadmap. Hard cap on workload containers so a hung subprocess can't pin cloud quota indefinitely. Two layers, belt-and-suspenders:

**Layer 1 — bootstrap timer.** New helpers `runWithTimeout` + `jobTimeoutFromEnv` in `agent/cmd/sockerless-cloudrun-bootstrap` and `agent/cmd/sockerless-gcf-bootstrap`. Reads `SOCKERLESS_JOB_TIMEOUT_SECONDS` (default 3600, negative → 0/disabled), starts the user subprocess, arms a timer; on fire sends SIGTERM, waits 30s, SIGKILLs, returns exit code 124 (matches GNU `timeout(1)`). Wired in 3 exec sites per bootstrap (sidecar mode, default-invoke, exec-envelope) — covers Cloud Run multi-container revisions + the Path B docker-exec round-trip.

**Layer 2 — cloud-native cap.** Sockerless backends set the cloud's native max-duration field as a safety net for bootstrap crashes:
- Cloud Run `TaskTemplate.Timeout`: derived from `core.JobTimeoutDefault()`, clamped to 24 h.
- ACA `ReplicaTimeout`: derived likewise, clamped to 7 d.
- Lambda `function.Timeout`: already defaults to 900 s (Lambda's hard cap; nothing to add).
- ECS Fargate has no native timeout — Layer 1 is the only path; sockerless doesn't yet inject a bootstrap into ECS workloads, so ECS is documented as effectively unlimited until a follow-up adds an ECS-side bootstrap.

**Shared config.** `backends/core/job_timeout.go` exposes `JobTimeoutEnvName`, `DefaultJobTimeoutSeconds=3600`, `JobTimeoutDefault()` (operator override via process env), and `JobTimeoutEnvIfUnset(env)` which respects per-job operator overrides via `docker run -e`. Both cloudrun + gcf backends use these for env injection on every workload container.

Tests: 5 unit tests in `agent/cmd/sockerless-cloudrun-bootstrap/main_test.go` (timer fires-on-hang, finishes-early, zero-exit, disabled-by-zero, env parse). 2 unit tests in `backends/core/job_timeout_test.go`. 1 integration test (`TestCloudRunJobTimeout`) submits alpine + `sleep 9999` with `SOCKERLESS_JOB_TIMEOUT_SECONDS=2`, expects exit 124 + log line.

Branch carries a small upfront doc reshuffle (drops Phase 68 multi-tenant pools and Phase 129 cost-tracking remainder per user direction; ordered roadmap 128 → 124 → 125 → 126 → 127 → 121b → 78 in PLAN.md and DO_NEXT.md).

## 2026-05-09 — Phase 135 Sim host model + 3-tier coverage (PR #129)

Architectural correction surfaced by user feedback: simulators conflated their own binary's compile arch with the workload's arch. Right model: **services provision hosts; hosts run workloads via Docker honouring the workload's `Architecture` field; sim's primary capacity contract is `linux/arm64`**. Sim binary itself stays host-native (Mac arm64 locally; CI is now linux/arm64 too via native ARM runners).

**135a–e (architectural).** `ContainerConfig.Architecture` field plumbed across all 3 sims to Docker `ImagePull` + `ContainerCreate` Platform; `parsePlatform("")` errors at the shared-lib boundary (no silent fallback). GCP Cloud Functions migrated from `StartProcess` to `StartContainerSync` (closes the literal BUG-949 site). AWS ECS `ExecuteCommand` fallback dropped. Per-product host-metadata services: AWS IMDSv2 + ECS task metadata v4 + instance-identity-document, GCP `metadata.google.internal/computeMetadata/v1/*` (project ID, instance, SA tokens, ID tokens), Azure IMDS `/metadata/instance` + existing identity routes. Workload-host wiring via env (`AWS_EC2_METADATA_SERVICE_ENDPOINT` / `ECS_CONTAINER_METADATA_URI_V4` / `GCE_METADATA_HOST` / `IDENTITY_ENDPOINT`). Static `host_dispatch_test.go` per sim asserts no production code path `os/exec`s a workload (allowlist for sim tooling like cloudbuild's `docker` CLI).

**135f (three-tier coverage).** Per user request: SDK + CLI + Terraform exercising recent sim additions across all 3 clouds. SDK tests via the official cloud-Go-SDK metadata clients (cloud.google.com/go/compute/metadata × 6, aws-sdk-go-v2/feature/ec2/imds × 4 with IMDSv2 token dance, azidentity.NewManagedIdentityCredential × 1). GCP CLI test for Compute Disks via gcloud. GCP Terraform test for `google_compute_disk`. AWS + Azure CLI/TF skipped because their metadata routes have no natural CLI/TF consumers (no `aws ec2 fetch-imds` command; no `azurerm` resource for IMDS).

**12 bugs surfaced + fixed in the same session** (BUG-949/972/975/976/977/978/979/980/981/982/983/984): GCF os/exec-of-workload, AWS ECS ExecuteCommand fallback, IMDS instance-identity-document missing, sdk-tests host-build-then-COPY pattern broke under linux/arm64 force, CI runners missing QEMU, gcloud crashed on incomplete zoneOp shape, ComputeDisk SizeGb rejecting unquoted JSON numbers, gcloud zone existence-probe 404, cli-tests + backend-integration-tests host-build pattern, golang base image too old (1.24→1.25), sim CI 5min timeout too tight under emulation, gcloud x86_64 install URL on ARM runner.

**CI: native ARM runners.** Final move was switching the 4 jobs that run Docker workloads (sim, test, test-e2e, smoke) from `ubuntu-latest` (amd64 + QEMU) to `ubuntu-24.04-arm`. Eliminates emulation overhead + QEMU edge cases, makes host arch == sim's primary capacity == workload arch — no platform-mismatch traps anywhere. Other jobs (ui, terraform, lint, build-check) stay on amd64 since they don't run Docker workloads.

## 2026-05-08 — Phase 134 Makefile standardization + sim test stability

Single-branch consolidation: `makefile-standardization` (PR #128). All 11 CI checks GREEN at sha a5056a0.

**Make surface unification.** Every leaf gets its own Makefile, and the top-level Makefile delegates by path (`make backends/ecs/build` runs `$(MAKE) -C backends/ecs build`). Shared recipes live in `make/{help,colors,go-app,go-lib,ui-app,stack}.mk`; each leaf Makefile is 5–10 lines of variables + an `include`. Auto-generated `make help` from `## comment` lines via awk. Stack orchestration via `.stack-pids/` PID files for `make stack-aws-ecs` etc. Per-app `RUN_ENV` for sim-backend wiring without coupling.

The CI lint runner doesn't carry bun, so the original fan-out `lint` target (which hit UI packages whose `lint` recipe shells `bun install`) blew up with `bun: not found`. Split surface: top-level `lint` runs Go-only; new `lint-ui` covers UI packages; `lint-all` runs both. Mirrors what each CI job already had installed.

**README rewrite + 17 doc updates.** Replaced the old Quick Start with the `make stack-aws-ecs` flow; added a comprehensive "Make targets" section. 17 backend / manual-tests / examples/terraform docs migrated from raw `go build -o sockerless-backend-X ...` to `make backends/X/build`.

**BUG-973/974 — sim test stability.** The Makefile work surfaced two real bugs that had been masked by a `Dockerfile.test`-missing build error on main: `TestECS_TaskLogsToCloudWatch` (aws) and `TestContainerApps_JobArithmetic{Invalid,Logs}` (azure) used fixed `time.Sleep(2s)` to wait for container completion before asserting. Slow CI runners exceed 2s on image pull + container start. Both rewritten as `require.Eventually(30s, 250ms)` polling for the actual condition. Per the no-pre-existing rule, both filed in BUGS.md and fixed in the same session.

## 2026-05-08 — PR #127: Phase 129 #4 + sim parity prep + bleephub Phases 130/131/132

Single PR carrying five threads, all stacked on the `phase-130` branch per the single-work-branch rule.

**Phase 129 #4 — owner-linked orphan-Service sweep.** Extended the dispatcher's 2-minute Cleanup ticker to reap orphan `sockerless-svc-*` Services left behind when a runner-task dies before issuing ContainerRemove. The dispatcher-generic rule (`feedback_dispatcher_generic.md`) forbids the dispatcher from injecting any `SOCKERLESS_*` env into the runner-task — so the owner identifier had to be discovered sockerless-side. Cloud Run already auto-injects `CLOUD_RUN_JOB` on every Job execution. Sockerless reads it via `gcp-common/owner_label.go::OwnerRunnerTaskLabelValue` and stamps `sockerless_owner_runner_task=<jobID>` on every pod-Service it creates (cloudrun + gcf). Cleanup builds a set of live-owner Job IDs from `ListManaged`, lists `sockerless-svc-*` per region, deletes any whose owner Job is gone or terminal. Services with empty owner labels are left to the existing flat idle-time sweep (legacy). Live verification deferred to next live-cloud session.

**Forward-looking sim parity** (Phase 126 + 127 prep). GCP `iamcredentials.generateIdToken` added to `simulators/gcp/iam.go` with `mintSimIdToken` helper in `oauth2.go`. GCP Compute Disks CRUD added to `simulators/gcp/compute.go::registerComputeDisks` (Insert / Get / List / Delete / Resize / SetLabels + aggregated-list + zonal-ops). 6 new SDK tests; 8 new SIM_PARITY_MATRIX rows under "forward-looking (no current backend caller; SDK-test-validated)".

**Phase 130 — bleephub workflow runs / jobs / runners REST.** `bleephub/gh_actions_rest.go` registers 10 GitHub-shape routes: runs list/get/jobs/cancel/rerun/delete + jobs get/logs + runners list/delete. `stableJobID` (FNV-1a 64-bit) maps internal UUIDs to int64 GitHub-style IDs. `rerun` returns 422 pointing at Phase 131's dispatch route. 14 new tests.

**Phase 131 — bleephub workflows REST + UI dispatch.** New `WorkflowFile` entity (file-level, distinct from the run-level `Workflow`) with go-git tree-walk discovery from each repo's in-memory storer at HEAD. Auto-register on `POST /api/v3/bleephub/workflow` submit. Routes: `GET /actions/workflows`, `GET .../workflows/{id}` (numeric ID, exact path, or filename), `GET .../workflows/{id}/runs`, `POST .../workflows/{id}/dispatches`. UI: `WorkflowsPage` refactored into Workflows + Runs tabs; per-workflow "Run workflow" dialog. 10 Go + 4 UI tests.

**Phase 132 — bleephub apps + oauth completeness.** `GET /api/v3/user/installations` + `/repositories`; `DELETE /api/v3/installation/token`; OAuth web flow (`GET /login/oauth/authorize` HTML form + `?auto=1` auto-approve + form-POST companion); `POST /login/oauth/access_token` extended with `authorization_code` grant alongside the existing device flow. UI: `AppsPage` + `OAuthPage`. 14 Go + 6 UI tests.

**Admin UI scoping decision** (recorded so it doesn't get re-asked): bleephub admin lives in bleephub UI itself. The sockerless-admin app stays focused on its existing scope (projects / containers / processes / resources / cleanup / contexts) — coupling bleephub-specific admin into sockerless-admin would mix two independently-deployed products.

## 2026-05-07 — Phase 123 + 8/8 cells GREEN (milestone closed)

The 17-iteration cells-5+6 saga ended. Phase 123 (storage backing driver abstraction with `gcs-sync`) shipped, cells 5+6 went GREEN at v17, the 8/8 runner-integration milestone closed. Per-bug fix detail in [BUGS.md](BUGS.md); cell URLs in [STATUS.md](STATUS.md).

**`gcs-sync` data plane** replaced FUSE-on-object-store for shared workspaces. The runner-task tars `/tmp/runner-work` to a per-exec GCS object before forwarding the exec POST; the JOB pod-Service bootstrap untars from the same object before running the subprocess, then tars the modified workspace back; the runner-task untars on response. Pure GCS SDK calls — no FUSE in the data path. Per-step granularity matches GH actions/runner's per-step script pattern.

**The `SOCKERLESS_SYNC_MOUNTS` / `SOCKERLESS_SYNC_VOLUMES` split** carried two distinct lists, joined by name at the bootstrap: mounts (name=mountpath) injected at materialize time on the JOB main container's spec; volumes (name=gs://bucket/object) injected per-exec via the envelope's `Env` field. Together they let the bootstrap know both *what* to sync (mount + name) and *where* (per-exec GCS object) without baking the GCS object name into the long-lived Service spec.

**BUG-970 — regional CPU quota debt**. Cells 5+6 v15 hit "container failed to bind PORT=8080" on later container deploys. Root cause: every materialized pod-Service was setting `MinInstanceCount=1` so the revision stayed warm — but with 5+ pod-Services per pipeline that pinned ~10 vCPU of regional quota per pipeline, accumulated across iterations as orphans. Structural fix: `MinInstanceCount=0` on all pod-Service revisions; cold-start latency on first /exec POST after idle measured at <5 s (acceptable). Phase 129 #4 (the next session) closed the second half — owner-link orphan GC.

**ECS test regression** — found a no-fallbacks violation hiding in the `handleContainerWait` fast-path (a synthetic exit-code default that masked real failures). Same-session fix.

## 2026-05-04 → 2026-05-06 — Cells 5/6/7/8 saga (Phase 122d–122m)

Multi-week march from "GCP cells exist on paper" → 4/4 GCP runner cells GREEN. Headlines:

- **Cell 7 GREEN first** (heavy-workload, GitLab × cloudrun) — broke open the materialize-pod-Service path; gitlab-runner stage scripts delivered via tar-pack persist.
- **Cell 8 architectural deep dive** — gcf overlay-build + Functions Gen2 ↔ Cloud Run service auto-wiring; OCI v1 tar layout + label-filter syntax compliance landed in the sim. Async-deploy pattern shipped (BUG-923).
- **BUG-947 — GCSFuse vs git-checkout** — Cell 7 v50 hung at git checkout because GCSFuse invalidates open handles when the underlying object is rewritten (per-step event.json updates). Path A (chosen): emptyDir + per-job Service revision. Path B (rejected): keep FUSE, batch event.json. Path A drove the eventual Phase 123 storage-driver abstraction.
- **Vanilla-runner architecture pivot** (Phase 122j) — confirmed dispatcher must be GitHub/GitLab-runner-vanilla; sockerless lives in the runner image. `feedback_dispatcher_generic.md` codified.
- **Dispatcher rate-limit + gcf pool quota + 3-layer BUG-944** (Phase 122i) — dispatcher implements strict rate-limit handling per `feedback_strict_rate_limit.md`: sleep `max(retryAfter, resetIn) * 1.10 + 1s`, never resume at the boundary.
- **Phase 121 GCP simulator hardening** — real OAuth2 + GCS-on-disk + Cloud Build REST + Cloud Functions Gen2 ↔ Cloud Run service auto-wiring (`seedServiceV2Defaults`) + proto-JSON enum decoding (`enumString` for LaunchStage + Condition.State) + cloud-faithful HTTP-invoke of overlay containers + OCI v1 tar layout + Cloud Logging-style label filters.

## 2026-04-30 — Phase 110 (4 AWS runner cells GREEN, PR #122)

GH×ECS, GH×Lambda, GL×ECS, GL×Lambda all GREEN. 32 bugs closed (845–876). Lambda-side closures: stdin payload for gitlab-runner stage scripts (BUG-875); library/ rejection on AR proxy (BUG-876); overlay-image pattern for reverse-agent injection. Self-sufficient teardown landed (`null_resource sockerless_runtime_sweep` per cloud — `terragrunt destroy` no longer needs ad-hoc runtime cleanup).

## 2026-04-27 — Phase 109 strict cloud-API fidelity audit (PR #121)

19-item audit closing the gap between "tests pass" and "wire shape matches real GCP/AWS/Azure". Lambda VpcConfig from real subnet CIDR; AWS Secrets Manager + SSM Parameter Store + KMS + DynamoDB; GCP `compute.firewalls` + `compute.routers`/Cloud NAT + `iam.generateAccessToken` + operations endpoint persistence; Azure IMDS token endpoint + Blob Container ARM CRUD + NSG priority+direction validation + Private DNS AAAA/CNAME/MX/PTR/SRV/TXT records + NAT Gateways + Route Tables + Container Apps/Jobs Azure-AsyncOperation polling + Key Vault ARM+data plane + ARM `SystemData.createdAt` preservation. No-fakes audit on test fixtures.

## 2026-04-27 — Post-PR-#118 audit + Phase 104 framework + Phase 108 + Phase 106/107 prep (PR #120)

Phase 104 framework migration completed: 13 typed adapters, every dispatch site routed, framework renamed to drop the 104 suffix. Cloud-native typed drivers across every cloud backend (Logs/Attach/Exec/Signal/FS/Commit/ProcList — 44/91 matrix cells cloud-native). `core.ImageRef` typed domain object at the typed Registry boundary. Libpod-shape golden tests for 8 handlers. Phase 108 sim-parity matrix audit (33 AWS + 16 GCP + 28 Azure rows ✓). Phase 106/107 real-runner harnesses scaffolded under `tests/runners/{github,gitlab}/`. Manual-tests directory consolidated; redundant simulator-parity docs deleted; 633 task-archive `.md` files dropped from `_tasks/done/`. Repo-wide `Phase NN` / `BUG-NNN` comment strip.

## Round-7 / Round-8 / Round-9 live-AWS sweeps (PRs #117, #118)

Three rounds of live-AWS testing in `eu-west-1` against ECS + Lambda, replaying [manual-tests/02-aws-runbook.md](manual-tests/02-aws-runbook.md). 46 bugs closed (BUG-770..819).

- **Round-7**: ImageRemove correctness; ECS task lifecycle (rename, restart, kill-signal mapping); libpod compat; OCI push auth + config-blob; Lambda bootstrap PID + heartbeat; registry persistence robustness.
- **Round-8 + 9**: Real registry-to-registry layer mirror (BUG-788, closes 4 retroactive bugs); live SSM frame capture → exit-code marker; sync `docker stop`; per-network SG isolation; Lambda Active-waiter; per-cloud `null_resource sockerless_runtime_sweep` so `terragrunt destroy` is self-sufficient.

These rounds proved the live-AWS path before Phase 110 began integrating real CI runners.

## Older closed phases (compressed)

| Phase(s) | Headline | PR |
|---|---|---|
| 96 / 98 / 99 / 100 / 101 / 102 + 13-bug audit | Reverse-agent + SSM machinery for `docker top / stat / cp / get-archive / put-archive / export / diff / commit / pause`. Shared `core.ReverseAgentRegistry` + `HandleReverseAgentWS`. Sim parity for cloud-native exec/attach. | #115 |
| 91–95 | Real per-cloud volumes — `docker volume create` provisions EFS access points (AWS), GCS buckets (GCP), Azure Files shares (Azure). FaaS invocation-lifecycle tracker + GCP label-value charset compliance. | #114 |
| 87 / 88 / 89 / 90 | Cloud Run Services + ACA Apps (internal-ingress workloads, peers via Cloud DNS / Private DNS CNAMEs). Stateless audit + no-fakes sweep. | #113 |
| 86 | Simulator parity + Lambda agent-as-handler. Pre-commit contract: every new sim handler needs SDK+CLI+terraform coverage. | #112 |

Earlier phases (≤ Phase 85) summarized in PR descriptions and git log.

## Stack & structure

- **Simulators** — `simulators/{aws,gcp,azure}/`, separate Go modules. `simulators/<cloud>/shared/` for container + network helpers; `sdk-tests/` / `cli-tests/` / `terraform-tests/` for external validation.
- **Backends** — 7 backends (`backends/{docker,ecs,lambda,cloudrun,cloudrun-functions,aca,azure-functions}`). Each a separate Go module. Cloud-common shared: `backends/{aws,gcp,azure}-common/`. Core driver + shared types: `backends/core/`.
- **Agent** — `agent/` with sub-commands for the in-container driver + Lambda bootstrap. Shared simulator library: `github.com/sockerless/simulator`.
- **Frontend** — Docker REST API. `cmd/sockerless/` zero-dep CLI. UI SPA at `ui/` (Bun / React 19 / Vite / React Router 7 / TanStack / Tailwind 4 / Turborepo), embedded via Go `!noui` build tag.
- **Tests** — `tests/` for cross-backend e2e; `tests/upstream/` for external-suite replays (act, gitlab-ci-local); `tests/runners/{github,gitlab}/` for real-runner harnesses (build-tag-gated); `tests/terraform-integration/`; `smoke-tests/` for per-cloud Docker-backed smokes.
- **Bleephub** — GitHub-API simulator. 147 routes today (apps, orgs, repos, issues, PRs, hooks, secrets, webhooks). Phase 130 adds workflow-runs / jobs / runners; Phase 131 adds workflows REST + UI dispatch; Phase 132 adds full app installations + OAuth web flow + Apps Manager UI.

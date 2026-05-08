# Sockerless — What We Built

Docker-compatible REST API that runs containers on cloud backends (ECS, Lambda, Cloud Run, GCF, ACA, AZF) or local Docker. 7 backends, 3 cloud simulators, validated against SDKs / CLIs / Terraform. Designed to power CI runners (GitHub Actions + GitLab Runner) on cloud serverless capacity — see [docs/RUNNERS.md](docs/RUNNERS.md).

State [STATUS.md](STATUS.md) · roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · architecture [specs/](specs/) (start with [specs/SOCKERLESS_SPEC.md](specs/SOCKERLESS_SPEC.md), [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md), [specs/BACKEND_STATE.md](specs/BACKEND_STATE.md), [specs/SIM_PARITY_MATRIX.md](specs/SIM_PARITY_MATRIX.md)).

This file keeps narrative — *why* we did each phase, what was surprising, what blocked. Per-bug detail belongs in [BUGS.md](BUGS.md); code-level detail in `git log`.

## 2026-05-08 — Phase 129 #4 owner-linked orphan-Service sweep

Extended the dispatcher's 2-minute Cleanup ticker to reap orphan `sockerless-svc-*` Services left behind when a runner-task dies before issuing ContainerRemove. The variant DO_NEXT.md called the better long-term shape: couples cleanup to runner-task lifetime instead of a flat idle-time check.

The dispatcher-generic rule (`feedback_dispatcher_generic.md`) forbids the dispatcher from injecting any `SOCKERLESS_*` env into the runner-task — so the owner identifier had to be discovered sockerless-side. Cloud Run already auto-injects `CLOUD_RUN_JOB` on every Job execution. Sockerless reads it via the new `gcp-common/owner_label.go::OwnerRunnerTaskLabelValue` helper and stamps `sockerless_owner_runner_task=<jobID>` on every pod-Service it creates (cloudrun + gcf both). The dispatcher's Cleanup builds a set of live-owner Job IDs from the existing ListManaged result, lists `sockerless-svc-*` Services per region, and deletes any whose owner Job is gone or terminal. Services with empty owner labels are left alone (legacy / non-Cloud-Run-Job sockerless) — a flat idle-time sweep is the right tool for those.

Plumbing is small (one env var read + one label + one ListServices call) and produces a precise GC signal. Verified at unit-test level (`spawner_test.go`, `owner_label_test.go`) + module build + go vet clean across `gcp-common`, `cloudrun`, `cloudrun-functions`, `github-runner-dispatcher-gcp`. Live verification deferred to next live-cloud session per DO_NEXT.md.

Also pivoted to **forward-looking sim parity** in the same session: GCP `iamcredentials.generateIdToken` (Phase 126 prep) added to `simulators/gcp/iam.go`; Compute Disks CRUD (Phase 127 GCP `pd-ephemeral` prep) queued. Then survey + plan for **bleephub Phase 130/131/132** (workflow-runs / workflows / apps + oauth REST + UI gaps to make bleephub a true GitHub-API drop-in).

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

# Sockerless — Roadmap

> **Goal:** Replace Docker Engine with Sockerless for any Docker API client — `docker run`, `docker compose`, TestContainers, CI runners — backed by real cloud infrastructure (AWS, GCP, Azure).

State [STATUS.md](STATUS.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/](specs/).

## Guiding principles

1. **Docker API fidelity** — match Docker's REST API exactly.
2. **Real execution** — simulators and backends actually run commands; no stubs, fakes, or mocks.
3. **External validation** — proven by unmodified external test suites.
4. **No new frontend abstractions** — Docker REST API is the only interface.
5. **Driver-first handlers** — handler code routes through driver interfaces.
6. **LLM-editable files** — source files under 400 lines.
7. **GitHub API fidelity** — bleephub works with unmodified `gh` CLI.
8. **State persistence** — every task ends with a state save (PLAN / STATUS / WHAT_WE_DID / DO_NEXT / BUGS / memory).
9. **No fallbacks, no defers** — every functional gap is a real bug; every bug gets a real fix in the same session it surfaces; cross-cloud sweep on every find.
10. **Sim parity per commit** — any new SDK call added to a backend updates [specs/SIM_PARITY_MATRIX.md](specs/SIM_PARITY_MATRIX.md) and adds the sim handler in the same commit.
11. **Single work-branch rule** — all in-flight work lands on one branch; no side branches that risk abandonment. User handles every merge.

## Closed phases

Headline-only. Per-bug detail in [BUGS.md](BUGS.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md).

| PR | Phases | Headline | Bug range |
|---|---|---|---|
| #112–115 | 86–102 | Sim parity; stateless backends; real volumes; FaaS invocation tracking; reverse-agent exec/cp/diff/commit/pause; Docker pod synthesis; ACA console exec; ECS SSM ops; OCI push; log fidelity. | 661–769 |
| #117 | Round-7 | Live-AWS bug sweep. | 770–785 |
| #118 | Round-8 / 9 | Stateless invariant; real layer mirror; sync `docker stop`; per-network SG isolation; live SSM frame capture → exit-code marker; `sh -c` exec wrap; busybox-compat find/stat; Lambda invoke waiter; tag-based InvocationResult persistence; per-cloud terragrunt sweep. | 786–819 |
| #120 | 104 / 105 / 108 | Driver framework migration (13 typed adapters, every dispatch site routed); cloud-native typed drivers across every backend (44/91 matrix cells cloud-native); `core.ImageRef` typed domain object; libpod-shape golden tests; sim-parity matrix audit (33 AWS + 16 GCP + 28 Azure ✓); real-runner harnesses scaffolded under `tests/runners/{github,gitlab}/`. | 802; 820–844 |
| #121 | 109 | Strict cloud-API fidelity audit (19 audit items): Lambda VpcConfig from real subnet CIDR; AWS Secrets Manager + SSM + KMS + DynamoDB; GCP firewalls + Cloud NAT + IAM tokens + operations persistence; Azure IMDS + Blob ARM + NSG validation + Private DNS records + NAT Gateways + Route Tables + ACA Async-Op polling + Key Vault ARM/data; ARM SystemData preservation. | (audit items) |
| #122 | 110 | Runner integration — 4 AWS cells GREEN (GH×ECS, GH×Lambda, GL×ECS, GL×Lambda). | 845–876 |
| #123 | 118 + 120 + 121 + 122k/m + 123 | FaaS pod overlays (gcf + lambda); 4 GCP runner cells GREEN; cloud-faithful GCP simulator hardening; storage-backing driver abstraction (`emptyDir` / `gcs-sync` / `gcs-fuse`). 8/8 cells GREEN end-state. | 877–971 |
| #125 | CI reorg | Workflows reorganized: zero auto-fire on main; live-tests-{cloud}. | n/a |

## Active work

Living on `phase-130` (PR #127). Single-PR rule per phase, but multiple closely-related phases stack here per the single-work-branch rule.

### Phase 129 #4 — orphan pod-Service GC (✅ shipped 2026-05-08)

Owner-link via `CLOUD_RUN_JOB`. Sockerless reads the Cloud-Run-injected env var (no `SOCKERLESS_*` injection by dispatcher), stamps `sockerless_owner_runner_task` on each `sockerless-svc-*` Service, dispatcher's existing 2-min Cleanup deletes any whose owner Cloud Run Job is gone or terminal. Code: `gcp-common/owner_label.go` · `cloudrun/servicespec.go` · `cloudrun-functions/pod_service.go` · `github-runner-dispatcher-gcp/internal/spawner/spawner.go` · `cmd/.../main.go::Cleanup`. Spec: `specs/CLOUD_RESOURCE_MAPPING.md § Orphan pod-Service GC (owner-link pattern)`. Live verification deferred to next live-cloud session.

### Sim parity prep (in progress)

Sims must serve every cloud API the backends use today + the planned driver work (Phases 124–127). Pre-existing parity matrix is at 77/77 ✓ for current backends. Forward-looking gaps:

- [x] **GCP `iamcredentials.generateIdToken`** — `simulators/gcp/iam.go` extended to handle `:generateIdToken` action alongside existing `:generateAccessToken`. Helper `mintSimIdToken` in `oauth2.go`. Phase 126 prep.
- [ ] **GCP Compute Disks CRUD** — `simulators/gcp/compute.go`. Zonal disks (Insert/Get/List/Delete/Resize/SetLabels) + aggregated list. Phase 127 GCP `pd-ephemeral` prep.
- [ ] **SDK + CLI tests** for both, under `simulators/gcp/{sdk-tests,cli-tests}`.
- [ ] **`specs/SIM_PARITY_MATRIX.md`** — add new rows.

### Phase 130 — bleephub workflow-runs / jobs / runners REST (queued)

Goal: unmodified `gh` CLI + the existing GitHub-runner-dispatcher work against bleephub end-to-end (so the 8/8 cells could run against bleephub instead of real GitHub for hermetic test coverage).

New file `bleephub/gh_actions_rest.go` registering:

- `GET /api/v3/repos/{o}/{r}/actions/runs` (with `?status=`, `?branch=`, `?event=`)
- `GET /api/v3/repos/{o}/{r}/actions/runs/{run_id}`
- `GET /api/v3/repos/{o}/{r}/actions/runs/{run_id}/jobs`
- `GET /api/v3/repos/{o}/{r}/actions/jobs/{job_id}` + `/logs`
- `POST /api/v3/repos/{o}/{r}/actions/runs/{run_id}/cancel` + `/rerun`
- `DELETE /api/v3/repos/{o}/{r}/actions/runs/{run_id}`
- `GET /api/v3/repos/{o}/{r}/actions/runners` + `DELETE .../runners/{id}`

JSON converters bridge `Workflow`/`WorkflowJob`/`Agent` → GitHub-shape JSON. Tests in `bleephub/gh_actions_test.go`.

### Phase 131 — bleephub workflows REST + UI dispatch (queued)

User chose "more complete": auto-parse `.github/workflows/*.yml` from a repo-on-disk; the bleephub UI gains workflow-dispatch.

- `GET /api/v3/repos/{o}/{r}/actions/workflows` (list YAML files from repo-on-disk)
- `GET /api/v3/repos/{o}/{r}/actions/workflows/{id}` + `/runs`
- `POST /api/v3/repos/{o}/{r}/actions/workflows/{id}/dispatches` (with `inputs`, `ref`)
- UI: refactor `WorkflowsPage` into Workflows + Runs tabs; dispatch form.

### Phase 132 — bleephub apps + oauth completeness (queued)

- `GET /user/installations`, `GET /user/installations/{id}/repositories`, `DELETE /installation/token`.
- `GET /login/oauth/authorize` (web flow companion to existing device flow).
- UI: Apps Manager + OAuth Debug pages. Admin UI gains bleephub admin sub-pages.

## Driver-generalization roadmap (Phases 124–127, queued)

Storage backing was the pilot (Phase 123): cloud-agnostic core interface, per-cloud impls, operator-pluggable selection, no-fallbacks at registry resolve. Same template for the next three driver dimensions.

Each phase template:

1. `api/<dim>_driver.go` — enum + struct fields on the relevant config.
2. `backends/core/<dim>_driver.go` — driver interface + registry + no-op default.
3. `backends/<cloud>-common/<dim>_<impl>.go` — per-cloud impls.
4. `backends/<cloud-product>/<dim>_translator.go` — per-backend translator to that cloud's protobuf.
5. Operator config: TOML / env var that selects the driver per backend.
6. **No-fallbacks at resolve** — unset / unknown driver name returns an error.
7. Migration of existing inline calls to the registry.

Each phase starts with a `specs/CLOUD_RESOURCE_MAPPING.md` design pass cataloging current ad-hoc paths per backend before any code lands.

### Phase 124 — Network driver

How containers in the same user-defined network discover and talk. Today: hardcoded per backend (Cloud Map for ECS; `/etc/hosts` injection via `SOCKERLESS_HOST_ALIASES` for cloudrun/gcf; multi-container revision loopback for pod-Services). Driver categories: `host-aliases`, `cloud-dns`, `service-mesh`, `nat-gateway-only`.

### Phase 125 — DNS driver

How `<container-name>.<network>` resolves. Today: per-cloud heuristics. Driver categories: `cloud-map`, `cloud-dns-zone`, `service-discovery`, `private-dns-zone`. Cloud APIs already covered by sims.

### Phase 126 — Access driver

Container-to-container auth, ingress IAM, service-account binding. Today: scattered. Driver categories: `iam-role`, `id-token`, `mTLS`, `none-internal`. Sim prereq: `generateIdToken` (✅ shipping in current branch).

### Phase 127 — Storage driver expansion (nice-to-have)

Open up the `BackingSpec` union (currently EmptyDir + GCS) cloud-agnostic. Drivers: `pd-ephemeral` (GCP), `efs-ephemeral` (AWS — already covered), `azure-files-ephemeral` (Azure). Sim prereq: GCP Compute Disks (in progress in current branch).

## Live-cloud / cost-control roadmap (queued)

### Phase 128 — Runner job timeout (configurable)

Hard cap on Cloud Run Job / Lambda / ECS task duration so a hung subprocess can't pin quota indefinitely. **Default 1 h.** Operator override via dispatcher TOML `runner_job_timeout` + bootstrap env `SOCKERLESS_JOB_TIMEOUT_SECONDS`. Per-cloud max: Cloud Run Jobs 24 h; Lambda 15 min; ECS Fargate ~unlimited. Behaviour at timeout: SIGTERM → 30 s grace → SIGKILL; bootstrap reports exit code 124 (matches GNU `timeout(1)`). Test: `sleep 9999` step → expect 1 h timeout → arithmetic-suite resumes on next job.

### Phase 129 — Cost tracking + stale-resource cost-cap

Phase 129 #4 (orphan-svc owner-link sweep) shipped 2026-05-08. Remainder:

1. **BigQuery billing export** — enable on the live billing account at fresh-project creation. Free at our volume.
2. **Per-session resource labels** (`sockerless_session=<run-id>`) on every Cloud Run Service / Job / AR repo / GCS bucket / VPC connector sockerless creates.
3. **Per-session budget alert** via Cloud Billing Budget API ($5 alert / $20 hard cap, label-scoped).
4. **Stale-resource sweeper** — owner-link Service GC ✅ shipped; remaining: Cloud Run Jobs older than 1 h not RUNNING; GCS `workspace/` prune via existing `PruneStaleObjects`.
5. **Session-end teardown** — `make teardown-live-gcp` calls `gcloud projects delete <project>`. GCP soft-delete with 30-day undo is the safety net. Procedure documented in `docs/GCP_LIVE_TEARDOWN.md`.

**Action: Phase 128 + remaining Phase 129 must ship before the next live-cloud session brings up a fresh project** — without those the same regional-CPU-quota debt cycle from 2026-05-07 repeats.

## Audit / fidelity tracks (in flight, rolling)

### Phase 105 — Libpod-shape conformance (rolling — waves 1-3 landed)

Golden-file tests pinning bleephub responses to libpod's exact JSON shape. Continues as new endpoints land.

### Phase 106 — Real GitHub Actions runner integration (in flight)

Harness scaffold under `tests/runners/github/`. Live cells (1, 2, 5, 6) GREEN; runner-side fidelity work continues.

### Phase 107 — Real GitLab runner integration (in flight)

Harness scaffold under `tests/runners/gitlab/`. Live cells (3, 4, 7, 8) GREEN; runner-side fidelity work continues.

### Phase 121b — Azure simulator hardening (queued)

Azure-side mirror of Phase 121 (cloud-faithful sim hardening for ACA + AZF). Open question: how much of the GCP-style cloud-faithful work transfers (proto-JSON enum decoding, real OAuth2 token endpoints, label-filter syntax) to Azure idioms.

## Other queued

### Phase 68 — Multi-tenant backend pools

Named pools of backends with scheduling and resource limits. P68-001 done; 9 sub-tasks remain (registry, router, limiter, lifecycle, metrics, scheduling, limits, tests, state save). Fold in Phase 106's label-based-dispatch as the headline use case.

### Phase 78 — UI polish

Dark mode, design tokens, error handling UX, container detail modal, auto-refresh, performance audit, accessibility, E2E smoke, documentation.

## Future ideas

- GraphQL subscriptions for real-time event streaming.
- Full GitHub App permission scoping.
- Webhook delivery UI.
- Cost controls (per-pool spending limits, auto-shutdown).
- Sockerless GCE-style backend (would unlock Phase 127 GCP `pd-ephemeral` for real workloads).

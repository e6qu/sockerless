# Sockerless — Roadmap

> **Goal:** Replace Docker Engine with Sockerless for any Docker API client — `docker run`, `docker compose`, TestContainers, CI runners — backed by real cloud infrastructure (AWS, GCP, Azure).

State [STATUS.md](STATUS.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/](specs/).

## Guiding principles

1. **Docker API fidelity** — match Docker's REST API exactly.
2. **Real execution** — sims and backends actually run commands; no stubs, fakes, or mocks.
3. **External validation** — proven by unmodified external test suites.
4. **No new frontend abstractions** — Docker REST API is the only interface.
5. **Driver-first handlers** — handler code routes through driver interfaces.
6. **LLM-editable files** — source files under 400 lines.
7. **GitHub API fidelity** — bleephub works with unmodified `gh` CLI.
8. **State persistence** — every task ends with a state save (PLAN / STATUS / WHAT_WE_DID / DO_NEXT / BUGS / memory).
9. **No fallbacks, no defers** — every functional gap is a real bug; every bug gets a real fix in the same session it surfaces; cross-cloud sweep on every find.
10. **Sim parity per commit** — any new SDK call added to a backend updates [specs/SIM_PARITY_MATRIX.md](specs/SIM_PARITY_MATRIX.md) and adds the sim handler in the same commit.
11. **Single work-branch rule** — all in-flight work lands on one branch; no side branches that risk abandonment. User handles every merge.

## Closed phases (PR index)

Headline-only. Per-bug detail in [BUGS.md](BUGS.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md).

| PR | Phases | Headline | Bug range |
|---|---|---|---|
| #112–115 | 86–102 | Sim parity; stateless backends; real volumes; FaaS invocation tracking; reverse-agent exec/cp/diff/commit/pause; Docker pod synthesis; ACA console exec; ECS SSM ops; OCI push; log fidelity. | 661–769 |
| #117 | Round-7 | Live-AWS bug sweep. | 770–785 |
| #118 | Round-8 / 9 | Stateless invariant; real layer mirror; sync `docker stop`; per-network SG isolation; live SSM frame capture → exit-code marker; `sh -c` exec wrap; busybox-compat find/stat; Lambda invoke waiter; tag-based InvocationResult persistence; per-cloud terragrunt sweep. | 786–819 |
| #120 | 104 / 105 / 108 | Driver framework migration (13 typed adapters); cloud-native typed drivers (44/91 cells); `core.ImageRef` typed domain object; libpod-shape golden tests; sim-parity matrix audit; real-runner harnesses. | 802; 820–844 |
| #121 | 109 | Strict cloud-API fidelity audit (19 items): Lambda VpcConfig from real subnet CIDR; AWS Secrets Manager + SSM + KMS + DynamoDB; GCP firewalls + Cloud NAT + IAM tokens + operations persistence; Azure IMDS + Blob ARM + NSG + Private DNS records + NAT Gateways + Route Tables + ACA Async-Op + Key Vault; ARM SystemData. | (audit) |
| #122 | 110 | Runner integration — 4 AWS cells GREEN. | 845–876 |
| #123 | 118 + 120–123 | FaaS pod overlays (gcf + lambda); 4 GCP runner cells GREEN; cloud-faithful GCP sim; storage-backing driver (`emptyDir` / `gcs-sync` / `gcs-fuse`). **8/8 GREEN end-state.** | 877–971 |
| #125 | CI reorg | Workflows reorganized: zero auto-fire on main; live-tests-{cloud}. | n/a |
| #127 | 129#4 + 130–132 | Orphan pod-Service GC (owner-link via `CLOUD_RUN_JOB`); sim parity prep (GCP `generateIdToken` + Compute Disks); bleephub workflow runs / workflows / apps + oauth REST + UI dispatch + AppsPage + OAuthPage. | n/a |
| #128 | 134 | Makefile standardization + per-app leaf Makefiles + stack orchestration; 17 doc updates; sim test stability (BUG-973/974). | 973–974 |
| #129 | 135 | **Sim host model + 3-tier coverage.** Workloads dispatch through Docker honouring explicit `Architecture` (sim's `linux/arm64` capacity); per-cloud-product host-metadata services (AWS IMDSv2 + ECS task v4 + instance-identity-document; GCP `metadata.google.internal/computeMetadata/v1`; Azure IMDS `/metadata/instance` + identity); static no-`os/exec`-of-workload check; SDK metadata tests (cloud.google.com/go/compute/metadata × 6, aws-sdk-go-v2/feature/ec2/imds × 4, azidentity ManagedIdentityCredential × 1); GCP CLI test for Compute Disks via gcloud; GCP Terraform test (`google_compute_disk`); native `ubuntu-24.04-arm` CI runners (no QEMU). | 949, 972, 975–984 |
| #130 | 128 | Runner job timeout. Bootstrap timer (`runWithTimeout` in cloudrun + gcf bootstraps; SIGTERM → 30s grace → SIGKILL → exit 124) + cloud-native cap (cloudrun TaskTemplate.Timeout, ACA ReplicaTimeout, Lambda 900s) derived from `core.JobTimeoutDefault()`. `SOCKERLESS_JOB_TIMEOUT_SECONDS` contract; per-job override via `docker run -e` wins. | n/a |

## Roadmap (ordered)

Pick from the top. Each phase's `Pick from the top` rule: don't start the next until the previous closes (or the user explicitly redirects).

### 1. Phase 128 — Runner job timeout (live-cloud cost gate)

Hard cap on Cloud Run Job / Lambda / ECS task duration so a hung subprocess can't pin quota indefinitely. Default 1 h. Operator override via dispatcher TOML `runner_job_timeout` + bootstrap env `SOCKERLESS_JOB_TIMEOUT_SECONDS`. Per-cloud max: Cloud Run 24 h; Lambda 15 min; ECS Fargate ~unlimited. At timeout: SIGTERM → 30 s grace → SIGKILL; bootstrap reports exit 124 (matches GNU `timeout(1)`). Test: `sleep 9999` step → 1 h timeout → arithmetic-suite resumes on next job. **Blocks the next live-cloud session**: without it, a hung subprocess pins quota indefinitely (the failure mode that drove the 2026-05-07 ~$90 burn).

### 2. Phase 124 — Network driver

How containers in the same user-defined network discover and talk. Driver categories: `host-aliases` / `cloud-dns` / `service-mesh` / `nat-gateway-only`. Sim prereq: already covered.

### 3. Phase 125 — DNS driver

How `<container-name>.<network>` resolves. Driver categories: `cloud-map` / `cloud-dns-zone` / `service-discovery` / `private-dns-zone`. Sim prereq: already covered. Depends on 124's network primitives.

### 4. Phase 126 — Access driver

Container-to-container auth, ingress IAM, service-account binding. Driver categories: `iam-role` / `id-token` / `mTLS` / `none-internal`. Sim prereq: ✅ `generateIdToken` (PR #127).

### 5. Phase 127 — Storage driver expansion

Open up the `BackingSpec` union (currently EmptyDir + GCS) cloud-agnostic. Drivers: `pd-ephemeral` GCP / `efs-ephemeral` AWS (already covered) / `azure-files-ephemeral`. Sim prereq: ✅ Compute Disks (PR #127).

### 6. Phase 121b — Azure sim hardening

Azure-side mirror of Phase 121 (cloud-faithful sim hardening for ACA + AZF). Open question: how much of the GCP-style work (proto-JSON enum decoding, real OAuth2 token endpoints, label-filter syntax) transfers to Azure idioms.

### 7. Phase 78 — UI polish

Dark mode, design tokens, error handling UX, container detail modal, auto-refresh, performance audit, accessibility, E2E smoke, documentation.

## Driver phase template (124–127)

Storage backing (Phase 123) is the worked pilot: cloud-agnostic core interface, per-cloud impls, operator-pluggable selection, no-fallbacks at registry resolve. Each driver phase follows the same 7-step template:

1. `api/<dim>_driver.go` — enum + struct fields on the relevant config.
2. `backends/core/<dim>_driver.go` — driver interface + registry + no-op default.
3. `backends/<cloud>-common/<dim>_<impl>.go` — per-cloud impls.
4. `backends/<cloud-product>/<dim>_translator.go` — per-backend translator to that cloud's protobuf.
5. Operator config: TOML / env var that selects the driver per backend.
6. **No-fallbacks at resolve** — unset / unknown driver name returns an error.
7. Migration of existing inline calls to the registry.

Each phase starts with a `specs/CLOUD_RESOURCE_MAPPING.md` design pass cataloging current ad-hoc paths per backend before any code lands.

## Audit / fidelity tracks (rolling)

### Phase 105 — Libpod-shape conformance

Golden-file tests pinning bleephub responses to libpod's exact JSON shape. Continues as new endpoints land.

### Phase 106 — Real GitHub Actions runner integration

Harness scaffold under `tests/runners/github/`. Live cells (1, 2, 5, 6) GREEN; runner-side fidelity work continues.

### Phase 107 — Real GitLab runner integration

Harness scaffold under `tests/runners/gitlab/`. Live cells (3, 4, 7, 8) GREEN; runner-side fidelity work continues.

## Future ideas

- GraphQL subscriptions for real-time event streaming.
- Full GitHub App permission scoping.
- Webhook delivery UI.
- Sockerless GCE-style backend (would unlock Phase 127 GCP `pd-ephemeral` for real workloads).

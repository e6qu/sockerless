# Sockerless — Roadmap

> **Goal:** Replace Docker Engine with Sockerless for any Docker API client — `docker run`, `docker compose`, TestContainers, CI runners — backed by real cloud infrastructure (AWS, GCP, Azure).

State [STATUS.md](STATUS.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/](specs/) · vibe catalogue [docs/VIBE_CODING.md](docs/VIBE_CODING.md).

## Guiding principles

1. **Docker API fidelity** — match Docker's REST API exactly.
2. **GitHub API fidelity (bleephub)** — match GitHub's REST + GraphQL paths and shapes exactly, modulo base domain. Real `gh` CLI must work directly against bleephub.
3. **Real execution** — sims and backends actually run commands; no stubs, fakes, or mocks.
4. **External validation** — proven by unmodified external test suites (`gh` binary, `actions/runner`, real Docker SDKs, Terraform providers).
5. **Driver-first handlers** — handler code routes through driver interfaces.
6. **LLM-editable files** — source files under 400 lines.
7. **State persistence** — every task ends with a state save (STATUS / DO_NEXT / WHAT_WE_DID / MEMORY / `_tasks/done/`).
8. **No fallbacks, no skips, no defers, no fakes** — every functional gap is a real bug; every bug gets a real fix in the same session it surfaces. We are not in legacy maintenance — no shims for old behaviour. If real GitHub does X, bleephub does X.
9. **Sim parity per commit** — any new SDK call adds a sim handler + matrix row in the same commit.
10. **Single work-branch rule** — all in-flight work lands on one branch. User handles every merge.
11. **Cross-cloud is permanently off the table** — cloud-specific drivers extend the generic shape; cross-cloud duplication is fine, in-cloud duplication consolidates into `*-common`.
12. **Components stay decoupled from admin / UI.** Sims, backends, bleephub remain independently configurable, buildable, runnable. Admin reads only what they already expose (`/v1/health`, `/v1/info`, env vars).
13. **Persistence is opt-in + fail-loud.** Operator-requested persistence (`BLEEPHUB_PERSIST=true`, `SIM_PERSIST=true`) that fails to open or write must surface the error, never silently degrade (BUG-985/986).
14. **No phase or bug IDs in code comments.** Keep that metadata in commits / PRs / BUGS.md only; code comments document the *why*, not the lineage.

## Closed phases (PR index)

Headline-only. Per-bug detail in [BUGS.md](BUGS.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md).

| PR | Phases | Headline |
|---|---|---|
| #112–123 | 86–123 | Sim parity; stateless backends; FaaS pod overlays; storage-backing driver pilot; **8/8 runner cells GREEN.** |
| #125 | CI reorg | Workflows reorganized: zero auto-fire on main; live-tests-{cloud}. |
| #128–134 | 124–134 | Driver framework + makefile std + sim host model + arm64 CI runners + job timeout + network/dns/access/storage drivers. |
| #135–136 | 121b | Azure sim hardening, driver consolidation pattern B, network-discovery adapter consolidation, AZF/Lambda DNS, Azure AD access. |
| #137–142 | 78–84 | UI polish + admin orchestration (`sockerless.yaml` topology, `TopologyManager`, lifecycle endpoints, UI Topology page, per-instance logs + console, cloud-resources rollup, sim UI parity, per-instance state isolation + BUG-985/986). |
| #143–144 | 85–86 | Config edit + hot reload; health + supervision surface (exit-code capture, `/diagnostics`, `<UnhealthyDiagnosticPanel>`). |
| #145–146 | 87 + 87b | Observability stack (otel-collector + VictoriaLogs + Jaeger) + component-side OTel SDK wiring. |
| #147–149 | 91 + 91b + 91c | `BackingMemory` translator across 5 backends; Lambda volume_translator framework migration; cloudrun + gcf `BackingPDEphemeral` rejection. |
| #150 | 87c | zerolog → OTel logs bridge across all 12 components. |
| #151 | 87d + 92 | Trace propagation + MeterProvider + runtime metrics; `Backing: gcs-fuse` deregistered on cloudrun + gcf. |
| #152 | docs | `docs/POD_MATERIALIZATION.md` — per-backend pod materialization walked through GH + GitLab runners. |
| #153 | 153 | bleephub ↔ GitHub API parity + SQLite persistence + real `gh` CLI compat. |
| #154 | 154 | Broad GitHub API sweep — reactions, releases, deployments + environments, PR review comments + threads, Checks, Actions OIDC + JWKS, Pages, branch protection. |
| #155–156 | 155–156 | bleephub-specific + project-wide docs refresh; GCP dep bump. |
| #157 | 157 | Component ⇄ reference-adaptor docs sweep started (`backends/docker` only). |
| #158 | 158 | BUG-991 + BUG-992 fixes; `docs/VIBE_CODING.md` 23-pattern catalogue; `docs/GOLANG_STRONG_TYPING.md`; 3 project-local Claude skills. |
| #159 | 159 | AWS sim — CloudFront + ACM + Route 53 + WAFv2 + Amplify + IAM SLR/OIDC (11 sub-tasks, `TestStackProductionShape` cross-resource invariants). Merged 2026-05-15 at `236a387f`. |
| #160 | 160 | Two new project-local skills (`sim-handler-checklist`, `cross-resource-stack-test`) + `adaptor-fidelity-check` refinement; component-README adaptor-led sweep completed across 6 backends + 2 simulators + bleephub + `cmd/sockerless` + new `cmd/sockerless-admin/README.md` + rewritten `simulators/README.md`. Phase 157 Track A closed. Merged 2026-05-16 at `aeb0ac6e`. |
| #161 | 161 | Comprehensive vibe-slop sweep — 18 BUGs closed + bleephub GraphQL completion. Merged 2026-05-16 at `841f2456`. |
| #162 | 162 | Vibe-coding catalogue refresh — 12 new patterns (24–35) + `avoid-vibe-slop` skill expanded 17 → 26 checklist items. Doc-only. Merged 2026-05-16 at `4f602988`. |
| #163 | 163 | Makefile legacy alias rip-out + docs sweep. Merged 2026-05-16 at `d5b9d22a`. |
| #164 | 164 | Second vibe-slop sweep + terraform-provider test expansion (19 BUGs: 1014–1032). GCP terraform-tests 4 → 11 resources; Azure terraform-tests 1 → 5. Merged 2026-05-17 at `616dcd98`. |
| #165 | 165 | Third vibe-slop sweep (4 BUGs: 1033–1036) + sim test-pyramid expansion (2 BUGs: 1038/1039 + a GCS object selfLink sub-defect) + codex CLI review (2 BUGs: 1043/1044) + continuity-doc compression (~1700 → ~870 lines). 3 Open BUGs staged forward to Phase 166. Merged 2026-05-17 at `288b76d3`. |
| #167 | 166 | Real fixes for the 3 Phase-165 follow-up Open BUGs (1040 Azure azurerm + 1041 GCP IAM SA via correct `iam_beta_custom_endpoint` setting + 1042 AWS 5 sim handler gaps closed real) + codex review finding state-persistence gaps (BUG-1045). 1044 fixed / 0 open at merge. Merged 2026-05-17 at `49050c2d`. |
| #168 | 167–168 | FaaS exec unification on mandatory reverse-agent WebSocket, Path B invoke dispatch removed, AZF bootstrap hardening, cleanup/lifetime semantics tightened. Merged 2026-05-18 at `3565e413`. |
| #169 | 168 follow-up | Runner attach hardening and final CI stabilization. Merged 2026-05-18 at `0bd75902`. |
| #170 | 168 follow-up | FaaS runner smokes for Lambda/Cloud Run/GCF/ACA/AZF, Make/CI wiring, AZF bootstrap coverage, GCP Artifact Registry endpoint-fidelity fix covered by SDK/gcloud/Terraform/OCI, and live-validation runbook. Merged 2026-05-18 at `a5639811`. |
| #172 | pod-model follow-up | Simulator pod materialization fidelity — multi-container ECS / Cloud Run / ACA jobs+apps materialize all containers with shared-localhost contract. BUG-1096 + 1097 closed. Merged 2026-05-23 at `1c1fd92`. |
| #179 | 173 | Simulator wire-fidelity sweep across all 3 clouds. ~180 new sim ops + 6 new project-local skills + sentinel-header logging in shared middleware. Closed issues #173–#178 + BUG-1098..1104. Merged 2026-05-24 at `64a13a8`. |
| #180 | 174 | Skill-sweep audit + community-issue triage. 3 rounds; BUG-1105..1119 closed; 8 GitHub issues #181–#188 closed. Azure File/Queue/Table data planes added; streaming-envelope helper wired into 9 upload handlers. Merged 2026-05-24 at `7a5d588`. |
| #192 | 175 | Second skill-sweep audit + 3 community-filed issues (#189/#190/#191). 14 BUGs closed (1120–1133); persistence-strip wrapper records; CI test-job split 1→3 (22 jobs); new `timeless-comments` skill. Merged 2026-05-24 at `ca11405`. |
| #200 | 176 | 8 community-filed issues (#190 reopened + #193..#199) + 12 in-PR audit findings + path-style storage dispatcher + `make hooks`. 8 BUGs closed (1134–1141). Merged 2026-05-24 at `2d8e604`. |
| #202 | 177 | KV WWW-Authenticate URL reopen (#193) + S3 bucket-subresources (#201) + 4 meta-skill improvements (sim-canonical-config-test extension, surface-table-completeness, reopen-postmortem, tf-tests parity). 6 BUGs closed (1142–1147). Merged 2026-05-24 at `aa847b1`. |
| #211 | 178 | 9 community-filed issues (#196 reopen + #203..#210) + 5 class-of-bug remediations (proactive surface-table seed (46 tables), mux-overlap scanner, paged-iterator rule, state-machine skill, CI-gated tf-tests matrix). 25 BUGs closed (1148–1173) including 12 real handler/shape gaps the new tf-azure gate surfaced. Merged 2026-05-25 at `7a7c9f0`. |
| #216 | 179 | 2 reopens (#209/#210) + 3 new issues (#213/#214/#215). BUG-1174..1180 closed: GCP Redis upgrade/failover keying, Pub/Sub IAM verbs, Azure real-shape listKeys, Resources Tags API, Service Bus authorizationRules, AWS IAM policy/instance-profile lifecycle, API Gateway method/integration responses. Merged 2026-05-25 at `9620a53`. |
| #217 | 179 follow-up | README badge refresh from Phase 179 plus hook portability fixes (`lint-changed.sh`, `check-cloud-backend-isolation.sh`). Merged 2026-05-25 at `f0da588`. |
| #219 | issue #218 | GCP Secret Manager lifecycle endpoints: ListSecretVersions, UpdateSecret labels, DeleteSecret, replication metadata, payload CRC32C; covered by SDK, gcloud CLI, and Terraform tests. Merged 2026-05-25 at `06ee3a5`. |
| issue #264 | simulator test-contract matrix | `specs/SIM_TEST_COVERAGE_MATRIX.md` now tracks SDK/CLI/Terraform coverage for every canonical simulator surface table, enforced by pre-commit and CI. AWS DynamoDB/SQS/SNS gained direct CLI coverage; GCP Terraform gained Cloud Functions v2, Cloud Build trigger, Pub/Sub, and Cloud Logging sink/metric coverage with the simulator routes needed by the provider. |
| #221 | issue #220 | Azure Blob `GET /?comp=list` now emits per-container `<Properties>` with `Last-Modified` and quoted `Etag`; container ETags persist from create through get/list. Raw wire + real Azure CLI regressions added. |

## Active phase

**Idle on `main`.** No phase in flight after simulator test-contract matrix backfill for issue #264.

BUGS.md: **1218 filed · 1214 fixed · 6 open · 2 false positives.** Open work is BUG-1075, BUG-1104, BUG-1203, BUG-1204, BUG-1206, and BUG-1207.

**Live-cloud (Track A / BUG-1075) remains deprioritized** per 2026-05-23 user directive — revisit after operator decides; no near-term phase queued.

## Future phases

### Foundational simulator parity

Continue the audit-derived missing-service work in issue order unless a new community issue changes priority: VM/instance lifecycle APIs, managed load balancers, NAT/public-IP parity, and surface-table cleanup. Every added public API slice ships with SDK, CLI, and Terraform-provider coverage in the same PR.

### Track A — Live-cloud validation (deprioritized; one branch per cell)

**Deprioritized 2026-05-23** per user directive: simulator-fidelity bugs and missing-service coverage (Phases 173–181) come first. Track A resumes after the simulator phases land.

Lambda live · Cloud Run Services + ACA Apps live · AZF cloud-dns live · Lambda service-mesh live · ACA/AZF Azure AD live. Teardown self-sufficient per `feedback_teardown_aggressive.md`.

### Track B — Skill maturation (post-Phase 158)

Candidate additional skills as new patterns surface: `state-save`, `spec-first-implementation`, `cross-cloud-sweep`.

### Track C — Phase 91d (bookmarked indefinitely)

Real `pd-ephemeral` on cloudrun + gcf. Cloud Run's `runpb.Volume` lacks a PD field. Don't reopen until cloud capability changes.

## Driver phase template

Storage backing (Phase 127) is the pilot. Each driver phase follows:

1. `api/<dim>_driver.go` — enum + struct fields on the relevant config.
2. `backends/core/<dim>_driver.go` — driver interface + registry + no-op default.
3. `backends/<cloud>-common/<dim>_<impl>.go` — per-cloud impl (pattern B: shared by both backends in that cloud).
4. `backends/<cloud-product>/server.go` — wires the per-cloud driver into the backend's registry at startup.
5. Operator config: env var selects the driver per backend.
6. **No-fallbacks at resolve** — unset / unknown driver name returns an error.
7. Migration of existing inline calls to the registry.

Each phase starts with a `specs/CLOUD_RESOURCE_MAPPING.md` design pass.

## Future ideas

- GraphQL subscriptions for real-time event streaming.
- Sockerless GCE-style backend (would unlock Phase 91d real `pd-ephemeral` for real workloads).
- Marketplace / billing on bleephub — out of scope until a real consumer asks.

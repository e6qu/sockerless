# Sockerless — Roadmap

> **Goal:** Replace Docker Engine with Sockerless for any Docker API client — `docker run`, `docker compose`, TestContainers, CI runners — backed by real cloud infrastructure (AWS, GCP, Azure).

State [STATUS.md](STATUS.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/](specs/).

## Guiding principles

1. **Docker API fidelity** — match Docker's REST API exactly.
2. **GitHub API fidelity (bleephub)** — match GitHub's REST + GraphQL paths and shapes exactly, modulo base domain.
3. **Real execution** — sims and backends actually run commands; no stubs, fakes, or mocks.
4. **External validation** — proven by unmodified external test suites.
5. **Driver-first handlers** — handler code routes through driver interfaces.
6. **LLM-editable files** — source files under 400 lines.
7. **State persistence** — every task ends with a state save (STATUS.md / DO_NEXT.md / WHAT_WE_DID.md / MEMORY.md / `_tasks/done/`).
8. **No fallbacks, no skips, no defers, no fakes** — every functional gap is a real bug; every bug gets a real fix in the same session it surfaces; cross-cloud sweep on every find.
9. **Sim parity per commit** — any new SDK call adds a sim handler + matrix row in the same commit.
10. **Single work-branch rule** — all in-flight work lands on one branch. User handles every merge.
11. **Cross-cloud is permanently off the table** — cloud-specific drivers extend the generic shape; cross-cloud duplication is fine, in-cloud duplication consolidates into `*-common`.
12. **Components stay decoupled from admin / UI.** Sims, backends, bleephub remain independently configurable, buildable, runnable. Admin reads only what they already expose (`/v1/health`, `/v1/info`, env vars). No admin-required env vars on components, no startup registration, no "I'm being managed" hooks.

## Closed phases (PR index)

Headline-only. Per-bug detail in [BUGS.md](BUGS.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md).

| PR | Phases | Headline |
|---|---|---|
| #112–123 | 86–123 | Sim parity; stateless backends; FaaS pod overlays; storage-backing driver pilot; **8/8 runner cells GREEN.** |
| #125 | CI reorg | Workflows reorganized: zero auto-fire on main; live-tests-{cloud}. |
| #128 | 134 | Makefile standardization + per-app leaf Makefiles + stack orchestration. |
| #129 | 135 | Sim host model + 3-tier coverage + native arm64 CI runners. |
| #130 | 128 | Runner job timeout (bootstrap timer + cloud-native cap). |
| #131 | 124 | Network discovery driver (host-aliases / cloud-dns / service-mesh / nat-gateway-only). |
| #132 | 125 | DNS driver (cloud-map / cloud-dns-zone / private-dns-zone / service-discovery / none). |
| #133 | 126 | Access driver (iam-role / id-token / mTLS / none-internal). |
| #134 | 127 | Storage driver expansion (pd-ephemeral / efs-ephemeral / azure-files-ephemeral). |
| #135–136 | 121b | Azure sim hardening, driver consolidation pattern B, network-discovery adapter consolidation, AZF/Lambda DNS, Azure AD access. |
| #137–142 | 78–84 | UI polish + admin orchestration (`sockerless.yaml` topology, `TopologyManager`, lifecycle endpoints, UI Topology page, per-instance logs + console, cloud-resources rollup, sim UI parity, per-instance state isolation + BUG-985/986). |
| #143–144 | 85–86 | Config edit + hot reload; health + supervision surface (exit-code capture, `/diagnostics`, `<UnhealthyDiagnosticPanel>`). |
| #145–146 | 87 + 87b | Observability stack (otel-collector + VictoriaLogs + Jaeger) + component-side OTel SDK wiring. |
| #147–149 | 91 + 91b + 91c | `BackingMemory` translator across 5 backends; Lambda volume_translator framework migration; cloudrun + gcf `BackingPDEphemeral` rejection. |
| #150 | 87c | zerolog → OTel logs bridge across all 12 components. |
| #151 | 87d + 92 | Trace propagation + MeterProvider + runtime metrics + `make stack-observability-validate`; `Backing: gcs-fuse` deregistered on cloudrun + gcf (closes BUG-944, ships BUG-987). |
| #152 | docs | `docs/POD_MATERIALIZATION.md` — per-backend pod materialization walked through GH + GitLab runners. |

## Active + planned phases

Each entry: scope, why, acceptance. Pick from [DO_NEXT.md](DO_NEXT.md).

### Phase 153 — bleephub ↔ GitHub API signature parity (planned)

Every bleephub HTTP endpoint matches real GitHub's path + request shape + response shape exactly, modulo base domain. Spec: [specs/BLEEPHUB_GITHUB_API_PARITY.md](specs/BLEEPHUB_GITHUB_API_PARITY.md).

Seven gap buckets surfaced by the 2026-05-12 audit:

1. **Missing endpoints** — `GET /apps/{slug}`, `/orgs/{org}/installation`, `/users/{username}/installation`, `PUT|DELETE /app/installations/{id}/suspended`, `GET /installation/repositories`, `PUT|DELETE /user/installations/{id}/repositories/{repo_id}`, hook delivery redelivery, app-level webhook config, OAuth `applications/{client_id}/token` family, Checks API.
2. **Permission enforcement** — `ghs_` / `ghu_` tokens are currently rubber-stamped; gate write endpoints on installation permissions.
3. **Repository selection** — `repository_selection: "selected"` with an allow-list (today: hard-coded `"all"`).
4. **Webhook payload + headers** — `installation: {id}` on every event; `X-GitHub-Hook-ID`, `X-GitHub-Hook-Installation-Target-Type`, `X-GitHub-Hook-Installation-Target-ID`, `X-Hub-Signature` (SHA1).
5. **App-targeted webhook events** — `installation`, `installation_repositories`, `installation_target`, `github_app_authorization`.
6. **OAuth token prefixes + refresh tokens** — `gho_` (OAuth user-to-server), `ghu_` (App user-to-server), `ghr_` (refresh), `ghs_` (server-to-server, existing).
7. **JSON shape** — `*_url` HATEOAS fields, `installations_count`, `suspended_at`, `single_file_name`, etc.

Acceptance: probot reference test suite (or octokit-app) round-trips against `http://localhost:5555` modulo base URL. `go test ./...` green in `bleephub/`. UI surfaces installation CRUD + suspend + repo selection + PEM viewer + token mgmt.

### Live-cloud validation track

Per-backend live-cloud sweeps separate from unit/sim CI. Live-AWS ECS validated 2026-04-20. Outstanding:

- Lambda live (deferred from Phase 86).
- Cloud Run Services / ACA Apps live (closed in code 2026-04-21 behind `UseService` / `UseApp`).
- AZF + cloud-dns on Azure live (new in #136).
- Lambda + service-mesh on AWS live (new in #136).
- ACA / AZF + Azure AD access on Azure live (new in #136).

One branch per cell; teardown self-sufficient per `feedback_teardown_aggressive.md`.

### Phase 91d — Real pd-ephemeral on cloudrun + gcf

**Bookmarked indefinitely.** Cloud Run's `runpb.Volume` lacks a PD field; Admin API doesn't expose PD attach as a first-class primitive. Real implementation requires either a sockerless GCE-style backend or a Cloud Run feature change. Reject-with-pointers shape (Phase 91c, PR #149) stays in place.

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
- Marketplace / billing on bleephub (currently out of scope — most apps don't use them; revisit if a real consumer asks).

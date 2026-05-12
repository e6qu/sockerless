# Sockerless — Roadmap

> **Goal:** Replace Docker Engine with Sockerless for any Docker API client — `docker run`, `docker compose`, TestContainers, CI runners — backed by real cloud infrastructure (AWS, GCP, Azure).

State [STATUS.md](STATUS.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/](specs/).

## Guiding principles

1. **Docker API fidelity** — match Docker's REST API exactly.
2. **GitHub API fidelity (bleephub)** — match GitHub's REST + GraphQL paths and shapes exactly, modulo base domain. Including request-body tolerances: if real GitHub accepts string-coerced booleans (what `gh api -f` sends), bleephub accepts them too. The `gh` CLI must work directly against bleephub — not via URL hackery.
3. **Real execution** — sims and backends actually run commands; no stubs, fakes, or mocks.
4. **External validation** — proven by unmodified external test suites (the `gh` binary, the official `actions/runner`, real Docker SDKs, Terraform providers).
5. **Driver-first handlers** — handler code routes through driver interfaces.
6. **LLM-editable files** — source files under 400 lines.
7. **State persistence** — every task ends with a state save (STATUS.md / DO_NEXT.md / WHAT_WE_DID.md / MEMORY.md / `_tasks/done/`).
8. **No fallbacks, no skips, no defers, no fakes** — every functional gap is a real bug; every bug gets a real fix in the same session it surfaces; cross-cloud sweep on every find. **In particular: we are not in legacy maintenance — no shims for old bleephub behavior.** If real GitHub does X, bleephub does X.
9. **Sim parity per commit** — any new SDK call adds a sim handler + matrix row in the same commit.
10. **Single work-branch rule** — all in-flight work lands on one branch. User handles every merge.
11. **Cross-cloud is permanently off the table** — cloud-specific drivers extend the generic shape; cross-cloud duplication is fine, in-cloud duplication consolidates into `*-common`.
12. **Components stay decoupled from admin / UI.** Sims, backends, bleephub remain independently configurable, buildable, runnable. Admin reads only what they already expose (`/v1/health`, `/v1/info`, env vars). No admin-required env vars on components, no startup registration, no "I'm being managed" hooks.
13. **Persistence is opt-in + fail-loud.** Operator-requested persistence (`BLEEPHUB_PERSIST=true`, `SIM_PERSIST=true`) that fails to open must `log.Fatalf`. Never silently fall back to in-memory (BUG-985/986).

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
| #153 | 153 | bleephub ↔ GitHub API parity + SQLite persistence + real `gh` CLI compat (13 sub-tasks; Docker harness 50/50 PASS). |
| #154 | 154 | Broad GitHub API sweep — reactions, releases, deployments + environments, PR review comments + threads, Checks, Actions OIDC + JWKS, Pages, branch protection. |
| #155 | 155 | bleephub-specific docs refresh — `bleephub/README.md`, `docs/BLEEPHUB_GH_CLI.md`, `specs/BLEEPHUB_GITHUB_API_PARITY.md`, `ARCHITECTURE.md` block. |
| #156 | 156 | Project-wide docs refresh + bleephub Quick start + `gh` CLI `--hostname` clarification + GCP `google.golang.org/api` v0.278.0 → v0.279.0. |

## Active + planned phases

Each entry: scope, why, acceptance. Pick from [DO_NEXT.md](DO_NEXT.md).

### Phase 157 — Component ⇄ reference-adaptor docs sweep (in flight)

Every component in the repo is paired with an external **reference adaptor** (docker CLI / aws CLI / gcloud / az / Terraform providers / gh CLI / browser). The adaptor is simultaneously the component's validation harness (tests drive the real adaptor), utility (how users actually invoke the component), and reference (defines "correct" behaviour).

Acceptance per component:

- One README section per component, leading with the adaptor + minimum version.
- "Validation" line points at the test path that drives the real adaptor + last-green count.
- "Wiring" section is ≤5 lines (env / endpoint / creds).
- "Sample" block contains a real captured output from running the command.
- "Out of scope" subsection enumerates deferred adaptor capabilities.

Headline deliverable: `simulators/README.md` end-to-end showcase — three loop variants (AWS sim ↔ ECS backend, GCP sim ↔ Cloud Run backend, Azure sim ↔ ACA backend) each terminating in `docker run alpine echo hi` round-tripping through a real simulator. ≤15 lines of bash from zero to round-trip per variant.

Component matrix + commit layout in [DO_NEXT.md § Phase 157](DO_NEXT.md). Out of scope: live-cloud cells (separate tracks), code changes (docs only), bleephub (covered by #155 + #156).

### Phase 153–156 — Closed

bleephub ↔ GitHub API parity (153) + broad GitHub API sweep (154) + bleephub docs (155) + project-wide docs (156). Headlines in the PR index above; narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md); per-bug detail in [BUGS.md](BUGS.md). Spec at [specs/BLEEPHUB_GITHUB_API_PARITY.md](specs/BLEEPHUB_GITHUB_API_PARITY.md).

### Phase 158 — BUG-991 fix + vibe-coding catalogue + Claude skills (in flight)

Three pieces on one branch:

1. **BUG-991 fix** — `handleContainerWait`'s non-CloudState branch + `BaseServer.ContainerWait`'s `condition=removed` fallback both replaced. Handler now calls `s.self.ContainerInspect` to verify existence (delegates to upstream on passthrough backends), then `s.self.ContainerWait` for the actual block. Removed silent-success on missing-resource per "no fallback-hiding-bugs."
2. **`docs/VIBE_CODING.md`** — sourced anti-pattern catalogue (23 patterns, ~20 primary sources), each pattern mapped to a sockerless-specific failure mode + policy + bug-ID where applicable. Lives as the project's contract on what "vibe-coding done responsibly" means here.
3. **`.claude/skills/{avoid-vibe-slop,adaptor-fidelity-check,manual-test}/SKILL.md`** — three project-local Claude skills that operationalise the catalogue. Skeptical-of-imports approach: all three written from scratch against this repo, no external skill imports.

Acceptance: `docker run --rm alpine:3.20 echo hi` succeeds against `backends/docker` (verified manually 2026-05-13). `go test ./...` green. New files validated by pre-commit hooks.

### Phase 159 — Passthrough-list-endpoints sweep (planned, post-158)

BUG-992 surfaced during the BUG-991 investigation. The same handler shape as BUG-991 affects every list endpoint that reads `s.Store.X.List()` directly without delegating to `s.self.XList()`:

- `GET /images/json` — `handle_images.go:264 handleImageList`
- `GET /volumes` — same pattern
- `GET /networks` — same pattern
- Possibly more (audit per `MEMORY.md` § cross-cloud sweep)

For passthrough backends (docker) these return `[]` even when the upstream daemon has resources, because the local Store is empty.

Fix shape: handlers enumerate resources via `s.self.X` first (which on passthrough delegates upstream, on cloud reads CloudState), then merge with Store entries if both have content. Cross-cloud sweep on every find.

Acceptance: `docker images`, `docker volume ls`, `docker network ls` return the upstream daemon's actual resources against `backends/docker`. Existing tests green. Cloud backends unaffected (they populate Store via CloudState).

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

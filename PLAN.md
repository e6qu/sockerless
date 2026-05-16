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

## Active phase

### Phase 161 — Comprehensive vibe-slop sweep + fixes (in flight on `phase-161-vibe-slop-sweep`)

Sockerless is a vibe-coded project. The published 23-pattern catalogue at [`docs/VIBE_CODING.md`](docs/VIBE_CODING.md) plus the project-local `avoid-vibe-slop` skill exist precisely so this sweep is an explicit phase, not a perpetual side-quest. Phase 161 runs the checklist across **every layer** (backends, simulators, bleephub, cmd, agent, api), files every concrete violation as a BUG, and lands real fixes in one PR.

Scope locked at 12 BUGs (BUG-994 … BUG-1005). Per-bug detail in [BUGS.md](BUGS.md); fix-shape decisions in [DO_NEXT.md § Phase 161](DO_NEXT.md). Categories:

| BUG | Pattern (VIBE_CODING.md §) | Area | Fix shape |
|---|---|---|---|
| 994 | 8 — phase/bug refs in code comments | repo-wide (~60 occurrences) | Sweep: drop the phase/bug ID; rewrite as a *why* line if the context is load-bearing. |
| 995 | 11/12 — HTTP handler reads `s.Store` directly | `backends/core/handle_extended.go`, `handle_images.go`, `handle_libpod.go` | Delegate to `s.self.<Method>` siblings of BUG-991/992. |
| 996 | 1 — `_ = sim.ReadJSON(...)` swallows | `simulators/{aws,gcp,azure}/*.go` (~18 occurrences) | Either propagate the parse error (mandatory body) or drain via `io.Copy(io.Discard, …)` with a `why` comment (optional body). |
| 997 | 1 + persistence invariant — `_ = st.persist.Put/Delete` | `bleephub/store.go`, `gh_apps_store.go`, `gh_apps_user_tokens.go` | Wrap puts via a `persistPut` helper that `log.Fatalf`s on write failure, matching the open-failure rule. |
| 998 | 1 — `decodeRegistryAuth` returns `("","")` on malformed header | `backends/core/handle_images.go` | Distinguish empty header from malformed header; propagate decode error as `400` to caller. |
| 999 | 8 — `core.TagSet.InstanceID` marked `Deprecated` but heavily used | `backends/core/tags.go` + ~27 callers | Either complete the migration to `Cluster` or remove the misleading deprecation comment. |
| 1000 | 9 + 15 — `handleOAuthToken` returns valid 1-year `alg:none` JWT for any input | `bleephub/auth.go` | Validate `grant_type` + `client_assertion` JWT signature against the App's public key per real GitHub. |
| 1001 | 9 — `alwaysNil` / `emptyList` GraphQL resolvers for ProjectV2 + PR review threads | `bleephub/gh_issues_graphql.go`, `gh_pulls_graphql.go` | Implement real lookups or return GraphQL field-level errors per the spec; never fake data. |
| 1002 | 9 — Azure ACR replications list returns `[]` when registry missing | `simulators/azure/acr.go` | Verify parent registry exists; return `ResourceNotFound` Azure error shape. |
| 1003 | 14 — single-call-site `buildOCIHandler` premature abstraction | `simulators/gcp/artifactregistry.go` | Inline the helper. |
| 1004 | 8 — `bph_`-prefixed seeded admin token (legacy shim) | `bleephub/store.go` `SeedDefaultUser` | Switch seeded admin token to `ghp_` prefix per real GitHub; update fixtures. |
| 1005 | 5 — 3-deep defensive nil chain in workflows fail-fast | `bleephub/workflows.go` | Trace upstream nil source; normalise on parse so the runtime path is single-deref. |

Acceptance:

- All 12 BUGs closed (Open → Resolved history in BUGS.md).
- `go test ./...` green in every touched Go module.
- `bun test` green in every touched UI package (if any touched).
- No new BUGs surface during the sweep that aren't closed in the same PR — unfound vibe slop counts the same as un-found bugs.
- BUGS.md count moves `993 filed / 993 fixed / 0 open` → `1005 filed / 1005 fixed / 0 open`.
- This phase's PR (`#161`) opens against `main`. User merges.

Out of scope:

- TypeScript / UI sweep — deferred to a follow-up phase if the Go sweep surfaces a sibling pattern in the UI.
- Live-cloud validation track (separate cells, separate branches).
- Phase 91d (cloud-primitive blocker).
- Slopsquatted-dependency audit — `check-latest-deps` already covers drift; manual upstream-existence audit would be a separate phase if scope justifies it.

## Future phases

### Track A — Live-cloud validation (one branch per cell)

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

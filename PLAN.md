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

Sockerless is a vibe-coded project. The published 23-pattern catalogue at [`docs/VIBE_CODING.md`](docs/VIBE_CODING.md) plus the project-local `avoid-vibe-slop` skill exist precisely so this sweep is an explicit phase, not a perpetual side-quest. Phase 161 runs the checklist across **every layer** (backends, simulators, bleephub, cmd, agent, api), files every concrete violation as a BUG, and lands real fixes in one PR. User directive at phase open: no legacy support, no fallbacks, no error-swallowing — silent degradation is itself a bug.

Closed in this PR (13 fixes):

| BUG | Pattern | Area | Fix |
|---|---|---|---|
| 1000 | 9 + 15 (auth bypass) | `bleephub/auth.go::handleOAuthToken` | Validate `client_assertion` RS256 JWT against the agent's registered RSA public key per Azure DevOps OAuth2 jwt-bearer flow. OAuth-envelope errors (400 / 401). Tests rewritten to drive a real keypair + signed assertion. |
| 997 | 1 + persistence invariant | `bleephub/{store,store_repos,gh_apps_store,gh_apps_user_tokens,gh_oauth}.go` | Added `Persistence.MustPut` + `MustDelete` (`log.Fatalf` on write failure); swept 18 call-sites. |
| 995 | 11/12 (handler bypasses `s.self`) | `backends/core/handle_extended.go`, `handle_images.go`, `handle_libpod.go`, `handle_containers_query.go` | Delegated `handleSystemDf` / `handleContainerList` / `handleImagePrune` to `s.self.<Method>`; consolidated the richer prune logic into `BaseServer.ImagePrune`; extracted `collectContainers` helper shared by `handleLibpodContainerList` (fixes a latent pending-create-drop bug in the no-CloudState branch). |
| 998 | 1 (silent auth-decode swallow) | `backends/core/handle_images.go` | Deleted dead `decodeRegistryAuth`; tightened the inline `handleImagePush` auth path to return 400 on malformed base64 / JSON. Tests rewritten via httptest. |
| 1001 | 9 (fake-data GraphQL resolvers) | `bleephub/gh_issues_graphql.go`, `gh_pulls_graphql.go` | Replaced `alwaysEmptyString` on NonNull ProjectV2 / PRComment fields with `unreachableFieldErr` that returns a clear GraphQL error if invoked (resolvers were unreachable in practice; making the contract honest). |
| 1002 | 9 (missing parent-exists check) | `simulators/azure/acr.go` | Replications list verifies parent registry exists; returns Azure `ResourceNotFound` envelope. |
| 996 | 1 (sim handler ReadJSON swallow) | `simulators/{aws,gcp,azure}/*.go` (18 sites) | Replaced every `_ = sim.ReadJSON(...)` with error-propagating pattern using cloud-appropriate error envelope (`AWSErrorf` / `AzureErrorf` / `GCPErrorf`). |
| 994 | 8 (phase / BUG refs in code) | repo-wide (~115 occurrences) | Two-pass script across `backends/`, `simulators/`, `bleephub/`, `cmd/`, `api/`, `tests/`, `github-runner-dispatcher-*/`. Stripped phase/BUG references; preserved the *why* context when load-bearing. Caught + fixed one regression where the script lost trailing newlines on 3 lines. |
| 999 | 8 (misleading deprecation) | `backends/core/tags.go::InstanceID` | Audit confirmed InstanceID + Cluster are distinct, both load-bearing. Dropped the misleading deprecation comment; clarified each field's role. |
| 1004 | 8 (legacy shim) | `bleephub/store.go::SeedDefaultUser` | Switched seeded admin token from `bph_` to `ghp_` matching real GitHub; swept all fixture / test / doc / UI references. |
| 1005 | 5 (defensive nil chain) | `bleephub/workflows.go` | Extracted into `JobDef.FailFast()` method handling every nil case (including nil receiver) — runtime path is now single-deref. |
| 1003 | 14 (premature abstraction) | `simulators/gcp/artifactregistry.go::buildOCIHandler` | Inlined the single-call-site helper. |
| 1008 | 8 + dead code | OTel `InitTracer` in 6 modules | Deleted the legacy entry point superseded by `InitObservability`; migrated `otel_test.go` in each module. |

Surfaced + filed as new Open BUGs for Phase 162 (out of scope for #161 — staged so the PR stays reviewable; each is a multi-file rip-out):

- **BUG-1006** — `cmd/sockerless-admin/config.go` + `cmd/sockerless/client.go` silently fall back to "old JSON contexts" when `config.yaml` is missing. Rip out the JSON fallback; require config.yaml or surface an error.
- **BUG-1007** — `cmd/sockerless-admin` legacy migration scaffolding (`DeriveLegacyInstances`, `MigrateLegacyProjects`, `legacyDir`, `ProjectConfig` dual shape). Drop the entire migration plumbing.
- **BUG-1009** — `github-runner-dispatcher-gcp` handles "services without an owner label" as legacy with a future cleanup. Drop the legacy branch; error on encountering them.

Acceptance for #161:

- 13 BUGs closed (994 + 995 + 996 + 997 + 998 + 999 + 1000 + 1001 + 1002 + 1003 + 1004 + 1005 + 1008).
- 3 BUGs filed as Open + scoped for Phase 162 (1006, 1007, 1009).
- 1 candidate finding reclassified as false positive (envOrDefault — documented-default-value semantics).
- `go test ./...` green in every touched Go module.
- BUGS.md count: `1011 filed / 1006 fixed / 4 open (1001 + 1006 + 1007 + 1009) / 2 false positives`.
- This phase's PR opens against `main`. User merges.

Out of scope:

- TypeScript / UI sweep — deferred.
- Live-cloud validation track (separate cells, separate branches).
- Phase 91d (cloud-primitive blocker).
- Slopsquatted-dependency audit — `check-latest-deps` already covers drift.

### Phase 162 — Legacy / fallback rip-out (filed during Phase 161)

Three Open BUGs surfaced during Phase 161 that exceeded #161's reviewable scope: BUG-1006 (CLI + admin JSON-context fallback), BUG-1007 (admin legacy migration scaffolding), BUG-1009 (gh-runner-dispatcher legacy services). Per user direction, all three rip out legacy support entirely (no compat period, no deprecation, no opt-in fallback) — the project is under active development and no real users carry on-disk JSON contexts that need migrating from prod.

BUG-1001 also remains Open (real ProjectV2 / PR-review-thread implementation), but lower priority since the `unreachableFieldErr` from BUG-1001's interim fix makes the contract honest until the surfaces are actually implemented.

Branch + commit layout to be decided when the phase starts. Acceptance: 4 BUGs closed, 0 open, `bleephub/` + `cmd/sockerless-admin` + `cmd/sockerless` + `github-runner-dispatcher-gcp` tests green.

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

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
| #164 | 164 | Second vibe-slop sweep + terraform-provider test expansion (19 BUGs: 1014–1032). GCP terraform-tests 4 → 11 resources; Azure terraform-tests 1 → 5; surfaced + fixed 2 real sim defects (BUG-1029 GCP secret-version state handlers, BUG-1030 port-allocator race in terraform-tests). Merged 2026-05-17 at `616dcd98`. |

## Active phase

### Phase 165 — Third vibe-slop sweep + sim test-pyramid expansion + continuity-doc compression (in flight on `phase-165-vibe-slop-sweep-3-test-pyramid`)

User directive (2026-05-17): re-run vibe-slop on a fresh main; plan test-pyramid expansion against real adaptors (SDK + terraform-provider + CLI) for implemented slices; single PR with sub-phases; verify after every significant chunk; prune obsolete continuity-doc info for cross-compaction durability.

Three layered tracks on one PR:

1. **Vibe-slop sweep #3 (4 BUGs: 1033–1036).** Fresh-eyes pass after Phase 161 (18) + Phase 164 (19). 5 silent `io.Copy(w, rc)` swallows in image-stream + build response paths (1033); dead `fmt.Sprintf` silencer with misleading "used by demuxer" comment (1034); `w.Write` style inconsistency at 3 outlier sites (1035); ~50 test-file docstrings still anchored on Phase / sub-phase metadata — the BUG-994 / 1014 / 1026 sweep stopped at production-code (1036).

2. **Sim test-pyramid expansion (3 P0 BUGs: 1037–1039).** External-validation principle (PLAN.md §1). Audit surfaced terraform-provider gaps: AWS missing 11 load-bearing resources (Lambda, S3, DynamoDB, KMS, SecretsManager, EFS, SSM, EC2); GCP missing 8 (Cloud Functions Gen2 — runner-workload primitive! — IAM, GCS object, Compute, Build, Logging, PubSub); Azure widest — only 5 networking primitives covered, both runner backends (ACA + AZF) entirely terraform-uncovered.

3. **Continuity-doc compression.** STATUS / DO_NEXT / PLAN / WHAT_WE_DID grew to ~1700 lines across 5 files. Prune to actionable-across-compaction shape: keep invariants + active-phase scope + last-3-phase headlines + forward tracks; drop closed-phase sub-task tables + per-BUG narratives (covered by BUGS.md). Target ≤ ~50% current line count.

Sub-task layout (P165.0–P165.10) in [DO_NEXT.md](DO_NEXT.md).

Acceptance:
- 7 BUGs (1033–1039) closed in this PR.
- `go test ./...` green in every touched Go module.
- `TestTerraformApplyDestroy` green for all three cloud terraform-tests modules after expansion.
- Continuity docs ≤ ~50% current line count.
- 11 standard CI checks green per push.
- User merges PR #165.

Out of scope (carry forward):
- TypeScript / UI vibe-slop (Phase 161 backlog).
- Live-cloud validation track.
- P1 terraform-test deepening (CloudFront full-distribution, CloudWatch Metrics, Cloud Build, Application Insights, Operations).

### Phase 164 — Second vibe-slop sweep + terraform-provider test expansion (merged at `616dcd98`)

13 granular commits closed 19 BUGs (1014–1032) across five layered passes. GCP terraform-tests expanded 4 → 11 resources covering 6 sim slices (surfaced 2 real sim defects in the process); Azure terraform-tests expanded 1 → 5 resources. Narrative + per-pass breakdown in [WHAT_WE_DID.md](WHAT_WE_DID.md); per-BUG fix detail in [BUGS.md](BUGS.md); per-commit detail in `git log 616dcd98^..616dcd98`.

### Phase 161 — First comprehensive vibe-slop sweep + bleephub GraphQL completion (merged at `841f2456`)

User directive: no legacy support, no fallbacks, no error-swallowing — silent degradation is itself a bug. 13 fixes + mid-PR bleephub GraphQL completion (PR.comments, reviewThreads, ProjectV2 with fields, edit history, minimization, issue/PR locking, PR.milestone) with real `gh` CLI smoke tests + ProjectManager instance-based lifecycle rewrite. BUG-1006/1007/1009/1011 staged forward + closed across the rest of the 16x sub-task table. Per-fix detail in `git log 841f2456`; narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md).

### Phase 163 — Makefile legacy alias rip-out + docs sweep (merged at `d5b9d22a`)

Single commit. Dropped pure-alias targets (sim-test-*, test-{unit,e2e,agent,core,bleephub}, bleephub-test, bleephub-gh-test) — every alias just delegated to `$(MAKE) -C <dir>` which the `%/<target>` pattern rule already covers. Side-fix: `FORCE` dep so the pattern rule isn't short-circuited by a name-vs-dir collision (e.g. `bleephub/test/`).

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

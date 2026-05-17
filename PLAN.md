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

Phase 161 was the first comprehensive sweep (18 BUGs closed). Phase 164 re-runs the [`avoid-vibe-slop`](.claude/skills/avoid-vibe-slop/SKILL.md) checklist with fresh eyes after `docs/VIBE_CODING.md` grew from 23 → 35 patterns in Phase 162 + the skill expanded from 17 → 26 checklist items. Pattern 26 / 32 (re-verification with fresh eyes) explicitly predicted the first sweep would rubber-stamp some violations — it did. **19 new BUGs closed (1014–1032).**

The phase ran in five passes per user direction (each pass = a new request layered onto the open PR):

1. **First-pass survey** (P164.0–P164.8) — 9 BUGs (1014–1022) filed up front. Headlines: BUG-994 phase-ref sweep was incomplete at ~10 production-code sites; `(BUG-944)` literally embedded in a Cloud Functions volume-translator operator-visible error string with a matching test substring assertion (pattern-28 anti-pattern); BUG-996 cross-cloud silent-decode sibling in bleephub handlers + AWS/GCP sims + `backends/core` exec & libpod handlers; dead helpers in `bleephub/webhooks_payloads.go` with `//nolint:unused // callers land in subsequent commits` directives that never landed; stale `//nolint:unused` pragmas on context helpers that now have real callers; six unused-import silencers.
2. **Re-verification pass** (P164.9) — 3 further BUGs (1023–1025) per pattern 26 / 32: the `stringifyJobState` dead helper in github-runner-dispatcher-gcp; the `httputil.DumpRequest` silencer in tools/http-trace; three silent `pktline.Encoder` swallows in bleephub git_http.go (now Debug-level logged).
3. **Third-pass user-requested sweep** — 3 more BUGs (1026–1028): two test files asserting on Phase metadata in error strings (pattern 28); one naked `t.Skip()` with no message; Azure terraform-tests docs↔code mismatch ("azurerm" in docs vs `azurestack` actually used).
4. **Terraform-provider test expansion** (user-requested, P164.10) — GCP terraform-tests expanded from 4 resources to 11 covering 6 sim slices (compute, dns, artifactregistry, cloud_run_v2 Service + Job, storage, secretmanager). Surfaced + closed 3 sim defects: missing GCP secret-version state-transition handlers (`:enable`/`:disable`/`:destroy` + bare-version GET); same close-then-bind port-allocator race in terraform-tests that Phase 160 fixed only in sdk-tests; the test-coverage expansion itself.
5. **Azure terraform expansion** (P164.11) — Azure terraform-tests expanded from 1 resource (resource_group) to 5 (+ virtual_network, subnet, network_security_group, network_security_rule). Pre-validated via curl; all 5 sim handlers return 200/201 with canonical ARM-id payload. AWS terraform-tests was already comprehensive (394 lines + cross-resource invariants from Phase 159) — not touched.

Per the user directive *"any fixes to land on a single PR; if more extensive changes are needed they can be planned into multiple phases, and use the so-called 'continuity' docs on this repo, with granular commits and check of CI each time"*: 13 granular commits on one branch, CI green between each, single PR.

Acceptance:

- All 19 BUGs (1014–1032) closed in this PR.
- `go test ./...` green in every touched module (bleephub / backends/core / backends/lambda / backends/ecs / simulators/gcp / simulators/aws/sdk-tests / cmd/sockerless-admin / github-runner-dispatcher-gcp).
- GCP terraform-tests `TestTerraformApplyDestroy` PASSes locally (11 resources provisioned + destroyed).
- Azure terraform-tests CI-validated in Docker.
- 11 standard CI checks green per push.
- User merges PR #164.

Out of scope (filed as future BUGs if surfaced):

- Live-cloud validation track (separate cells, separate branches).
- UI / TypeScript sweep (deferred from Phase 161).
- Slopsquatted-dependency audit (handled by `check-latest-deps` hook).
- File-length refactoring for the 14 Go files over 1000 lines (LLM-editability principle 6) — out of Phase 164 scope.

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

### Phase 161 mid-PR expansion — bleephub GraphQL completion

Folded into PR #161 at user request after the re-verification pass. The placeholder-resolver pattern from BUG-1001 ("contract honest via `unreachableFieldErr`") proved that empty connections + unreachable resolvers is a tolerable interim, but the user's preference is to complete each surface with real lookups now rather than carry the placeholders.

Surfaces, each landing in its own commit with real `gh` CLI smoke for fidelity:

- **P161.16 — `PullRequest.comments` ✅**: Wired to bleephub's existing `Comments` store (real GitHub stores PR + Issue conversation comments in the same table; bleephub now mirrors via `Comment.ParentType`). `PRComment` `unreachableFieldErr` resolvers replaced with real data resolvers. Smoke: `gh pr comment` + `gh pr view --json comments`.
- **P161.17 — `PullRequest.reviewThreads`**: Add `PullRequestReviewThread` + `PullRequestReviewComment` GraphQL types. Wire the new connection to the existing `PRReviewCommentStore` (drives the REST `/pulls/{n}/comments` surface via `gh_pr_comments.go`) + the `gh_pr_threads.go` resolve/unresolve state. Smoke: `gh pr view --json reviewThreads`.
- **P161.18 — ProjectV2**: New `ProjectV2Store` + REST endpoints (`/orgs/{org}/projectsV2`, `/projects/{project_id}/items`) + GraphQL types (`ProjectV2`, `ProjectV2Item`, `ProjectV2ItemFieldValue` with at least `SingleSelectValue`) + `addProjectV2ItemById` mutation. `Issue.projectItems` returns real items when issues are added to projects. Smoke: `gh project create` + `gh project item-add` + `gh issue view --json projectItems`.
- **P161.19 — Comment edit history**: Add `Comment.LastEditedAt *time.Time` + `Comment.EditorID int`. REST `PATCH /repos/{}/issues/comments/{id}` + `/pulls/comments/{id}` mutate the body + set edit metadata. GraphQL fields wired: `includesCreatedEdit`, `lastEditedAt`, `editor` on both `IssueComment` and `PRComment` (currently `alwaysFalse` / `alwaysNil`). Smoke: `gh api PATCH .../comments/{id}` then `gh pr view --json comments` shows the edited body + metadata.
- **P161.20 — Comment minimization** (off-topic / outdated / resolved / duplicate / spam / abuse): GraphQL `minimizeComment` + `unminimizeComment` mutations against `IssueComment` / `PRComment`. `Comment.MinimizedReason` enum tracked on the store. `isMinimized` + `minimizedReason` GraphQL fields wired to real state (currently truthful-but-static `alwaysFalse` / `alwaysNil`). Smoke: GraphQL mutation directly + verify subsequent `comments` query reflects the minimized state.
- **P161.21 — PR review thread resolve/unresolve integration**: `gh_pr_threads.go` already exposes `resolveReviewThread` / `unresolveReviewThread` mutations. Integrate the persisted resolution state into the new `PullRequest.reviewThreads` connection from P161.17 so each thread carries `isResolved` + `resolvedBy`. Smoke: `gh pr view --json reviewThreads` shows resolved state correctly after a resolve mutation.
- **P161.22 — Issue / PR locking (moderation)** ✅: REST `PUT /repos/{}/issues/{n}/lock` + `DELETE /repos/{}/issues/{n}/lock` (also matches the PR-as-issue path since PRs share the issue lock endpoint on real GitHub). GraphQL `lockLockable` + `unlockLockable` mutations. `Issue.locked` + `Issue.activeLockReason` + `PullRequest.locked` + `PullRequest.activeLockReason` fields wired. `LockReason` enum (OFF_TOPIC, RESOLVED, SPAM, TOO_HEATED). Comment-create handlers reject with 403 when the parent is locked. Smoke: `gh issue lock` then attempt `gh issue comment` → expect 403.
- **P161.23 — PR milestones**: PR.MilestoneID already tracked on the model. Wire `PullRequest.milestone` GraphQL resolver to look up the real Milestone (currently returns `alwaysNil`). REST `PATCH /repos/{}/pulls/{n}` accepts a `milestone` field to set/clear the PR's milestone. Smoke: `gh pr edit --milestone <num>` then `gh pr view --json milestone` returns the milestone.

Each commit:
- Lands the implementation
- Lands a `gh` CLI smoke test under `bleephub/test/` or as a Go test that shells out to `gh`
- Updates `specs/BLEEPHUB_GITHUB_API_PARITY.md` row for the closed surface
- Updates BUGS.md (BUG-1001 finally closes after P161.18)

### Phase 163 — Makefile legacy alias rip-out + docs sweep (in flight)

User directive: "remove the legacy behaviour of the `make` actions as well as any other 'legacy' functionality; sockerless has no legacy, it's under active development; we must not remove or reduce tests or reduce CI either; sweep docs for old `make` calls and replace them with new ones."

Single commit on `phase-163-legacy-make-rip-out`. Scope = Makefile + make/*.mk + docs. Zero Go-file changes. Per-backend integration tests reached via the existing `%/<target>` pattern rule (now with a `FORCE` dep so the rule isn't short-circuited by a target colliding with a real subdir).

### Track — Continued legacy / fallback rip-out (filed during Phase 161, still open)

Filed during Phase 161 but staged out of scope:

- **BUG-1011** (P1) — `ProjectConfig.SimPort/BackendPort/LogLevel` + `project_manager.go` lifecycle. Rewrite ProjectManager to drive lifecycle from `Topology.Instances` instead of the legacy "1 sim + 1 backend per project" shape.
- **BUG-1009** (P3 continued) — gh-runner-dispatcher escalation (hard error vs grace-window delete on unlabeled services).

Branch + commit layout to be decided when the phase starts.

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

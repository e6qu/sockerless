# Do Next

Status [STATUS.md](STATUS.md) · roadmap [PLAN.md](PLAN.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · vibe catalogue [docs/VIBE_CODING.md](docs/VIBE_CODING.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Where we are

Phase 163 merged 2026-05-16 (PR #163, `d5b9d22a` on `origin/main`) — Makefile legacy alias rip-out + docs sweep.

**Phase 164 in flight on `phase-164-vibe-slop-sweep-2`**: second vibe-slop sweep, user directive: *"re-familiarize yourself with the docs here and then run the vibe slop removal skill; we want any fixes to land on a single PR; if more extensive changes are needed then they can be planned into multiple phases, and use the so-called 'continuity' docs on this repo, with granular commits and check of CI each time."*

First-pass survey filed 9 BUGs (1014–1022) in `BUGS.md § Open`; sub-task table below.

Default "user merges every PR" remains in force.

## Phase 164 sub-task table (sub-task ordering = severity)

| Sub | Status | BUG | What |
|---|---|---|---|
| **P164.0** | ✅ | — | Branch from `origin/main` + survey + 9 BUGs filed + continuity-doc opening. |
| **P164.1** | ✅ | 1015 | `backends/cloudrun-functions/volume_translator.go:95` — stripped `(BUG-944)` literal from operator-visible error string; rewrote `volume_translator_test.go:78` assertion from the contract ("gcs-sync" + "gcs-fuse" + "Cloud Functions") rather than the bug-ref substring. |
| **P164.2** | ✅ | 1016 | bleephub strict-decode: replaced `_ = json.NewDecoder(r.Body).Decode(...)` in `gh_misc_endpoints.go` (OIDC custom sub PUT, Pages create, branch protection PUT) + `gh_issue_moderation.go` (issue lock) with `if err := Decode(...); err != nil && !errors.Is(err, io.EOF) { writeGHError(400, "Problems parsing JSON") }`. `io.EOF` carve-out preserves the legitimate empty-body path. New `gh_misc_endpoints_decode_test.go` covers malformed-JSON → 400 + empty-body OK on the two endpoints that accept empty bodies. |
| **P164.3** | ✅ | 1017 | Sim strict-decode sweep: WAFv2 UpdateRuleGroup checks every Unmarshal error per surrounding sibling envelope; Amplify StartJob strict-decodes with `io.EOF` carve-out; GCP cloud functions surfaces malformed `SOCKERLESS_USER_ENTRYPOINT/_CMD` as invocation error (`{"error":"…"}, exit=1`); AR `registerDockerImageFromManifest` logs decode failure + falls back to request `contentType`; cloudrunjobs `newLRO` panics with typeName on the in-process marshal/unmarshal — would be a programmer error if ever hit. Verified `SOCKERLESS_TEST_TARGET=sim go test ./...` green in `simulators/gcp` + `simulators/aws/sdk-tests` (WAF / Amplify / Stack). |
| **P164.4** | ✅ | 1018 | `handleExecStart` now surfaces `InvalidParameterError` before hijacking the conn (a malformed body would otherwise emit `unrecognized stream` on the wire once hijacked). `handleLibpodContainerCreate` checks both unmarshal errors — Docker-compat + podman specgen — since field sets differ (`command` vs `Cmd`); a malformed lowercase field would have passed the second decode while failing the first, yielding misleading "no image" downstream. |
| **P164.5** | ✅ | 1019 | `functionToContainer` now returns `(api.Container, error)`; surfaces explicit "malformed SOCKERLESS_LABELS base64/JSON on function X" errors. `queryFunctions` caller logs `Warn` + skips the inconsistent resource (matches `queryPodServiceContainers` partial-failure pattern). Sentinel `sockerlessLabelsPresent` distinguishes "env var absent → legacy fallback" from "present but malformed → error". |
| **P164.6** | ◻ | 1020 | Rip `buildPullRequestPayloadWithInstallation` + `buildIssuesPayloadWithInstallation` from `bleephub/webhooks_payloads.go` (zero callers; `//nolint:unused // callers land in the workflow-trigger commit` from Phase 153 lineage never landed). |
| **P164.7** | ◻ | 1021 + 1022 | Drop stale `//nolint:unused` pragmas on `bleephub/gh_middleware.go` context helpers (consumers DID land — pattern 27 / 8 / 33). Sweep unused-import silencers (`var _ = json.Marshal` + variants) across `bleephub/webhooks_payloads.go`, `gh_pr_threads.go`, `gh_app_hooks_rest.go`, `simulators/aws/amplify.go`. Audit `bleephub/gh_request_decode.go` flexInt64 nolint:unused pragmas for live callers before stripping. |
| **P164.8** | ◻ | 1014 | Repo-wide phase-ref sweep continuation. Touch the ~10 production-code sites the BUG-994 sweep missed; preserve the *why* when load-bearing per the BUG-994 rule. |
| **P164.9** | ◻ | — | Re-verification pass (pattern 26 / 32). Walk the avoid-vibe-slop checklist again with fresh eyes; file any new findings. |
| **P164.10** | ◻ | — | Final state save + push + open PR. |

## Phase 163 scope

Rip the "Legacy aliases" section out of the top-level Makefile + the make/*.mk legacy framing. Sweep all docs to replace removed `make X` invocations with their canonical path-delegation form. Keep every recipe that does real work (Docker harnesses for smoke / tf-int / e2e / upstream / bleephub-gh-docker-test, plus `check-backend-coverage{,-enforce}`); only drop pure aliases.

### Changes

| Layer | Removed | Replaced with |
|---|---|---|
| Top-level Makefile | `sim-test-{ecs,lambda,cloudrun,gcf,aca,azf}` | `make backends/<name>/test-integration` |
| Top-level Makefile | `sim-test-{aws,gcp,azure,all}` (composites) | `make test-integration` fan-out, or chain per-backend targets |
| Top-level Makefile | `test-{unit,e2e,agent,core,bleephub}` | `make agent/test`, `make backends/core/test`, `make bleephub/test`, `make tests/test` |
| Top-level Makefile | `bleephub-test`, `bleephub-gh-test` | `make bleephub/test`, `make bleephub/test-integration` |
| Makefile header | `# Legacy aliases preserved at the bottom (…)` comment | Section-header explanation of the surviving cross-cutting Docker suites |
| Makefile pattern rule | `%/<target>: ...` short-circuited when target name collides with real dir (e.g. `bleephub/test/`) | Added `FORCE` phony dep so recipe always delegates into the per-app Makefile |
| make/stack.mk | "legacy 1-sim + 1-backend tuple" comment | "pre-canned 1-sim + 1-backend + admin topology" |
| make/components.mk | same | same |

Docs updated: `README.md`, `FEATURE_MATRIX.md`, `ARCHITECTURE.md`, `backends/README.md`, `simulators/README.md`, `bleephub/README.md`, `tests/README.md`, `docs/MAKEFILE_STANDARD.md`, `.claude/skills/manual-test/SKILL.md`. Stale invented targets fixed: `stack-aws-ecs-up`/`-down` → `stack-aws-ecs`/`stack-down`; `e2e-github-aws-ecs` → `e2e-github-ecs`; `docker-tf-int-test-azure` → `tf-int-test-azure`.

### Acceptance bar (Phase 163)

- `make help` parses cleanly.
- `make backends/ecs/test-integration` (and equivalents for the other 5 backends) resolves and delegates.
- `make bleephub/test`, `make bleephub/test-integration`, `make tests/test`, `make agent/test`, `make backends/core/test` all resolve through the pattern rule despite the `bleephub/test/` directory collision.
- `go test ./...` green in `api`, `agent`, `bleephub`, `backends/core`, `cmd/sockerless-admin` (touched zero Go files, but sanity smoke confirms nothing regressed).
- No `make sim-test-*`, `make bleephub-test`, `make bleephub-gh-test`, `make test-{unit,e2e,agent,core,bleephub}` refs remain in docs / scripts / CI.
- CI workflows (`ci.yml`, `e2e-vs-simulators.yml`, `.gitlab-ci.yml`) untouched in the Makefile-target columns they relied on (`make lint`, `make e2e-github-*`, `make e2e-gitlab-*`, `simulators/<cloud> && make sdk-test`).

## Earlier phases (closed)

### Phase 161 — Vibe-slop sweep (merged PR #161)

Run the [`avoid-vibe-slop`](../.claude/skills/avoid-vibe-slop/SKILL.md) checklist across **every layer** (backends, simulators, bleephub, cmd, agent, api). File every concrete violation. Fix every BUG in this PR.

### Sub-task / commit layout

Each BUG = one commit (sometimes two if BUGS.md update is separate from the fix). Commits land in severity order — auth-bypass first, fail-loud invariant next, handler-delegation next, then the bigger sweeps.

| Sub | Status | BUG | What |
|---|---|---|---|
| **P161.0** | ✅ | — | State save + branch + 12 findings filed in BUGS.md. |
| **P161.1** | ✅ | BUG-1000 | bleephub OAuth `handleOAuthToken` validates `client_assertion` JWT against agent's registered RSA public key per Azure DevOps OAuth2 jwt-bearer flow. Tests rewritten to drive real keypair + signed assertion. |
| **P161.2** | ✅ | BUG-997 | `Persistence.MustPut` + `MustDelete` + 18-site sweep — bleephub persistence write failures now `log.Fatalf`, matching the open-failure invariant. |
| **P161.3** | ✅ | BUG-995 | `handleSystemDf` / `handleContainerList` / `handleImagePrune` delegate to `s.self.<Method>`; consolidated the richer prune logic into `BaseServer.ImagePrune`; extracted `collectContainers` helper for `handleLibpodContainerList` (fixes a latent pending-create-drop bug). |
| **P161.4–7** | ✅ | BUG-998 / 1002 / 1003 / 1004 / 1005 / 996 | Batched smaller fixes — `decodeRegistryAuth` dead-code rip + `handleImagePush` fail-loud; Azure ACR replications parent-exists check; inline `buildOCIHandler`; seeded admin `bph_` → `ghp_`; matrix fail-fast 3-deep nil chain → `JobDef.FailFast()` method; 18 sim `_ = sim.ReadJSON(...)` sites swept. |
| **P161.8–9** | ✅ | BUG-994 / 999 / 1001 | Repo-wide phase/BUG-ref sweep (~115 occurrences, two-pass script + targeted fixups for one bot regression). `core.TagSet.InstanceID` deprecation comment dropped (audit confirmed both fields are load-bearing). Bleephub GraphQL `alwaysEmptyString` resolvers on unreachable NonNull fields → `unreachableFieldErr` for an honest contract. |
| **P161.10** | ✅ | BUG-1008 | Deleted legacy `InitTracer` OTel entry point in 6 modules; migrated `otel_test.go` to `InitObservability`. |
| **P161.11** | ✅ | — | Filed BUG-1006 / 1007 / 1009 as Open for Phase 162 (legacy-rip-out exceeds #161 reviewable scope). BUG-1010 candidate reclassified as false positive. |
| **P161.12** | ◻ | — | Final state save + push + open PR #161. |

### Discipline reminders

Before each sub-task commit, read [`.claude/skills/avoid-vibe-slop/SKILL.md`](../.claude/skills/avoid-vibe-slop/SKILL.md). Specifically:

- Q2 "What is the reference adaptor?" — every fix must preserve the reference-adaptor contract (`gh` CLI for bleephub; Docker SDK for backends/core; AWS/GCP/Azure SDK + Terraform for sims).
- Q5 "Right fix, not quick fix" — BUG-994 is *not* "delete every line that says Phase". It's "preserve the *why* when load-bearing, drop the metadata." BUG-995 is *not* "add a stub `SystemDf` method on every backend". It's "the `api.Backend` method already exists — call it."
- Q8 "Is the test driving the real adaptor?" — fixes that touch BUG-1000 (OAuth JWT validation) get verified with a real `gh auth login` flow against bleephub, not a mock.

### Acceptance bar

- All 12 BUGs in BUGS.md move from `Open` to `Resolved history`.
- `go test ./...` green in every touched Go module.
- `bun test` green in every touched UI package (none expected; flag if any touched).
- CI green on the PR's 11 standard checks.
- No newly-surfaced vibe-slop instances during the sweep go un-filed.

## Resumable tracks after Phase 161 merges

### Phase 162 — Legacy / fallback rip-out (filed during Phase 161)

Three Open BUGs surfaced during the Phase 161 sweep that exceeded #161's reviewable scope. Per the user's "no legacy support / no fallbacks during active development" directive, all three are simple rip-outs (no deprecation period, no opt-in compat).

- **BUG-1006** (P1) — `cmd/sockerless-admin/config.go` + `cmd/sockerless/client.go` silently fall back to "old JSON contexts" when `config.yaml` is missing. Fix: drop the JSON fallback in both `discoverFromContexts` and `listContexts`; require `config.yaml` or error.
- **BUG-1007** (P1) — `cmd/sockerless-admin/{instance,topology_store,topology_manager,project}.go` legacy migration scaffolding (`DeriveLegacyInstances`, `MigrateLegacyProjects`, `legacyDir`, `ProjectConfig` dual shape). Fix: rip the entire migration plumbing + the legacy `(SimPort / BackendPort)` shape on `ProjectConfig`; delete dependent tests.
- **BUG-1009** (P2) — `github-runner-dispatcher-gcp/cmd/.../main.go::~351` "Services without an owner label are legacy (pre-owner-label rollout) — leave them alone." Fix: surface unknown services as an error rather than papering over with a future cleanup.

Plus BUG-1001 (real ProjectV2 / PR-review-thread implementation), lower priority — the `unreachableFieldErr` placeholder from Phase 161 keeps the contract honest until the surfaces are implemented.

### Track A — Live-cloud validation

Lambda live · Cloud Run Services + ACA Apps live · AZF cloud-dns live · Lambda service-mesh live · ACA/AZF Azure AD live. One branch per cell; teardown self-sufficient per `feedback_teardown_aggressive.md`.

### Track B — UI vibe-slop sweep

TypeScript / UI sweep deferred from Phase 161. If the Go sweep surfaces a sibling pattern in the UI (`ui/packages/*/src/`), this is the follow-up.

### Track C — Skill maturation

Candidate additional skills as new patterns surface from Phase 161 close-out: `state-save`, `spec-first-implementation`, `cross-cloud-sweep`.

### Track D — Phase 91d (bookmarked indefinitely)

Real `pd-ephemeral` on cloudrun + gcf. Don't reopen until cloud capability changes.

## Invariants snapshot (full list in STATUS.md + VIBE_CODING.md)

- Never auto-merge; user merges every PR.
- Components decoupled from admin / UI.
- No fakes / no fallbacks / no silent shims.
- Persistence opt-in + fail-loud on both open AND write (BUG-985/986/997).
- HTTP handlers dispatch through `s.self.<Method>`; never read `s.Store` directly (BUG-991/992/995).
- No phase / BUG-ID references in code comments (BUG-994).
- `gh` CLI is the reference adaptor for bleephub.
- `aws --debug` + SDK serializer source are the reference for sim handler wire shapes.
- `specs/CLOUD_RESOURCE_MAPPING.md` is authoritative for cloud-mapping.

## Session-resume checklist

1. `git fetch origin && git checkout phase-161-vibe-slop-sweep && git pull` (or `git checkout main && git pull --ff-only` if 161 merged).
2. `git log --oneline -15` to see what's already on the branch.
3. Read STATUS.md + this file + the recent commits + BUGS.md § Open.
4. Read [`.claude/skills/avoid-vibe-slop/SKILL.md`](../.claude/skills/avoid-vibe-slop/SKILL.md) before writing any fix.
5. Pick the next `◻` row from the sub-task table above; the table is ordered by severity, so lowest unchecked = next.
6. Fix it. `go test ./...` in the touched module. Move the BUG from Open → Resolved history in BUGS.md with a one-line summary.
7. State save: update this file's sub-task status, STATUS.md bug counts, WHAT_WE_DID.md narrative.
8. Commit and push. CI green per push.

# Do Next

Status [STATUS.md](STATUS.md) · roadmap [PLAN.md](PLAN.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · vibe catalogue [docs/VIBE_CODING.md](docs/VIBE_CODING.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Where we are

Phase 160 merged 2026-05-16 (PR #160, `aeb0ac6e` on `origin/main`).

**Phase 161 in flight on `phase-161-vibe-slop-sweep`**: comprehensive vibe-slop sweep + fixes in one PR. 12 BUGs filed (BUG-994 … BUG-1005) covering 7 anti-pattern categories from `docs/VIBE_CODING.md`. The catalogue exists precisely so this kind of sweep is an explicit phase, not a perpetual side-quest.

Default "user merges every PR" remains in force.

## Phase 161 scope

Run the [`avoid-vibe-slop`](../.claude/skills/avoid-vibe-slop/SKILL.md) checklist across **every layer** (backends, simulators, bleephub, cmd, agent, api). File every concrete violation. Fix every BUG in this PR.

### Sub-task / commit layout

Each BUG = one commit (sometimes two if BUGS.md update is separate from the fix). Commits land in severity order — auth-bypass first, fail-loud invariant next, handler-delegation next, then the bigger sweeps.

| Sub | Status | BUG | What |
|---|---|---|---|
| **P161.0** | ✅ | — | State save + branch + 12 findings filed in BUGS.md. |
| **P161.1** | ◻ | BUG-1000 | `bleephub/auth.go::handleOAuthToken` — validate `client_assertion` JWT against App's public key + `grant_type` per real GitHub `/login/oauth/access_token`. No more `alg:none` 1-year JWT for any input. |
| **P161.2** | ◻ | BUG-997 | `bleephub/{store,gh_apps_store,gh_apps_user_tokens}.go` — `_ = st.persist.Put/Delete(...)` ignored errors. Add a `persistPut` helper that `log.Fatalf`s on write failure, matching the open-failure invariant. |
| **P161.3** | ◻ | BUG-995 | `backends/core/handle_extended.go` + `handle_images.go` + `handle_libpod.go` — HTTP handlers reading `s.Store.*` directly instead of dispatching through `s.self.<Method>`. Siblings of BUG-991/992. Delegate via `s.self.SystemDf`, `s.self.ImagePrune`, `s.self.ContainerList`, etc. |
| **P161.4** | ◻ | BUG-998 | `backends/core/handle_images.go::decodeRegistryAuth` — distinguish empty header (no auth) from malformed header (real client bug). Propagate decode error as `400` to caller. |
| **P161.5** | ◻ | BUG-1001 | `bleephub/gh_issues_graphql.go` + `gh_pulls_graphql.go` — `alwaysNil` / `emptyList` resolvers for ProjectV2 + PR review threads. Replace with real lookups; if a surface is genuinely out of scope, return a GraphQL field-level error, never fake data. |
| **P161.6** | ◻ | BUG-1002 | `simulators/azure/acr.go` — Replications list returns `[]` when parent registry missing. Add parent-exists check; return real `ResourceNotFound` Azure error envelope. |
| **P161.7** | ◻ | BUG-996 | `simulators/{aws,gcp,azure}/*.go` — ~18 sites of `_ = sim.ReadJSON(r, &req)`. For mandatory-body handlers: propagate parse error. For optional-body handlers: switch to `io.Copy(io.Discard, r.Body)` with a `why` comment so the choice is explicit. |
| **P161.8** | ◻ | BUG-994 | Repo-wide sweep: remove the ~60 phase / BUG-ID references in production code comments. Preserve the *why* when load-bearing; drop the metadata. |
| **P161.9** | ◻ | BUG-999 | `backends/core/tags.go::InstanceID` is marked `Deprecated: use Cluster instead` but has 27+ active callers. Either complete the migration to `Cluster` or remove the misleading deprecation. |
| **P161.10** | ◻ | BUG-1004 | `bleephub/store.go::SeedDefaultUser` — `bph_`-prefixed seeded admin token is a legacy shim left over from pre-Phase-153. Switch to `ghp_` per real GitHub (rule: if real GitHub does X, bleephub does X). |
| **P161.11** | ◻ | BUG-1005 | `bleephub/workflows.go` — 3-deep `if foundJob.Def != nil && foundJob.Def.Strategy != nil && foundJob.Def.Strategy.FailFast != nil`. Normalise on YAML parse so the runtime path is single-deref. |
| **P161.12** | ◻ | BUG-1003 | `simulators/gcp/artifactregistry.go::buildOCIHandler` single-call-site abstraction. Inline. |
| **P161.13** | ◻ | — | Final state save + push + open PR #161. |

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

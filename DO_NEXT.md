# Do Next

Status [STATUS.md](STATUS.md) · roadmap [PLAN.md](PLAN.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · vibe catalogue [docs/VIBE_CODING.md](docs/VIBE_CODING.md).

## Where we are

**Phase 178 — 1 reopen + 8 community-filed + 4 class-of-bug remediations, single PR, 7 stages.** Branch `phase-178-community-issues`, in flight.

PR #202 (Phase 177) merged at `aa847b1`. User immediately filed 8 new issues (#203..#210) and reopened #196 (S3 multipart `ListParts` missed in the Phase 176/177 sweeps). The pattern across these and the earlier #190/#193/#196 reopen chain decomposes into **four classes of bug** that current skills don't catch:

1. **Partial-table coverage (Class 1)** — `surface-table-completeness` is reactive; the 30+ sim service surfaces without a table are silent ✗ everywhere. Issue #196's reopen is the canonical proof: bucket subresources tabled, multipart wasn't.
2. **Mux pattern collision (Class 2)** — collapsed-port sim has no scanner to flag pattern shadowing; the wrong handler responds with a plausible error so wire-shape probes miss it. Surfaces in #204 + #208.
3. **Wire-shape drift on List / paged endpoints (Class 3)** — single-record envelope from a List endpoint silently passes `.Value[0]`-style SDK tests. Surfaces in #203.
4. **State-machine fakery (Class 4)** — sim stores data flat; documented state transitions (versioning, soft-delete, snapshot states, failover) aren't modelled. Surfaces in #203 + #205 + #207 + #209.

Phase 178 closes the 9 community-filed items AND the 4 class-of-bug remediations on a single branch.

BUGS.md: **1160 filed · 1145 fixed · 15 open.** Open: BUG-1075 (live-cloud) + BUG-1104 (audit-cadence) + BUG-1148..1160 (Phase 178 scope).

## Phase 178 scope — 13 BUGs across 7 stages

| Stage | Commits | BUGs | Notes |
|---|---|---|---|
| A — infrastructure | 1-5 | 1157 (proactive seed) · 1158 (mux scanner) · 1159 (paged rule) · 1160 (state-machine skill) | Builds scaffolding subsequent stages use. mux scanner runs in warn mode initially. |
| B — AWS routing | 6-8 | 1154 (awsQuery Version dispatch) · 1150 (S3 vs API Gateway v2) | Closes #204 + #208. Graduates mux scanner to gating. |
| C — AWS ops | 9-11 | 1148 (S3 ListParts) · 1153 (RDS snapshots + SNS + SQS) | Closes #196 reopen + #207. |
| D — Azure KV state | 12-15 | 1149 (versioning + paged) · 1151 (PATCH + soft-delete) | Closes #203 + #205. |
| E — Azure services | 16-19 | 1152 (App Service config) · 1156 (PG FS + APIM + Cache Redis) | Closes #206 + #210. |
| F — GCP ops | 20-23 | 1155 (Cloud SQL + Memorystore + Pub/Sub + API Gateway IAM) | Closes #209. |
| G — final audit + PR | 24-26 | tf-tests parity + skill audit + PR open | — |

## Stage A — current

Commit 1 (this commit): file BUG-1148..1160 + continuity-doc reset.
Commit 2 — proactive `specs/SIM_SURFACE_TABLES/` seed (~30 services, just ops the sim already touches + obvious siblings).
Commit 3 — `mux-overlap-scan` skill + scanner + pre-commit hook (warn).
Commit 4 — extend `sim-canonical-config-test` with paged-iterator rule + backfill existing tables.
Commit 5 — `sim-state-machine-completeness` skill.

Checkpoint A after commit 5: 30+ tables, scanner reports baseline overlaps, all 3 SDK suites still green.

## Cross-stage rules

- Single branch `phase-178-community-issues`; no merge between stages — only test checkpoints.
- Each checkpoint failure: fix in-branch on the next commit; re-run checkpoint; move on.
- BUGS.md strikethrough updates land in the commit that closes each BUG.
- GitHub issues stay open during the phase; close after PR merges.
- Continuity docs refresh only at stage boundaries.
- mux-overlap-scan in warn mode through Stage A; graduates to gating in Stage B commit 7.

## Invariants snapshot (full list in STATUS.md)

- Never auto-merge; user merges every PR.
- Single-branch rule per phase; never more than 1 PR open.
- File BUGs *before* fixing.
- No fakes / no fallbacks / no silent shims.
- Every sim surface tested at BOTH the SDK layer AND the terraform-provider layer (when a provider resource exists).
- Every reopen carries a postmortem trail.
- Every closed-enumeration surface has a `specs/SIM_SURFACE_TABLES/<name>.md` with no silent ✗ rows.
- Every List* op has a paged-iterator test.
- Every stateful resource type has a state-machine skill assertion.
- `make hooks` on every fresh clone.

## Session-resume checklist

1. `git fetch origin && git status && git log --oneline -8`.
2. `gh pr list --state open` — Phase 178 PR open yet? (Should be no until Stage G.)
3. Read STATUS.md + this file + BUGS.md § Open.
4. Read `.claude/skills/avoid-vibe-slop/SKILL.md` before any code change.
5. Pick the next ◻ Phase 178 commit; run checkpoint after each stage boundary.

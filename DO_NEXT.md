# Do Next

Status [STATUS.md](STATUS.md) · roadmap [PLAN.md](PLAN.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · vibe catalogue [docs/VIBE_CODING.md](docs/VIBE_CODING.md).

## Where we are

**Phase 177 — 1 reopen + 1 community-filed + 4 process improvements.** Branch `phase-177-community-issues`.

PR #200 (Phase 176) merged at `2d8e604` closed BUG-1134..1141 + 8 GitHub issues. User testing against the merged build reopened **#193** (KV `WWW-Authenticate` `authorization` URL breaks the Azure SDK's `parseTenant` — only 3 path segments, SDK indexes `[3]` and panics) and filed **#201** (S3 bucket-level PUT/DELETE subresources route to CreateBucket → 409).

The pattern between these two reopens / adjacents + the earlier #190 reopen is the load-bearing finding: my SDK tests bypassed the contract the real SDK actually exercises (raw `net/http` + `Bearer fake-token` instead of `azsecrets.NewClient`; only the user-named subset of a closed operation table instead of the full enumeration). Phase 177 closes both the code fixes AND the process gaps that let them ship.

BUGS.md: **1147 filed · 1139 fixed · 8 open.** Open: BUG-1075 (live-cloud) + BUG-1104 (audit-cadence) + BUG-1142..1147 (Phase 177 scope).

## Phase 177 scope — 6 BUGs

| BUG | Sev | Surface | Headline |
|---|---|---|---|
| 1142 | P1 | AWS S3 | Bucket-level PUT/DELETE subresources route to CreateBucket → 409. Table-driven dispatcher for 15 PUT + ≥10 DELETE ops; back configs in per-bucket `sim.MakeStore`. Issue #201. |
| 1143 | P0 | Azure KV | `Bearer authorization="http://<host>", resource="..."` breaks SDK challenge parser (3 path segments). Emit `http://<host>/00000000-…` so SDK `parseTenant` extracts the placeholder UUID. Issue #193 reopened. |
| 1144 | P1 | skills | Extend `sim-canonical-config-test` SKILL to refuse raw `net/http` when an official SDK package exists for the service. Refactor every existing raw-HTTP sdk-test that has an SDK equivalent. |
| 1145 | P1 | specs+skills | `specs/SIM_SURFACE_TABLES/` — one MD per service with full op enumeration (sim-impl + sdk-test + tf-test columns). New skill `surface-table-completeness/SKILL.md` enforcing full-table audit before any "fixed" claim. |
| 1146 | P1 | skills | New skill `reopen-postmortem/SKILL.md` — every reopen BUG carries (a) what test passed but should have failed, (b) what SDK code path was missed, (c) what new canonical-SDK-client test catches the regression. Reopens are P0 by default. |
| 1147 | P1 | terraform-tests | Every sim surface that has a terraform-provider resource gets a tf-tests entry alongside the sdk-test. Phase 177 adds tf-tests for every S3 subresource fixed under 1142 + every KV op covered by 1143. Extend `sim-canonical-config-test` to cover provider config. |

Implementation order:

1. **Continuity-doc reset** — STATUS.md / DO_NEXT.md / this file / BUGS.md as commit 1.
2. **BUG-1143** — KV authorization URL fix + canonical `azsecrets.NewClient` test that fails on the pre-fix build.
3. **BUG-1142** — S3 bucket-subresource dispatcher (table-driven) + per-subresource sdk-tests + tf-tests.
4. **BUG-1145** — `specs/SIM_SURFACE_TABLES/aws-s3-subresources.md`, `azure-kv-operations.md`, plus skill MD, populated from the 1142 + 1143 work.
5. **BUG-1147** — tf-tests for the new S3 subresources + KV ops; extend canonical-config-test for provider config.
6. **BUG-1144** — extend `sim-canonical-config-test` SKILL with the raw-HTTP rule; refactor every existing raw-HTTP sdk-test in `simulators/<cloud>/sdk-tests/` that has an SDK equivalent.
7. **BUG-1146** — `reopen-postmortem/SKILL.md` + backfill postmortems for BUG-1134, BUG-1135 (and BUG-1130 + BUG-1131 if they qualify).
8. **In-PR audit pass** — silent-error-swallow + dead-code + canonical-config + emitted-url + streaming-body + backpedal + timeless-comments + fake-implementation + surface-table-completeness + reopen-postmortem.
9. **Push, open PR, wait for CI, user merges.**

## Invariants snapshot (full list in STATUS.md)

- Never auto-merge; user merges every PR.
- Single-branch rule per phase; never more than 1 PR open.
- File BUGs *before* fixing.
- No fakes / no fallbacks / no silent shims — fail loud or ask, never silently degrade.
- Every sim surface tested at BOTH the SDK layer AND the terraform-provider layer (when a provider resource exists).
- Every reopen carries a postmortem trail.
- `make hooks` on every fresh clone.

## Session-resume checklist

1. `git fetch origin` + `git status` + `git log --oneline -8`.
2. `gh pr list --state open` — Phase 177 PR open?
3. Read STATUS.md + this file + BUGS.md § Open.
4. Read [`.claude/skills/avoid-vibe-slop/SKILL.md`](.claude/skills/avoid-vibe-slop/SKILL.md) before any code change.
5. Pick the next ◻ Phase 177 BUG; commit when verified.

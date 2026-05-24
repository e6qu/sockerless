# Do Next

Status [STATUS.md](STATUS.md) · roadmap [PLAN.md](PLAN.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · vibe catalogue [docs/VIBE_CODING.md](docs/VIBE_CODING.md).

## Where we are

**Phase 176 — 8 new/reopened community-filed issues. Branch `phase-176-community-issues`.**

PR #192 merged at `ca11405` closed BUG-1120..1133 + 3 GitHub issues (#189/#190/#191). The user re-tested against the merged build and reopened **#190** (path-style storage dispatch didn't work — the fix required ARM registration that real Azurite users don't do) and filed 7 new issues (#193–#199).

BUGS.md: **1141 filed · 1131 fixed · 10 open.** The 10 open: BUG-1075 (live-cloud, deprioritized) + BUG-1104 (audit cadence) + BUG-1134..1141 (Phase 176 scope).

## Phase 176 scope — 8 BUGs

| BUG | Issue | Sev | Cloud | Headline |
|---|---|---|---|---|
| 1134 | #190 reopened | P1 | Azure | Storage path-style needs ARM-prefix exclusion, not known-account allow-list (Azurite-permissive) |
| 1135 | #193 | P0 | Azure | KV data plane: `WWW-Authenticate: Bearer` 401 on unauthenticated probe — every Azure SDK consumer blocked |
| 1136 | #194 | P1 | AWS | RDS + ElastiCache: empty `<EngineVersion>` on omit — populate per-engine GA default |
| 1137 | #195 | P0 | Azure | Service Bus REST data plane: real handlers (SendMessage 201, Receive-and-Delete 200/204, Peek-Lock 201/204, CompleteLock 204) + in-memory message store |
| 1138 | #196 | P1 | AWS | S3 subresources: multipart family, Object Tagging CRUD, CopyObject |
| 1139 | #197 | P1 | GCP | `/v1/operations` returns GCS-shaped 404 (same shape as #183) — register explicit handler + tighten GCS catch-all |
| 1140 | #198 | P1 | GCP | GCS: missing `Objects.compose`, body-form `name` on resumable+multipart, http→https URL emission |
| 1141 | #199 | P1 | AWS | Lambda subresources: PublishVersion, CreateAlias, AddPermission, CreateFunctionUrlConfig — proper restJson1 envelopes |

Implementation order (P0 first, then by cloud cluster to minimize cross-file context-switching):

1. **BUG-1135** KV WWW-Authenticate (P0) — middleware in `simulators/azure/keyvault.go` wraps all data-plane routes.
2. **BUG-1137** Service Bus REST (P0) — new in-memory store + 4 real handlers; topic+sub variants.
3. **BUG-1134** path-style storage (reopened) — ARM-prefix exclusion in `blob.go` + `storage_dataplane.go`.
4. **BUG-1139** GCP /v1/operations routing leak — explicit handler + GCS catch-all tightening.
5. **BUG-1140** GCS compose + body-form name + https — new handler, body-parse for upload init, URL helper.
6. **BUG-1141** Lambda subresources — register 4 endpoint families in `lambda.go`.
7. **BUG-1138** S3 subresources — query-string dispatcher in `s3.go`.
8. **BUG-1136** RDS+ElastiCache engine defaults — per-engine maps.

Each fix lands with a real SDK test (no auth-bypass quirks, no path-prefix hacks per BUG-1104 invariant).

## Final validation

After all 8 BUGs close, re-run the 6 specialist skills (per BUG-1104 cadence — Phase 176 is the third audit):
1. `silent-error-swallow-scan`
2. `dead-code-silencer-scan`
3. `sim-canonical-config-test`
4. `sim-emitted-url-roundtrip`
5. `sim-streaming-body-handler`
6. `backpedal-pattern-audit`

Plus the new `timeless-comments` skill across the Phase 176 diff.

## Open BUGs after Phase 176 lands

Only BUG-1075 (live-cloud, deprioritized) + BUG-1104 (audit cadence) remain Open. Everything community-filed gets a real fix in-PR.

## Invariants snapshot (full list in STATUS.md)

- Never auto-merge; user merges every PR.
- Single-branch rule per phase; never more than 1 PR open.
- File BUGs *before* fixing.
- No fakes / no fallbacks / no silent shims — fail loud or ask, never silently degrade.
- Driver pluggability preserved: one driver per dimension.
- `specs/CLOUD_RESOURCE_MAPPING.md` is authoritative.

## Session-resume checklist

1. `git fetch origin && git checkout phase-176-community-issues && git pull --rebase origin main`.
2. `git log --oneline -10`.
3. Read STATUS.md + this file + BUGS.md § Open.
4. Read [`.claude/skills/avoid-vibe-slop/SKILL.md`](.claude/skills/avoid-vibe-slop/SKILL.md) before any code change.
5. Pick the next ◻ Phase 176 BUG; commit when verified.

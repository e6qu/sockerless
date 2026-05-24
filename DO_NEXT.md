# Do Next

Status [STATUS.md](STATUS.md) · roadmap [PLAN.md](PLAN.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · vibe catalogue [docs/VIBE_CODING.md](docs/VIBE_CODING.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Where we are

**Phase 174 merged 2026-05-24 (squash `7a5d588`).** No active branch; sitting on `main`.

BUGS.md: **1119 filed · 1117 fixed · 2 open · 2 false positives.** Open: BUG-1075 (live-cloud cells, deprioritized 2026-05-23) + BUG-1104 (meta tracking — periodic skill-sweep cadence).

## Phase 175 — second skill-sweep audit (next)

Re-run the 6 specialist skills across the now-larger codebase (Phase 173 + Phase 174 sims merged):

1. `silent-error-swallow-scan` — find any `_ = sim.ReadJSON` / silent decode / discarded `err` introduced in the Phase 174 round-2 storage_dataplane.go + streaming.go work.
2. `dead-code-silencer-scan` — any new `var _ = ...` import silencers or "reserved for future" symbols.
3. `sim-canonical-config-test` — quirk patterns vs SDK / CLI / Terraform deserializers in newly added handlers.
4. `sim-emitted-url-roundtrip` — fresh scan for emitted URLs that callers cannot follow back; round 3 of Phase 174 caught 5 leftovers, so a re-run after the merge will catch any I still missed.
5. `sim-streaming-body-handler` — any new upload handlers that don't go through `openStreamingBody`.
6. `backpedal-pattern-audit` — any new BUG-1016/1017/1020/1022 shape recurrences in commits since Phase 173 merged.

Launched in parallel via subagents; results consolidated, BUGs filed, then Phase 175 branch created if anything substantive falls out.

## Open BUGs

- **BUG-1075** — Live-cloud cell validation. Deprioritized 2026-05-23; revisit only when operator decides.
- **BUG-1104** — Meta tracking entry for the audit cadence. Phase 174 was the first audit; Phase 175 is the second. Stays open as a forever-recurring reminder.

## Invariants snapshot (full list in STATUS.md)

- Never auto-merge; user merges every PR.
- Single-branch rule per phase.
- File BUGs *before* fixing.
- Verify each significant chunk; don't batch fixes.
- No fallbacks anywhere: no silent substitution, no "best-effort with logging," no transparent re-invoke.
- Driver pluggability preserved: one driver per dimension.
- `gh` CLI is the reference adaptor for bleephub.
- SDK / CLI / Terraform provider call sequences differ — endpoint-fidelity fixes need all three external-client layers.
- `specs/CLOUD_RESOURCE_MAPPING.md` is authoritative for "how does sockerless model X on cloud Y."

## Resumable tracks (longer-horizon)

- **Track A** — Live-cloud validation (one branch per cell). Deprioritized.
- **Track B** — UI / TypeScript vibe-slop sweep (carried from Phase 161).
- **Track C** — Phase 91d (bookmarked; needs Cloud Run protobuf field).
- **Track D** — Phase 166 follow-up gaps: GCP CF Gen2 + Pub/Sub + Compute terraform coverage; Azure KV data-plane terraform coverage. Filed informally.
- **Track E** — Real-runner simulator arithmetic checks once a sim-backed backend is started on `SOCKERLESS_DOCKER_HOST`.

## Session-resume checklist

1. `git fetch origin && git checkout main && git pull origin main`.
2. `git log --oneline -10`.
3. Create or check out the active work branch, then read STATUS.md + this file + PLAN.md + BUGS.md § Open.
4. Read [`.claude/skills/avoid-vibe-slop/SKILL.md`](.claude/skills/avoid-vibe-slop/SKILL.md) before any code change.
5. Pick the next ◻ sub-task; mark it `in_progress` in tasks; commit when verified.

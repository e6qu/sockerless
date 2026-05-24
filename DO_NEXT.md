# Do Next

Status [STATUS.md](STATUS.md) · roadmap [PLAN.md](PLAN.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · vibe catalogue [docs/VIBE_CODING.md](docs/VIBE_CODING.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Where we are

**Phase 175 — second skill-sweep audit. Branch `phase-175-skill-sweep`. PR #192, awaiting user merge.**

14 BUGs closed (1120–1133), 3 GitHub issues closed (#189 Pub/Sub PATCH, #190 Azurite-compatible path-style Azure storage, #191 KV https hard-code), 2 skill extensions + 1 new skill (`timeless-comments`), CI `test` job split into 22 matrix jobs, 5 FaaS smoke tests de-flaked. All 6 specialist skills re-validated clean on the final diff.

BUGS.md: **1133 filed · 1131 fixed · 2 open · 2 false positives.** Open: BUG-1075 (live-cloud cells, deprioritized) + BUG-1104 (meta tracking — periodic skill-sweep cadence).

## Open BUGs

- **BUG-1075** — Live-cloud cell validation. Deprioritized; revisit only when operator decides.
- **BUG-1104** — Meta tracking for the audit cadence. Phase 174 was the first audit, Phase 175 the second; stays open as a perpetual reminder.

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

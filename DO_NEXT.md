# Do Next

Status [STATUS.md](STATUS.md) · roadmap [PLAN.md](PLAN.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · vibe catalogue [docs/VIBE_CODING.md](docs/VIBE_CODING.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Where we are

Phase 166 merged 2026-05-17 (PR #167, `49050c2d` on `origin/main`). All Open BUGs closed at that point.

**Phase 167 in flight on `phase-167-pod-model-analysis`** — doc-only analysis + Phase 168 plan. Branch contains the comparison of pod abstraction across 7 backends, root-cause of the "12-step CI job = 12+ min" symptom (Path B silent fallback in lambda + cloudrun + cloudrun-functions), and the Phase 168 design (see PLAN.md § Active phase for the full plan).

**Phase 168 plan ready for user review** before implementation starts. 3 product decisions confirmed by user; 6 sizing / disposition questions still pending.

## Phase 167 sub-task status

| Sub | Status | What |
|---|---|---|
| **P167.0** | ✅ | Branch off main; scope pod-model analysis. |
| **P167.1** | ✅ | Pod model survey per backend (parallel Explore agents). |
| **P167.2** | ✅ | Runner ↔ backend call sequence trace (GH Actions + GitLab). |
| **P167.3** | ✅ | Driver matrix (network / dns / access / storage). |
| **P167.4** | ✅ | "12 steps = 12 min" root cause → Lambda Path B silent fallback (smoking gun: `backends/lambda/backend_delegates.go:210-213`). |
| **P167.5** | ✅ | FaaS simplification options drafted (Models A / B / C); user picked A. |
| **P167.6** | ✅ | Codex review caught 3 corrections (AZF Path-A-only, tmpfs scope, no clamping); applied. |
| **P167.7** | ✅ | Phase 168 plan drafted + codex-reviewed + corrections applied. |
| **P167.8** | ✅ | Consolidate to canonical docs (PLAN.md gets Phase 168 plan; temp `docs/POD_MODEL_*.md` files deleted). Self-caught: cloudrun also has Path B (filed as BUG-1054 added to scope). |
| **P167.9** | ◻ | User reviews plan + answers 6 pending decisions. |
| **P167.10** | ◻ | Open PR for Phase 167 (doc-only) once user is happy; user merges. |

## Phase 168 pending decisions (user, please answer)

3 confirmed in PLAN.md § Active phase (Q4 execStartViaInvoke ripped; Q6 cleanup failures propagate; Q7 FaaS pod lifetime hard limit). 6 remaining:

| # | Question | My proposal |
|---|---|---|
| Q1 | tmpfs default size | 2 GiB |
| Q2 | tmpfs exhaustion behaviour | fail loud + operator guidance ("raise SOCKERLESS_<BACKEND>_TMPFS_SIZE_MIB or switch SOCKERLESS_<BACKEND>_STORAGE_BACKING to a persistent driver") |
| Q3 | reverse-agent registration timeout in ContainerStart | 90 sec; env-var `SOCKERLESS_<BACKEND>_BOOTSTRAP_TIMEOUT_SEC` |
| Q5 | `pd-ephemeral` disposition | stays registered as opt-in (no namespace move); operator can `SOCKERLESS_<BACKEND>_STORAGE_BACKING=pd-ephemeral` |
| Q8 | tmpfs default scope (codex correction) | 3 backends only: cloudrun + cloudrun-functions + ACA (lambda + azf cloud platforms reject `BackingMemory`) |
| Q9 | tmpfs size validation (codex correction) | startup fail-loud if `TMPFS_SIZE_MIB > function_memory - reserved`; no silent clamping |

## Phase 168 ready-to-start checklist

Once user approves the plan:

1. Branch off `origin/main` (after Phase 167 merges).
2. File 9 BUGs at P168.0 (1046–1054):
   - 1046: Lambda Path B silent fallback
   - 1047: GCF Path B preferred default
   - 1048: ContainerStart doesn't wait for reverse-agent dial-back
   - 1049: tmpfs default + size config (scope = cloudrun + cloudrun-functions + ACA)
   - 1050: delete execStartViaInvoke files + CloudExecDriver parallel interface
   - 1051: tmpfs exhaustion guidance + startup size validation
   - 1052: cleanup-path strict error propagation
   - 1053: FaaS pod lifetime > platform max → fail loud at next exec
   - 1054 (added by self-check): cloudrun Path B (missed in initial Phase 167 analysis)
3. Implement P168.1..P168.10 per the plan in PLAN.md.
4. E2E test acceptance: 12-step CI job <60s wall-clock on lambda / gcf / azf / cloudrun.
5. Codex review pass before merge.

## Invariants snapshot (full list in STATUS.md)

- Never auto-merge; user merges every PR.
- Single-branch rule.
- File BUGs *before* fixing.
- Verify each significant chunk; don't batch fixes.
- **No fallbacks anywhere**: no silent substitution, no "best-effort with logging," no transparent re-invoke. If a primary path fails, surface it loudly to the operator.
- Driver pluggability preserved: each backend registers ONE driver per dimension; operator can swap; no primary-with-backup pairs.
- `gh` CLI is the reference adaptor for bleephub.
- Terraform provider call sequences differ from raw SDK — both test layers required.
- `specs/CLOUD_RESOURCE_MAPPING.md` is authoritative.

## Resumable tracks (longer-horizon)

- **Track A** — Live-cloud validation (one branch per cell).
- **Track B** — UI / TypeScript vibe-slop sweep (carried from Phase 161).
- **Track C** — Phase 91d (bookmarked; needs cloud capability change).
- **Track D** — Phase 166 follow-up gaps: GCP Cloud Functions Gen2 + Pub/Sub + Compute instance/template terraform coverage; Azure Key Vault data-plane terraform coverage. Filed informally; can become a Phase 169 if leverage materialises.

## Session-resume checklist

1. `git fetch origin && git checkout phase-167-pod-model-analysis && git pull` (or `git checkout main && git pull --ff-only` if 167 merged).
2. `git log --oneline -10`.
3. Read STATUS.md + this file + PLAN.md § Active phase (Phase 168 plan) + BUGS.md § Open.
4. Read [`.claude/skills/avoid-vibe-slop/SKILL.md`](.claude/skills/avoid-vibe-slop/SKILL.md) before any code change.
5. If user hasn't yet answered the 6 pending questions → wait or surface them again.
6. Once approved, start P168.0.

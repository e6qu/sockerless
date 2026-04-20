# Next Steps

## Current session — Phase C live-AWS session 2 (in progress)

**Branch:** `post-phase86-continuation` off `origin/main` at commit `7f054e0`.
**Plan:** `~/.claude/plans/purring-sprouting-dusk.md` (approved).
**AWS account:** `729079515331` (root), eu-west-1 + us-east-1 — **clean slate** confirmed (no residue from Session 1).

**Bug-logging rule:** Every failure surfaced during Phase C — runbook errors, terragrunt errors, e2e failures, teardown residue, anything unexpected — lands in `BUGS.md` with a BUG-NNN number before (or alongside) the fix. Root cause + reproduction + fix commit. No paper-overs.

### Staged execution

| Phase | What | Cost | Time |
|---|---|---|---|
| 0 | Preflight — fix TG_DIR in smoke scripts, bootstrap terraform state buckets + DynamoDB lock table, build backend + bootstrap binaries | $0 | 10min |
| 1 | ECS infra up (Runbook 0) — `terragrunt apply` in `terraform/environments/ecs/live/` | ~$0.01 + $0.05/hr | 7min |
| 2 | ECS smoke (Runbook 1) — docker run, logs, cross-container DNS, exec via SSM | — | 5min |
| 3 | Lambda infra up — `terragrunt apply` in `terraform/environments/lambda/live/` | ~$0 | 3min |
| 4 | Lambda baseline (Runbook 2) — docker run + logs + kill | ~$0.001 | 5min |
| 5 | **CONDITIONAL** — Lambda agent-as-handler (Runbook 3) needs a public callback URL (ngrok / cloudflared). Skip if unavailable. | ~$0.001 | 10min |
| 6 | E2E live tests — `tests/e2e-live-tests/github-runner/run.sh --mode live` + gitlab counterpart × ecs + lambda backends | ~$0.03 (smoke) to ~$0.08 (full matrix) | 30min to 2h |
| 7 | Teardown (Runbook 6) — `terragrunt destroy` both envs, residue audit, retain state buckets | — | 3min |
| 8 | Doc updates — write `_tasks/P86-AWS-manual-runbook-session2.md`, fill `docs/runner-capability-matrix.md` live columns, update PLAN/STATUS/WHAT_WE_DID/DO_NEXT, commit | $0 | 15min |

### Pause-points (user confirmation required)

- **A** after Phase 0 (scripts + state + binaries look right) before first `terragrunt apply`.
- **B** after Phase 1 (infra up, outputs sane).
- **C** after Phase 2 (per-command PASS/FAIL; any FAIL → bug fix before continuing).
- **D** before Phase 5 (tunnel strategy).
- **E** before Phase 6 (e2e breadth — smoke subset vs. full matrix).
- **F** after Phase 7 if any residue found.
- **G** before `/clear` after Phase 8.

### Session budget

| Scope | Time | Cost |
|---|---|---|
| Narrow (Phases 0-4 + 6 smoke + 7) | ~1h 30min | ~$0.15 |
| Full matrix (+ 5, + 6 full) | ~3h | ~$0.50 |

## After Phase C

Once Phase C closes out, Phase 86 is fully done. Candidates for the next session:

- **Phase 68** — Multi-Tenant Backend Pools. P68-001 done; P68-002→010 pending (pool registry, request router, concurrency limiter, lifecycle API, metrics, round-robin, resource limits, tests).
- **Phase 78** — UI Polish: dark mode, design tokens, error UX, container detail modal, auto-refresh, performance audit, accessibility, E2E smoke, documentation.

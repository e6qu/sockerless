# Sockerless — Status

**102 phases closed. 819 bugs tracked — 817 fixed, 2 open (BUG-804/806 libpod shape — queued for Phase 105 by maintainer). 1 false positive. Branch `round-8-bug-sweep` open with rounds 8 + 9 stacked on PR #118.**

See [PLAN.md](PLAN.md) (roadmap), [BUGS.md](BUGS.md) (bug log), [WHAT_WE_DID.md](WHAT_WE_DID.md) (narrative), [DO_NEXT.md](DO_NEXT.md) (resume pointer), [specs/](specs/) (architecture).

## Branch state

- **`round-8-bug-sweep`** — open as PR #118. Rounds 8 + 9 both stacked here per maintainer direction (single-branch-per-phase rule). All CI green at last commit; MERGEABLE.
- **`origin/main`** — clean at PR #117 merge. Local `main` synced.
- **`origin-gitlab/main`** — mirror, lags; pushed when convenient.

## Round-9 (in progress)

Per-test crosswalk of [PLAN_ECS_MANUAL_TESTING.md](PLAN_ECS_MANUAL_TESTING.md) against [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md), live ECS + Lambda. Working state in [docs/manual-test-spec-crosswalk.md](docs/manual-test-spec-crosswalk.md) (the file's `## Status` block names the next pending test, so post-compaction resume picks up cleanly).

ECS side **complete** — Tracks A/B/C/E/F/G/I (~80 tests). Bugs filed and fixed in-track: BUG-801, 803, 805, 813 (start polling), 789/798 (SSM exit-code marker), 815 (exec sh -c wrap), 816 (busybox find compat), 817 (stat tab format), 795 (filter substring match). 1 withdrawn (802 — measurement artifact). 2 queued for Phase 105 (804, 806 — libpod shape).

Lambda Track D **complete** — D1-D9 all pass with prebuilt overlay (`r9-overlay` pushed to `729079515331.dkr.ecr.eu-west-1.amazonaws.com/sockerless-live-lambda:r9-overlay`). Bugs filed and fixed in-track: BUG-807 (wait-for-Active waiter), 808 (PrebuiltOverlayImage independent of CallbackURL), 809 (ExecStart hijack-before-error), 810 (stale "loaded from disk" log), 811 (tag-based InvocationResult persistence + replay), 812 (LastModified RFC3339Nano conversion).

Phase 104 (cross-backend driver framework) drafted in PLAN.md as the next major work item; ships as the natural home for Phase 103 (overlay-rootfs) and operator-overridable per-cloud driver swaps (e.g. Kaniko for builds).

## Recent merges (compressed — full detail in [WHAT_WE_DID.md](WHAT_WE_DID.md))

| PR | Summary |
|---|---|
| #117 | Round-7 live-AWS sweep — 16 bugs (BUG-770..785) |
| #116 | Post-PR-#115 state-doc refresh |
| #115 | Phases 96/98/98b/99/100/101/102 + 13-bug audit sweep |
| #114 | Phase 91 ECS EFS volumes + BUG-735/736/737 |
| #113 | Phases 87/88 (CR Services + ACA Apps) + 89 (stateless audit) + 90 (no-fakes) |
| #112 | Phase 86 — sim parity + Lambda agent-as-handler + live-AWS ECS validation |

## Open work pointers

- **BUG-804/806**: libpod-shape divergences for `pod inspect` (returns array; libpod expects object) and `pod stop` (Errs serialization). Queued for Phase 105 by maintainer.
- **Phase 103**: overlay-rootfs bootstrap mode — ships under Phase 104 as alternate FSDiff/Commit drivers.
- **Phase 104**: cross-backend driver framework — design locked; piecemeal delivery, dimension at a time. See PLAN.md for the dimension list and refactor order.
- **Phase 105**: libpod-shape conformance.
- **GCP / Azure live runbooks** — terraform live envs to add, then port the round-7/8/9 sweep against each.

## Test counts (head of `round-8-bug-sweep`)

| Category | Count |
|---|---|
| Core unit | 312 |
| Cloud SDK/CLI | AWS 68, GCP 64, Azure 57 |
| Sim-backend integration | 77 |
| GitHub E2E | 186 |
| GitLab E2E | 132 |
| Terraform | 75 |
| UI/Admin/bleephub | 512 |
| Lint (18 modules) | 0 |
| Round-8 live-AWS manual sweep | 278 tests; 274 pass + 4 BUG-799 ghosts (now fixed) |
| Round-9 live-AWS manual sweep | ~90 tests; ECS A/B/C/E/F/G/I + Lambda D complete; coverage rows C12-C15 added |

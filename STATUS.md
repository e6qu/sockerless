# Sockerless — Status

**102 phases closed. 831 bugs tracked — 831 fixed, 0 open. 1 false positive. PR #118 merged (Rounds 8 + 9); PR #120 open with the post-merge audit + phase plan (BUG-802 withdrawn; BUG-638/640/646/648 backfilled as retroactively closed by BUG-788; BUG-804/806 libpod-shape; BUG-820..831 fallback / synthetic-data findings). Project rule (recorded as principle #9 in PLAN.md): no defer, no fakes, no fallbacks. Phase 104 (cross-backend driver framework) is the next active work, followed by Phases 106 (GitHub Actions runner) + 107 (GitLab runner) + 108 (sim feature parity).**

See [PLAN.md](PLAN.md) (roadmap), [BUGS.md](BUGS.md) (bug log), [WHAT_WE_DID.md](WHAT_WE_DID.md) (narrative), [DO_NEXT.md](DO_NEXT.md) (resume pointer), [specs/](specs/) (architecture).

## Branch state

- **`main`** — synced with `origin/main` at PR #119 merge (squash commit `b547ee9`).
- **`post-pr-118-bug-audit-and-phases`** — open as PR #120. Audit pass with 16 bug closures + phase plan refresh (Phases 106/107/108 added).
- **`origin-gitlab/main`** — mirror, lags; pushed when convenient.

## Round-8 + Round-9 (closed via PR #118)

Per-test crosswalk of [PLAN_ECS_MANUAL_TESTING.md](PLAN_ECS_MANUAL_TESTING.md) against [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md), live ECS + Lambda. Working state archived in [docs/manual-test-spec-crosswalk.md](docs/manual-test-spec-crosswalk.md).

- ECS Tracks A/B/C/E/F/G/I (~80 tests) — bugs fixed: 801, 803, 805, 813 (start polling), 789/798 (SSM exit-code marker), 815 (exec sh -c wrap), 816 (busybox find compat), 817 (stat tab format), 795 (filter substring match), 818 (sim ECS exec parser), 819 (terragrunt sweep parity).
- Lambda Track D (9 tests) — bugs fixed: 807 (wait-for-Active waiter), 808 (PrebuiltOverlayImage independent of CallbackURL), 809 (ExecStart hijack-before-error), 810 (stale "loaded from disk" log), 811 (tag-based InvocationResult persistence + replay), 812 (LastModified RFC3339Nano conversion).
- 2 queued for Phase 105 (804, 806 — libpod shape). 1 withdrawn (802 — measurement artifact).

AWS infra torn down post-merge. Root-account IAM key `AKIA2TQEGRDBRV2KFW6L` deactivation has to be done by the maintainer via the AWS Console (CLI can't manage root-account keys).

## Recent merges (compressed — full detail in [WHAT_WE_DID.md](WHAT_WE_DID.md))

| PR | Summary |
|---|---|
| #120 (open) | Post-PR-#118 bug audit + phase plan — 16 closures (BUG-802 + 638/640/646/648 retroactive + 804/806 libpod-shape + 820..829 fallback/synthetic-data audit). Phases 106/107/108 added. |
| #119 | Post-PR-#118 state-doc refresh — Phase 104 promoted to active. |
| #118 | Round-8 + Round-9 live-AWS sweep — 30 bugs (BUG-786..819), per-cloud terragrunt sweep parity |
| #117 | Round-7 live-AWS sweep — 16 bugs (BUG-770..785) |
| #116 | Post-PR-#115 state-doc refresh |
| #115 | Phases 96/98/98b/99/100/101/102 + 13-bug audit sweep |
| #114 | Phase 91 ECS EFS volumes + BUG-735/736/737 |

## Open work pointers

- **Phase 104** — Cross-backend driver framework. 13 dimension lifts; piecemeal delivery, sim parity per commit.
- **Phase 105 (rolling)** — libpod-shape conformance. First wave done (BUG-804/806). Remaining: cross-walk every other libpod handler against upstream shapes; add golden tests.
- **Phase 106** — Real GitHub Actions runner integration (post-Phase-104). Canonical workload sweep against ECS + Lambda first.
- **Phase 107** — Real GitLab runner integration (post-Phase-104). docker-executor; dind sub-test; kube-executor follow-up.
- **Phase 108** — Cross-simulator feature parity audit. Parity matrix + gap closure.
- **Phase 103**: overlay-rootfs bootstrap mode — ships under Phase 104 as alternate FSDiff/Commit drivers.
- **Phase 104**: cross-backend driver framework — design locked; piecemeal delivery, dimension at a time. See PLAN.md for the dimension list and refactor order.
- **Phase 105**: libpod-shape conformance.
- **GCP / Azure live runbooks** — terraform live envs to add, then port the round-7/8/9 sweep against each.

## Test counts (head of `main`)

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
| Round-8 + Round-9 live-AWS manual sweep | ~150 tests across both rounds; 30 bugs fixed (BUG-786..819); coverage rows C12-C15 added |

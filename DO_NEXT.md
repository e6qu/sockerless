# Do Next

Resume pointer for the next session / post-compaction. Updated after every task.

## Branch state

`round-8-bug-sweep` — PR #118 open. Rounds 8 + 9 stacked here per maintainer direction. CI green at last commit; MERGEABLE. Local `main` synced with `origin/main` at PR #117 merge.

## Up next (in execution order)

**Round-9 — all manual tests + bug fixes done; teardown remains.**
Live working state: [docs/manual-test-spec-crosswalk.md](docs/manual-test-spec-crosswalk.md). All ECS + Lambda tracks complete. The Lambda overlay image lives at `729079515331.dkr.ecr.eu-west-1.amazonaws.com/sockerless-live-lambda:r9-overlay`; cross-built binaries staged at `/tmp/r9-overlay/`.

1. **Round-9 wrap** — verify CI green on the latest push; teardown AWS (`terragrunt destroy` ECS + Lambda); deactivate IAM key `AKIA2TQEGRDBRV2KFW6L`; PR #118 ready for maintainer merge.

After round-9 closes, queued work picks up per [PLAN.md](PLAN.md):

- **Phase 104 — cross-backend driver framework.** Piecemeal delivery; design locked. Lift each dimension at a time, no behaviour change per commit. See PLAN.md for the dimension list and refactor order.
- **Phase 103 — overlay-rootfs bootstrap.** Ships under Phase 104 as alternate FSDiff/Commit drivers.
- **Phase 105 — libpod-shape conformance.** Independent of Phase 104; can run in parallel. Closes BUG-804 (`pod inspect` returns array), BUG-806 (`PodStopReport.Errs` shape).
- **Live-cloud runbooks** — GCP (Phase 87) + Azure (Phase 88) terraform live envs to add, then port the round-7/8/9 sweep against each.
- **Phase 68** — Multi-Tenant Backend Pools (P68-001 done; 9 sub-tasks remaining).
- **Phase 78** — UI Polish.

## Cross-links

- Roadmap: [PLAN.md](PLAN.md)
- Phase roll-up: [STATUS.md](STATUS.md)
- Narrative: [WHAT_WE_DID.md](WHAT_WE_DID.md)
- Bug log: [BUGS.md](BUGS.md)
- Architecture: [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md), [specs/BACKEND_STATE.md](specs/BACKEND_STATE.md), [specs/SOCKERLESS_SPEC.md](specs/SOCKERLESS_SPEC.md)
- Manual-test runbook: [PLAN_ECS_MANUAL_TESTING.md](PLAN_ECS_MANUAL_TESTING.md)
- **Round-9 working state:** [docs/manual-test-spec-crosswalk.md](docs/manual-test-spec-crosswalk.md) — read its `## Status` block for the next pending test.

## Operational state

- **Live AWS infra (eu-west-1):** ECS + Lambda **provisioned** for round-9; teardown at end.
- **IAM key** `AKIA2TQEGRDBRV2KFW6L` reactivated for round-9; **deactivate at end** of round per maintainer.
- **CI** PR #118 — pending verification on the Round-9 D-track + BUG-789/798/807..817/795 commit. Previous commit `c7afe38` was 9/10 (test failed on the BUG-813 flake — fixed in that commit, verified locally count=5).

# Do Next

Snapshot pointer for the next session. Updated after every task.

## Branch state

`round-8-bug-sweep` — PR #118 open with 13 bugs fixed (BUG-786..800), all 10 CI checks SUCCESS, MERGEABLE. **Round-9 manual sweep in progress on the same branch (commits will be additional fixes / doc updates).**

## Up next

**Round-9 manual sweep — per-test crosswalk against the spec.** Live working state: [docs/manual-test-spec-crosswalk.md](docs/manual-test-spec-crosswalk.md). That file's `## Status` block names the next pending test; resume from there.

Order of business:

1. **Continue the per-test walkthrough.** Each test runs, results recorded in the crosswalk file, mismatches filed as BUG-801..NNN. ECS Tracks A→B→C→E→F→G→I, then Lambda Track D with a prebuilt overlay image.
2. **Build the Lambda overlay image** before D-track. `agent/cmd/sockerless-lambda-bootstrap` → push to ECR → `SOCKERLESS_LAMBDA_PREBUILT_OVERLAY_IMAGE=<ecr-uri>`. (Track D uses option (b) per round-9 decision — see crosswalk file.)
3. **Add coverage-gap tests** to `PLAN_ECS_MANUAL_TESTING.md` after the walkthrough — see the crosswalk file's "Coverage gaps" section.
4. **Teardown after sweep** + new commit on this branch + PR #118 update + CI re-run.

After round-9 is closed, prior queued items resume:

- **Live-cloud burn-in** for GCP / Azure / Lambda (the runbooks for those are still the paper ones from earlier rounds).
- **BUG-721** SSM ack-format proper fix — needs live AWS agent diff (BUG-789/798 from round-8 may share root cause).
- **Phase 103** overlay-rootfs bootstrap for FaaS+CR+ACA — replaces Phase 98 `find -newer /proc/1` heuristic.
- **Phase 68** Multi-Tenant Backend Pools (P68-001 done; 9 sub-tasks remain in PLAN.md).
- **Phase 78** UI Polish.

## Cross-links

- Roadmap: [PLAN.md](PLAN.md)
- Phase roll-up: [STATUS.md](STATUS.md)
- Narrative: [WHAT_WE_DID.md](WHAT_WE_DID.md)
- Bug log: [BUGS.md](BUGS.md)
- Architecture: [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md), [specs/BACKEND_STATE.md](specs/BACKEND_STATE.md)
- Manual-test runbook: [PLAN_ECS_MANUAL_TESTING.md](PLAN_ECS_MANUAL_TESTING.md)
- **Round-9 working state:** [docs/manual-test-spec-crosswalk.md](docs/manual-test-spec-crosswalk.md)

## Operational state

- Local `main` at commit `f7ca1d2` (PR #117 merged, last clean state).
- Branch `round-8-bug-sweep` is **2 commits ahead** of `origin/main` (round-8 fix + CI patch); PR #118 open and green.
- `origin-gitlab/main` is a mirror, behind; push when convenient.
- Live AWS infra (eu-west-1): ECS + Lambda **provisioned** for round-9; teardown at end.

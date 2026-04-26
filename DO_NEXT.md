# Do Next

Resume pointer for the next session / post-compaction. Updated after every task.

## Branch state

`round-8-bug-sweep` — PR #118 open. Rounds 8 + 9 stacked here per maintainer direction. CI green at last commit; MERGEABLE. Local `main` synced with `origin/main` at PR #117 merge.

## Up next (in execution order)

**Round-9 manual sweep — Lambda Track D blocked on local Docker daemon.**
Live working state: [docs/manual-test-spec-crosswalk.md](docs/manual-test-spec-crosswalk.md). That file's `## Status` block names the next pending test.

1. **Build the Lambda overlay image.** Cross-built binaries already at `/tmp/r9-overlay/sockerless-{agent,lambda-bootstrap}` (linux/amd64) and Dockerfile staged. To resume:
   ```bash
   cd /tmp/r9-overlay && unset DOCKER_HOST
   docker build --platform linux/amd64 -t r9-lambda-overlay:latest .
   source /Users/zardoz/projects/sockerless/aws.sh
   aws ecr get-login-password --region eu-west-1 | docker login --username AWS --password-stdin 729079515331.dkr.ecr.eu-west-1.amazonaws.com
   docker tag r9-lambda-overlay:latest 729079515331.dkr.ecr.eu-west-1.amazonaws.com/sockerless-live-lambda:r9-overlay
   docker push 729079515331.dkr.ecr.eu-west-1.amazonaws.com/sockerless-live-lambda:r9-overlay
   ```
   Set `SOCKERLESS_LAMBDA_PREBUILT_OVERLAY_IMAGE=729079515331.dkr.ecr.eu-west-1.amazonaws.com/sockerless-live-lambda:r9-overlay` and restart the Lambda backend; D1-D9 run.

2. **A46 NotImpl wrapper** — translate the bootstrap-PID-file exit-64 case in `backends/ecs/ssm_ops.go::ContainerSignalViaSSM` into a clean `NotImplementedError` per spec. Currently surfaces as a generic `kill -STOP failed (exit -1):` (artifact of BUG-789/798).

3. **Coverage-gap test rows** in `PLAN_ECS_MANUAL_TESTING.md`. Per the crosswalk file's "Coverage gaps" section, add rows verifying `sockerless-restart-count` tag value, `sockerless-kill-signal` tag presence + exit-code mapping, ImagePush layer-byte content (non-public.ecr.aws sources), `Store.LayerContent` cache eviction behaviour.

4. **Round-9 wrap** — final state-doc commit; teardown AWS (terragrunt destroy ECS + Lambda); deactivate IAM key `AKIA2TQEGRDBRV2KFW6L`; CI re-run + verify green; PR #118 ready for maintainer merge.

After round-9 closes, queued work picks up per [PLAN.md](PLAN.md):

- **Phase 104 — cross-backend driver framework.** Piecemeal delivery; design locked. Lift each dimension at a time, no behaviour change per commit. See PLAN.md for the dimension list and refactor order.
- **Phase 103 — overlay-rootfs bootstrap.** Ships under Phase 104 as alternate FSDiff/Commit drivers.
- **Phase 105 — libpod-shape conformance.** Independent of Phase 104; can run in parallel. Closes BUG-804 (`pod inspect` returns array), BUG-806 (`PodStopReport.Errs` shape).
- **Live-cloud runbooks** — GCP (Phase 87) + Azure (Phase 88) terraform live envs to add, then port the round-7/8/9 sweep against each.
- **BUG-721 / BUG-789 / BUG-798** — live-AWS SSM frame parsing. Needs WS frame capture against a live exec session.
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
- **CI** PR #118 last commit `664d37c` (Round-9 Tracks E+F+G+I + BUG-805 fix) — 10/10 SUCCESS.

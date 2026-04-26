# Do Next

Resume pointer for the next session / post-compaction. Updated after every task.

## Branch state

`main` — current. Synced with `origin/main` at PR #118 merge (squash commit `204e25e`). No active feature branch.

## Up next (in execution order)

**Phase 104 — Cross-backend driver framework.** Design locked in [PLAN.md](PLAN.md); piecemeal delivery, one dimension at a time, no behaviour change per commit. First dimension to lift is `ExecDriver` since it's the smallest and the existing `core.Drivers.Exec` already exists — the work is to expand the `Drivers` struct into `DriverSet` with the 13 typed dimensions, add `DriverContext`, and migrate ExecDriver into the new shape with sim-parity tests.

Suggested order for the dimension lifts:
1. Skeleton: `backends/core/drivers/{types.go, set.go, override.go}` + DriverContext + Describe()-based NotImpl composition rule.
2. Lift `ExecDriver` (smallest; already exists in `core.Drivers.Exec`).
3. Lift `AttachDriver` (lift `core.AttachViaCloudLogs` into a typed CloudLogsReadOnlyAttach).
4. Lift `LogsDriver` (touchpoints already exist per backend; just typed).
5. `FSReadDriver`, `FSWriteDriver`, `FSDiffDriver`, `FSExportDriver` — currently in `backends/<cloud>/ssm_ops.go` / `reverse_agent.go` and similar.
6. `ProcListDriver`, `SignalDriver`, `StatsDriver`, `CommitDriver`, `BuildDriver`, `RegistryDriver`.

Phase 103 (overlay-rootfs bootstrap) ships under Phase 104 as alternate FSRead/FSWrite/FSDiff/FSExport/Commit drivers gated behind `SOCKERLESS_OVERLAY_ROOTFS=1`.

After Phase 104:

- **Phase 106 — Real GitHub Actions runner integration.** End-to-end `actions/runner` binary against sockerless via DOCKER_HOST. ECS + Lambda first; rest gated on Phase 104. Canonical workload sweep (matrix, services, artifacts, secrets, fail-fast).
- **Phase 107 — Real GitLab runner integration.** GitLab Runner docker-executor → sockerless. Same coverage shape as Phase 106. dind sub-test included. Kubernetes-executor as a follow-up under Phase 104.
- **Phase 108 — Cross-simulator feature parity audit.** Walk every cloud-API call sockerless makes; build a parity matrix (rows = SDK calls, columns = aws/gcp/azure sim); fix every gap in-phase per the no-defer rule.

Independent of Phase 104 (can run in parallel):

- **Phase 105 — libpod-shape conformance (rolling).** First wave landed: BUG-804 (`PodInspectResponse` mirrors `define.InspectPodData` + golden test) and BUG-806 (`PodActionResponse.Errs` normalised to `[]`; HTTP 409 + ErrorModel for failures). Remaining: cross-walk every other libpod handler against upstream shapes; add golden tests; verify against a real podman client.
- **Live-cloud runbooks** — GCP (Phase 87) + Azure (Phase 88) terraform live envs to add, then port the round-7/8/9 sweep against each. New per-cloud `null_resource sockerless_runtime_sweep` (BUG-819 fix) means destroys are self-sufficient.

## Manual step left for maintainer (post round-9)

Deactivate root-account access key `AKIA2TQEGRDBRV2KFW6L` via the AWS Console (`IAM → Security credentials → Access keys`). The CLI cannot deactivate root-account keys.

Other queued work per [PLAN.md](PLAN.md):

- **Phase 68** — Multi-Tenant Backend Pools (P68-001 done; 9 sub-tasks remaining).
- **Phase 78** — UI Polish.

## Cross-links

- Roadmap: [PLAN.md](PLAN.md)
- Phase roll-up: [STATUS.md](STATUS.md)
- Narrative: [WHAT_WE_DID.md](WHAT_WE_DID.md)
- Bug log: [BUGS.md](BUGS.md)
- Architecture: [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md), [specs/BACKEND_STATE.md](specs/BACKEND_STATE.md), [specs/SOCKERLESS_SPEC.md](specs/SOCKERLESS_SPEC.md)
- Manual-test runbook: [PLAN_ECS_MANUAL_TESTING.md](PLAN_ECS_MANUAL_TESTING.md)
- Round-9 working state (archive): [docs/manual-test-spec-crosswalk.md](docs/manual-test-spec-crosswalk.md)

## Operational state

- **Live AWS infra (eu-west-1):** torn down at PR #118 close. Future live runs need a fresh `terragrunt apply` (per-cloud sweeps mean teardown is self-sufficient now — BUG-819 fix).
- **IAM key** `AKIA2TQEGRDBRV2KFW6L`: **maintainer must deactivate via AWS Console** (root-account key, IAM API doesn't allow operator-issued status changes).
- **CI** at PR #118 merge: 10/10 PASS on commit `b39ce42`.

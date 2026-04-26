# Do Next

Resume pointer for the next session / post-compaction. Updated after every task.

## Branch state

- `main` synced with `origin/main` at PR #119 merge (squash commit `b547ee9`).
- `post-pr-118-bug-audit-and-phases` — **open as PR #120**, 19+ commits ahead of main. Cumulative: 22 bugs closed; Phase 104 skeleton + two dimension lifts (Exec + Attach); Phase 105 second + third waves of golden shape tests (7 handlers covered); **Phase 108 closed in-branch — 77/77 sim-parity matrix rows ✓** (33 AWS / 16 GCP / 28 Azure).

## Up next (in execution order)

**Active: PR #120 — audit + Phase 104 skeleton + Phase 105 + Phase 108 closure.** Per maintainer instruction the branch stays open while work continues. Audit + sim-parity tracks landed: 22 bugs closed (BUG-802 + 638/640/646/648 + 804/806 + 820..831 + 832/833/834/835), Phase 108 closed, Phase 104 lifts 1+2 done.

## Up next on this branch (in execution order)

1. **Phase 104 third dimension lift — `LogsDriver`.** Per-backend log-fetcher already exists today (`CloudLogFetchFunc` in `core.AttachViaCloudLogs`); lift it into a typed `LogsDriver104` so `docker logs <id>` flows through `DriverSet104.Logs`. Tests pin the contract.
2. **Phase 104 fourth dimension lift — `SignalDriver`.** SignalToExitCode exists (BUG-826 hardened SIGTERM=143/SIGKILL=137); typed `SignalDriver104` with WrapLegacy adapter + per-backend overrides.
3. **Phase 104 first per-backend migration** — wire the docker backend's exec call site through `DriverSet104.Exec` (using `WrapLegacyExec`). Smallest backend; no cloud round-trips; integration tests in `tests/exec_test.go` verify parity.
4. **Remaining Phase 104 lifts** — FSRead/Write/Diff/Export, Commit, Build, Stats, ProcList, Registry. Piecemeal one per commit; sim parity per commit.
5. **Phase 105 fourth wave** (lower priority) — events stream, exec start hijack shape, container CRUD beyond list. The first three waves already cover the high-risk handlers; this is rounding out coverage.

After Phase 104 reaches first-dimension parity (ExecDriver lifted across all 7 backends), the typed framework is ready for Phase 106 (GitHub Actions runner) and Phase 107 (GitLab runner) to exercise it against real CI workloads.

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
- ~~**Phase 108 — Cross-simulator feature parity audit.**~~ ✓ closed 2026-04-26 in PR #120 (BUG-832/833/834/835 fixes; 77/77 matrix rows ✓). Standing rule strengthened: any new SDK call added to a backend must update `specs/SIM_PARITY_MATRIX.md` + add the sim handler in the same commit.

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

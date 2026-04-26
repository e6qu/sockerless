# Do Next

Resume pointer for the next session / post-compaction. Updated after every task. Roadmap detail lives in [PLAN.md](PLAN.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md); bug log in [BUGS.md](BUGS.md).

## Branch state

- `main` synced with `origin/main` at PR #119 merge.
- **`post-pr-118-bug-audit-and-phases`** — open as PR #120, ~20 commits ahead. Cumulative: 22 bugs closed; Phase 104 skeleton + lifts 1+2 (Exec, Attach); Phase 105 waves 1-3 (8 libpod-shape handlers); Phase 108 closed (77/77 sim-parity matrix ✓).

## Up next on this branch

1. **Phase 104 lift 3 — `LogsDriver`.** Per-backend log-fetcher already exists (`CloudLogFetchFunc` in `core.AttachViaCloudLogs`); lift it into a typed `LogsDriver104` so `docker logs <id>` flows through `DriverSet104.Logs`. Tests pin the contract.
2. **Phase 104 lift 4 — `SignalDriver`.** SignalToExitCode hardened in BUG-826 (SIGTERM=143 / SIGKILL=137); typed `SignalDriver104` with WrapLegacy adapter + per-backend overrides.
3. **Phase 104 first per-backend migration** — wire docker backend's exec call site through `DriverSet104.Exec` (using `WrapLegacyExec`). Smallest backend, no cloud round-trips, integration tests in `tests/exec_test.go` verify parity.
4. **Remaining Phase 104 lifts** — FSRead/Write/Diff/Export, Commit, Build, Stats, ProcList, Registry. Piecemeal, one per commit, sim parity per commit.
5. **Phase 105 wave 4** (lower priority) — events stream, exec start hijack shape, container CRUD beyond list.

After this branch's Phase 104 work reaches first-dimension parity (Exec lifted across all 7 backends), the typed framework is ready for Phase 106/107 (real CI runners) to exercise it.

## Cross-links

- Roadmap: [PLAN.md](PLAN.md)
- Phase roll-up: [STATUS.md](STATUS.md)
- Narrative: [WHAT_WE_DID.md](WHAT_WE_DID.md)
- Bug log: [BUGS.md](BUGS.md)
- Architecture: [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md), [specs/BACKEND_STATE.md](specs/BACKEND_STATE.md), [specs/SOCKERLESS_SPEC.md](specs/SOCKERLESS_SPEC.md), [specs/SIM_PARITY_MATRIX.md](specs/SIM_PARITY_MATRIX.md)
- Manual-test runbook: [PLAN_ECS_MANUAL_TESTING.md](PLAN_ECS_MANUAL_TESTING.md)

## Operational state

- **Live AWS infra (eu-west-1)** torn down at PR #118 close; per-cloud sweeps mean re-apply + destroy are self-sufficient (BUG-819).
- **AWS root-account key `AKIA2TQEGRDBRV2KFW6L`** — deactivated by maintainer 2026-04-26. Ask the maintainer to reactivate before any future live-AWS test pass (Phase 106 ECS work, future round-10 sweeps).

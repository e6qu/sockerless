# Do Next

Resume pointer for the next session / post-compaction. Updated after every task. Roadmap detail lives in [PLAN.md](PLAN.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md); bug log in [BUGS.md](BUGS.md).

## Branch state

- `main` synced with `origin/main` at PR #119 merge.
- **`post-pr-118-bug-audit-and-phases`** — open as PR #120, ~37 commits ahead. Cumulative: 22 bugs closed; **Phase 104 framework migration complete** — all 13 typed adapters shipped, every dispatch site flows through `TypedDriverSet` (Exec, Attach, Logs, Signal, ProcList, FSDiff, FSRead, FSWrite, FSExport, Commit, Build, Registry); framework renamed to drop 104 suffix (`ExecDriver`, `AttachDriver`, `TypedDriverSet`); Phase 105 waves 1-3 (8 libpod-shape handlers); Phase 108 closed (77/77 sim-parity matrix ✓); manual-tests directory + repo-wide phase/bug-ref strip from code + docs.

## Up next on this branch

1. **Per-backend cloud-native typed driver overrides.** Now that every dispatch site flows through `TypedDriverSet`, replace legacy adapter defaults with cloud-native typed drivers slot-by-slot. Quick wins:
   - Lambda → `NewCloudLogsLogsDriver` for `Typed.Logs` (already shipped, just needs `s.Typed.Logs = ...` in Lambda's NewServer).
   - Cloud Run / GCF / ACA / AZF → same `NewCloudLogsLogsDriver` pattern.
   - Lambda / CR / GCF / AZF → `NewCloudLogsAttachDriver` for `Typed.Attach` (FaaS read-only attach).
2. **Phase 105 wave 4** (lower priority) — events stream, exec start hijack shape, container CRUD beyond list.
3. **Phase 106/107** — real GitHub Actions / GitLab Runner integration. Architecture in PLAN.md; needs scaffolding under `tests/runners/{github,gitlab}/`.

After typed-driver overrides land in at least one backend per cloud (Lambda for AWS, GCF for GCP, AZF for Azure), the framework has demonstrated cloud-native exit paths and is ready for Phase 106/107 to exercise it against real CI workloads.

## Cross-links

- Roadmap: [PLAN.md](PLAN.md)
- Phase roll-up: [STATUS.md](STATUS.md)
- Narrative: [WHAT_WE_DID.md](WHAT_WE_DID.md)
- Bug log: [BUGS.md](BUGS.md)
- Architecture: [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md), [specs/BACKEND_STATE.md](specs/BACKEND_STATE.md), [specs/SOCKERLESS_SPEC.md](specs/SOCKERLESS_SPEC.md), [specs/SIM_PARITY_MATRIX.md](specs/SIM_PARITY_MATRIX.md)
- Manual-test runbooks: [manual-tests/](manual-tests/)

## Operational state

- **Live AWS infra (eu-west-1)** torn down at PR #118 close; per-cloud sweeps mean re-apply + destroy are self-sufficient (BUG-819).
- **AWS root-account key `AKIA2TQEGRDBRV2KFW6L`** — deactivated by maintainer 2026-04-26. Ask the maintainer to reactivate before any future live-AWS test pass (Phase 106 ECS work, future round-10 sweeps).

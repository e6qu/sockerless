# Do Next

Resume pointer for the next session / post-compaction. Updated after every task. Roadmap detail lives in [PLAN.md](PLAN.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md); bug log in [BUGS.md](BUGS.md).

## Branch state

- `main` synced with `origin/main` at PR #119 merge.
- **`post-pr-118-bug-audit-and-phases`** — open as PR #120, ~35 commits ahead. Cumulative: 22 bugs closed; Phase 104 skeleton + all 13 typed adapters; framework renamed to drop 104 suffix (`ExecDriver`, `AttachDriver`, `TypedDriverSet`); 11 dispatch sites migrated to `TypedDriverSet` (Logs, Signal, ProcList, FSDiff, FSRead, FSWrite, FSExport, Commit, Build, Registry-Pull ×3, Registry-Push ×2); Phase 105 waves 1-3 (8 libpod-shape handlers); Phase 108 closed (77/77 sim-parity matrix ✓); manual-tests directory + repo-wide phase/bug-ref strip from code + docs.

## Up next on this branch

1. **Exec + Attach dispatch migration.** `BaseServer.ExecStart` returns `io.ReadWriteCloser`; the typed `ExecDriver.Exec` writes to a passed `conn`. Different control flow — the handler currently hijacks then copies; the typed driver needs the hijacked conn handed in. Re-architect `handleExecStart` to hijack first, then dispatch via `s.Typed.Exec.Exec(dctx, opts, conn)`. Same shape for Attach.
2. **Per-backend cloud-native typed driver overrides.** Now that all dispatch sites flow through `TypedDriverSet`, replace the legacy adapter defaults with real typed cloud drivers — e.g. ECS Logs → CloudWatch streaming; Lambda Logs → CloudWatch via the existing `NewCloudLogsLogsDriver`; FaaS attach → already wired adapter just needs slot replacement.
3. **Phase 105 wave 4** (lower priority) — events stream, exec start hijack shape, container CRUD beyond list.

After this branch's typed framework reaches first-dimension parity across all 7 backends (each backend overrides at least one slot with a cloud-native typed driver), the framework is ready for Phase 106/107 (real CI runners) to exercise it.

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

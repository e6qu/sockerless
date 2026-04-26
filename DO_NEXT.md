# Do Next

Resume pointer for the next session / post-compaction. Updated after every task. Roadmap detail lives in [PLAN.md](PLAN.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md); bug log in [BUGS.md](BUGS.md).

## Branch state

- `main` synced with `origin/main` at PR #119 merge.
- **`post-pr-118-bug-audit-and-phases`** — open as PR #120, ~37 commits ahead. Cumulative: 22 bugs closed; **Phase 104 framework migration complete** — all 13 typed adapters shipped, every dispatch site flows through `TypedDriverSet` (Exec, Attach, Logs, Signal, ProcList, FSDiff, FSRead, FSWrite, FSExport, Commit, Build, Registry); framework renamed to drop 104 suffix (`ExecDriver`, `AttachDriver`, `TypedDriverSet`); Phase 105 waves 1-3 (8 libpod-shape handlers); Phase 108 closed (77/77 sim-parity matrix ✓); manual-tests directory + repo-wide phase/bug-ref strip from code + docs.

## Up next on this branch

1. **Per-backend cloud-native overrides for the remaining dimensions.** Logs + Attach are done across all 6 cloud backends. Remaining slots that have cloud-native paths to wire:
   - **Exec** — ECS via SSM ExecuteCommand; FaaS+CR+ACA via the existing reverse-agent narrow drivers (already in `s.Drivers.Exec`; just need a typed wrapper that calls them directly).
   - **FS*** — same pattern: ECS via SSM tar/find/stat; FaaS via reverse-agent.
   - **Signal** — ECS via SSM kill; FaaS via reverse-agent kill.
   - **Commit / Build / Registry / ProcList** — per-cloud paths exist (CodeBuild, CloudBuild, ACR Tasks, etc.); typed wrappers slot in.
2. **Phase 105 wave 4** (lower priority) — events stream, exec start hijack shape, container CRUD beyond list.
3. **Phase 106/107** — real GitHub Actions / GitLab Runner integration. Architecture in PLAN.md; needs scaffolding under `tests/runners/{github,gitlab}/`.

The typed driver framework is now demonstrably operational across all 6 cloud backends for the streaming dimensions (Logs, Attach). Phase 106/107 can start in parallel — runner integration doesn't depend on more dimensions being lifted.

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

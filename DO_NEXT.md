# Do Next

Resume pointer for the next session / post-compaction. Updated after every task. Roadmap detail lives in [PLAN.md](PLAN.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md); bug log in [BUGS.md](BUGS.md).

## Branch state

- `main` synced with `origin/main` at PR #119 merge.
- **`post-pr-118-bug-audit-and-phases`** — open as PR #120, ~37 commits ahead. Cumulative: 22 bugs closed; **Phase 104 framework migration complete** — all 13 typed adapters shipped, every dispatch site flows through `TypedDriverSet` (Exec, Attach, Logs, Signal, ProcList, FSDiff, FSRead, FSWrite, FSExport, Commit, Build, Registry); framework renamed to drop 104 suffix (`ExecDriver`, `AttachDriver`, `TypedDriverSet`); Phase 105 waves 1-3 (8 libpod-shape handlers); Phase 108 closed (77/77 sim-parity matrix ✓); manual-tests directory + repo-wide phase/bug-ref strip from code + docs.

## Up next on this branch

1. **Wrapper removal + interface tightening (post-migration).** Now that every cloud backend has cloud-native typed drivers wired for the major dimensions, the legacy `WrapLegacyXxx` / `LegacyXxxFn` scaffolding in `backends/core/driver_adapt_*.go` becomes removable. Drop it together with the equivalent shrinkage of api.Backend (a coordinated landing, not piecemeal). Then tighten the typed interfaces: typed enums for Signal/RestartCondition, structured Stats / Build options instead of `map[string]any` / `api.ImageBuildOptions`-pass-through, `ImageRef` domain type for registry refs, etc. Tracked in PLAN.md § Phase 104.
2. **Remaining cloud-native typed dimensions.** Commit/Build/Registry/Stats typed cloud paths — Commit via reverse-agent (existing `CommitContainerViaAgent`), Build via per-cloud build service typed wrappers, Registry via typed cloud-registry impls, Stats via cloud metrics endpoints. Lower priority since these are not on the runner-integration hot path.
3. **Phase 106/107 live-cloud runs.** Harnesses are in place (`tests/runners/{github,gitlab}/`); next step is provisioning live ECS via `manual-tests/01-infrastructure.md` and running the harness end-to-end against a real GitHub repo + GitLab project. First findings get filed as bugs.
4. **Phase 105 wave 4** (lower priority) — events stream, exec start hijack shape, container CRUD beyond list.

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

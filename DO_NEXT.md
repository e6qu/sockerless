# Do Next

Resume pointer for the next session / post-compaction. Updated after every task. Roadmap detail lives in [PLAN.md](PLAN.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md); bug log in [BUGS.md](BUGS.md).

## Branch state

- `main` synced with `origin/main` at PR #119 merge.
- **`post-pr-118-bug-audit-and-phases`** — open as PR #120, ~37 commits ahead. Cumulative: 22 bugs closed; **Phase 104 framework migration complete** — all 13 typed adapters shipped, every dispatch site flows through `TypedDriverSet` (Exec, Attach, Logs, Signal, ProcList, FSDiff, FSRead, FSWrite, FSExport, Commit, Build, Registry); framework renamed to drop 104 suffix (`ExecDriver`, `AttachDriver`, `TypedDriverSet`); Phase 105 waves 1-3 (8 libpod-shape handlers); Phase 108 closed (77/77 sim-parity matrix ✓); manual-tests directory + repo-wide phase/bug-ref strip from code + docs.

## Up next on this branch

1. **Wrapper removal + interface tightening (post-migration cleanup).** Two coordinated changes:
   - Either give docker the same cloud-native typed driver treatment (calling the docker SDK from a typed driver), or accept that "legacy adapter wrapping s.self" is the permanent path for backends where the api.Backend method *is* the cloud-native impl. Once decided, drop unused `WrapLegacyXxx` / `LegacyXxxFn` scaffolding accordingly and shrink api.Backend.
   - **Add `core.ImageRef` domain type** (`{Domain, Path, Tag, Digest}` + `ParseImageRef` + `String()`) and migrate the typed Registry interface + the 10+ ad-hoc parse sites (`backends/core/{registry.go,backend_impl.go,handle_docker_api.go}`, `backends/docker/backend_impl.go`, `backends/aws-common/build.go`, etc.). Same shape: typed enums for Signal / RestartCondition. Tracked in PLAN.md § Phase 104 "Stronger type safety".
2. **Phase 106/107 live-cloud runs.** Harnesses exist (`tests/runners/{github,gitlab}/`). Next step: reactivate AWS root-account key (currently deactivated; see [STATUS.md](STATUS.md)), provision live ECS via [manual-tests/01-infrastructure.md](manual-tests/01-infrastructure.md), and run the harness end-to-end against a real GitHub repo + GitLab project. First findings get filed as bugs.
3. **Phase 105 wave 4** (lower priority) — events stream, exec start hijack shape, container CRUD beyond list.

The runner-integration hot path (Logs/Attach/Exec/Signal/FS/Commit) is fully cloud-native typed across the cloud backends. Build/Registry/Stats stay on legacy adapters — their api.Backend methods already are the cloud-native paths, so a typed wrapper would just re-route the same call.

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

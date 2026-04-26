# Do Next

Resume pointer for the next session / post-compaction. Updated after every task. Roadmap detail lives in [PLAN.md](PLAN.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md); bug log in [BUGS.md](BUGS.md).

## Branch state

- `main` synced with `origin/main` at PR #119 merge.
- **`post-pr-118-bug-audit-and-phases`** — open as PR #120, ~50 commits ahead. Cumulative state:
  - 22 bug closures (BUG-802 + 638/640/646/648 retro + 804/806 + 820..831 + 832..835).
  - **Phase 104 framework migration complete** — all 13 typed adapters shipped, every dispatch site flows through `TypedDriverSet` (Exec, Attach, Logs, Signal, ProcList, FSDiff, FSRead/Write/Export, Commit, Build, Registry). Framework renamed to drop the temporary `104` suffix (`ExecDriver`, `AttachDriver`, `TypedDriverSet`).
  - **Cloud-native typed drivers across every backend.** 44/91 cells in the per-backend matrix are cloud-native, bypassing api.Backend; the rest stay on legacy adapters whose api.Backend method already does the cloud-native thing. Matrix: [specs/DRIVERS.md](specs/DRIVERS.md).
  - **`core.ImageRef`** typed domain object lands at the typed Registry boundary — first instance of the interface-tightening track.
  - Phase 105 waves 1-3 (8 libpod-shape handlers).
  - Phase 108 closed (77/77 sim-parity matrix ✓).
  - Phase 106/107 harnesses scaffolded (build-tag-gated; live runs pending operator key reactivation).
  - manual-tests directory + repo-wide phase/bug-ref strip from code + docs.

## Up next on this branch

1. **Wrapper removal pass.** Decide: either give docker typed cloud-native drivers (calling docker SDK directly from typed drivers) so every cell in the matrix is cloud-native, or accept that "legacy adapter wrapping s.self" is permanent for backends whose api.Backend method *is* the cloud-native impl. Once decided, drop unused `WrapLegacyXxx` / `LegacyXxxFn` scaffolding and shrink api.Backend correspondingly. Coordinated landing.
2. **Further interface tightening.** ImageRef proves the pattern. Next:
   - **Typed Signal enum** — `core.Signal` constants for SIGTERM/SIGKILL/etc., `SignalDriver.Kill(dctx, Signal)` instead of `string`. Parser at the handler boundary.
   - **`ResolveImageReg(ImageRef) (RegistryConfig, error)`** helper to migrate the registry-resolution call sites (`registry.go`, `image_manager.go`, `backend_impl_ext.go`, etc.) from `splitImageRefRegistry` onto the typed ImageRef path. Adds the docker-hub default rewrites the bare `ParseImageRef` parser intentionally skips.
   - **Structured `Stats` struct** instead of map[string]any-shaped JSON.
3. **Phase 106/107 live-cloud runs.** Reactivate AWS root-account key (see [STATUS.md](STATUS.md)), provision live ECS via [manual-tests/01-infrastructure.md](manual-tests/01-infrastructure.md), run the GH harness against a real repo + GitLab harness against the `origin-gitlab` mirror. First findings get filed as bugs.
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

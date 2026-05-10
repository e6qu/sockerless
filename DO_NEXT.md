# Do Next

Roadmap [PLAN.md](PLAN.md) · status [STATUS.md](STATUS.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Branch

`phase-79-topology-store`. PR #137 merged 2026-05-10 (Phase 78 polish + Phase 79 step 1 — Instance type). New branch carries Phase 79 step 2 onward.

## Resume here

**Phase 79 complete on this branch (PR #138). Phase 80 next.**

Phase 79 ships in PR #138:
- ✓ Step 1: `Instance` type (in #137).
- ✓ Step 2: `Topology` struct + YAML store at `sockerless.yaml` + `MigrateLegacyProjects`.
- ✓ Step 3: `TopologyManager` singleton + read/write REST surface (`/api/v1/topology`).
- ✓ Step 4: `make/components.mk` granular targets (`start-component`, `stop-component`, `rebuild-component`, `logs-component`, `status-components`, `stop-components`); `stack-X-Y` macros rewritten as composition of `rebuild-component` + `start-component`.
- ✓ Step 5: `TopologyManager.AllocatePort` walks the configured pool, skips claimed + in-use ports.
- ✓ Step 6: lifecycle REST endpoints (`POST /api/v1/topology/projects/{p}/instances/{i}/{start|stop|rebuild}`) shell `make {start,stop,rebuild}-component`. Per-instance config map serialised to `.stack-pids/<name>.env` and passed via `ENV_FILE=` so admin can hand any env vars to the component without admin needing to know what they mean.

**Next: Phase 80 — admin UI: topology page + per-instance lifecycle.** Replace ProjectsPage with a project + instance tree; per-instance Start/Stop/Rebuild controls; "Add instance" form (kind + name + port + per-component config); edit/delete; port registry view (allocated + free ranges).

## Invariants (re-state on every commit)

- **Components stay decoupled.** No admin-required env vars on sims/backends/bleephub. Admin reads only what they already expose (`/v1/health`, `/v1/info`, env vars). For Phase 87 observability: components emit OTLP only when `OTEL_EXPORTER_OTLP_ENDPOINT` is set in their env. Unset = today's stdout behaviour.
- **No fallbacks.** Unknown config values fail-loud. No silent defaults.
- **CI green per commit.** Each commit must be independently testable.
- **Test target gating.** All backend integration tests require `SOCKERLESS_TEST_TARGET=sim|cloud` (no skip).
- **No docs-only PRs.** Pair docs updates with implementation work on the same branch / PR.

## Roadmap on this branch

Phases 79.2 → 80 → 81 → 82 → 83 → 84 → 85 → 86 → 87. See [PLAN.md](PLAN.md) for sub-steps. Will likely split into multiple PRs once the natural seam appears (e.g. after Phase 86 ships, Phase 87 — observability — is independent and can land as its own PR).

## After this branch

- Phases 91–94 — real per-cloud volume provisioning (lifts `emptyDir` → real `pd-ephemeral` / `efs-ephemeral` / `azure-files-ephemeral`).
- Live-cloud validation track — Lambda live, Cloud Run Services + ACA Apps live, AZF cloud-dns + Lambda service-mesh + ACA/AZF Azure AD live.

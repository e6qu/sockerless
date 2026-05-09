# Do Next

Roadmap [PLAN.md](PLAN.md) · status [STATUS.md](STATUS.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Branch

`phase-79-topology-store`. PR #137 merged 2026-05-10 (Phase 78 polish + Phase 79 step 1 — Instance type). New branch carries Phase 79 step 2 onward.

## Resume here

**Phase 79 step 4: granular `make` targets.**

State of play on this branch:
- ✓ Step 1: `Instance` type (in #137).
- ✓ Step 2: `Topology` struct + YAML store + `MigrateLegacyProjects`.
- ✓ Step 3: `TopologyManager` singleton + REST endpoints (`GET /api/v1/topology`, `PUT /api/v1/topology`, `GET /api/v1/topology/instances`, `GET /api/v1/topology/projects/{project}/instances/{instance}`). Bootstrap calls `LoadOrMigrate` so admin reads the yaml or migrates from legacy on first start.

Next (step 4):

1. Add `make/components.mk` with granular targets:
   - `make start-component KIND=sim CLOUD=aws NAME=my-sim PORT=4500`
   - `make stop-component NAME=my-sim`
   - `make rebuild-component KIND=sim CLOUD=aws`
   - `make logs-component NAME=my-sim`
2. Existing `make stack-X-Y` targets in `make/stack.mk` rewrite as wrappers that compose `start-component` calls.
3. PID + log files keyed by component name in `.stack-pids/<name>.{pid,log}`.

Then 79.5 (free-port helper + auto-allocation) and 79.6 (admin lifecycle endpoints invoke the new make targets) per [PLAN.md](PLAN.md).

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

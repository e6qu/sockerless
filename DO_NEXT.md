# Do Next

Roadmap [PLAN.md](PLAN.md) · status [STATUS.md](STATUS.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Branch

`main` (clean). PR #138 merged 2026-05-10 (Phase 79 complete + Phase 87 plan + cloud-resource-mapping consolidation). Start a new branch for Phase 80.

## Resume here

**Phase 80 — admin UI: topology page + per-instance lifecycle.**

Backend surface (already shipped in #138):
- `GET /api/v1/topology` — full topology read.
- `PUT /api/v1/topology` — full topology replace (validated).
- `GET /api/v1/topology/instances` — flat list of `{project, instance}` refs.
- `POST /api/v1/topology/projects` / `DELETE /api/v1/topology/projects/{p}` — surgical project add/remove.
- `POST /api/v1/topology/projects/{p}/instances` / `GET|PUT|DELETE /api/v1/topology/projects/{p}/instances/{i}` — surgical instance CRUD.
- `POST /api/v1/topology/projects/{p}/instances/{i}/{start|stop|rebuild}` — lifecycle (shells `make`).
- `GET /api/v1/topology/projects/{p}/instances/{i}/status` — per-instance status (running, PID, health).
- `POST /api/v1/topology/allocate-port` — allocate a free port from `ports.ranges[<kind>]`.

Phase 80 build (UI only):
1. Replace `ProjectsPage` in admin UI with project + instance tree view.
2. Per-instance Start / Stop / Rebuild buttons (POST to lifecycle endpoints, toast on success/failure).
3. "Add project" + "Add instance" forms — instance form renders per-kind fields (sim → cloud + port; backend → cloud + backend kind + sim-port + port; bleephub → port). Auto-allocate-port button.
4. Edit / delete per instance + per project (with confirmation).
5. Port registry view — show `ports.ranges` configured ranges + which ports are claimed by which instance.
6. Per-instance status row (running / health) via the status endpoint, polled every 2s when the instance row is expanded.

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

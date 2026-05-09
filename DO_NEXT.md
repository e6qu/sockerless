# Do Next

Roadmap [PLAN.md](PLAN.md) · status [STATUS.md](STATUS.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Branch

`phase-79-topology-store`. PR #137 merged 2026-05-10 (Phase 78 polish + Phase 79 step 1 — Instance type). New branch carries Phase 79 step 2 onward.

## Resume here

**Phase 79 step 2: `sockerless.yaml` topology store.**

State of play after #137:
- `cmd/sockerless-admin/instance.go` — `Instance` type + per-kind `Validate` + `DeriveLegacyInstances`.
- Existing `ProjectConfig` still uses one-JSON-per-project at `~/.sockerless/admin/projects/*.json` and the SimPort + BackendPort tuple shape.

Next:

1. Add `Topology` struct (`{ Projects []ProjectConfig; Ports PortConfig }`) and YAML marshaller in `cmd/sockerless-admin/topology_store.go`.
2. Each `ProjectConfig` gains `Instances []Instance` (additive — old SimPort/BackendPort fields stay for back-compat).
3. Loader logic: if `./sockerless.yaml` exists → use it. Else if `~/.sockerless/admin/projects/*.json` exist → read them, derive instances via `DeriveLegacyInstances`, write `./sockerless.yaml`, leave the JSONs alone for now.
4. Tests: round-trip YAML, legacy-migration produces expected instance list.

Then phases 79.3–79.6 (REST endpoints, make targets, port allocator, migration) per [PLAN.md](PLAN.md).

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

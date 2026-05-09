# Do Next

Roadmap [PLAN.md](PLAN.md) · status [STATUS.md](STATUS.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Branch

`docs/state-save-post-121b-finish` (PR #137). Per user direction this PR keeps growing — Phase 78 (UI polish, complete) + Phase 79+ (admin orchestration, in progress) all land here.

## Resume here

**Phase 79 step 2: `sockerless.yaml` topology store.**

Where we are: `Instance` type + tests landed (`cmd/sockerless-admin/instance.go`, `instance_test.go`). Existing `ProjectConfig` still uses one-JSON-per-project at `~/.sockerless/admin/projects/*.json` and the SimPort + BackendPort tuple shape.

Next:

1. Add `Topology` struct (`{ Projects []ProjectConfig; Ports PortConfig }`) and YAML marshaller in `cmd/sockerless-admin/topology_store.go`.
2. Each `ProjectConfig` gains `Instances []Instance` (additive — old SimPort/BackendPort fields stay for back-compat).
3. Loader logic: if `./sockerless.yaml` exists → use it. Else if `~/.sockerless/admin/projects/*.json` exist → read them, derive instances via `DeriveLegacyInstances`, write `./sockerless.yaml`, leave the JSONs alone for now.
4. Tests: round-trip YAML, legacy-migration produces expected instance list.

Then phases 79.3–79.6 (REST endpoints, make targets, port allocator, migration) per [PLAN.md](PLAN.md).

## Invariants (re-state on every commit)

- **Components stay decoupled.** No admin-required env vars on sims/backends/bleephub. Admin reads only what they already expose (`/v1/health`, `/v1/info`, env vars).
- **No fallbacks.** Unknown config values fail-loud. No silent defaults.
- **CI green per commit.** Each commit on PR #137 must be independently testable.
- **Test target gating.** All backend integration tests require `SOCKERLESS_TEST_TARGET=sim|cloud` (no skip).

## After PR #137

- Phases 91–94 — real per-cloud volume provisioning (lifts `emptyDir` → real `pd-ephemeral` / `efs-ephemeral` / `azure-files-ephemeral`).
- Live-cloud validation track — Lambda live, Cloud Run Services + ACA Apps live, AZF cloud-dns + Lambda service-mesh + ACA/AZF Azure AD live.

See [PLAN.md](PLAN.md) for full sub-task lists.

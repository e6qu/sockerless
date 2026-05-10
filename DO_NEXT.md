# Do Next

Roadmap [PLAN.md](PLAN.md) · status [STATUS.md](STATUS.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Branch

`phase-84-instance-state-isolation` — Phase 84 work in flight (3 implementation commits + state save). Open a PR when ready; PR #141 already merged.

## Status

Phase 84 implementation is done on this branch. After it lands, **Phase 85 — config edit + hot reload** is the next pickup. See bottom of this file for the Phase 85 brief.

## Resume here — Phase 84 (per-instance state isolation) — implementation done

Branch has 5 implementation commits + state save ready to PR:

1. `phase 84 / BUG-985: sim NewServer fails loud on persistence open` — sim shared `NewServer(cfg) *Server` → `(*Server, error)`.
2. `phase 84: admin injects SIM_DATA_DIR per topology instance` — `InstanceLifecycle.Start` gains `project string`, new `managedEnvFor` + `mergeConfig` helpers, 5 admin tests.
3. `phase 84: multi-instance isolation tests across 3 sims` — 5 test cases × 3 clouds = 15 tests.
4. `phase 84 / BUG-986: MakeStore fails loud on per-table failure` — log.Fatalf inside `MakeStore` so half-persistent state isn't possible across a restart.
5. `phase 84: make purge-state operator targets` — `make purge-state PROJECT=<p> NAME=<i>` and `make purge-state-all`.

When ready: PR opened as #142 (already pushed). CI cycle ~7 min.

**What this does NOT change.** No refactor of `Persist` / `OpenDB`. No `start-component` change in make/components.mk — the env file already flows through.

## Phase 85 — Config edit + hot reload (next pickup)

**Goal.** Admin UI can edit instance config and trigger reload-or-restart based on whether the touched key is hot-reloadable.

**Today.** `Instance.Config` is a `map[string]string` rendered into `.stack-pids/<n>.env` at start. Edits via the UI today require a manual stop + edit + start cycle — there's no in-place reload.

**Phase 85 deliverables.**

1. **Config-key annotation.** Admin-side metadata: per-key entry `{name, hot_reloadable: bool, restart_required: bool, doc: string}`. Lives in admin code (not on the component) — per the components-decoupled invariant, admin owns the operator's mental model.
2. **Edit endpoint.** `PUT /api/v1/topology/projects/{p}/instances/{i}/config` writes back to `sockerless.yaml` via `TopologyManager.UpdateInstance`. Returns `{updated, hot_reloadable_changes, restart_required_changes}` so the UI can decide.
3. **Reload endpoint.** `POST /api/v1/topology/projects/{p}/instances/{i}/reload` re-renders the env file and signals the component (SIGHUP for processes that handle it; otherwise no-op + return error). Components without a SIGHUP handler return 501 — the UI falls through to a restart prompt.
4. **UI.** Edit modal on `/ui/topology` that reads annotation metadata, marks restart-required keys, applies the right action.

**Out of scope.** Don't add SIGHUP handling to components in this phase — the reload endpoint just signals; whether the component does anything is its concern. Phase 85's contract is "admin sends the signal", not "components hot-reload everything".

**Files to touch (rough):**

- `cmd/sockerless-admin/config_metadata.go` (new) — per-key metadata table.
- `cmd/sockerless-admin/api_topology.go` — add the two endpoints.
- `cmd/sockerless-admin/instance_lifecycle.go` — `Reload(inst)` shells `make reload-component` or sends SIGHUP via PID file.
- `make/components.mk` — `reload-component KIND= NAME=` target.
- `ui/packages/admin/src/pages/TopologyPage.tsx` + `InstanceForm.tsx` — surface the annotated config + reload-vs-restart action.

## Invariants (re-state on every commit)

- **Components stay decoupled.** No admin-required env vars on sims/backends/bleephub. Admin reads only what they already expose (`/v1/health`, `/v1/info`, env vars). For Phase 87 observability: components emit OTLP only when `OTEL_EXPORTER_OTLP_ENDPOINT` is set in their env. Unset = today's stdout behaviour.
- **No fallbacks.** Unknown config values fail-loud. No silent defaults.
- **CI green per commit.** Each commit must be independently testable.
- **Test target gating.** All backend integration tests require `SOCKERLESS_TEST_TARGET=sim|cloud` (no skip).
- **No docs-only PRs.** Pair docs updates with implementation work on the same branch / PR.

## Roadmap

Phases 79.2 → 80 → 81 → 82 ✓ all in #140. Next: 83 → 84 → 85 → 86 → 87. After 87: 91–94 (real per-cloud volume provisioning) + the live-cloud validation track (Lambda live, Cloud Run Services / ACA Apps live, AZF cloud-dns live, Lambda service-mesh live, ACA/AZF Azure AD live). See [PLAN.md](PLAN.md) for sub-steps. Will likely split into multiple PRs once natural seams appear (e.g. Phase 87 — observability — is independent and can land standalone).

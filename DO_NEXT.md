# Do Next

Roadmap [PLAN.md](PLAN.md) · status [STATUS.md](STATUS.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Branch

`phase-85-config-edit-hot-reload` — Phase 85 work in flight (2 implementation commits + state save). Open a PR when ready; PR #142 already merged.

## Status

Phase 85 implementation is done on this branch. After it lands, **Phase 86 — health + supervision surface** is the next pickup. See bottom of this file for the Phase 86 brief.

## Resume here — Phase 85 (config edit + hot reload) — implementation done

Branch has 2 implementation commits + state save ready to PR:

1. `phase 85: config edit + reload endpoints + curated metadata` — three pieces in one commit (tightly coupled): `config_metadata.go` curated table + `ClassifyChanges` helper, `PUT /api/v1/topology/projects/{p}/instances/{i}/config` (writes via `UpdateInstance`, returns classification), `POST /api/v1/topology/projects/{p}/instances/{i}/reload` (shells new `make reload-component`, kill -HUP via PID file). 9 unit tests + 7 endpoint tests.
2. `phase 85: ConfigEditModal + per-row hot/restart badges` — new `<ConfigEditModal>` opens from a "config" button on InstanceRow. Per-row hot/restart badges. Save → server classifies → footer offers Reload / Reload (partial) + Restart / Close based on what changed. 6 vitest cases.

When ready: `git push -u origin phase-85-config-edit-hot-reload && gh pr create`. CI ~7 min.

**What this does NOT change.** No SIGHUP handling on components — Phase 85 ships the signal path only; component absorption is per-binary. No InstanceForm refactor — full-instance edit (name/kind/cloud/port/sim) stays as-is; ConfigEditModal handles the config-only flow because the metadata-driven UX only makes sense in that narrow context.

## Phase 86 — Health + supervision surface (next pickup)

**Goal.** Admin marks an instance unhealthy when its process exits, when `/v1/health` returns non-2xx, or when the probe doesn't complete within 5 s. UI surfaces the failing signal + last-N log lines + diagnostic links. **No auto-restart** — operator-driven recovery.

**Today.** `instance_status.go` already implements `readInstanceStatus(inst)` returning `{Project, Name, Running, PID, Health, HealthDetail}`:

- `Running`: signal-0 PID probe (per-OS).
- `Health`: 1 s `/v1/health` probe; "ok" / "unhealthy" / "unknown".
- `HealthDetail`: probe error string when unhealthy.

The `/api/v1/topology/projects/{p}/instances/{i}/status` endpoint serves it (Phase 79 step 7). The TopologyPage already polls every 2 s and shows the StatusBadge + health_detail.

**What's missing for Phase 86.**

1. **Diagnostic-on-unhealthy.** When Health is unhealthy or Running is false unexpectedly (PID file present but process gone), the UI should show:
   - Last N lines of `.stack-pids/<n>.log` (Phase 81 SSE endpoint already has the read path — non-follow mode returns last N lines as JSON).
   - Direct link to `/ui/topology/:p/:i/logs` for full tail.
   - Direct link to `/ui/topology/:p/console` to poke at the instance.
   - Recent exit code if the process exited (read from a new `.stack-pids/<n>.exit` file written by `start-component`?).
2. **Capture exit code.** `make start-component` currently runs the binary and writes the PID; if the process exits, nothing's recorded. Need to capture the exit code so the UI can show "exited(<code>) at <ts>" instead of bare "stopped". Likely via a wrapper script or a small Go helper.
3. **Unhealthy threshold.** 5 s timeout from the brief is bigger than the current 1 s. Bump probe timeout — may need to adjust how often `readInstanceStatus` is called too (don't block the page render on slow probes).
4. **UI.** New `<UnhealthyDiagnosticPanel>` rendered when `health === "unhealthy"` or process gone unexpectedly. Lives on TopologyPage InstanceRow (collapsible) or its own modal.

**Out of scope.** No auto-restart — explicitly deferred. No alerting. No multi-instance rollup of health (that's Phase 87 observability territory).

**Files to touch (rough):**

- `cmd/sockerless-admin/instance_status.go` — bump probe timeout to 5 s; add exit-code field if file present.
- `cmd/sockerless-admin/api_topology.go` — extend the status response shape.
- `make/components.mk` — wrap start-component to capture exit code on process termination.
- `ui/packages/admin/src/pages/TopologyPage.tsx` + new `<UnhealthyDiagnosticPanel>` component.

## Invariants (re-state on every commit)

- **Components stay decoupled.** No admin-required env vars on sims/backends/bleephub. Admin reads only what they already expose (`/v1/health`, `/v1/info`, env vars). For Phase 87 observability: components emit OTLP only when `OTEL_EXPORTER_OTLP_ENDPOINT` is set in their env. Unset = today's stdout behaviour.
- **No fallbacks.** Unknown config values fail-loud. No silent defaults.
- **CI green per commit.** Each commit must be independently testable.
- **Test target gating.** All backend integration tests require `SOCKERLESS_TEST_TARGET=sim|cloud` (no skip).
- **No docs-only PRs.** Pair docs updates with implementation work on the same branch / PR.

## Roadmap

Phases 79.2 → 80 → 81 → 82 ✓ all in #140. Next: 83 → 84 → 85 → 86 → 87. After 87: 91–94 (real per-cloud volume provisioning) + the live-cloud validation track (Lambda live, Cloud Run Services / ACA Apps live, AZF cloud-dns live, Lambda service-mesh live, ACA/AZF Azure AD live). See [PLAN.md](PLAN.md) for sub-steps. Will likely split into multiple PRs once natural seams appear (e.g. Phase 87 — observability — is independent and can land standalone).

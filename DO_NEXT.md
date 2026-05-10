# Do Next

Roadmap [PLAN.md](PLAN.md) · status [STATUS.md](STATUS.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Branch

`phase-83-sim-ui-parity` — Phase 83 work in flight (5 commits). Open a PR when ready; PR #140 already merged.

## Status

Phase 83 implementation is done on this branch. After it lands, **Phase 84 — per-instance state isolation + persistence** is the next pickup. See bottom of this file for the Phase 84 brief.

## Resume here — Phase 83 (sim UI parity) — implementation done

Branch has 5 commits ready to PR:

1. `phase 83: add shared ResourceListPage to @sockerless/ui-core` — new component + 5 vitest cases.
2. `phase 83: refactor simulator-aws pages onto ResourceListPage` — 6 pages.
3. `phase 83: refactor simulator-gcp pages onto ResourceListPage` — 6 pages.
4. `phase 83: refactor simulator-azure pages onto ResourceListPage` — 6 pages.
5. `phase 83: retire legacy admin pages superseded by topology` — `/ui/resources` + `/ui/projects/:name` + `/ui/projects/:name/logs` deleted; companion API client methods + types removed.

When ready: `git push -u origin phase-83-sim-ui-parity` then `gh pr create`. CI cycle is ~7 min; verify all 11 checks green before merging.

## Phase 84 — Per-instance state isolation + persistence (next pickup)

**Goal.** Sims gain optional persistent state across restarts, multiple sim instances of the same cloud coexist with isolated state.

**Today.** Sims already accept `SOCKERLESS_<X>_DATA_DIR` for SQLite persistence (see `simulators/aws/shared/server.go:OpenDB`). The defaults land everything under `/tmp/sockerless-sim-<provider>/` — that's *one* shared dir per cloud, so two sim-aws instances would collide.

**Phase 84 deliverables.**

1. **Topology-driven SIM_STATE_DIR.** When admin starts a sim instance via `make start-component`, set `SIM_STATE_DIR=$(CURDIR)/.sockerless-state/<project>/<instance>/`. The make target sources `.stack-pids/<name>.env` so per-instance config flows through; just add `SIM_STATE_DIR` to the env file admin writes.
2. **Sim-side wiring.** Each `simulator-{aws,gcp,azure}` reads `SIM_STATE_DIR` (or its existing `SOCKERLESS_<X>_DATA_DIR`) before falling back to the `/tmp` default. Per the no-fallbacks principle, when admin orchestrates, the env var is always set; the `/tmp` default is for stand-alone manual use only.
3. **Multi-instance test.** `simulators/aws/instance_isolation_test.go` (or similar) brings up two sim-aws instances on different ports + state dirs, asserts a resource created in instance A doesn't show up in instance B.
4. **Volume cleanup.** `make stop-component KIND=sim NAME=<n>` should NOT auto-delete `.sockerless-state/<project>/<instance>/` — operators choose when to wipe state. Add a separate `make purge-state` target for explicit cleanup.

**Out of scope.** Don't refactor the existing `Persist` / `OpenDB` paths. Phase 84 is purely about path resolution + isolation, not storage backend changes.

**Files to touch (rough):**

- `cmd/sockerless-admin/instance_lifecycle.go` — write `SIM_STATE_DIR` into the env file when kind=sim.
- `make/components.mk` — passthrough or default for `SIM_STATE_DIR` if not set.
- `simulators/{aws,gcp,azure}/shared/server.go` — read `SIM_STATE_DIR` first, fall back to existing per-cloud var, then to `/tmp`.
- `simulators/{aws,gcp,azure}/<integration>_test.go` — multi-instance isolation test.

## Invariants (re-state on every commit)

- **Components stay decoupled.** No admin-required env vars on sims/backends/bleephub. Admin reads only what they already expose (`/v1/health`, `/v1/info`, env vars). For Phase 87 observability: components emit OTLP only when `OTEL_EXPORTER_OTLP_ENDPOINT` is set in their env. Unset = today's stdout behaviour.
- **No fallbacks.** Unknown config values fail-loud. No silent defaults.
- **CI green per commit.** Each commit must be independently testable.
- **Test target gating.** All backend integration tests require `SOCKERLESS_TEST_TARGET=sim|cloud` (no skip).
- **No docs-only PRs.** Pair docs updates with implementation work on the same branch / PR.

## Roadmap

Phases 79.2 → 80 → 81 → 82 ✓ all in #140. Next: 83 → 84 → 85 → 86 → 87. After 87: 91–94 (real per-cloud volume provisioning) + the live-cloud validation track (Lambda live, Cloud Run Services / ACA Apps live, AZF cloud-dns live, Lambda service-mesh live, ACA/AZF Azure AD live). See [PLAN.md](PLAN.md) for sub-steps. Will likely split into multiple PRs once natural seams appear (e.g. Phase 87 — observability — is independent and can land standalone).
